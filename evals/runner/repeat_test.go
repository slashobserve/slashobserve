package main

import (
	"io"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestRepeatFlagBounds(t *testing.T) {
	for _, value := range []string{"0", "21", "-1"} {
		if _, err := parseFlags([]string{"--repeat", value}, io.Discard); err == nil || !strings.Contains(err.Error(), "--repeat must be between 1 and 20") {
			t.Errorf("--repeat %s: %v", value, err)
		}
	}
	if o, err := parseFlags(nil, io.Discard); err != nil || o.repeat != 1 {
		t.Fatalf("default repeat %d, %v", o.repeat, err)
	}
	if o, err := parseFlags([]string{"--repeat", "20"}, io.Discard); err != nil || o.repeat != 20 {
		t.Fatalf("--repeat 20: %d, %v", o.repeat, err)
	}
}

func TestRepeatRunsEachScenarioInTurnAsSeparateResults(t *testing.T) {
	f, c := newFake(t, 3, 40*time.Millisecond)
	results := runAll(t.Context(), c, Pricing{}, sampleScenarios(2), options{parallel: 3, repeat: 3, timeout: 5 * time.Second}, io.Discard)
	if len(results) != 6 {
		t.Fatalf("%d results, want 6", len(results))
	}
	for i, r := range results {
		wantID, wantRun := sampleScenarios(2)[i/3].ID, i%3+1
		if r.ID != wantID || r.Run != wantRun || r.RequestID == "" {
			t.Errorf("result %d is %s run %d (request %q), want %s run %d", i, r.ID, r.Run, r.RequestID, wantID, wantRun)
		}
		if r.Run > 1 && r.StartedAt.Before(results[i-1].FinishedAt) {
			t.Errorf("%s run %d started before run %d finished", r.ID, r.Run, r.Run-1)
		}
	}
	if f.maxActive < 2 {
		t.Fatalf("max active %d: different scenarios should run side by side", f.maxActive)
	}
}

func TestRepeatOnceLeavesTheRunNumberOut(t *testing.T) {
	_, c := newFake(t, 3, 10*time.Millisecond)
	var stderr strings.Builder
	results := runAll(t.Context(), c, Pricing{}, sampleScenarios(1), options{parallel: 1, repeat: 1, timeout: 5 * time.Second}, &stderr)
	if len(results) != 1 || results[0].Run != 0 || stderr.String() != "FAIL  s-0\n" {
		t.Fatalf("results %+v, log %q", results, stderr.String())
	}
}

func repeatedResults() []Result {
	return []Result{
		{ID: "a", Run: 1, Kind: kindCreate, Passed: true, LatencySeconds: scored(10), CostUSD: scored(0.01)},
		{ID: "a", Run: 2, Kind: kindCreate, Passed: false, LatencySeconds: scored(30), CostUSD: scored(0.03)},
		{ID: "a", Run: 3, Kind: kindCreate, Passed: true, LatencySeconds: scored(20), CostUSD: scored(0.02)},
		{ID: "b", Run: 1, Kind: kindInvestigate, Passed: true, DoneWhen: true, LatencySeconds: scored(40)},
		{ID: "b", Run: 2, Kind: kindInvestigate, Passed: true, DoneWhen: true, LatencySeconds: scored(50)},
		{ID: "b", Run: 3, Kind: kindInvestigate, Skipped: true},
	}
}

func TestRepeatedRunsCountInEveryRateAndPerScenario(t *testing.T) {
	agg := aggregate(repeatedResults())
	if agg.Scenarios != tally(4, 5) || agg.Skipped != 1 {
		t.Fatalf("scenarios %+v, skipped %d", agg.Scenarios, agg.Skipped)
	}
	if want := (tallies{"a": tally(2, 3), "b": tally(2, 2)}); len(agg.Runs) != 2 || agg.Runs["a"] != want["a"] || agg.Runs["b"] != want["b"] {
		t.Fatalf("runs %v, want %v", agg.Runs, want)
	}
	if agg.Latency == nil || agg.Latency.Max != 50 || agg.Latency.P50 != 30 || agg.Cost.Priced != 3 {
		t.Fatalf("latency %+v, cost %+v", agg.Latency, agg.Cost)
	}
	markdown := renderMarkdown(Report{Aggregates: agg, Results: repeatedResults()}, gateOutcome{})
	for _, line := range []string{"| Scenario | Runs passed | Rate |", "| a | 2 of 3 | 67% |", "| b | 2 of 2 | 100% |", "| a run 2 |", "| b run 3 |"} {
		if !strings.Contains(markdown, line) {
			t.Errorf("markdown lacks %q:\n%s", line, markdown)
		}
	}
	if single := aggregate([]Result{{ID: "a", Passed: true}}); single.Runs != nil {
		t.Fatalf("a single run reports runs %v", single.Runs)
	}
}

func TestDoneWhenGateScalesWithTheRepeatCount(t *testing.T) {
	limits := gateLimits{minDoneWhen: 2, doneWhen: 2, repeat: 3}
	cases := []struct {
		got  Tally
		want []string
	}{
		{tally(5, 6), []string{"5 of 6 done-when runs passed, below --min-done-when 2 in each of 3 repeats"}},
		{tally(6, 6), nil},
		{tally(1, 3), nil},
	}
	for _, tc := range cases {
		if got := doneWhenGateFailures(tc.got, limits); !slices.Equal(got, tc.want) {
			t.Errorf("%+v: failures %q, want %q", tc.got, got, tc.want)
		}
	}
	if got := doneWhenGateFailures(tally(1, 2), gateLimits{minDoneWhen: 2, doneWhen: 2}); !slices.Equal(got, []string{"1 of 2 done-when scenarios passed, below --min-done-when 2"}) {
		t.Fatalf("one run: %q", got)
	}
}

func TestCompareMatchesRepeatedRunsByScenario(t *testing.T) {
	base := Report{Model: "m", Results: []Result{
		{ID: "a", Passed: true, LatencySeconds: scored(15), CostUSD: scored(0.02)},
		{ID: "b", Passed: true},
	}}
	next := Report{Model: "m", Results: repeatedResults()}
	markdown := renderCompare(base, next, "base.json", "new.json")
	for _, line := range []string{
		"| a | 1 of 1 (100%) -> 2 of 3 (67%) | 15.0s / 20.0s | +5.0s | $0.0200 / $0.0200 | +$0.0000 |",
		"| b | 1 of 1 (100%) -> 2 of 2 (100%) |",
		"| m | 2 of 2 | 4 of 5 | +2 |",
	} {
		if !strings.Contains(markdown, line) {
			t.Errorf("comparison lacks %q:\n%s", line, markdown)
		}
	}
}
