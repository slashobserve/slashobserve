package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

type listFlag []string

func (l *listFlag) String() string { return strings.Join(*l, ",") }

func (l *listFlag) Set(v string) error {
	*l = append(*l, v)
	return nil
}

type options struct {
	server, token      string
	scenarios, pricing string
	ids, tags          listFlag
	list               bool
	parallel           int
	timeout            time.Duration
	out, markdown      string
	minPass            float64
	minDoneWhen        int
	minDoneWhenSet     bool
}

const defaultMinDoneWhen = 8

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func parseFlags(args []string, stderr io.Writer) (options, error) {
	var o options
	fs := flag.NewFlagSet("runner", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&o.server, "server", os.Getenv("OBSERVE_SERVER"), "base URL of the SlashObserve server (env OBSERVE_SERVER)")
	fs.StringVar(&o.token, "token", os.Getenv("OBSERVE_TOKEN"), "service-account key (env OBSERVE_TOKEN)")
	fs.StringVar(&o.scenarios, "scenarios", "evals/scenarios", "directory of scenario files")
	fs.StringVar(&o.pricing, "pricing", "evals/pricing.yaml", "model pricing file")
	fs.Var(&o.ids, "id", "run only this scenario id (repeatable)")
	fs.Var(&o.tags, "tag", "run only scenarios with this tag (repeatable)")
	fs.BoolVar(&o.list, "list", false, "print the selected scenarios and exit")
	fs.IntVar(&o.parallel, "parallel", 2, "scenarios to run at a time, at most 3 (the server's per-key limit of active observe requests)")
	fs.DurationVar(&o.timeout, "timeout", 180*time.Second, "how long to wait for one agent request")
	fs.StringVar(&o.out, "out", "", "write the JSON report to this file")
	fs.StringVar(&o.markdown, "markdown", "", "write the Markdown report to this file")
	fs.Float64Var(&o.minPass, "min-pass", 0.8, "minimum share of scenarios that must pass")
	fs.IntVar(&o.minDoneWhen, "min-done-when", defaultMinDoneWhen, "minimum done-when scenarios that must pass when all of them ran")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "min-done-when" {
			o.minDoneWhenSet = true
		}
	})
	switch {
	case o.parallel < 1 || o.parallel > maxPerPrincipal:
		return o, fmt.Errorf("--parallel must be between 1 and %d, the server's limit of queued or running observe requests per key", maxPerPrincipal)
	case o.minPass < 0 || o.minPass > 1:
		return o, errors.New("--min-pass must be between 0 and 1")
	case o.timeout <= 0:
		return o, errors.New("--timeout must be positive")
	}
	return o, nil
}

func run(args []string, stdout, stderr io.Writer) int {
	o, err := parseFlags(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return fail(stderr, err)
	}
	all, err := loadScenarios(o.scenarios)
	if err != nil {
		return fail(stderr, err)
	}
	selected, err := filterScenarios(all, o.ids, o.tags)
	if err != nil {
		return fail(stderr, err)
	}
	if o.list {
		width := 0
		for _, sc := range selected {
			width = max(width, len(sc.ID))
		}
		for _, sc := range selected {
			_, _ = fmt.Fprintf(stdout, "%-*s %-10s done_when=%t tags=%s\n", width, sc.ID, sc.Category, sc.DoneWhen, strings.Join(sc.Tags, ","))
		}
		return 0
	}
	doneWhen := countDoneWhen(all)
	switch {
	case doneWhen == 0:
	case !o.minDoneWhenSet:
		o.minDoneWhen = min(defaultMinDoneWhen, doneWhen)
	case o.minDoneWhen < 0 || o.minDoneWhen > doneWhen:
		return fail(stderr, fmt.Errorf("--min-done-when must be between 0 and %d, the number of done_when scenarios", doneWhen))
	}
	return execute(o, selected, doneWhen, stdout, stderr)
}

func countDoneWhen(scenarios []Scenario) int {
	n := 0
	for _, sc := range scenarios {
		if sc.DoneWhen {
			n++
		}
	}
	return n
}

func execute(o options, selected []Scenario, doneWhen int, stdout, stderr io.Writer) int {
	pricing, err := loadPricing(o.pricing)
	if err != nil {
		return fail(stderr, err)
	}
	c, err := newClient(o.server, o.token)
	if err != nil {
		return fail(stderr, err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	info, err := c.agent(ctx)
	if err != nil {
		return fail(stderr, fmt.Errorf("read the agent configuration: %w", err))
	}
	if !info.Available || info.Provider == nil || info.Model == nil {
		return fail(stderr, errors.New("the server has no LLM provider configured"))
	}

	report := Report{StartedAt: time.Now().UTC(), Provider: *info.Provider, Model: *info.Model}
	report.Results = runAll(ctx, c, pricing, selected, o, stderr)
	report.Aggregates = aggregate(report.Results)
	gates := evaluateGates(report.Aggregates, o.minPass, o.minDoneWhen, doneWhen)

	markdown := renderMarkdown(report, gates)
	_, _ = io.WriteString(stdout, markdown)
	if err := writeReports(o, report, markdown); err != nil {
		return fail(stderr, err)
	}
	if ctx.Err() != nil || !gates.ok() {
		return 1
	}
	return 0
}

func fail(stderr io.Writer, err error) int {
	_, _ = fmt.Fprintln(stderr, "runner:", err)
	return 2
}

func writeReports(o options, report Report, markdown string) error {
	if o.out != "" {
		if err := writeJSON(o.out, report); err != nil {
			return fmt.Errorf("write %s: %w", o.out, err)
		}
	}
	if o.markdown != "" {
		if err := os.WriteFile(o.markdown, []byte(markdown), 0o600); err != nil {
			return fmt.Errorf("write %s: %w", o.markdown, err)
		}
	}
	return nil
}

func runAll(ctx context.Context, c *client, pricing Pricing, scenarios []Scenario, o options, stderr io.Writer) []Result {
	results := make([]Result, len(scenarios))
	slots := make(chan struct{}, o.parallel)
	var wg sync.WaitGroup
	var logMu sync.Mutex
	for i, sc := range scenarios {
		wg.Add(1)
		slots <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			var abandoned *AgentRequest
			results[i], abandoned = runScenario(ctx, c, pricing, sc, o.timeout)
			logMu.Lock()
			verdict := "PASS"
			if !results[i].Passed {
				verdict = "FAIL"
			}
			_, _ = fmt.Fprintf(stderr, "%s  %s\n", verdict, sc.ID)
			logMu.Unlock()
			if abandoned != nil {
				logMu.Lock()
				_, _ = fmt.Fprintf(stderr, "draining %s (up to %s)\n", abandoned.ID, graceLabel(c.grace))
				logMu.Unlock()
				c.drain(ctx, *abandoned)
			}
		}()
	}
	wg.Wait()
	return results
}

func graceLabel(d time.Duration) string {
	if d%time.Minute == 0 {
		return fmt.Sprintf("%dm", int(d/time.Minute))
	}
	return d.String()
}

var slotWaitNotice = 10 * time.Second

func runScenario(ctx context.Context, c *client, pricing Pricing, sc Scenario, timeout time.Duration) (Result, *AgentRequest) {
	res := Result{ID: sc.ID, Prompt: sc.Prompt, Category: sc.Category, DoneWhen: sc.DoneWhen, Tags: sc.Tags, Status: "not_started"}
	obs := observation{panels: map[string]panelOutcome{}}

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	createStart := time.Now()
	req, err := c.createObserveWhenFree(waitCtx, sc.Prompt)
	slotWait := time.Since(createStart)
	if err != nil {
		res.Error = "create the request: " + err.Error()
		if slotsFull(err) && waitCtx.Err() != nil {
			res.Error = fmt.Sprintf("no request slot within %s: %s", timeout, err)
		}
		obs.request = AgentRequest{Status: res.Status}
		return finish(sc, pricing, res, obs), nil
	}
	req, err = c.waitObserve(waitCtx, req)
	obs.request = req
	res.RequestID, res.Status = req.ID, req.Status
	switch {
	case waitCtx.Err() != nil && err != nil:
		res.Error = fmt.Sprintf("timed out after %s waiting for the agent", timeout)
		if slotWait > slotWaitNotice {
			res.Error += fmt.Sprintf(" (%.0fs waiting for a request slot)", slotWait.Seconds())
		}
		if ctx.Err() != nil {
			res.Error = "interrupted while waiting for the agent"
		}
	case err != nil:
		res.Error = "poll the request: " + err.Error()
	case req.Error != nil:
		res.Error = *req.Error
	}
	if err == nil && req.Status == "succeeded" {
		types, typesErr := c.datasourceTypes(ctx)
		if typesErr != nil {
			res.Error = "read the datasources: " + typesErr.Error()
		} else {
			obs.datasourceTypes = types
			inspect(ctx, c, req, &obs, &res)
		}
	}
	var abandoned *AgentRequest
	if !req.finished() {
		abandoned = &req
	}
	return finish(sc, pricing, res, obs), abandoned
}

func inspect(ctx context.Context, c *client, req AgentRequest, obs *observation, res *Result) {
	if req.DashboardID == nil || req.ResultVersion == nil {
		obs.specErr = "the request succeeded without a saved version"
		return
	}
	res.DashboardID = *req.DashboardID
	raw, err := c.getVersionSpec(ctx, *req.DashboardID, *req.ResultVersion)
	if err != nil {
		obs.specErr = "read the saved version: " + err.Error()
		return
	}
	dash, err := spec.Parse(raw)
	if err != nil {
		obs.specErr = "the saved spec does not parse: " + err.Error()
		return
	}
	obs.dashboard = dash
	for _, panel := range dataPanels(dash) {
		data, err := c.getPanelData(ctx, *req.DashboardID, *req.ResultVersion, panel.ID)
		obs.panels[panel.ID] = panelOutcome{data: data, err: err}
	}
}

func finish(sc Scenario, pricing Pricing, res Result, obs observation) Result {
	res.Checks = runChecks(sc, obs)
	res.Passed = allPassed(res.Checks)
	res.Usage = obs.request.Usage
	res.CostUSD = pricing.cost(obs.request.Usage)
	if obs.dashboard != nil {
		res.Panels = len(dataPanels(obs.dashboard))
	}
	if elapsed, ok := latency(obs.request); ok {
		seconds := elapsed.Seconds()
		res.LatencySeconds = &seconds
	}
	return res
}
