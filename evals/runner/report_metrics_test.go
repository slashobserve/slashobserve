package main

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var metricsEpoch = time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)

func at(seconds int) time.Time { return metricsEpoch.Add(time.Duration(seconds) * time.Second) }

func saveStep(seconds int, phase, tool, status string) Step {
	return Step{At: at(seconds), Phase: phase, Tool: tool, Status: status}
}

func near(got *float64, want float64) bool {
	return got != nil && math.Abs(*got-want) < 1e-9
}

func TestFirstPanelIsTheFirstAcceptedSaveAfterARejectedOne(t *testing.T) {
	steps := []Step{
		saveStep(2, "discover", "list_metrics", "ok"),
		saveStep(5, "save", "create_dashboard", "error"),
		saveStep(9, "repair", "create_dashboard", "ok"),
		saveStep(20, "save", "update_panels", "ok"),
	}
	got := firstPanelSeconds(metricsEpoch, earliestSave(steps, nil))
	if !near(got, 9) {
		t.Fatalf("first panel seconds %v, want 9", got)
	}
}

func TestFirstPanelIgnoresOkStepsThatAreNotSaves(t *testing.T) {
	steps := []Step{
		saveStep(1, "save", "lint_query", "ok"),
		saveStep(2, "query", "update_panels", "ok"),
		saveStep(3, "discover", "create_dashboard", "ok"),
	}
	if got := earliestSave(steps, nil); got != nil {
		t.Fatalf("earliest save %v, want none", got)
	}
}

func TestFirstPanelSurvivesAPollThatNoLongerListsTheFirstSave(t *testing.T) {
	var polls atomic.Int32
	responses := []AgentRequest{
		{ID: "r1", Status: "running", Steps: []Step{saveStep(7, "save", "create_dashboard", "ok"), saveStep(12, "save", "update_panels", "ok")}},
		{ID: "r1", Status: "running", Steps: []Step{saveStep(12, "save", "update_panels", "ok"), saveStep(15, "save", "update_panels", "ok")}},
		{ID: "r1", Status: "succeeded", Steps: []Step{saveStep(15, "save", "update_panels", "ok"), saveStep(18, "save", "update_panels", "ok")}},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		i := min(int(polls.Add(1))-1, len(responses)-1)
		_ = json.NewEncoder(w).Encode(map[string]AgentRequest{"request": responses[i]})
	}))
	t.Cleanup(srv.Close)
	c, err := newClient(srv.URL, "key")
	if err != nil {
		t.Fatal(err)
	}
	c.poll = 5 * time.Millisecond

	req, err := c.waitObserve(t.Context(), AgentRequest{ID: "r1", CreatedAt: metricsEpoch})
	if err != nil {
		t.Fatal(err)
	}
	if polls.Load() < 3 {
		t.Fatalf("polled %d times, want 3", polls.Load())
	}
	if got := firstPanelSeconds(metricsEpoch, req.FirstSaveAt); !near(got, 7) {
		t.Fatalf("first panel seconds %v, want 7", got)
	}
}

func TestFirstPanelIsNilWithoutAnAcceptedSave(t *testing.T) {
	if got := firstPanelSeconds(metricsEpoch, nil); got != nil {
		t.Errorf("no save: %v", got)
	}
	save := at(4)
	if got := firstPanelSeconds(time.Time{}, &save); got != nil {
		t.Errorf("no created_at: %v", got)
	}
}

func TestSuggestAndImportKindsHaveNoFirstPanel(t *testing.T) {
	save := at(6)
	req := AgentRequest{CreatedAt: metricsEpoch, FirstSaveAt: &save}
	for _, kind := range []string{kindSuggest, kindChatImport, kindSizeGuard} {
		if got := withMetrics(Result{}, kind, req); got.FirstPanelSeconds != nil {
			t.Errorf("%s: first panel %v, want nil", kind, *got.FirstPanelSeconds)
		}
	}
	for _, kind := range []string{kindCreate, kindRevise, kindFixUnverified} {
		if got := withMetrics(Result{}, kind, req); !near(got.FirstPanelSeconds, 6) {
			t.Errorf("%s: first panel %v, want 6", kind, got.FirstPanelSeconds)
		}
	}
}

func TestPanelsPerMinuteCountsFromTheFirstPanel(t *testing.T) {
	first, done := at(30), at(150)
	req := AgentRequest{FirstSaveAt: &first, FinishedAt: &done}
	if got := panelsPerMinute(req, 3); !near(got, 1.0) {
		t.Errorf("3 panels over 2 minutes: %v, want 1.0", got)
	}
	if got := panelsPerMinute(req, 1); got != nil {
		t.Errorf("1 panel: %v, want nil", got)
	}
	if got := panelsPerMinute(AgentRequest{FinishedAt: &done}, 5); got != nil {
		t.Errorf("no first save: %v, want nil", got)
	}
	if got := panelsPerMinute(AgentRequest{FirstSaveAt: &done, FinishedAt: &done}, 5); got != nil {
		t.Errorf("no time elapsed: %v, want nil", got)
	}
}

func TestPerCallRatiosOfACreate(t *testing.T) {
	req := AgentRequest{Usage: &Usage{ModelCalls: 4, InputTokens: 100, CacheReadTokens: 200, CacheWriteTokens: 100, OutputTokens: 40}}
	got := withMetrics(Result{Panels: 8}, kindCreate, req)
	if !near(got.CallsPerPanel, 0.5) || !near(got.InputTokensPerCall, 100) || !near(got.OutputTokensPerCall, 10) {
		t.Errorf("calls per panel %v, input per call %v, output per call %v", got.CallsPerPanel, got.InputTokensPerCall, got.OutputTokensPerCall)
	}
	revise := withMetrics(Result{Panels: 8}, kindRevise, req)
	if revise.CallsPerPanel != nil || revise.PanelsPerMinute != nil {
		t.Errorf("a revise reports create-only metrics: %v %v", revise.CallsPerPanel, revise.PanelsPerMinute)
	}
	if empty := withMetrics(Result{}, kindCreate, AgentRequest{Usage: &Usage{}}); empty.CallsPerPanel != nil || empty.InputTokensPerCall != nil {
		t.Errorf("no model calls: %v %v", empty.CallsPerPanel, empty.InputTokensPerCall)
	}
}

func TestUsageDecodesFromAnOlderServerAsZeros(t *testing.T) {
	var req AgentRequest
	if err := json.Unmarshal([]byte(`{"id":"r","usage":{"model_calls":3}}`), &req); err != nil {
		t.Fatal(err)
	}
	u := req.Usage
	if u.ModelCalls != 3 || u.BuildSaves != 0 || u.Renewals != 0 || u.SaveNowRefusals != 0 || u.StalledTurns != 0 || u.OnePanelRefusals != 0 || req.Steps != nil {
		t.Errorf("decoded %+v", u)
	}
}

func metricsResult(id string, first, rate float64, usage *Usage) Result {
	r := Result{ID: id, Category: "prometheus", Passed: true, Usage: usage, FirstPanelSeconds: &first}
	if rate > 0 {
		r.PanelsPerMinute = &rate
	}
	return r
}

func TestAggregatesSummarizeFirstPanelAndRefusals(t *testing.T) {
	results := []Result{
		metricsResult("a", 10, 2, &Usage{BuildSaves: 5, Renewals: 1, SaveNowRefusals: 2, StalledTurns: 1, OnePanelRefusals: 1}),
		metricsResult("b", 30, 4, &Usage{BuildSaves: 3, SaveNowRefusals: 1}),
		metricsResult("c", 20, 0, nil),
		{ID: "d", Category: "prometheus", Passed: true, Kind: kindSuggest},
	}
	agg := aggregate(results)
	fp := agg.FirstPanel
	if fp == nil || fp.N != 3 || fp.P50 != 20 || fp.P90 != 30 || fp.Max != 30 {
		t.Fatalf("first panel %+v", fp)
	}
	if !near(agg.PanelsPerMinuteP50, 2) {
		t.Errorf("panels per minute p50 %v", agg.PanelsPerMinuteP50)
	}
	if agg.BuildSaves != 8 || agg.Renewals != 1 || agg.SaveNowRefusals != 3 || agg.StalledTurns != 1 || agg.OnePanelRefusals != 1 {
		t.Errorf("totals %+v", agg)
	}
	if none := aggregate([]Result{{ID: "x", Passed: true}}); none.FirstPanel != nil || none.PanelsPerMinuteP50 != nil {
		t.Errorf("no runs with a first panel: %+v", none.FirstPanel)
	}
}

func TestMarkdownLeadsWithTimeToFirstPanel(t *testing.T) {
	results := []Result{
		metricsResult("rebuild-a", 12.5, 3, &Usage{BuildSaves: 7, SaveNowRefusals: 2, OnePanelRefusals: 1}),
		metricsResult("rebuild-b", 40, 0, &Usage{BuildSaves: 1}),
	}
	agg := aggregate(results)
	markdown := renderMarkdown(Report{Aggregates: agg, Results: results}, gateOutcome{})
	if want := "Time to first panel: p50 12.5s, p90 40.0s, max 40.0s (2 runs)\n"; !strings.Contains(markdown, "\n"+want+"\n| Summary | Value |") {
		t.Errorf("markdown lacks %q before the summary table:\n%s", want, markdown)
	}
	for _, want := range []string{"| Scenario | First panel | Panels/min | Saves | Refusals |", "| rebuild-a | 12.5s | 3.0 | 7 | 3 |", "| rebuild-b | 40.0s | n/a | 1 | 0 |"} {
		if !strings.Contains(markdown, want) {
			t.Errorf("markdown lacks %q:\n%s", want, markdown)
		}
	}
	for _, line := range strings.Split(markdown, "\n") {
		if strings.HasPrefix(line, "| rebuild-a ") && !strings.HasPrefix(line, "| rebuild-a |") {
			t.Errorf("the id must be its own first column: %q", line)
		}
	}
	empty := renderMarkdown(Report{Aggregates: aggregate(nil)}, gateOutcome{})
	if !strings.Contains(empty, "\nTime to first panel: no runs\n") {
		t.Errorf("no runs:\n%s", empty)
	}
}

func TestJSONKeepsTheScenariosTally(t *testing.T) {
	results := []Result{metricsResult("a", 5, 0, &Usage{BuildSaves: 1}), {ID: "b", Passed: true}}
	payload, err := json.Marshal(Report{Aggregates: aggregate(results), Results: results})
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	for _, want := range []string{`"scenarios":{"passed":2,"total":2,"rate":1}`, `"first_panel":{"p50":5,"p90":5,"max":5,"n":1}`, `"first_panel_seconds":5`} {
		if !strings.Contains(text, want) {
			t.Errorf("JSON lacks %s:\n%s", want, text)
		}
	}
}

func TestMatrixLeadsWithFirstPanelP50(t *testing.T) {
	with := Report{Provider: "p1", Model: "m1", Aggregates: aggregate([]Result{metricsResult("a", 12, 0, nil), metricsResult("b", 18, 0, nil)})}
	without := Report{Provider: "p2", Model: "m2", Aggregates: aggregate([]Result{{ID: "c", Passed: true}})}
	table := renderMatrix([]matrixResult{{name: "alpha", report: with}, {name: "beta", report: without}})
	header := "| Kind | alpha | beta |\n| --- | --- | --- |\n| First panel p50 | 12.0s | n/a |\n"
	if !strings.Contains(table, header) {
		t.Errorf("the first matrix row is not the first panel p50:\n%s", table)
	}
}
