package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func writeFile(t *testing.T, name, body string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestMatrixExampleLoads(t *testing.T) {
	entries, err := loadMatrix(filepath.Join("..", "matrix.example.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 2 || entries[0].Name == "" || entries[0].EnvFile == "" {
		t.Fatalf("entries %+v", entries)
	}
}

func TestMatrixFileValidation(t *testing.T) {
	cases := map[string]struct {
		body string
		want string
	}{
		"valid":             {"models:\n  - name: a\n    env_file: one.env\n  - name: b.2\n    env_file: two.env\n", ""},
		"unknown field":     {"models:\n  - name: a\n    env_file: one.env\n    model: x\n", "model"},
		"unknown top level": {"models: []\nextra: 1\n", "extra"},
		"no models":         {"models: []\n", "at least one entry"},
		"bad name":          {"models:\n  - name: A/b\n    env_file: one.env\n", "name"},
		"blank name":        {"models:\n  - name: \"\"\n    env_file: one.env\n", "name"},
		"no env file":       {"models:\n  - name: a\n", "env_file is required"},
		"duplicate name":    {"models:\n  - name: a\n    env_file: one.env\n  - name: a\n    env_file: two.env\n", "used twice"},
		"empty file":        {"", "empty file"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := loadMatrix(writeFile(t, "matrix.yaml", tc.body, 0o600))
			switch {
			case tc.want == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
				t.Fatalf("error %v does not contain %q", err, tc.want)
			}
		})
	}
	if _, err := loadMatrix(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Error("a missing file should fail")
	}
}

func recordingScript(t *testing.T, failOn string) (script, log string) {
	t.Helper()
	log = filepath.Join(t.TempDir(), "calls.log")
	body := "#!/bin/sh\necho \"$@\" >> '" + log + "'\n"
	if failOn != "" {
		body += "case \"$2\" in *" + failOn + "*) echo refused >&2; exit 3;; esac\n"
	}
	return writeFile(t, "stack.sh", body, 0o700), log
}

func readTestFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path) //nolint:gosec // a path the test built under its own temporary directory
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func scriptCalls(t *testing.T, log string) []string {
	t.Helper()
	if _, err := os.Stat(log); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return strings.Split(strings.TrimSpace(string(readTestFile(t, log))), "\n")
}

func cannedReport(provider, model string, passed, total int, kind string) Report {
	results := make([]Result, total)
	for i := range results {
		results[i] = Result{ID: kind + "-" + string(rune('a'+i)), Kind: kind, Passed: i < passed}
	}
	return Report{Provider: provider, Model: model, Aggregates: aggregate(results), Results: results}
}

func TestMatrixRunsEntriesInOrderAndWritesOneReportEachAndACombinedTable(t *testing.T) {
	script, log := recordingScript(t, "")
	out := filepath.Join(t.TempDir(), "reports")
	entries := []matrixEntry{{Name: "alpha", EnvFile: "/envs/alpha.env"}, {Name: "beta", EnvFile: "/envs/beta.env"}}
	reports := []Report{cannedReport("p1", "m1", 3, 4, kindFixUnverified), cannedReport("p2", "m2", 1, 2, kindCreate)}
	var ran, ready int
	var events []string
	realSwitch := scriptSwitcher(script, io.Discard)
	step := matrixStep{
		switchTo: func(ctx context.Context, envFile string) error {
			events = append(events, "switch "+filepath.Base(envFile))
			return realSwitch(ctx, envFile)
		},
		saveLogs: func(_ context.Context, path string) error {
			events = append(events, "logs "+filepath.Base(path))
			return nil
		},
		ready: func(context.Context) error { ready++; return nil },
		run: func(context.Context) (Report, gateOutcome, error) {
			ran++
			return reports[ran-1], gateOutcome{}, nil
		},
	}
	ok, err := runMatrix(t.Context(), entries, step, out, io.Discard)
	if err != nil || !ok {
		t.Fatalf("ok %t, err %v", ok, err)
	}
	if got := scriptCalls(t, log); len(got) != 2 || got[0] != "server /envs/alpha.env" || got[1] != "server /envs/beta.env" || ready != 2 {
		t.Fatalf("script calls %v, readiness waits %d", got, ready)
	}
	wantEvents := []string{"switch alpha.env", "logs alpha.server.log", "switch beta.env", "logs beta.server.log"}
	if !slices.Equal(events, wantEvents) {
		t.Errorf("events %v, want %v", events, wantEvents)
	}
	for _, name := range []string{"alpha.json", "alpha.md", "beta.json", "beta.md", "matrix.md"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Errorf("missing %s: %v", name, err)
		}
	}
	combined := readTestFile(t, filepath.Join(out, "matrix.md"))
	for _, line := range []string{"| alpha | p1 | m1 | 3 of 4 (75%) | 0 |", "| beta | p2 | m2 | 1 of 2 (50%) | 0 |", "| Kind | alpha | beta |", "| create | n/a | 1 of 2 (50%) |", "| fix_unverified | 3 of 4 (75%) | n/a |"} {
		if !strings.Contains(string(combined), line) {
			t.Errorf("matrix.md lacks %q:\n%s", line, combined)
		}
	}
}

func TestMatrixKeepsGoingAfterAFailedEntryAndReportsIt(t *testing.T) {
	script, log := recordingScript(t, "broken")
	out := t.TempDir()
	entries := []matrixEntry{{Name: "first", EnvFile: "/envs/broken.env"}, {Name: "second", EnvFile: "/envs/fine.env"}}
	var stderr strings.Builder
	step := matrixStep{
		switchTo: scriptSwitcher(script, &stderr),
		saveLogs: func(context.Context, string) error { return errors.New("no containers") },
		ready:    func(context.Context) error { return nil },
		run: func(context.Context) (Report, gateOutcome, error) {
			return cannedReport("p", "m", 1, 1, kindCreate), gateOutcome{}, nil
		},
	}
	ok, err := runMatrix(t.Context(), entries, step, out, &stderr)
	if err != nil || ok {
		t.Fatalf("ok %t, err %v: a failed entry must fail the run without aborting it", ok, err)
	}
	if got := scriptCalls(t, log); len(got) != 2 {
		t.Fatalf("script calls %v, want both entries tried", got)
	}
	if !strings.Contains(stderr.String(), "matrix: first: switch the server") {
		t.Errorf("stderr %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "save the server logs of second: no containers") {
		t.Errorf("a failed log save must be reported without failing the run: %q", stderr.String())
	}
	if _, err := os.Stat(filepath.Join(out, "first.json")); err == nil {
		t.Error("a failed entry must not leave a report")
	}
	if _, err := os.Stat(filepath.Join(out, "second.json")); err != nil {
		t.Errorf("the second entry should have its report: %v", err)
	}
}

func TestMatrixFailsWhenAGateFailsAndStopsWhenCancelled(t *testing.T) {
	entries := []matrixEntry{{Name: "a", EnvFile: "a.env"}, {Name: "b", EnvFile: "b.env"}}
	switched := 0
	step := matrixStep{
		switchTo: func(context.Context, string) error { switched++; return nil },
		saveLogs: func(context.Context, string) error { return nil },
		ready:    func(context.Context) error { return nil },
		run: func(context.Context) (Report, gateOutcome, error) {
			return cannedReport("p", "m", 0, 1, kindCreate), gateOutcome{failures: []string{"pass rate 0%"}}, nil
		},
	}
	if ok, err := runMatrix(t.Context(), entries, step, t.TempDir(), io.Discard); err != nil || ok {
		t.Fatalf("a failed gate should fail the matrix: ok %t, err %v", ok, err)
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	switched = 0
	if ok, err := runMatrix(cancelled, entries, step, t.TempDir(), io.Discard); err != nil || ok || switched != 0 {
		t.Fatalf("a cancelled run must not start entries: ok %t, switched %d, err %v", ok, switched, err)
	}
}

func TestScriptSwitcherReportsAScriptFailure(t *testing.T) {
	script, _ := recordingScript(t, "bad")
	var stderr strings.Builder
	err := scriptSwitcher(script, &stderr)(t.Context(), "/envs/bad.env")
	if err == nil || !strings.Contains(stderr.String(), "refused") {
		t.Fatalf("err %v, stderr %q", err, stderr.String())
	}
}

func TestWaitReadyPollsUntilReadyzAnswers200(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/readyz" {
			t.Errorf("polled %s", r.URL.Path)
		}
		if hits.Add(1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	t.Cleanup(srv.Close)
	if err := waitReady(t.Context(), srv.URL+"/", 10*time.Second); err != nil || hits.Load() != 3 {
		t.Fatalf("err %v after %d polls", err, hits.Load())
	}

	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	t.Cleanup(down.Close)
	if err := waitReady(t.Context(), down.URL, 700*time.Millisecond); err == nil || !strings.Contains(err.Error(), "/readyz") {
		t.Fatalf("a server that never gets ready should time out: %v", err)
	}
}

func TestExecuteMatrixSwitchesWaitsRunsAndNeverReadsTheEnvFile(t *testing.T) {
	f := &fakeServer{limit: 3, runFor: 0, finishAt: map[string]time.Time{}}
	mux := http.NewServeMux()
	mux.Handle("/api/", f.handler())
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c, err := newClient(srv.URL, "key")
	if err != nil {
		t.Fatal(err)
	}
	c.poll, c.backoff = 5*time.Millisecond, 5*time.Millisecond

	script, log := recordingScript(t, "")
	envFile := writeFile(t, "model.env", "SLASHOBSERVE_LLM_API_KEY=do-not-read\n", 0o600)
	matrix := writeFile(t, "matrix.yaml", "models:\n  - name: only\n    env_file: "+envFile+"\n", 0o600)
	out := filepath.Join(t.TempDir(), "out")
	o := options{matrix: matrix, out: out, stackScript: script, readyTimeout: 5 * time.Second, parallel: 1, timeout: 5 * time.Second, minPass: 0, faults: newShopFaults(nil)}

	var stderr strings.Builder
	rc := executeMatrix(t.Context(), c, Pricing{}, sampleScenarios(1), o, 0, &stderr)
	if rc != 1 {
		t.Fatalf("rc %d: the fake agent fails its request, so the pass rate gate fails; stderr %q", rc, stderr.String())
	}
	if got := scriptCalls(t, log); len(got) != 1 || got[0] != "server "+envFile {
		t.Fatalf("script calls %v", got)
	}
	report := readTestFile(t, filepath.Join(out, "only.json"))
	if !strings.Contains(string(report), `"model": "replay"`) {
		t.Fatalf("report %s", report)
	}
	if strings.Contains(stderr.String(), "do-not-read") || strings.Contains(string(report), "do-not-read") {
		t.Fatal("the env file's content must never reach the runner's output")
	}

	if rc := executeMatrix(t.Context(), c, Pricing{}, sampleScenarios(1), options{matrix: filepath.Join(t.TempDir(), "none.yaml"), out: out}, 0, io.Discard); rc != 2 {
		t.Errorf("a missing matrix file should exit 2, got %d", rc)
	}
}

func TestComposeLogSaverWritesTheServerLogsOfTheEvalsProject(t *testing.T) {
	bin := t.TempDir()
	args := filepath.Join(bin, "args")
	script := "#!/bin/sh\nprintf '%s ' \"$@\" >" + args + "\necho 'server log line'\n"
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0o700); err != nil { //nolint:gosec // an executable test double
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	path := filepath.Join(t.TempDir(), "alpha.server.log")

	if err := composeLogSaver(io.Discard)(t.Context(), path); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(readTestFile(t, args))); got != "compose -p slashobserve-evals logs --no-color server" {
		t.Errorf("docker ran with %q", got)
	}
	if got := strings.TrimSpace(string(readTestFile(t, path))); got != "server log line" {
		t.Errorf("log file holds %q", got)
	}

	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if err := composeLogSaver(io.Discard)(cancelled, path); err != nil {
		t.Errorf("logs must still be saved after an interrupt: %v", err)
	}
}
