package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

const (
	defaultReadyTimeout = 2 * time.Minute
	readyPoll           = 500 * time.Millisecond
	logSaveTimeout      = time.Minute

	evalsComposeProject = "slashobserve-evals"
	serverLogSuffix     = ".server.log"
)

var matrixNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

type matrixEntry struct {
	Name    string `yaml:"name"`
	EnvFile string `yaml:"env_file"`
}

type matrixFile struct {
	Models []matrixEntry `yaml:"models"`
}

func loadMatrix(path string) ([]matrixEntry, error) {
	content, err := os.ReadFile(path) //nolint:gosec // the path comes from the operator's flags
	if err != nil {
		return nil, err
	}
	var file matrixFile
	if err := decodeStrict(content, &file); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(file.Models) == 0 {
		return nil, fmt.Errorf("%s: models must list at least one entry", path)
	}
	seen := map[string]bool{}
	for i, m := range file.Models {
		switch {
		case !matrixNamePattern.MatchString(m.Name):
			return nil, fmt.Errorf("%s: models[%d].name %q must match %s", path, i, m.Name, matrixNamePattern)
		case blank(m.EnvFile):
			return nil, fmt.Errorf("%s: models[%d].env_file is required", path, i)
		case seen[m.Name]:
			return nil, fmt.Errorf("%s: models[%d].name %q is used twice", path, i, m.Name)
		}
		seen[m.Name] = true
	}
	return file.Models, nil
}

type matrixStep struct {
	switchTo func(ctx context.Context, envFile string) error
	saveLogs func(ctx context.Context, path string) error
	ready    func(ctx context.Context) error
	run      func(ctx context.Context) (Report, gateOutcome, error)
}

type matrixResult struct {
	name   string
	report Report
}

func runMatrix(ctx context.Context, entries []matrixEntry, step matrixStep, out string, stderr io.Writer) (bool, error) {
	if err := os.MkdirAll(out, 0o750); err != nil {
		return false, err
	}
	allOK := true
	var done []matrixResult
	serving := ""
	saveServing := func() {
		if serving == "" {
			return
		}
		if err := step.saveLogs(ctx, filepath.Join(out, serving+serverLogSuffix)); err != nil {
			_, _ = fmt.Fprintf(stderr, "matrix: save the server logs of %s: %v\n", serving, err)
		}
	}
	for _, entry := range entries {
		if ctx.Err() != nil {
			allOK = false
			break
		}
		_, _ = fmt.Fprintf(stderr, "matrix: %s\n", entry.Name)
		saveServing()
		serving = entry.Name
		report, gates, err := runMatrixEntry(ctx, entry, step)
		if err != nil {
			allOK = false
			_, _ = fmt.Fprintf(stderr, "matrix: %s: %v\n", entry.Name, err)
			continue
		}
		markdown := renderMarkdown(report, gates)
		if err := writeReports(options{out: filepath.Join(out, entry.Name+".json"), markdown: filepath.Join(out, entry.Name+".md")}, report, markdown); err != nil {
			return false, err
		}
		allOK = allOK && gates.ok()
		done = append(done, matrixResult{name: entry.Name, report: report})
	}
	saveServing()
	if err := os.WriteFile(filepath.Join(out, "matrix.md"), []byte(renderMatrix(done)), 0o600); err != nil {
		return false, err
	}
	return allOK, nil
}

func runMatrixEntry(ctx context.Context, entry matrixEntry, step matrixStep) (Report, gateOutcome, error) {
	if err := step.switchTo(ctx, entry.EnvFile); err != nil {
		return Report{}, gateOutcome{}, fmt.Errorf("switch the server: %w", err)
	}
	if err := step.ready(ctx); err != nil {
		return Report{}, gateOutcome{}, fmt.Errorf("wait for the server: %w", err)
	}
	return step.run(ctx)
}

func renderMatrix(results []matrixResult) string {
	var b strings.Builder
	b.WriteString("# Eval matrix\n\n")
	b.WriteString("| Entry | Provider | Model | Scenarios passed | Skipped |\n| --- | --- | --- | --- | --- |\n")
	kinds := map[string]bool{}
	for _, r := range results {
		agg := r.report.Aggregates
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %d |\n", r.name, escapeCell(r.report.Provider), escapeCell(r.report.Model), percent(agg.Scenarios), agg.Skipped)
		for kind := range agg.Kinds {
			kinds[kind] = true
		}
	}
	b.WriteString("\n| Kind |")
	divider := "| --- |"
	for _, r := range results {
		fmt.Fprintf(&b, " %s |", r.name)
		divider += " --- |"
	}
	b.WriteString("\n" + divider + "\n")
	b.WriteString("| First panel p50 |")
	for _, r := range results {
		if stats := r.report.Aggregates.FirstPanel; stats != nil {
			fmt.Fprintf(&b, " %.1fs |", stats.P50)
		} else {
			b.WriteString(" n/a |")
		}
	}
	b.WriteString("\n")
	for _, kind := range slices.Sorted(maps.Keys(kinds)) {
		fmt.Fprintf(&b, "| %s |", kind)
		for _, r := range results {
			if t, ok := r.report.Aggregates.Kinds[kind]; ok {
				fmt.Fprintf(&b, " %s |", percent(t))
			} else {
				b.WriteString(" n/a |")
			}
		}
		b.WriteString("\n")
	}
	writeSizeGuardRow(&b, results)
	return b.String()
}

func writeSizeGuardRow(b *strings.Builder, results []matrixResult) {
	if !slices.ContainsFunc(results, func(r matrixResult) bool { return r.report.Aggregates.SizeGuard.Total > 0 }) {
		return
	}
	b.WriteString("| size_guard, rejected as expected |")
	for _, r := range results {
		if t := r.report.Aggregates.SizeGuard; t.Total > 0 {
			fmt.Fprintf(b, " %d of %d |", t.Passed, t.Total)
		} else {
			b.WriteString(" n/a |")
		}
	}
	b.WriteString("\n")
}

func scriptSwitcher(script string, stderr io.Writer) func(context.Context, string) error {
	return func(ctx context.Context, envFile string) error {
		cmd := exec.CommandContext(ctx, script, "server", envFile) //nolint:gosec // the script path is the operator's own checkout
		cmd.Stdout, cmd.Stderr = stderr, stderr
		return cmd.Run()
	}
}

func composeLogSaver(stderr io.Writer) func(context.Context, string) error {
	return func(ctx context.Context, path string) error {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), logSaveTimeout)
		defer cancel()
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) //nolint:gosec // the path is under the operator's --out
		if err != nil {
			return err
		}
		cmd := exec.CommandContext(ctx, "docker", "compose", "-p", evalsComposeProject, "logs", "--no-color", "server")
		cmd.Stdout, cmd.Stderr = file, stderr
		runErr := cmd.Run()
		return errors.Join(runErr, file.Close())
	}
}

func waitReady(ctx context.Context, base string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	client := &http.Client{Timeout: callTimeout}
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/readyz", nil)
		if err != nil {
			return err
		}
		if resp, err := client.Do(req); err == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxBodyBytes))
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return errors.New("/readyz did not answer 200 in time")
		case <-time.After(readyPoll):
		}
	}
}

func executeMatrix(ctx context.Context, c *client, pricing Pricing, selected []Scenario, o options, doneWhen int, stderr io.Writer) int {
	entries, err := loadMatrix(o.matrix)
	if err != nil {
		return fail(stderr, err)
	}
	step := matrixStep{
		switchTo: scriptSwitcher(o.stackScript, stderr),
		saveLogs: composeLogSaver(stderr),
		ready:    func(ctx context.Context) error { return waitReady(ctx, c.base, o.readyTimeout) },
		run: func(ctx context.Context) (Report, gateOutcome, error) {
			return runSuite(ctx, c, pricing, selected, o, doneWhen, stderr)
		},
	}
	ok, err := runMatrix(ctx, entries, step, o.out, stderr)
	if err != nil {
		return fail(stderr, err)
	}
	if !ok || ctx.Err() != nil {
		return 1
	}
	return 0
}
