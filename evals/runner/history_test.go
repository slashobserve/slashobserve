package main

import (
	"context"
	"errors"
	"io"
	"slices"
	"sync"
	"testing"
	"time"
)

func scenarioWithRange(id string, from ...string) Scenario {
	sc := sampleScenarios(1)[0]
	sc.ID = id
	sc.Expect.TimeRangeFrom = from
	return sc
}

func planIndexes(plan []dispatch) []int {
	out := make([]int, len(plan))
	for i, d := range plan {
		out[i] = d.index
	}
	return out
}

func waitsAt(plan []dispatch) []int {
	var out []int
	for i, d := range plan {
		if d.awaitHistory {
			out = append(out, i)
		}
	}
	return out
}

func TestHistoryNeedComesFromTheTimeRangeTheScenarioExpects(t *testing.T) {
	if got := historyNeed(scenarioWithRange("a")); got != 0 {
		t.Errorf("no time range: need %s", got)
	}
	if got := historyNeed(scenarioWithRange("a", "now-1h", "now-60m")); got != time.Hour {
		t.Errorf("an hour: need %s", got)
	}
	if got := historyNeed(scenarioWithRange("a", "now-15m", "now-6h")); got != 6*time.Hour {
		t.Errorf("the widest range counts: need %s", got)
	}
}

func TestDispatchPlanRunsScenariosWithoutHistoryFirstAndLeastDemandingNeedFirst(t *testing.T) {
	scenarios := []Scenario{
		scenarioWithRange("wide", "now-6h"),
		scenarioWithRange("plain-1"),
		scenarioWithRange("narrow", "now-15m"),
		scenarioWithRange("plain-2"),
	}
	plan := dispatchPlan(scenarios, true)
	if got, want := planIndexes(plan), []int{1, 3, 2, 0}; !slices.Equal(got, want) {
		t.Fatalf("order %v, want %v", got, want)
	}
	if got, want := waitsAt(plan), []int{2}; !slices.Equal(got, want) {
		t.Fatalf("waits before positions %v, want %v", got, want)
	}
}

func TestDispatchPlanWithNoHistoryNeededNeverWaits(t *testing.T) {
	scenarios := sampleScenarios(3)
	plan := dispatchPlan(scenarios, true)
	if got, want := planIndexes(plan), []int{0, 1, 2}; !slices.Equal(got, want) {
		t.Fatalf("order %v, want %v", got, want)
	}
	if waits := waitsAt(plan); len(waits) != 0 {
		t.Fatalf("waits before positions %v, want none", waits)
	}
}

func TestDispatchPlanWhenEveryScenarioNeedsHistoryWaitsOnceBeforeTheFirst(t *testing.T) {
	scenarios := []Scenario{scenarioWithRange("b", "now-6h"), scenarioWithRange("a", "now-1h"), scenarioWithRange("c", "now-6h")}
	plan := dispatchPlan(scenarios, true)
	if got, want := planIndexes(plan), []int{1, 0, 2}; !slices.Equal(got, want) {
		t.Fatalf("order %v, want %v", got, want)
	}
	if got, want := waitsAt(plan), []int{0}; !slices.Equal(got, want) {
		t.Fatalf("waits before positions %v, want %v", got, want)
	}
}

func TestDispatchPlanWithoutTheFlagKeepsScenarioOrderAndNeverWaits(t *testing.T) {
	scenarios := []Scenario{scenarioWithRange("wide", "now-6h"), scenarioWithRange("plain")}
	plan := dispatchPlan(scenarios, false)
	if got, want := planIndexes(plan), []int{0, 1}; !slices.Equal(got, want) {
		t.Fatalf("order %v, want %v", got, want)
	}
	if waits := waitsAt(plan); len(waits) != 0 {
		t.Fatalf("waits before positions %v, want none", waits)
	}
}

func TestAwaitHistoryRunsTheWaitOnceAndKeepsResultsInScenarioOrder(t *testing.T) {
	_, c := newFake(t, 1, 10*time.Millisecond)
	scenarios := []Scenario{scenarioWithRange("needy", "now-6h"), scenarioWithRange("plain-1"), scenarioWithRange("plain-2")}
	var mu sync.Mutex
	var waited []time.Time
	o := options{parallel: 1, repeat: 2, timeout: 5 * time.Second, awaitHistory: true, waitHistory: func(context.Context) error {
		time.Sleep(100 * time.Millisecond)
		mu.Lock()
		defer mu.Unlock()
		waited = append(waited, time.Now())
		return nil
	}}
	results := runAll(t.Context(), c, Pricing{}, scenarios, o, io.Discard)
	if len(waited) != 1 {
		t.Fatalf("the wait ran %d times, want once", len(waited))
	}
	var ids []string
	for _, r := range results {
		ids = append(ids, r.ID)
	}
	if want := []string{"needy", "needy", "plain-1", "plain-1", "plain-2", "plain-2"}; !slices.Equal(ids, want) {
		t.Fatalf("result order %v, want %v", ids, want)
	}
	for _, r := range results {
		switch {
		case r.ID == "needy" && r.Run == 1 && r.StartedAt.Before(waited[0]):
			t.Errorf("needy started at %s, before the wait ended at %s", r.StartedAt, waited[0])
		case r.ID != "needy" && r.Run == 1 && r.StartedAt.After(waited[0]):
			t.Errorf("%s started at %s, after the wait ended at %s", r.ID, r.StartedAt, waited[0])
		}
	}
}

func TestAwaitHistoryFlagSelectsTheWaitThroughTheStackScript(t *testing.T) {
	o, err := parseFlags([]string{"--await-history"}, io.Discard)
	if err != nil || !o.awaitHistory || o.waitHistory == nil {
		t.Fatalf("--await-history: %+v, %v", o.awaitHistory, err)
	}
	o, err = parseFlags(nil, io.Discard)
	if err != nil || o.awaitHistory || o.waitHistory != nil {
		t.Fatalf("default: %+v, %v", o.awaitHistory, err)
	}
}

func TestFailedHistoryWaitFailsTheScenariosThatNeedHistoryWithTheWaitError(t *testing.T) {
	_, c := newFake(t, 1, 10*time.Millisecond)
	scenarios := []Scenario{scenarioWithRange("needy", "now-6h"), scenarioWithRange("plain")}
	waitErr := errors.New("wait-history: exit status 1")
	o := options{parallel: 1, repeat: 2, timeout: 5 * time.Second, awaitHistory: true, waitHistory: func(context.Context) error { return waitErr }}
	results := runAll(t.Context(), c, Pricing{}, scenarios, o, io.Discard)
	if len(results) != 4 {
		t.Fatalf("%d results, want 4", len(results))
	}
	for _, r := range results {
		switch r.ID {
		case "needy":
			if r.Passed || r.Error != waitErr.Error() {
				t.Errorf("needy run %d: passed %v, error %q, want a failure with %q", r.Run, r.Passed, r.Error, waitErr)
			}
		case "plain":
			if r.Error == waitErr.Error() {
				t.Errorf("plain run %d failed with the wait error", r.Run)
			}
		}
	}
}

func TestHistoryWaitPassesTheK8sProfileToTheStackScript(t *testing.T) {
	if got := historyWaitArgs(profileK8s); !slices.Equal(got, []string{"wait-history", "--k8s"}) {
		t.Fatalf("k8s: %v", got)
	}
	if got := historyWaitArgs(profileDefault); !slices.Equal(got, []string{"wait-history"}) {
		t.Fatalf("default: %v", got)
	}
}
