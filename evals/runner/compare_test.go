package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func scored(v float64) *float64 { return &v }

func compareFixtures(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	base := Report{StartedAt: time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC), Provider: "anthropic", Model: "claude-haiku-5-5", Results: []Result{
		{ID: "red", Kind: kindCreate, Passed: true, LatencySeconds: scored(40), CostUSD: scored(0.02), Quality: scored(0.8), FirstPanelSeconds: scored(10), Usage: &Usage{Model: "claude-haiku-5-5"}},
		{ID: "node", Kind: kindCreate, Passed: true, LatencySeconds: scored(60), CostUSD: scored(0.03), Quality: scored(0.6), FirstPanelSeconds: scored(20), Usage: &Usage{Model: "claude-haiku-5-5"}},
		{ID: "gone", Kind: kindCreate, Passed: true, Usage: &Usage{Model: "claude-haiku-5-5"}},
	}}
	next := Report{StartedAt: time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC), Provider: "anthropic", Model: "claude-haiku-5-5", Results: []Result{
		{ID: "red", Kind: kindCreate, Passed: true, LatencySeconds: scored(30), CostUSD: scored(0.015), Quality: scored(0.9), FirstPanelSeconds: scored(8), Usage: &Usage{Model: "claude-haiku-5-5"}},
		{ID: "node", Kind: kindCreate, Passed: false, LatencySeconds: scored(70), Quality: scored(0.5), FirstPanelSeconds: scored(25), Usage: &Usage{Model: "claude-haiku-5-5"}},
		{ID: "added", Kind: kindRevise, Passed: true, Usage: &Usage{Model: "claude-haiku-5-5"}},
	}}
	base.Aggregates, next.Aggregates = aggregate(base.Results), aggregate(next.Results)
	basePath, newPath := filepath.Join(dir, "base.json"), filepath.Join(dir, "new", "haiku.json")
	if err := os.MkdirAll(filepath.Dir(newPath), 0o750); err != nil {
		t.Fatal(err)
	}
	for path, r := range map[string]Report{basePath: base, newPath: next} {
		if err := writeJSON(path, r); err != nil {
			t.Fatal(err)
		}
	}
	return basePath, newPath
}

func TestCompareWritesScenarioAndModelDeltas(t *testing.T) {
	basePath, newPath := compareFixtures(t)
	var stdout, stderr strings.Builder
	if rc := run([]string{"--compare", basePath, newPath}, &stdout, &stderr); rc != 0 {
		t.Fatalf("rc = %d, stderr %s", rc, stderr.String())
	}
	written, err := os.ReadFile(filepath.Join(filepath.Dir(newPath), compareFile))
	if err != nil {
		t.Fatal(err)
	}
	if string(written) != stdout.String() {
		t.Fatalf("compare.md differs from the printed comparison:\n%s\n---\n%s", written, stdout.String())
	}
	for _, line := range []string{
		"| red | PASS | 40.0s / 30.0s | -10.0s | $0.0200 / $0.0150 | -$0.0050 | 0.80 / 0.90 | +0.10 |",
		"| node | PASS -> FAIL | 60.0s / 70.0s | +10.0s | $0.0300 / n/a | n/a | 0.60 / 0.50 | -0.10 |",
		"| claude-haiku-5-5 | 3 of 3 | 2 of 3 | -1 | $0.0500 | $0.0150 | -$0.0350 | 10.0s | 8.0s | -2.0s | 0.60 | 0.50 | -0.10 |",
		"Only in the base report: gone",
		"Only in the new report: added",
	} {
		if !strings.Contains(stdout.String(), line) {
			t.Errorf("comparison lacks %q:\n%s", line, stdout.String())
		}
	}
	if strings.Contains(stdout.String(), compareAllRow) {
		t.Errorf("one model needs no all-models row:\n%s", stdout.String())
	}
}

func TestCompareSummaryHasARowPerModelAndOneForAll(t *testing.T) {
	base := Report{Model: "claude-haiku-5-5", Results: []Result{{ID: "a", Passed: true, CostUSD: scored(0.01), Usage: &Usage{Model: "claude-haiku-5-5"}}}}
	next := Report{Model: "claude-sonnet-5-5", Results: []Result{{ID: "a", Passed: true, CostUSD: scored(0.05), Usage: &Usage{Model: "claude-sonnet-5-5"}}}}
	markdown := renderCompare(base, next, "base.json", "new.json")
	for _, line := range []string{
		"| claude-haiku-5-5 | 1 of 1 | n/a | n/a | $0.0100 | n/a | n/a |",
		"| claude-sonnet-5-5 | n/a | 1 of 1 | n/a | n/a | $0.0500 | n/a |",
		"| all models | 1 of 1 | 1 of 1 | +0 | $0.0100 | $0.0500 | +$0.0400 |",
	} {
		if !strings.Contains(markdown, line) {
			t.Errorf("summary lacks %q:\n%s", line, markdown)
		}
	}
}

func TestCompareAfterARunNeedsAPlaceForItsReport(t *testing.T) {
	cases := map[string][]string{
		"--compare needs --out":         {"--compare", "base.json"},
		"--compare cannot run a matrix": {"--compare", "base.json", "--matrix", "m.yaml", "--out", "dir"},
		"one report to compare":         {"--compare", "base.json", "a.json", "b.json"},
	}
	for want, args := range cases {
		if _, err := parseFlags(args, &strings.Builder{}); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%v: error %v, want %q", args, err, want)
		}
	}
}

func TestCompareFailsOnAMissingReport(t *testing.T) {
	basePath, _ := compareFixtures(t)
	var stderr strings.Builder
	if rc := run([]string{"--compare", basePath, filepath.Join(t.TempDir(), "none.json")}, &strings.Builder{}, &stderr); rc == 0 {
		t.Fatal("a missing report must fail")
	}
}

func TestCompareAfterARunWritesNextToItsOut(t *testing.T) {
	basePath, _ := compareFixtures(t)
	out := filepath.Join(t.TempDir(), "run.json")
	report := Report{Model: "claude-haiku-5-5", Results: []Result{{ID: "red", Passed: false, Usage: &Usage{Model: "claude-haiku-5-5"}}}}
	var stdout strings.Builder
	if err := compareWith(basePath, report, out, &stdout); err != nil {
		t.Fatal(err)
	}
	written := string(readTestFile(t, filepath.Join(filepath.Dir(out), compareFile)))
	if written != stdout.String() || !strings.Contains(written, "| red | PASS -> FAIL |") {
		t.Fatalf("compare.md:\n%s", written)
	}
}
