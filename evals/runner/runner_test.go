package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const scenarioHeader = `id: sample
prompt: sample prompt
category: prometheus
done_when: false
`

func writeScenario(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sample.yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestShippedScenariosLoad(t *testing.T) {
	scenarios, err := loadScenarios(filepath.Join("..", "scenarios"))
	if err != nil {
		t.Fatal(err)
	}
	today, err := selectScenarios(scenarios, options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(today) != 14 || countDoneWhen(today) != 10 {
		t.Fatalf("a run with no suite selects %d scenarios with %d done_when, want 14 and 10", len(today), countDoneWhen(today))
	}
}

func TestPromptRejectedMatchesOnlyTheServersAnswerForAnOverCapPrompt(t *testing.T) {
	over := strings.Repeat("a", maxPromptRunes+1)
	cases := map[string]struct {
		err    error
		prompt string
		want   bool
	}{
		"prompt cap":                    {&apiError{Status: http.StatusBadRequest, Code: "invalid_request"}, over, true},
		"request body limit":            {&apiError{Status: http.StatusRequestEntityTooLarge, Code: "too_large"}, over, true},
		"wrapped":                       {fmt.Errorf("create: %w", &apiError{Status: http.StatusRequestEntityTooLarge, Code: "too_large"}), over, true},
		"body limit on a short prompt":  {&apiError{Status: http.StatusRequestEntityTooLarge, Code: "too_large"}, "short", false},
		"bad request on a short prompt": {&apiError{Status: http.StatusBadRequest, Code: "invalid_request"}, "short", false},
		"unrelated 413":                 {&apiError{Status: http.StatusRequestEntityTooLarge, Code: "other"}, over, false},
		"unauthorized":                  {&apiError{Status: http.StatusUnauthorized, Code: "unauthorized"}, over, false},
		"not an api error":              {errors.New("connection refused"), over, false},
		"no error":                      {nil, over, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := promptRejected(tc.err, tc.prompt); got != tc.want {
				t.Fatalf("promptRejected = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestScenarioValidation(t *testing.T) {
	cases := map[string]struct {
		extra string
		want  string
		mixed bool
	}{
		"valid": {
			extra: "expect:\n  datasources: [prometheus]\n  metrics: [up]\n  units: [reqps, \"µs\"]\n  threshold_at: {percent: 1}\n  time_range_from: [now-1h]\n  changed_paths: [/time]\n  query_contains_any:\n    - [offset 1d, offset 24h]\n",
		},
		"blank metric": {
			extra: "expect:\n  datasources: [prometheus]\n  metrics: [\"  \"]\n",
			want:  "expect.metrics must not contain an empty entry",
		},
		"empty table": {
			extra: "expect:\n  datasources: [prometheus, clickhouse]\n  tables: [\"\"]\n",
			mixed: true,
			want:  "expect.tables must not contain an empty entry",
		},
		"unknown unit": {
			extra: "expect:\n  datasources: [prometheus]\n  units: [reqps, requests]\n",
			want:  `unknown units "requests"`,
		},
		"unknown threshold unit": {
			extra: "expect:\n  datasources: [prometheus]\n  threshold_at: {pct: 1}\n",
			want:  `unknown units "pct"`,
		},
		"duplicate tag": {
			extra: "tags: [a, a]\nexpect:\n  datasources: [prometheus]\n",
			want:  `tags contain "a" twice`,
		},
		"blank tag": {
			extra: "tags: [\"\"]\nexpect:\n  datasources: [prometheus]\n",
			want:  "tags must not contain an empty entry",
		},
		"empty alternative group": {
			extra: "expect:\n  datasources: [prometheus]\n  query_contains_any:\n    - []\n",
			want:  "expect.query_contains_any[0]",
		},
		"blank changed path": {
			extra: "expect:\n  datasources: [prometheus]\n  changed_paths: [\"\"]\n",
			want:  "expect.changed_paths must not contain an empty entry",
		},
		"bad time range": {
			extra: "expect:\n  datasources: [prometheus]\n  time_range_from: [last-hour]\n",
			want:  "expect.time_range_from",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			header := scenarioHeader
			if tc.mixed {
				header = strings.Replace(header, "category: prometheus", "category: mixed", 1)
			}
			_, err := loadScenarios(writeScenario(t, header+tc.extra))
			switch {
			case tc.want == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
				t.Fatalf("error %v does not contain %q", err, tc.want)
			}
		})
	}
}

var testDatasourceTypes = map[string]string{"prom": "prometheus", "ch": "clickhouse"}

func dashboard(timeFrom string, panels ...spec.Panel) *spec.Dashboard {
	return &spec.Dashboard{Time: spec.TimeRange{From: timeFrom, To: "now"}, Panels: panels}
}

func observe(d *spec.Dashboard) observation {
	return observation{dashboard: d, datasourceTypes: testDatasourceTypes}
}

func threshold(value float64) spec.Threshold {
	return spec.Threshold{Value: &value}
}

func TestFeatureChecks(t *testing.T) {
	ex := Expect{
		QueryContainsAny: [][]string{{"offset 1d", "offset 24h"}, {"LIMIT 10", "topk(10)"}},
		TimeRangeFrom:    []string{"now-1h"},
		ThresholdAt:      map[string]float64{"percentunit": 0.01, "percent": 1},
	}
	prom := func(query string) spec.Panel {
		return spec.Panel{Type: spec.Timeseries, Datasource: "prom", Queries: []spec.Query{{Query: query}}, FieldProps: spec.FieldProps{Unit: "percentunit", Thresholds: []spec.Threshold{threshold(0.01)}}}
	}
	sql := func(query string) spec.Panel {
		return spec.Panel{Type: spec.Table, Datasource: "ch", Queries: []spec.Query{{Query: query}}}
	}
	good := observe(dashboard("now-1h",
		prom("sum(rate(x[5m])) or sum(rate(x[5m] offset  24h))"),
		sql("SELECT 1\nLIMIT   10"),
	))
	bad := observe(dashboard("now-6h",
		spec.Panel{Type: spec.Timeseries, Datasource: "prom", Queries: []spec.Query{{Query: "sum(rate(x[5m]))"}}, FieldProps: spec.FieldProps{Unit: "percentunit", Thresholds: []spec.Threshold{threshold(1)}}},
	))
	for name, check := range map[string]func(Expect, observation) CheckResult{
		"query":      checkQueryGroups,
		"time_range": checkTimeRange,
		"thresholds": checkThresholds,
	} {
		if r := check(ex, good); !r.Passed {
			t.Errorf("%s should pass: %s", name, r.Detail)
		}
		if r := check(ex, bad); r.Passed {
			t.Errorf("%s should fail", name)
		}
	}

	p50, p95 := []string{"0.5", "0.50"}, []string{"0.95", "0.950"}
	queryCases := []struct {
		name   string
		group  []string
		panels []spec.Panel
		want   bool
	}{
		{"upper case", []string{"limit 10"}, []spec.Panel{sql("SELECT 1 LIMIT 10")}, true},
		{"newline between words", []string{"limit 10"}, []spec.Panel{sql("SELECT 1\nLIMIT\n10\n")}, true},
		{"limit 100", []string{"limit 10"}, []spec.Panel{sql("SELECT 1 LIMIT 100")}, false},
		{"limit 10 by", []string{"limit 10"}, []spec.Panel{sql("SELECT 1 LIMIT 10 BY x")}, true},
		{"clickhouse topK", []string{"limit 10", "topk(10)"}, []spec.Panel{sql("SELECT topK(10)(Body) FROM otel_logs")}, true},
		{"p50 as 0.5", p50, []spec.Panel{prom("histogram_quantile(0.5, x)")}, true},
		{"p50 as 0.50", p50, []spec.Panel{prom("histogram_quantile(0.50, x)")}, true},
		{"p50 not in 0.95", p50, []spec.Panel{prom("histogram_quantile(0.95, x)")}, false},
		{"p50 not in 2.5", p50, []spec.Panel{prom("sum(x) > 2.5")}, false},
		{"p95 as 0.95", p95, []spec.Panel{prom("histogram_quantile(0.95, x)")}, true},
		{"p95 as 0.950", p95, []spec.Panel{prom("histogram_quantile(0.950, x)")}, true},
		{"p95 not in 0.995", p95, []spec.Panel{prom("histogram_quantile(0.995, x)")}, false},
		{"promql comment", []string{"offset 1d"}, []spec.Panel{prom("sum(x)\n# compare with offset 1d\n")}, false},
		{"promql real use after comment", []string{"offset 1d"}, []spec.Panel{prom("sum(x) # now\n or sum(x offset 1d)")}, true},
		{"hash inside a promql string", []string{"offset 1d"}, []spec.Panel{prom(`sum(x{a="#"} offset 1d)`)}, true},
		{"hash in sql is not a comment", []string{"limit 10"}, []spec.Panel{sql("SELECT '#' # x\nLIMIT 10")}, true},
	}
	for _, tc := range queryCases {
		obs := observe(dashboard("now-1h", tc.panels...))
		r := checkQueryGroups(Expect{QueryContainsAny: [][]string{tc.group}}, obs)
		if r.Passed != tc.want {
			t.Errorf("%s: passed %t, want %t", tc.name, r.Passed, tc.want)
		}
	}
}

func TestChecksFollowEveryQueryAndSkipNonDataPanels(t *testing.T) {
	multi := spec.Panel{
		ID: "multi", Type: spec.Timeseries, Datasource: "prom",
		Queries:    []spec.Query{{Query: "up"}, {Query: "SELECT count() FROM otel_logs", Datasource: "ch"}},
		FieldProps: spec.FieldProps{Thresholds: []spec.Threshold{{Color: "ok"}, threshold(1)}},
	}
	note := spec.Panel{ID: "note", Type: spec.Text}
	row := spec.Panel{ID: "r", Type: spec.Row, Panels: []spec.Panel{{ID: "child", Type: spec.Stat, Datasource: "ch", Queries: []spec.Query{{Query: "SELECT 1"}}}}}
	obs := observe(dashboard("now-1h", multi, note, row))

	if r := checkSource(Expect{Datasources: []string{"clickhouse", "prometheus"}}, obs); !r.Passed {
		t.Errorf("source: %s", r.Detail)
	}
	if r := checkQueries("tables", []string{"otel_logs"}, spec.ClickHouse, 0, 0, obs); !r.Passed {
		t.Errorf("tables: %s", r.Detail)
	}
	if r := checkPanels(Expect{MinPanels: 2, MaxPanels: 2}, obs); !r.Passed {
		t.Errorf("panels should count the two data panels: %s", r.Detail)
	}
	if r := checkPanels(Expect{MinPanels: 3, MaxPanels: 4}, obs); r.Passed {
		t.Error("panels should not count the text panel or the row")
	}
	if r := checkThresholds(Expect{ThresholdAt: map[string]float64{"": 1}}, obs); !r.Passed {
		t.Errorf("thresholds: %s", r.Detail)
	}
	if r := checkThresholds(Expect{ThresholdAt: map[string]float64{"": 0}}, obs); r.Passed {
		t.Error("a threshold without a value must not match")
	}
}

func TestGateUsesLoadedDoneWhenCount(t *testing.T) {
	agg := Aggregates{Scenarios: Tally{Rate: 1}, DoneWhen: Tally{Passed: 3, Total: 4}}
	if g := evaluateGates(agg, 0.8, 3, 4); !g.ok() {
		t.Fatalf("3 of 4 should pass --min-done-when 3: %v", g.failures)
	}
	if g := evaluateGates(agg, 0.8, 4, 4); g.ok() {
		t.Fatal("3 of 4 should fail --min-done-when 4")
	}
	if g := evaluateGates(agg, 0.8, 4, 10); !g.ok() {
		t.Fatal("the gate applies only when every done_when scenario ran")
	}
}

func TestParseFlagsBounds(t *testing.T) {
	for _, args := range [][]string{{"--parallel", "4"}, {"--parallel", "0"}} {
		if _, err := parseFlags(args, io.Discard); err == nil {
			t.Errorf("%v should be rejected", args)
		}
	}
	if o, err := parseFlags([]string{"--parallel", "3"}, io.Discard); err != nil || o.parallel != 3 {
		t.Errorf("--parallel 3 should be accepted: %v", err)
	}
}

func TestRunRejectsMinDoneWhenAboveCount(t *testing.T) {
	var stderr strings.Builder
	rc := run([]string{"--scenarios", filepath.Join("..", "scenarios"), "--min-done-when", "11", "--server", "http://127.0.0.1:1", "--token", "x"}, io.Discard, &stderr)
	if rc != 2 || !strings.Contains(stderr.String(), "--min-done-when must be between 0 and 10") {
		t.Fatalf("rc %d, stderr %q", rc, stderr.String())
	}
}

type fakeServer struct {
	mu        sync.Mutex
	limit     int
	runFor    time.Duration
	finishAt  map[string]time.Time
	rejected  int
	maxActive int
	getFails  int
	getHangs  int
	getBad    bool
	next      int
	code      string
}

func (f *fakeServer) active(now time.Time) int {
	n := 0
	for _, at := range f.finishAt {
		if now.Before(at) {
			n++
		}
	}
	return n
}

func (f *fakeServer) occupy(n int, until time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for range n {
		f.next++
		f.finishAt[fmt.Sprintf("occupant-%d", f.next)] = time.Now().Add(until)
	}
}

func (f *fakeServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/agent", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"available":true,"provider":"replay","model":"replay"}`)
	})
	mux.HandleFunc("POST /api/v1/observe", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		now := time.Now()
		if f.active(now) >= f.limit {
			f.rejected++
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"error":"too many active requests","code":"too_many_requests"}`)
			return
		}
		f.next++
		id := fmt.Sprintf("req-%d", f.next)
		f.finishAt[id] = now.Add(f.runFor)
		f.maxActive = max(f.maxActive, f.active(now))
		w.WriteHeader(http.StatusAccepted)
		_, _ = fmt.Fprintf(w, `{"request":{"id":%q,"status":"queued","created_at":%q}}`, id, now.Format(time.RFC3339Nano))
	})
	mux.HandleFunc("GET /api/v1/observe/{id}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		switch {
		case f.getFails > 0:
			f.getFails--
			f.mu.Unlock()
			w.WriteHeader(http.StatusBadGateway)
			return
		case f.getBad:
			f.mu.Unlock()
			_, _ = io.WriteString(w, `{"request": `)
			return
		case f.getHangs > 0:
			f.getHangs--
			f.mu.Unlock()
			time.Sleep(300 * time.Millisecond)
			return
		}
		id := r.PathValue("id")
		at := f.finishAt[id]
		f.mu.Unlock()
		created := at.Add(-f.runFor)
		body := map[string]any{"id": id, "created_at": created, "status": "running"}
		if !time.Now().Before(at) {
			body["status"], body["error"], body["finished_at"] = "failed", "boom", at
			if f.code != "" {
				body["failure_code"] = f.code
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"request": body})
	})
	return mux
}

func newFake(t *testing.T, limit int, runFor time.Duration) (*fakeServer, *client) {
	t.Helper()
	f := &fakeServer{limit: limit, runFor: runFor, finishAt: map[string]time.Time{}}
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)
	c, err := newClient(srv.URL, "key")
	if err != nil {
		t.Fatal(err)
	}
	c.poll, c.backoff, c.grace = 10*time.Millisecond, 10*time.Millisecond, 10*time.Second
	return f, c
}

func newFakeURL(t *testing.T, runFor time.Duration) string {
	t.Helper()
	f := &fakeServer{limit: 3, runFor: runFor, finishAt: map[string]time.Time{}}
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)
	return srv.URL
}

func sampleScenarios(n int) []Scenario {
	out := make([]Scenario, n)
	for i := range out {
		out[i] = Scenario{ID: fmt.Sprintf("s-%d", i), Prompt: "p", Category: categoryPrometheus, Expect: Expect{Datasources: []string{"prometheus"}, MinPanels: 1, MaxPanels: 12, MaxSeconds: 60}}
	}
	return out
}

func TestTimedOutRequestKeepsItsSlotUntilItFinishes(t *testing.T) {
	f, c := newFake(t, 3, 400*time.Millisecond)
	results := runAll(t.Context(), c, Pricing{}, sampleScenarios(6), options{parallel: 3, timeout: 100 * time.Millisecond}, io.Discard)
	if f.rejected != 0 || f.maxActive > 3 {
		t.Fatalf("rejected %d creates, max active %d", f.rejected, f.maxActive)
	}
	for _, r := range results {
		if !strings.Contains(r.Error, "timed out after 100ms") || r.RequestID == "" || r.Status == "failed" {
			t.Errorf("%s: error %q, request %q, status %q", r.ID, r.Error, r.RequestID, r.Status)
		}
	}
}

func TestCreateRetriesWhenSlotsAreFull(t *testing.T) {
	f, c := newFake(t, 3, 20*time.Millisecond)
	f.occupy(3, 150*time.Millisecond)
	res, _ := runScenario(t.Context(), c, Pricing{}, sampleScenarios(1)[0], options{timeout: 5 * time.Second})
	if res.Error != "boom" || res.RequestID == "" {
		t.Fatalf("error %q, request %q", res.Error, res.RequestID)
	}
	if f.rejected == 0 {
		t.Fatal("the first create should have been rejected")
	}
}

func TestCreateGivesUpAtTheScenarioDeadline(t *testing.T) {
	f, c := newFake(t, 3, 20*time.Millisecond)
	f.occupy(3, time.Hour)
	res, abandoned := runScenario(t.Context(), c, Pricing{}, sampleScenarios(1)[0], options{timeout: 100 * time.Millisecond})
	if !strings.Contains(res.Error, "no request slot within 100ms") || abandoned != nil {
		t.Fatalf("error %q, abandoned %v", res.Error, abandoned)
	}
}

func TestPollRetriesServerAndClientTimeoutErrors(t *testing.T) {
	f, c := newFake(t, 3, 20*time.Millisecond)
	f.getFails, f.getHangs = 2, 1
	c.http.Timeout = 100 * time.Millisecond
	res, abandoned := runScenario(t.Context(), c, Pricing{}, sampleScenarios(1)[0], options{timeout: 5 * time.Second})
	if res.Error != "boom" || res.Status != "failed" || abandoned != nil {
		t.Fatalf("error %q, status %q, abandoned %v", res.Error, res.Status, abandoned)
	}
}

func TestPerCallTimeoutIsNotReportedAsScenarioTimeout(t *testing.T) {
	f, c := newFake(t, 3, 20*time.Millisecond)
	f.getHangs = 1
	c.http.Timeout = 100 * time.Millisecond
	res, _ := runScenario(t.Context(), c, Pricing{}, sampleScenarios(1)[0], options{timeout: 5 * time.Second})
	if strings.Contains(res.Error, "timed out after") {
		t.Fatalf("error %q", res.Error)
	}
}

func TestUndecodablePollResponseFailsFast(t *testing.T) {
	f, c := newFake(t, 3, 20*time.Millisecond)
	f.getBad = true
	started := time.Now()
	res, abandoned := runScenario(t.Context(), c, Pricing{}, sampleScenarios(1)[0], options{timeout: 5 * time.Second})
	if !strings.HasPrefix(res.Error, "poll the request:") || time.Since(started) > time.Second {
		t.Fatalf("error %q after %s", res.Error, time.Since(started))
	}
	if abandoned == nil {
		t.Fatal("an unfinished request should be drained")
	}
}

func TestTimeoutMentionsLongSlotWait(t *testing.T) {
	old := slotWaitNotice
	slotWaitNotice = 50 * time.Millisecond
	t.Cleanup(func() { slotWaitNotice = old })
	f, c := newFake(t, 3, time.Hour)
	f.occupy(3, 150*time.Millisecond)
	res, _ := runScenario(t.Context(), c, Pricing{}, sampleScenarios(1)[0], options{timeout: 600 * time.Millisecond})
	if !strings.Contains(res.Error, "timed out after 600ms") || !strings.Contains(res.Error, "waiting for a request slot)") {
		t.Fatalf("error %q", res.Error)
	}
}

func TestDrainIsAnnounced(t *testing.T) {
	_, c := newFake(t, 3, 300*time.Millisecond)
	var stderr strings.Builder
	runAll(t.Context(), c, Pricing{}, sampleScenarios(1), options{parallel: 1, timeout: 50 * time.Millisecond}, &stderr)
	if !strings.Contains(stderr.String(), "draining req-1 (up to 10s)") || !strings.Contains(stderr.String(), "drained req-1: failed") {
		t.Fatalf("stderr %q", stderr.String())
	}
}

func TestMinDoneWhenDefaultFollowsCustomFolder(t *testing.T) {
	dir := writeScenario(t, strings.Replace(scenarioHeader, "done_when: false", "done_when: true", 1)+"expect:\n  datasources: [prometheus]\n")
	url := newFakeURL(t, 0)
	args := []string{"--scenarios", dir, "--server", url, "--token", "key", "--pricing", filepath.Join("..", "pricing.yaml")}

	var stdout, stderr strings.Builder
	if rc := run(args, &stdout, &stderr); rc != 1 || !strings.Contains(stdout.String(), "# Eval report") {
		t.Fatalf("rc %d, stdout %q, stderr %q", rc, stdout.String(), stderr.String())
	}

	stderr.Reset()
	if rc := run(append(args, "--min-done-when", "2"), io.Discard, &stderr); rc != 2 || !strings.Contains(stderr.String(), "--min-done-when must be between 0 and 1") {
		t.Fatalf("rc %d, stderr %q", rc, stderr.String())
	}
}

func TestFailedRunsCarryTheServersFailureCodeOrAreFlaggedUncoded(t *testing.T) {
	for name, code := range map[string]string{"coded": "rate_limited", "uncoded": ""} {
		t.Run(name, func(t *testing.T) {
			f, c := newFake(t, 3, 20*time.Millisecond)
			f.code = code
			results := runAll(t.Context(), c, Pricing{}, sampleScenarios(1), options{parallel: 1, timeout: 5 * time.Second}, io.Discard)
			got := results[0]
			if got.Status != "failed" || got.FailureCode != code || got.Uncoded != (code == "") {
				t.Fatalf("result = status %q, failure code %q, uncoded %v", got.Status, got.FailureCode, got.Uncoded)
			}
		})
	}
}

func TestReportCountsFailureCodesAndRendersThem(t *testing.T) {
	results := []Result{
		{ID: "a", Category: "prometheus", Status: "failed", FailureCode: "rate_limited"},
		{ID: "b", Category: "prometheus", Status: "failed", FailureCode: "rate_limited"},
		{ID: "c", Category: "prometheus", Status: "failed", FailureCode: "call_timeout"},
		{ID: "d", Category: "prometheus", Status: "succeeded", Passed: true},
	}
	report := Report{Aggregates: aggregate(results), Results: results}
	want := map[string]int{"rate_limited": 2, "call_timeout": 1}
	if !reflect.DeepEqual(report.Aggregates.FailureCodes, want) || report.Aggregates.Uncoded != 0 {
		t.Fatalf("failure codes = %v, uncoded %d, want %v", report.Aggregates.FailureCodes, report.Aggregates.Uncoded, want)
	}
	markdown := renderMarkdown(report, evaluateGates(report.Aggregates, 0, 0, 0))
	for _, line := range []string{"| call_timeout | 1 |", "| rate_limited | 2 |", "| Failure code |", "| a | n/a | n/a | n/a | n/a | FAIL | prometheus | n/a | n/a | rate_limited |"} {
		if !strings.Contains(markdown, line) {
			t.Errorf("markdown lacks %q:\n%s", line, markdown)
		}
	}
	payload, err := json.Marshal(report.Aggregates)
	if err != nil || !strings.Contains(string(payload), `"failure_codes":{"call_timeout":1,"rate_limited":2}`) {
		t.Fatalf("aggregates JSON = %s, %v", payload, err)
	}
	empty, err := json.Marshal(aggregate(nil))
	if err != nil || !strings.Contains(string(empty), `"failure_codes":{}`) {
		t.Fatalf("an empty report must still list failure_codes: %s, %v", empty, err)
	}
}

func TestGateFailsForAFailedRunWithoutAFailureCode(t *testing.T) {
	results := []Result{
		{ID: "a", Category: "prometheus", Status: "succeeded", Passed: true},
		{ID: "b", Category: "prometheus", Status: "failed", Uncoded: true},
	}
	agg := aggregate(results)
	gates := evaluateGates(agg, 0, 0, 0)
	if gates.ok() || len(gates.failures) != 1 || !strings.Contains(gates.failures[0], "1 failed runs have no failure code") {
		t.Fatalf("gates = %+v", gates)
	}
	if markdown := renderMarkdown(Report{Aggregates: agg, Results: results}, gates); !strings.Contains(markdown, "| uncoded | 1 |") || !strings.Contains(markdown, "| b | n/a | n/a | n/a | n/a | FAIL | prometheus | n/a | n/a | uncoded |") {
		t.Fatalf("markdown does not show the uncoded run:\n%s", markdown)
	}
	if g := evaluateGates(aggregate(results[:1]), 0, 0, 0); !g.ok() {
		t.Fatalf("a run without failures must not trip the gate: %v", g.failures)
	}
}

func okRequest(extra ...func(*AgentRequest)) AgentRequest {
	started := time.Now().Add(-5 * time.Second)
	finished := started.Add(4 * time.Second)
	req := AgentRequest{ID: "r1", Status: "succeeded", CreatedAt: started, FinishedAt: &finished}
	for _, f := range extra {
		f(&req)
	}
	return req
}

func failedWith(code string) func(*AgentRequest) {
	return func(r *AgentRequest) { r.Status, r.FailureCode = "failed", &code }
}

func withUsage(u Usage) func(*AgentRequest) {
	return func(r *AgentRequest) { r.Usage = &u }
}

func ptr[T any](v T) *T { return &v }

func TestFailureCodeCheck(t *testing.T) {
	cases := map[string]struct {
		expected string
		req      AgentRequest
		want     bool
	}{
		"expected code matches":          {"output_limit", okRequest(failedWith("output_limit")), true},
		"expected code differs":          {"output_limit", okRequest(failedWith("call_timeout")), false},
		"expected failure but succeeded": {"output_limit", okRequest(), false},
		"never started with the code":    {"prompt_too_long", AgentRequest{Status: "not_started", FailureCode: ptr("prompt_too_long")}, true},
		"no expectation and succeeded":   {"", okRequest(), true},
		"no expectation and failed":      {"", okRequest(failedWith("refused")), false},
		"no expectation and unfinished":  {"", AgentRequest{Status: "running"}, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := checkFailureCode(Expect{FailureCode: tc.expected}, observation{request: tc.req})
			if r.Passed != tc.want {
				t.Fatalf("passed %t, want %t (%s)", r.Passed, tc.want, r.Detail)
			}
		})
	}
	if r := checkFailureCode(Expect{}, observation{request: okRequest(failedWith("refused"))}); !strings.Contains(r.Detail, "refused") {
		t.Errorf("a failed run must show its code, got %q", r.Detail)
	}
}

func rowDashboard(childQuery, otherQuery string) *spec.Dashboard {
	child := spec.Panel{ID: "target", Type: spec.Stat, Datasource: "prom", Queries: []spec.Query{{Query: childQuery}}}
	other := spec.Panel{ID: "other", Type: spec.Stat, Datasource: "prom", Queries: []spec.Query{{Query: otherQuery}}}
	row := spec.Panel{ID: "row", Type: spec.Row, Panels: []spec.Panel{child, other}}
	top := spec.Panel{ID: "top", Type: spec.Timeseries, Datasource: "prom", Queries: []spec.Query{{Query: "up"}}}
	return &spec.Dashboard{Time: spec.TimeRange{From: "now-1h", To: "now"}, Panels: []spec.Panel{row, top}}
}

func TestOnlyTargetChangedFollowsKeyedPaths(t *testing.T) {
	parent := rowDashboard("broken", "ok")
	topChanged := rowDashboard("broken", "ok")
	topChanged.Panels[1].Queries[0].Query = "sum(up)"
	reordered := rowDashboard("fixed", "ok")
	reordered.Panels[0].Panels[0], reordered.Panels[0].Panels[1] = reordered.Panels[0].Panels[1], reordered.Panels[0].Panels[0]
	removedSibling := rowDashboard("fixed", "ok")
	removedSibling.Panels[0].Panels = removedSibling.Panels[0].Panels[:1]
	cases := map[string]struct {
		result *spec.Dashboard
		want   bool
	}{
		"only the target's query":       {rowDashboard("fixed", "ok"), true},
		"nothing changed":               {rowDashboard("broken", "ok"), true},
		"a sibling in the row changed":  {rowDashboard("fixed", "changed"), false},
		"a top-level panel changed":     {topChanged, false},
		"the row's panels reordered":    {reordered, false},
		"a sibling in the row was lost": {removedSibling, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			obs := observation{dashboard: tc.result, parent: parent, target: "target"}
			if r := checkOnlyTargetChanged(obs); r.Passed != tc.want {
				t.Fatalf("passed %t, want %t (%s)", r.Passed, tc.want, r.Detail)
			}
		})
	}
	if r := checkOnlyTargetChanged(observation{dashboard: parent}); r.Passed {
		t.Error("an unknown parent must not pass")
	}
	if r := checkOnlyTargetChanged(observation{parent: parent, target: "target"}); r.Passed {
		t.Error("a run without a saved spec must not pass")
	}
}

func TestChangedPathsAllowOnlyTheNamedPathsAndTheirChildren(t *testing.T) {
	parent := rowDashboard("a", "b")
	retimed := rowDashboard("a", "b")
	retimed.Time.From = "now-6h"
	retitled := rowDashboard("a", "b")
	retitled.Title = "Renamed"
	retimedAndEdited := rowDashboard("a", "changed")
	retimedAndEdited.Time.From = "now-6h"
	timeRef := Expect{ChangedPaths: []string{"/time"}}
	cases := map[string]struct {
		ex     Expect
		result *spec.Dashboard
		want   bool
	}{
		"only the time":                {timeRef, retimed, true},
		"nothing changed":              {timeRef, rowDashboard("a", "b"), true},
		"the title beside the time":    {timeRef, retitled, false},
		"a panel beside the time":      {timeRef, retimedAndEdited, false},
		"a path and its parent":        {Expect{ChangedPaths: []string{"/time/from"}}, retimed, true},
		"a sibling path is not a root": {Expect{ChangedPaths: []string{"/tim"}}, retimed, false},
		"two roots":                    {Expect{ChangedPaths: []string{"/time", "/title"}}, retitled, true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			obs := observation{dashboard: tc.result, parent: parent}
			if r := checkChangedPaths(tc.ex, obs); r.Passed != tc.want {
				t.Fatalf("passed %t, want %t (%s)", r.Passed, tc.want, r.Detail)
			}
		})
	}
	if r := checkChangedPaths(timeRef, observation{dashboard: parent}); r.Passed {
		t.Error("an unknown parent must not pass")
	}
	if r := checkChangedPaths(timeRef, observation{parent: parent}); r.Passed {
		t.Error("a run without a saved spec must not pass")
	}
}

func valueData(values ...any) PanelData {
	return PanelData{Frames: []Frame{{Fields: []Field{{Name: "time", Type: "time", Values: []any{1.0, 2.0}}, {Name: "value", Type: "number", Values: values}}}}}
}

func TestTargetDataAndUnverifiedCleared(t *testing.T) {
	dash := rowDashboard("fixed", "ok")
	withData := observation{dashboard: dash, target: "target", panels: map[string]panelOutcome{"target": {data: valueData(1.0, nil)}}}
	if r := checkTargetData(withData); !r.Passed {
		t.Errorf("target with a value: %s", r.Detail)
	}
	for name, outcome := range map[string]panelOutcome{
		"all null":    {data: valueData(nil, nil)},
		"query error": {err: errors.New("boom")},
	} {
		obs := observation{dashboard: dash, target: "target", panels: map[string]panelOutcome{"target": outcome}}
		if r := checkTargetData(obs); r.Passed {
			t.Errorf("%s should fail", name)
		}
	}
	if r := checkTargetData(observation{dashboard: dash, target: "target"}); r.Passed {
		t.Error("an unfetched target should fail")
	}
	if r := checkTargetData(observation{dashboard: dash, target: "gone"}); r.Passed {
		t.Error("a removed target should fail")
	}

	still := observation{dashboard: dash, target: "target", unverified: Unverified{Panels: []UnverifiedPanel{{PanelID: "target"}}}}
	other := observation{dashboard: dash, target: "target", unverified: Unverified{Panels: []UnverifiedPanel{{PanelID: "other"}}}}
	if checkUnverifiedCleared(still).Passed || !checkUnverifiedCleared(other).Passed {
		t.Error("only the target's own entry in unverified counts")
	}
}

func TestKeepsPanelsAndNewPanelData(t *testing.T) {
	parent := rowDashboard("a", "b")
	added := rowDashboard("a", "b")
	added.Panels = append(added.Panels, spec.Panel{ID: "new", Type: spec.Stat, Datasource: "prom", Queries: []spec.Query{{Query: "up"}}})
	dropped := rowDashboard("a", "b")
	dropped.Panels = dropped.Panels[:1]
	dropped.Panels[0].Panels = dropped.Panels[0].Panels[:1]

	if r := checkKeepsPanels(observation{dashboard: added, parent: parent}); !r.Passed {
		t.Errorf("an added panel keeps the others: %s", r.Detail)
	}
	r := checkKeepsPanels(observation{dashboard: dropped, parent: parent})
	if r.Passed || !strings.Contains(r.Detail, "other") || !strings.Contains(r.Detail, "top") {
		t.Errorf("removed panels should be named: passed %t, %q", r.Passed, r.Detail)
	}

	withValue := observation{dashboard: added, parent: parent, panels: map[string]panelOutcome{"new": {data: valueData(2.0)}}}
	if r := checkNewPanelData(withValue); !r.Passed {
		t.Errorf("new panel with data: %s", r.Detail)
	}
	noValue := observation{dashboard: added, parent: parent, panels: map[string]panelOutcome{"new": {data: valueData(nil)}}}
	if checkNewPanelData(noValue).Passed {
		t.Error("a new panel without data should fail")
	}
	if checkNewPanelData(observation{dashboard: parent, parent: parent}).Passed {
		t.Error("no new panel should fail")
	}
}

func podSeries(values map[string]any) panelOutcome {
	var out panelOutcome
	for pod, v := range values {
		out.data.Frames = append(out.data.Frames, valueData(0.0, v).Frames...)
		out.labels = append(out.labels, map[string]string{"pod": pod})
	}
	return out
}

func TestPanelValuesBand(t *testing.T) {
	dash := &spec.Dashboard{Panels: []spec.Panel{{ID: "ratio", Type: spec.Stat, Datasource: "prom", Queries: []spec.Query{{Query: `sum(rate(http_requests_total{pod="checkout-1",status=~"5.."}[2m]))`}}}}}
	band := PanelValue{QueryContains: "status=~", Series: map[string]string{"pod": "checkout-1"}, Min: 0.2, Max: 0.4}
	lastNonNull := panelOutcome{
		data:   PanelData{Frames: []Frame{{Fields: []Field{{Type: "time", Values: []any{1.0, 2.0, 3.0}}, {Type: "number", Values: []any{0.9, 0.3, nil}}}}}},
		labels: []map[string]string{{"pod": "checkout-1"}},
	}
	cases := map[string]struct {
		pv      PanelValue
		outcome panelOutcome
		want    bool
	}{
		"inside the band":              {band, podSeries(map[string]any{"checkout-1": 0.31}), true},
		"on the lower edge":            {band, podSeries(map[string]any{"checkout-1": 0.2}), true},
		"below the band":               {band, podSeries(map[string]any{"checkout-1": 0.05}), false},
		"above the band":               {band, podSeries(map[string]any{"checkout-1": 0.9}), false},
		"other pods are ignored":       {band, podSeries(map[string]any{"checkout-1": 0.3, "checkout-0": 0.9}), true},
		"series missing":               {band, podSeries(map[string]any{"checkout-0": 0.3}), false},
		"last non-null value is used":  {band, lastNonNull, true},
		"no value at all":              {band, podSeries(map[string]any{"checkout-1": nil}), false},
		"no series filter takes all":   {PanelValue{QueryContains: "status=~", Min: 0.2, Max: 0.4}, podSeries(map[string]any{"checkout-1": 0.3, "checkout-0": 0.9}), false},
		"query text not in any panel":  {PanelValue{QueryContains: "latency", Min: 0, Max: 1}, podSeries(map[string]any{"checkout-1": 0.3}), false},
		"labels were not fetched":      {band, panelOutcome{data: valueData(0.3)}, false},
		"the panel query errored":      {band, panelOutcome{err: errors.New("boom")}, false},
		"a value that is not a number": {band, podSeries(map[string]any{"checkout-1": "0.3"}), false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			obs := observation{dashboard: dash, datasourceTypes: testDatasourceTypes, panels: map[string]panelOutcome{"ratio": tc.outcome}}
			r := checkPanelValues(Expect{PanelValues: []PanelValue{tc.pv}}, obs)
			if r.Passed != tc.want {
				t.Fatalf("passed %t, want %t (%s)", r.Passed, tc.want, r.Detail)
			}
		})
	}
	if r := checkPanelValues(Expect{}, observation{}); !r.Passed {
		t.Error("without panel values the check has nothing to fail")
	}
}

func TestProposalNoVersionAddedAndImportedChecks(t *testing.T) {
	good := observation{target: "p3", suggestion: &Suggestion{Panel: json.RawMessage(`{"id":"p3","type":"stat"}`)}}
	if !checkProposal(good).Passed {
		t.Error("a proposal for the target should pass")
	}
	for name, obs := range map[string]observation{
		"other panel":   {target: "p3", suggestion: &Suggestion{Panel: json.RawMessage(`{"id":"p4"}`)}},
		"not an object": {target: "p3", suggestion: &Suggestion{Panel: json.RawMessage(`[1]`)}},
		"no suggestion": {target: "p3"},
	} {
		if checkProposal(obs).Passed {
			t.Errorf("%s should fail", name)
		}
	}
	if !checkNoVersionAdded(observation{}).Passed || checkNoVersionAdded(observation{versionAdded: true}).Passed || checkNoVersionAdded(observation{specErr: "x"}).Passed {
		t.Error("only a run that saved nothing and could verify it passes")
	}

	dashboardID, version := "d1", 1
	saved := okRequest(func(r *AgentRequest) { r.DashboardID, r.ResultVersion = &dashboardID, &version })
	if !checkImported(observation{request: saved, importRecord: &ImportRecord{ID: "i"}}).Passed {
		t.Error("a saved version with an import record should pass")
	}
	for name, obs := range map[string]observation{
		"no record":      {request: saved},
		"record failed":  {request: saved, importErr: "boom"},
		"no version":     {request: okRequest(), importRecord: &ImportRecord{}},
		"request failed": {request: okRequest(failedWith("refused"))},
	} {
		if checkImported(obs).Passed {
			t.Errorf("%s should fail", name)
		}
	}
}

func TestBudgetsReportUnknownInsteadOfPassing(t *testing.T) {
	ex := Expect{MaxModelCalls: 4, MaxOutputTokens: 3000, MaxSaveOutputTokens: 2000}
	cases := map[string]struct {
		usage      *Usage
		want       bool
		wantDetail string
	}{
		"within every budget":      {&Usage{ModelCalls: 3, OutputTokens: 2000, MaxSaveOutputTokens: 1500}, true, ""},
		"exactly on the limits":    {&Usage{ModelCalls: 4, OutputTokens: 3000, MaxSaveOutputTokens: 2000}, true, ""},
		"too many calls":           {&Usage{ModelCalls: 5, OutputTokens: 2000, MaxSaveOutputTokens: 1500}, false, "model calls 5, limit 4"},
		"too many output tokens":   {&Usage{ModelCalls: 3, OutputTokens: 3001, MaxSaveOutputTokens: 1500}, false, "output tokens 3001"},
		"save too large":           {&Usage{ModelCalls: 3, OutputTokens: 2000, MaxSaveOutputTokens: 9000}, false, "largest save output tokens 9000"},
		"provider reported tokens": {&Usage{ModelCalls: 3}, false, "unknown, the provider reported none: output tokens, largest save output tokens"},
		"no usage at all":          {nil, false, "unknown"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := checkBudgets(ex, observation{request: okRequest(func(r *AgentRequest) { r.Usage = tc.usage })})
			if r.Passed != tc.want || !strings.Contains(r.Detail, tc.wantDetail) {
				t.Fatalf("passed %t, detail %q", r.Passed, r.Detail)
			}
		})
	}
	only := checkBudgets(Expect{MaxModelCalls: 2}, observation{request: okRequest(withUsage(Usage{ModelCalls: 2}))})
	if !only.Passed {
		t.Errorf("a budget that is not set must not be asked for: %s", only.Detail)
	}
}

func TestRunChecksPicksTheSetForTheKind(t *testing.T) {
	names := func(sc Scenario, obs observation) string {
		var out []string
		for _, c := range runChecks(sc, obs) {
			out = append(out, c.Name)
		}
		return strings.Join(out, ",")
	}
	obs := observation{request: okRequest(withUsage(Usage{ModelCalls: 1, OutputTokens: 1, MaxSaveOutputTokens: 1}))}
	create := Expect{Datasources: []string{"prometheus"}, MinPanels: 1, MaxPanels: 12}
	cases := map[string]struct {
		sc   Scenario
		want string
	}{
		"fix":                   {Scenario{Kind: kindFixUnverified}, "failure_code,only_target_changed,target_data,unverified_cleared,latency"},
		"revise":                {Scenario{Kind: kindRevise}, "failure_code,keeps_panels,new_panel_data,panel_values,latency"},
		"revise a retime":       {Scenario{Kind: kindRevise, Expect: Expect{ChangedPaths: []string{"/time"}, TimeRangeFrom: []string{"now-6h"}}}, "failure_code,keeps_panels,panel_values,changed_paths,time_range,latency"},
		"revise a time range":   {Scenario{Kind: kindRevise, Expect: Expect{TimeRangeFrom: []string{"now-6h"}}}, "failure_code,keeps_panels,new_panel_data,panel_values,time_range,latency"},
		"suggest":               {Scenario{Kind: kindSuggest}, "failure_code,proposal,no_version_added,latency"},
		"chat import":           {Scenario{Kind: kindChatImport}, "failure_code,imported,latency"},
		"create":                {Scenario{Kind: kindCreate, Expect: create}, "validity,data,source,panels,latency"},
		"default kind":          {Scenario{Expect: create}, "validity,data,source,panels,latency"},
		"with budgets":          {Scenario{Kind: kindSuggest, Expect: Expect{MaxModelCalls: 3}}, "failure_code,proposal,no_version_added,latency,budgets"},
		"expects a code":        {Scenario{Kind: kindRevise, Expect: Expect{FailureCode: "output_limit", MaxModelCalls: 3}}, "failure_code"},
		"create that must fail": {Scenario{Kind: kindCreate, Expect: Expect{FailureCode: "rate_limited"}}, "failure_code"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := names(tc.sc, obs); got != tc.want {
				t.Fatalf("checks %s, want %s", got, tc.want)
			}
		})
	}
}

func TestPanelsToFetchLimitsRevisionLookups(t *testing.T) {
	parent := rowDashboard("a", "b")
	result := rowDashboard("a", "b")
	result.Panels = append(result.Panels, spec.Panel{ID: "new", Type: spec.Stat, Datasource: "prom", Queries: []spec.Query{{Query: "sum(rate(x[2m]))"}}})
	obs := observation{dashboard: result, parent: parent, target: "other", datasourceTypes: testDatasourceTypes}
	ids := func(sc Scenario) []string {
		var out []string
		for _, p := range dataPanels(result) {
			if panelsToFetch(sc, obs)(p) {
				out = append(out, p.ID)
			}
		}
		return out
	}
	if got := ids(Scenario{Kind: kindFixUnverified}); !slices.Equal(got, []string{"other"}) {
		t.Errorf("fix fetches %v, want only the target", got)
	}
	if got := ids(Scenario{Kind: kindRevise}); !slices.Equal(got, []string{"new"}) {
		t.Errorf("revise fetches %v, want only the new panel", got)
	}
	withValue := Scenario{Kind: kindRevise, Expect: Expect{PanelValues: []PanelValue{{QueryContains: "up", Max: 1}}}}
	if got := ids(withValue); !slices.Equal(got, []string{"top", "new"}) {
		t.Errorf("revise with a panel value fetches %v, want the matching panel and the new one", got)
	}
	if got := ids(Scenario{}); len(got) != 4 {
		t.Errorf("create fetches %v, want every data panel", got)
	}
}

type flowServer struct {
	t       *testing.T
	imports *fakeImports

	mu        sync.Mutex
	result    json.RawMessage
	panelData map[string]string
	requested []string
	revision  map[string]any
	chat      string
	imported  bool

	variableListing string
	duringChat      func()
	dataRejected    bool
}

const labelledFrame = `{"frames":[{"labels":{"pod":"checkout-1"},"fields":[{"name":"time","type":"time","values":[1,2]},{"name":"value","type":"number","values":[0.1,0.3]}]}]}`

func (f *flowServer) lastImportSpec() json.RawMessage {
	f.imports.mu.Lock()
	source := f.imports.posted[len(f.imports.posted)-1]
	f.imports.mu.Unlock()
	dash, err := convertSource(source, importOptions(testDatasourceTypes, nil))
	if err != nil {
		f.t.Error(err)
	}
	raw, err := json.Marshal(dash)
	if err != nil {
		f.t.Error(err)
	}
	return raw
}

func (f *flowServer) handler() http.Handler {
	mux := http.NewServeMux()
	imports := f.imports.handler(f.t)
	mux.Handle("GET /api/v1/datasources", imports)
	mux.Handle("POST /api/v1/imports", imports)
	now := func() time.Time { return time.Now().UTC() }
	finished := func(id string, extra map[string]any) map[string]any {
		body := map[string]any{
			"id": id, "status": "succeeded", "created_at": now().Add(-3 * time.Second), "finished_at": now(),
			"usage": map[string]any{"model_calls": 2, "output_tokens": 700, "max_save_output_tokens": 600},
		}
		for k, v := range extra {
			body[k] = v
		}
		return body
	}
	reply := func(w http.ResponseWriter, status int, body any) {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	}
	mux.HandleFunc("POST /api/v1/dashboards/{id}/revisions", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.revision = map[string]any{}
		_ = json.NewDecoder(r.Body).Decode(&f.revision)
		reply(w, http.StatusAccepted, map[string]any{"revision": map[string]any{"id": "rev-1", "status": "queued", "created_at": now()}})
	})
	mux.HandleFunc("POST /api/v1/observe", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Prompt string `json:"prompt"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.chat = body.Prompt
		duringChat := f.duringChat
		f.mu.Unlock()
		if duringChat != nil {
			duringChat()
		}
		if utf8.RuneCountInString(body.Prompt) > maxPromptRunes {
			reply(w, http.StatusBadRequest, map[string]any{"error": "prompt must be 1 to 4000 characters", "code": "invalid_request"})
			return
		}
		reply(w, http.StatusAccepted, map[string]any{"request": map[string]any{"id": "chat-1", "status": "queued", "created_at": now()}})
	})
	mux.HandleFunc("GET /api/v1/observe/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		extra := map[string]any{"dashboard_id": "dash-1", "result_version": 2}
		if id == "chat-1" {
			extra = map[string]any{"dashboard_id": "dash-9", "result_version": 1}
		}
		reply(w, http.StatusOK, map[string]any{"request": finished(id, extra)})
	})
	mux.HandleFunc("GET /api/v1/dashboards/{id}/versions/{v}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		result := f.result
		f.mu.Unlock()
		switch r.PathValue("v") {
		case "1":
			reply(w, http.StatusOK, map[string]any{"version": map[string]any{"version": 1, "spec": f.lastImportSpec(), "unverified": map[string]any{"panels": []any{}, "variables": []any{}}}})
		case "2":
			if result == nil {
				reply(w, http.StatusNotFound, map[string]any{"error": "not found", "code": "not_found"})
				return
			}
			reply(w, http.StatusOK, map[string]any{"version": map[string]any{"version": 2, "spec": result, "unverified": map[string]any{"panels": []any{}, "variables": []any{}}}})
		default:
			reply(w, http.StatusNotFound, map[string]any{"error": "not found", "code": "not_found"})
		}
	})
	mux.HandleFunc("GET /api/v1/dashboards/{id}/versions/{v}/panels/{panel}/data", func(w http.ResponseWriter, r *http.Request) {
		panel := r.PathValue("panel")
		f.mu.Lock()
		f.requested = append(f.requested, panel)
		data, ok := f.panelData[panel]
		rejected := f.dataRejected
		f.mu.Unlock()
		if rejected {
			reply(w, http.StatusUnprocessableEntity, map[string]any{"error": "variable has no values", "code": "variable_empty"})
			return
		}
		if !ok {
			data = labelledFrame
		}
		_, _ = io.WriteString(w, data)
	})
	mux.HandleFunc("GET /api/v1/dashboards/{id}/versions/{v}/variables/{name}/values", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		listing := f.variableListing
		f.mu.Unlock()
		switch listing {
		case "empty":
			reply(w, http.StatusOK, map[string]any{"values": []string{}, "truncated": false, "cached": false})
		case "invalid":
			reply(w, http.StatusUnprocessableEntity, map[string]any{"error": "invalid query", "code": "query_invalid"})
		default:
			reply(w, http.StatusOK, map[string]any{"values": []string{r.PathValue("name")}, "truncated": false, "cached": false})
		}
	})
	mux.HandleFunc("GET /api/v1/dashboards/{id}/import", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		f.imported = true
		f.mu.Unlock()
		reply(w, http.StatusOK, map[string]any{"id": "imp-9", "dashboard_id": "dash-9", "version": 1, "title": "t"})
	})
	mux.HandleFunc("POST /api/v1/dashboards/{id}/suggestions", func(w http.ResponseWriter, _ *http.Request) {
		reply(w, http.StatusAccepted, map[string]any{"suggestion": map[string]any{"id": "sug-1", "status": "queued"}})
	})
	mux.HandleFunc("GET /api/v1/dashboards/{id}/suggestions/{sid}", func(w http.ResponseWriter, _ *http.Request) {
		reply(w, http.StatusOK, map[string]any{"suggestion": map[string]any{
			"id": "sug-1", "status": "succeeded", "panel": map[string]any{"id": "p3", "type": "gauge"},
			"usage": map[string]any{"model_calls": 1, "output_tokens": 100, "max_save_output_tokens": 100},
		}})
	})
	return mux
}

func startFlow(t *testing.T, imports *fakeImports) (*flowServer, *client) {
	t.Helper()
	useCorpus(t)
	f := &flowServer{t: t, imports: imports, panelData: map[string]string{}}
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)
	c, err := newClient(srv.URL, secretToken)
	if err != nil {
		t.Fatal(err)
	}
	c.poll, c.backoff = 5*time.Millisecond, time.Millisecond
	return f, c
}

func pristineShopSpec(t *testing.T, edit func(*spec.Dashboard)) json.RawMessage {
	t.Helper()
	source, err := readCorpus(demoShop)
	if err != nil {
		t.Fatal(err)
	}
	dash, err := convertSource(source, importOptions(testDatasourceTypes, nil))
	if err != nil {
		t.Fatal(err)
	}
	if edit != nil {
		edit(dash)
	}
	raw, err := json.Marshal(dash)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func fixScenario() Scenario {
	return Scenario{
		ID: "fix-shop", Kind: kindFixUnverified, Prompt: `Fix the unverified panel "{panel_title}" ({panel_id}).`,
		Seed: &Seed{Corpus: demoShop, BreakPanel: true},
		Expect: Expect{
			OnlyTargetChanged: true, TargetReturnsData: true, UnverifiedCleared: true,
			MaxSaveOutputTokens: 2000, MinPanels: 1, MaxSeconds: 60,
		},
	}
}

func TestFixFlowSeedsTargetsThePanelAndChecksOnlyItChanged(t *testing.T) {
	useBreaker(t, renameCPUMetric)
	f, c := startFlow(t, &fakeImports{})
	f.result = pristineShopSpec(t, nil)
	res, abandoned := runScenario(t.Context(), c, Pricing{}, fixScenario(), options{timeout: 5 * time.Second, suite: suiteReplay})
	if !res.Passed || abandoned != nil || res.Skipped {
		t.Fatalf("result %+v", res)
	}
	if res.Kind != kindFixUnverified || res.Suite != suiteReplay || res.DashboardID != "dash-1" || res.RequestID != "rev-1" {
		t.Errorf("result identity %+v", res)
	}
	if f.revision["panel_id"] != "p3" || f.revision["base_version"] != float64(1) || f.revision["prompt"] != `Fix the unverified panel "CPU busy" (p3).` {
		t.Errorf("revision body %v", f.revision)
	}
	if res.Prompt != f.revision["prompt"] {
		t.Errorf("the report should carry the filled prompt, got %q", res.Prompt)
	}
	if !slices.Equal(f.requested, []string{"p3"}) {
		t.Errorf("panel data requested for %v, want only the target", f.requested)
	}
	if res.Usage == nil || res.Usage.MaxSaveOutputTokens != 600 {
		t.Errorf("usage %+v", res.Usage)
	}
}

func TestFlowsWithDataChecksSkipWhenAnUnlistableVariableBlocksPanelData(t *testing.T) {
	useBreaker(t, renameCPUMetric)
	revise := Scenario{ID: "revise-shop", Kind: kindRevise, Prompt: "Add a panel.", Seed: &Seed{Corpus: demoShop}, Expect: Expect{KeepsPanels: true, MinPanels: 1, MaxSeconds: 60}}
	overCap := fixScenario()
	overCap.Expect.FailureCode = "output_limit"
	cases := map[string]struct {
		sc         Scenario
		variables  []string
		listing    string
		rejected   bool
		wantReason string
		wantPassed bool
	}{
		"fix, no values":                   {sc: fixScenario(), variables: []string{"cluster"}, listing: "empty", rejected: true, wantReason: "no_data: variable cluster has no values"},
		"fix, listing fails":               {sc: fixScenario(), variables: []string{"cluster"}, listing: "invalid", rejected: true, wantReason: "no_data: variable cluster: http 422 query_invalid"},
		"revise, no values":                {sc: revise, variables: []string{"cluster"}, listing: "empty", rejected: true, wantReason: "no_data: variable cluster has no values"},
		"data rejected, variables list":    {sc: fixScenario(), variables: []string{"cluster"}, rejected: true},
		"data rejected, no unverified":     {sc: fixScenario(), listing: "empty", rejected: true},
		"variable unlistable, data served": {sc: fixScenario(), variables: []string{"cluster"}, listing: "empty", wantPassed: true},
		"fix expecting a failure":          {sc: overCap, variables: []string{"cluster"}, listing: "empty", rejected: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f, c := startFlow(t, &fakeImports{variables: tc.variables})
			f.result = pristineShopSpec(t, func(d *spec.Dashboard) {
				if tc.sc.Kind != kindRevise {
					return
				}
				d.Panels = append(d.Panels, spec.Panel{ID: "added", Type: spec.Stat, Title: "Added", Datasource: "prom", GridPos: spec.GridPos{Y: 500, W: 6, H: 4}, Queries: []spec.Query{{Query: "up"}}})
			})
			f.variableListing, f.dataRejected = tc.listing, tc.rejected
			res, abandoned := runScenario(t.Context(), c, Pricing{}, tc.sc, options{timeout: 5 * time.Second, suite: suiteReplay})
			if abandoned != nil || res.Skipped != (tc.wantReason != "") {
				t.Fatalf("skipped %t, want reason %q: %+v", res.Skipped, tc.wantReason, res)
			}
			if tc.wantReason == "" {
				if tc.wantPassed != res.Passed {
					t.Fatalf("passed %t, want %t: %q", res.Passed, tc.wantPassed, failedChecks(res))
				}
				return
			}
			if !strings.HasPrefix(res.Reason, tc.wantReason) || res.Status != "skipped" || res.Passed || len(res.Checks) != 0 {
				t.Fatalf("result %+v", res)
			}
		})
	}
}

func TestFixFlowFailsWhenAnotherPanelChanged(t *testing.T) {
	useBreaker(t, renameCPUMetric)
	f, c := startFlow(t, &fakeImports{})
	f.result = pristineShopSpec(t, func(d *spec.Dashboard) { d.Panels[0].Panels[0].Title = "renamed" })
	res, _ := runScenario(t.Context(), c, Pricing{}, fixScenario(), options{timeout: 5 * time.Second, suite: suiteReplay})
	if res.Passed || !strings.Contains(failedChecks(res), "only_target_changed") {
		t.Fatalf("passed %t, failed checks %q", res.Passed, failedChecks(res))
	}
}

func TestLiveFixSkipsWhenThePristineTargetHasNoData(t *testing.T) {
	useBreaker(t, renameCPUMetric)
	imports := &fakeImports{unverified: []string{"p3"}}
	f, c := startFlow(t, imports)
	f.result = pristineShopSpec(t, nil)
	res, abandoned := runScenario(t.Context(), c, Pricing{}, fixScenario(), options{timeout: 5 * time.Second, suite: suiteLive})
	if !res.Skipped || res.Reason != "no_data" || res.Status != "skipped" || res.Passed || abandoned != nil || len(res.Checks) != 0 {
		t.Fatalf("result %+v", res)
	}
	if len(imports.posted) != 1 || bytes.Contains(imports.posted[0], []byte("_broken_by_eval")) || f.revision != nil {
		t.Fatalf("%d imports, revision %v: the pristine copy should be the only import and no revision requested", len(imports.posted), f.revision)
	}
	if agg := aggregate([]Result{res}); agg.Scenarios.Total != 0 || agg.Skipped != 1 {
		t.Errorf("a skipped result must stay out of the tallies: %+v", agg)
	}
}

func TestLiveFixRunsWhenThePristineTargetVerifies(t *testing.T) {
	useBreaker(t, renameCPUMetric)
	imports := &fakeImports{}
	f, c := startFlow(t, imports)
	f.result = pristineShopSpec(t, nil)
	res, _ := runScenario(t.Context(), c, Pricing{}, fixScenario(), options{timeout: 5 * time.Second, suite: suiteLive})
	if res.Skipped || !res.Passed {
		t.Fatalf("result %+v", res)
	}
	if len(imports.posted) != 2 || bytes.Contains(imports.posted[0], []byte("_broken_by_eval")) || !bytes.Contains(imports.posted[1], []byte("_broken_by_eval")) {
		t.Fatalf("want the pristine import then the broken one, got %d imports", len(imports.posted))
	}
}

func TestReviseFlowChecksTheNewPanelsValueUnderAFault(t *testing.T) {
	imports := &fakeImports{}
	f, c := startFlow(t, imports)
	f.result = pristineShopSpec(t, func(d *spec.Dashboard) {
		d.Panels = append(d.Panels, spec.Panel{
			ID: "ratio", Type: spec.Stat, Title: "Checkout-1 error ratio", Datasource: "prom",
			GridPos: spec.GridPos{X: 0, Y: 500, W: 6, H: 4},
			Queries: []spec.Query{{Query: `sum(rate(http_requests_total{pod="checkout-1",status=~"5.."}[2m])) / sum(rate(http_requests_total{pod="checkout-1"}[2m]))`}},
		})
	})
	faults, calls := startShop(t, false)
	sc := Scenario{
		ID: "revise-shop", Kind: kindRevise, Prompt: "Add the error ratio of checkout-1.", Seed: &Seed{Corpus: demoShop},
		Fault: &errorFault,
		Expect: Expect{
			KeepsPanels: true, MinPanels: 1, MaxSeconds: 60,
			PanelValues: []PanelValue{{QueryContains: `pod="checkout-1",status=~`, Series: map[string]string{"pod": "checkout-1"}, Min: 0.2, Max: 0.4}},
		},
	}
	res, _ := runScenario(t.Context(), c, Pricing{}, sc, options{timeout: 5 * time.Second, suite: suiteLive, faults: faults})
	if !res.Passed {
		t.Fatalf("passed %t, failed checks %q, error %q", res.Passed, failedChecks(res), res.Error)
	}
	if f.revision["panel_id"] != nil || f.revision["base_version"] != float64(1) {
		t.Errorf("a revise task has no target panel: %v", f.revision)
	}
	if got := calls.snapshot(); len(got) != 2 || !strings.HasPrefix(got[0], "POST /fault?") || !strings.HasPrefix(got[1], "DELETE /fault") {
		t.Errorf("shop calls %v, want the fault set then cleared", got)
	}
	if got := slices.Compact(slices.Sorted(slices.Values(f.requested))); !slices.Equal(got, []string{"ratio"}) {
		t.Errorf("panel data requested for %v, want only the new panel", got)
	}

	f.panelData["ratio"] = strings.Replace(labelledFrame, "0.3]", "0.9]", 1)
	res, _ = runScenario(t.Context(), c, Pricing{}, sc, options{timeout: 5 * time.Second, suite: suiteLive, faults: faults})
	if res.Passed || !strings.Contains(failedChecks(res), "panel_values") {
		t.Fatalf("a value outside the band should fail panel_values: %q", failedChecks(res))
	}
}

func TestSuggestFlowChecksTheProposalAndThatNothingWasSaved(t *testing.T) {
	useBreaker(t, renameCPUMetric)
	f, c := startFlow(t, &fakeImports{})
	sc := Scenario{ID: "suggest-shop", Kind: kindSuggest, Prompt: `Improve "{panel_title}".`, Seed: &Seed{Corpus: demoShop, BreakPanel: true}, Expect: Expect{MinPanels: 1, MaxSeconds: 60, MaxModelCalls: 3}}
	res, abandoned := runScenario(t.Context(), c, Pricing{}, sc, options{timeout: 5 * time.Second, suite: suiteReplay})
	if abandoned != nil || !res.Passed || res.Status != "succeeded" || res.RequestID != "sug-1" {
		t.Fatalf("result %+v, failed checks %q", res, failedChecks(res))
	}
	if res.Usage == nil || res.Usage.ModelCalls != 1 || res.LatencySeconds == nil {
		t.Errorf("the suggestion's usage and latency should reach the report: %+v", res)
	}

	f.result = pristineShopSpec(t, nil)
	res, _ = runScenario(t.Context(), c, Pricing{}, sc, options{timeout: 5 * time.Second, suite: suiteReplay})
	if res.Passed || !strings.Contains(failedChecks(res), "no_version_added") {
		t.Fatalf("a suggestion that saved a version must fail: %q", failedChecks(res))
	}
}

func TestChatImportRecordsPromptTooLongOrTheImport(t *testing.T) {
	f, c := startFlow(t, &fakeImports{})
	big := Scenario{ID: "chat-import-big", Kind: kindChatImport, Prompt: "Import this Grafana dashboard:", Seed: &Seed{Corpus: demoShop}, Expect: Expect{FailureCode: "prompt_too_long", MinPanels: 1, MaxSeconds: 60}}
	res, abandoned := runScenario(t.Context(), c, Pricing{}, big, options{timeout: 5 * time.Second})
	if abandoned != nil || res.Status != "not_started" || res.FailureCode != "prompt_too_long" || res.Uncoded || !res.Passed {
		t.Fatalf("result %+v", res)
	}
	if res.Prompt != big.Prompt || utf8.RuneCountInString(f.chat) <= maxPromptRunes {
		t.Errorf("the report keeps the short prompt while the server got %d characters", utf8.RuneCountInString(f.chat))
	}

	big.Expect.FailureCode = ""
	if res, _ = runScenario(t.Context(), c, Pricing{}, big, options{timeout: 5 * time.Second}); res.Passed || res.FailureCode != "prompt_too_long" {
		t.Fatalf("an unexpected rejection must fail but still carry its code: %+v", res)
	}

	small := Scenario{ID: "chat-import-small", Kind: kindChatImport, Prompt: `Import this Grafana dashboard: {"title":"t","panels":[]}`, Expect: Expect{MinPanels: 1, MaxSeconds: 60}}
	res, _ = runScenario(t.Context(), c, Pricing{}, small, options{timeout: 5 * time.Second})
	if !res.Passed || res.DashboardID != "dash-9" || !f.imported {
		t.Fatalf("result %+v", res)
	}
}

func TestChatImportKeepsSeedingImportsOutOfTheAgentsImportSlot(t *testing.T) {
	f, c := startFlow(t, &fakeImports{})
	source, err := readCorpus(demoShop)
	if err != nil {
		t.Fatal(err)
	}
	seedDone := make(chan struct{})
	var seededDuringChat atomic.Bool
	f.duringChat = func() {
		go func() {
			defer close(seedDone)
			_, _ = c.createImport(t.Context(), source, nil)
		}()
		select {
		case <-seedDone:
			seededDuringChat.Store(true)
		case <-time.After(100 * time.Millisecond):
		}
	}
	small := Scenario{ID: "chat-import-small", Kind: kindChatImport, Prompt: `Import this Grafana dashboard: {"title":"t","panels":[]}`, Expect: Expect{MinPanels: 1, MaxSeconds: 60}}
	res, _ := runScenario(t.Context(), c, Pricing{}, small, options{timeout: 5 * time.Second})
	if !res.Passed {
		t.Fatalf("result %+v", res)
	}
	if seededDuringChat.Load() {
		t.Fatal("a seeding import ran while the chat import held the key's only import slot")
	}
	select {
	case <-seedDone:
	case <-time.After(5 * time.Second):
		t.Fatal("the seeding import never ran after the chat import finished")
	}
}

func TestSeedFailureIsReportedWithoutAnAgentRequest(t *testing.T) {
	useCorpus(t)
	useBreaker(t, func([]byte) ([]byte, error) { return nil, errors.New("nothing to break") })
	_, c := startFlow(t, &fakeImports{})
	res, _ := runScenario(t.Context(), c, Pricing{}, fixScenario(), options{timeout: time.Second, suite: suiteReplay})
	if res.Passed || res.Status != "not_started" || !strings.Contains(res.Error, "seed the dashboard") || res.Uncoded {
		t.Fatalf("result %+v", res)
	}
}

func TestSelectionWithoutASuiteIsTodaysSet(t *testing.T) {
	pool := []Scenario{
		{ID: "create-live", Kind: kindCreate, Suites: []string{suiteLive}},
		{ID: "create-smoke", Kind: kindCreate, Suites: []string{suiteLive, suiteReplay}, Tags: []string{tagReplaySmoke}},
		{ID: "fault-replay", Kind: kindCreate, Suites: []string{suiteReplay}},
		{ID: "fix-live", Kind: kindFixUnverified, Suites: []string{suiteLive}},
		{ID: "fix-both", Kind: kindFixUnverified, Suites: []string{suiteLive, suiteReplay}},
	}
	cases := map[string]struct {
		o    options
		want []string
	}{
		"no suite":    {options{}, []string{"create-live", "create-smoke"}},
		"replay":      {options{suite: suiteReplay}, []string{"create-smoke", "fault-replay", "fix-both"}},
		"live":        {options{suite: suiteLive}, []string{"create-live", "create-smoke", "fix-live", "fix-both"}},
		"smoke tag":   {options{tags: listFlag{tagReplaySmoke}}, []string{"create-smoke"}},
		"id in suite": {options{suite: suiteLive, ids: listFlag{"fix-live"}}, []string{"fix-live"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := selectScenarios(pool, tc.o)
			if err != nil {
				t.Fatal(err)
			}
			ids := make([]string, len(got))
			for i, sc := range got {
				ids[i] = sc.ID
			}
			if !slices.Equal(ids, tc.want) {
				t.Fatalf("selected %v, want %v", ids, tc.want)
			}
		})
	}
	_, err := selectScenarios(pool, options{ids: listFlag{"fault-replay"}})
	if err == nil || !strings.Contains(err.Error(), "--suite replay") {
		t.Fatalf("an id outside the selection should say which suite has it: %v", err)
	}
	if _, err := selectScenarios(pool, options{ids: listFlag{"missing"}}); err == nil || !strings.Contains(err.Error(), "unknown scenario id") {
		t.Fatalf("an unknown id: %v", err)
	}
}

func TestShippedScenariosKeepTheDefaultSelection(t *testing.T) {
	scenarios, err := loadScenarios(filepath.Join("..", "scenarios"))
	if err != nil {
		t.Fatal(err)
	}
	selected, err := selectScenarios(scenarios, options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 14 {
		t.Fatalf("the default selection has %d scenarios, want the 14 create scenarios of the live suite", len(selected))
	}
	smoke, err := selectScenarios(scenarios, options{tags: listFlag{tagReplaySmoke}})
	if err != nil || len(smoke) != 2 {
		t.Fatalf("replay-smoke selects %d scenarios, %v, want 2", len(smoke), err)
	}
}

func TestParseFlagsSuiteCorpusAndMatrix(t *testing.T) {
	cases := map[string]struct {
		args       []string
		wantCorpus string
		wantErr    string
	}{
		"no suite":                    {nil, corpusNone, ""},
		"replay defaults to all":      {[]string{"--suite", "replay"}, corpusAll, ""},
		"live defaults to the sample": {[]string{"--suite", "live"}, corpusSample, ""},
		"explicit corpus":             {[]string{"--suite", "replay", "--corpus", "none"}, corpusNone, ""},
		"unknown suite":               {[]string{"--suite", "nightly"}, "", "--suite must be replay or live"},
		"unknown corpus":              {[]string{"--suite", "live", "--corpus", "some"}, "", "--corpus must be all, sample or none"},
		"corpus without suite":        {[]string{"--corpus", "all"}, "", "--corpus needs --suite"},
		"matrix without out":          {[]string{"--matrix", "m.yaml"}, "", "--matrix needs --out"},
		"matrix with out":             {[]string{"--matrix", "m.yaml", "--out", "dir"}, corpusNone, ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			o, err := parseFlags(tc.args, io.Discard)
			switch {
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Fatalf("error %v, want %q", err, tc.wantErr)
			case tc.wantErr == "" && (err != nil || o.corpus != tc.wantCorpus):
				t.Fatalf("corpus %q, error %v, want %q", o.corpus, err, tc.wantCorpus)
			}
		})
	}
	o, err := parseFlags([]string{"--shop-url", "checkout-1=http://127.0.0.1:1", "--bootstrap-dex"}, io.Discard)
	if err != nil || !o.bootstrapDex || o.shops["checkout-1"] != "http://127.0.0.1:1" {
		t.Fatalf("options %+v, %v", o, err)
	}
}

func useCorpusTools(t *testing.T, tools corpusToolkit) {
	t.Helper()
	previous := corpusTools
	corpusTools = tools
	t.Cleanup(func() { corpusTools = previous })
}

func TestCorpusTasksJoinTheScenarioFilesAndNeverShareAnID(t *testing.T) {
	var gotSample map[string]bool
	var gotSuite string
	task := Scenario{ID: "corpus-fix-a", Kind: kindFixUnverified, Suites: []string{suiteReplay, suiteLive}}
	useCorpusTools(t, corpusToolkit{
		tasks: func(_ string, sample map[string]bool, suite string) ([]Scenario, error) {
			gotSample, gotSuite = sample, suite
			return []Scenario{task}, nil
		},
		sample: func(string) (map[string]bool, error) { return map[string]bool{"a.json": true}, nil },
	})
	files := []Scenario{{ID: "from-file"}}

	pool, err := withCorpusTasks(files, options{suite: suiteLive, corpus: corpusSample})
	if err != nil || len(pool) != 2 || !gotSample["a.json"] || gotSuite != suiteLive {
		t.Fatalf("pool %v, sample %v, suite %q, err %v", pool, gotSample, gotSuite, err)
	}
	if _, err := withCorpusTasks(files, options{suite: suiteReplay, corpus: corpusAll}); err != nil || gotSample != nil {
		t.Fatalf("corpus all takes no sample: %v, %v", gotSample, err)
	}
	if pool, err := withCorpusTasks(files, options{corpus: corpusNone}); err != nil || len(pool) != 1 {
		t.Fatalf("corpus none adds nothing: %v, %v", pool, err)
	}
	if _, err := withCorpusTasks([]Scenario{{ID: "corpus-fix-a"}}, options{suite: suiteReplay, corpus: corpusAll}); err == nil || !strings.Contains(err.Error(), "also a scenario file id") {
		t.Fatalf("a duplicate id must be refused: %v", err)
	}
	useCorpusTools(t, corpusToolkit{tasks: func(string, map[string]bool, string) ([]Scenario, error) { return nil, errors.New("no corpus") }})
	if _, err := withCorpusTasks(files, options{suite: suiteReplay, corpus: corpusAll}); err == nil || !strings.Contains(err.Error(), "no corpus") {
		t.Fatalf("a task error must surface: %v", err)
	}
}

func TestPrepareWritesFixturesAndSeriesUnderTheOutputDirectory(t *testing.T) {
	out := filepath.Join(t.TempDir(), "runtime")
	var capSeen int
	var dirs []string
	useCorpusTools(t, corpusToolkit{
		replay: func(dir string, tokenCap int) error { capSeen = tokenCap; dirs = append(dirs, dir); return nil },
		synth:  func(dir string) error { dirs = append(dirs, dir); return nil },
	})
	var stderr strings.Builder
	if rc := run([]string{"prepare", "--out", out}, io.Discard, &stderr); rc != 0 {
		t.Fatalf("rc %d, stderr %q", rc, stderr.String())
	}
	want := []string{filepath.Join(out, "llm"), filepath.Join(out, "synth")}
	if !slices.Equal(dirs, want) || capSeen != 16000 {
		t.Fatalf("generators ran for %v with cap %d, want %v and 16000", dirs, capSeen, want)
	}
	for _, dir := range want {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Errorf("%s was not created: %v", dir, err)
		}
	}
	if rc := run([]string{"prepare"}, io.Discard, &stderr); rc != 2 {
		t.Errorf("prepare without --out should exit 2, got %d", rc)
	}
	useCorpusTools(t, corpusToolkit{replay: func(string, int) error { return errors.New("disk full") }})
	stderr.Reset()
	if rc := run([]string{"prepare", "--out", out}, io.Discard, &stderr); rc != 2 || !strings.Contains(stderr.String(), "disk full") {
		t.Errorf("a generator error should exit 2 and say why: %d %q", rc, stderr.String())
	}
}

func TestReportTalliesKindsAndLeavesSkippedOut(t *testing.T) {
	results := []Result{
		{ID: "a", Kind: kindCreate, Category: "prometheus", Passed: true},
		{ID: "b", Kind: kindFixUnverified, Passed: true},
		{ID: "c", Kind: kindFixUnverified, Status: "failed", FailureCode: "output_limit"},
		{ID: "d", Kind: kindFixUnverified, Skipped: true, Reason: "no_data", Status: "skipped"},
		{ID: "e", Category: "clickhouse"},
	}
	agg := aggregate(results)
	want := map[string]Tally{kindCreate: {Passed: 1, Total: 2, Rate: 0.5}, kindFixUnverified: {Passed: 1, Total: 2, Rate: 0.5}}
	if !reflect.DeepEqual(map[string]Tally(agg.Kinds), want) || agg.Scenarios.Total != 4 || agg.Skipped != 1 {
		t.Fatalf("kinds %v, scenarios %+v, skipped %d", agg.Kinds, agg.Scenarios, agg.Skipped)
	}
	if _, ok := agg.Categories[""]; ok || len(agg.Categories) != 2 {
		t.Errorf("a kind without a category must not add an empty category row: %v", agg.Categories)
	}
	markdown := renderMarkdown(Report{Aggregates: agg, Results: results}, evaluateGates(agg, 0, 0, 0))
	for _, line := range []string{"| Kind | Passed | Rate |", "| fix_unverified | 1 of 2 | 50% |", "| Skipped, left out of the rates | 1 |", "| d | n/a | n/a | n/a | n/a | SKIP |", "no_data |"} {
		if !strings.Contains(markdown, line) {
			t.Errorf("markdown lacks %q:\n%s", line, markdown)
		}
	}
	payload, err := json.Marshal(agg)
	if err != nil || !strings.Contains(string(payload), `"kinds":{`) || !strings.Contains(string(payload), `"scenarios":{"passed":2,"total":4,"rate":0.5}`) {
		t.Fatalf("aggregates JSON %s, %v", payload, err)
	}
}

func TestReportKeepsTheKeysTheE2EScriptReads(t *testing.T) {
	results := []Result{{ID: "red-metrics-by-endpoint", Category: "prometheus", Passed: true}, {ID: "log-volume-by-service", Category: "clickhouse", Passed: true}}
	report := Report{Provider: "replay", Model: "replay", Aggregates: aggregate(results), Results: results}
	path := filepath.Join(t.TempDir(), "report.json")
	if err := writeJSON(path, report); err != nil {
		t.Fatal(err)
	}
	flat := strings.NewReplacer(" ", "", "\n", "").Replace(string(readTestFile(t, path)))
	for _, want := range []string{`"scenarios":{"passed":2,"total":2,"rate":1}`, `"model":"replay"`} {
		if !strings.Contains(flat, want) {
			t.Errorf("report lacks %s", want)
		}
	}
	if strings.Contains(flat, `"passed":false`) {
		t.Error("a passing report must hold no failed entry")
	}
}

func TestQueryChecksRequireTheRebuildShare(t *testing.T) {
	metricPanels := func(metrics ...string) observation {
		panels := make([]spec.Panel, len(metrics))
		for i, m := range metrics {
			panels[i] = spec.Panel{ID: m, Type: spec.Timeseries, Datasource: "prom", Queries: []spec.Query{{Query: "sum(rate(" + m + "[5m]))"}}}
		}
		return observe(dashboard("now-1h", panels...))
	}
	listed := []string{"a_total", "b_total", "c_total", "d_total"}
	odd := []string{"a_total", "b_total", "c_total"}
	cases := []struct {
		name   string
		listed []string
		share  float64
		have   []string
		want   bool
	}{
		{"share unset needs every metric", listed, 0, []string{"a_total", "b_total", "c_total"}, false},
		{"share unset passes with every metric", listed, 0, listed, true},
		{"half of four is exactly met", listed, 0.5, []string{"a_total", "b_total"}, true},
		{"one below half of four", listed, 0.5, []string{"a_total"}, false},
		{"half of three rounds up to two", odd, 0.5, []string{"a_total", "c_total"}, true},
		{"one of three is below half", odd, 0.5, []string{"b_total"}, false},
		{"share 1 needs all", listed, 1, []string{"a_total", "b_total", "c_total"}, false},
		{"unrelated metrics count for nothing", listed, 0.5, []string{"x_total", "y_total"}, false},
	}
	for _, tc := range cases {
		r := checkQueries("metrics", tc.listed, spec.Prometheus, tc.share, 0, metricPanels(tc.have...))
		if r.Passed != tc.want {
			t.Errorf("%s: passed %t, want %t (%s)", tc.name, r.Passed, tc.want, r.Detail)
		}
	}

	r := checkQueries("metrics", listed, spec.Prometheus, 0.5, 0, metricPanels("a_total"))
	for _, part := range []string{"1 of 4 found, need 2", "b_total, c_total, d_total"} {
		if !strings.Contains(r.Detail, part) {
			t.Errorf("detail %q does not name %q", r.Detail, part)
		}
	}

	sql := observe(dashboard("now-1h", spec.Panel{ID: "t", Type: spec.Table, Datasource: "ch", Queries: []spec.Query{{Query: "SELECT 1 FROM otel_logs"}}}))
	if r := checkQueries("tables", []string{"otel_logs", "otel_traces"}, spec.ClickHouse, 0.5, 0, sql); !r.Passed {
		t.Errorf("one of two tables meets a share of 0.5: %s", r.Detail)
	}
	if r := checkQueries("tables", []string{"otel_logs", "otel_traces"}, spec.ClickHouse, 0, 0, sql); r.Passed {
		t.Error("one of two tables must fail without a share")
	}

	ex := Expect{Metrics: listed, RebuildMetricShare: 0.5}
	if got := createChecks(ex, metricPanels("a_total", "b_total")); !slices.ContainsFunc(got, func(c CheckResult) bool { return c.Name == "metrics" && c.Passed }) {
		t.Errorf("createChecks ignored the share: %+v", got)
	}
}

func TestRequiredCountIsAShareOfTheNeedlesUpToTheCap(t *testing.T) {
	cases := []struct {
		name  string
		total int
		share float64
		limit int
		want  int
	}{
		{"a quarter of 229 is capped at 20", 229, 0.25, 20, 20},
		{"a quarter of 144 is capped at 20", 144, 0.25, 20, 20},
		{"a quarter of 80 is exactly the cap", 80, 0.25, 20, 20},
		{"a quarter of 50 rounds up to 13", 50, 0.25, 20, 13},
		{"a quarter of 3 rounds up to 1", 3, 0.25, 20, 1},
		{"no cap keeps the share", 229, 0.25, 0, 58},
		{"the cap cannot exceed the total", 3, 1, 20, 3},
		{"no share needs every needle", 229, 0, 20, 229},
		{"an empty list needs nothing", 0, 0.25, 20, 0},
	}
	for _, tc := range cases {
		if got := requiredCount(tc.total, tc.share, tc.limit); got != tc.want {
			t.Errorf("%s: requiredCount(%d, %v, %d) = %d, want %d", tc.name, tc.total, tc.share, tc.limit, got, tc.want)
		}
	}
}

func TestRebuildCoverageUsesTheCap(t *testing.T) {
	needles := make([]string, 144)
	panels := make([]spec.Panel, 0, 20)
	for i := range needles {
		needles[i] = fmt.Sprintf("m%03d_total", i)
		if i < 20 {
			panels = append(panels, spec.Panel{ID: needles[i], Type: spec.Timeseries, Datasource: "prom", Queries: []spec.Query{{Query: "sum(rate(" + needles[i] + "[5m]))"}}})
		}
	}
	obs := observe(dashboard("now-1h", panels...))
	if r := checkQueries("metrics", needles, spec.Prometheus, 0.25, 20, obs); !r.Passed {
		t.Fatalf("20 of 144 meets a quarter capped at 20: %+v", r)
	}
	if r := checkQueries("metrics", needles, spec.Prometheus, 0.25, 0, obs); r.Passed {
		t.Fatalf("20 of 144 is below an uncapped quarter: %+v", r)
	}
	if r := checkQueries("metrics", needles, spec.Prometheus, 0.25, 21, obs); r.Passed || !strings.Contains(r.Detail, "20 of 144 found, need 21") {
		t.Fatalf("a cap of 21 needs 21: %+v", r)
	}
	ex := Expect{Metrics: needles, RebuildMetricShare: 0.25, RebuildMetricCap: 20}
	if got := createChecks(ex, obs); !slices.ContainsFunc(got, func(c CheckResult) bool { return c.Name == "metrics" && c.Passed }) {
		t.Errorf("createChecks ignored the cap: %+v", got)
	}
}

func TestSizeGuardScenariosMustExpectThePromptCap(t *testing.T) {
	base := Scenario{ID: "size-guard-demo", Kind: kindSizeGuard, Prompt: strings.Repeat("x", maxPromptRunes+1), Expect: Expect{FailureCode: failurePromptTooLong}}
	sc := base
	applyDefaults(&sc)
	if err := validateScenario(sc); err != nil {
		t.Fatalf("a size guard scenario with a long prompt: %v", err)
	}
	other := base
	other.Expect.FailureCode = "rate_limited"
	applyDefaults(&other)
	if err := validateScenario(other); err == nil || !strings.Contains(err.Error(), "must expect failure_code prompt_too_long") {
		t.Fatalf("another failure code: %v", err)
	}
	none := base
	none.Expect.FailureCode = ""
	applyDefaults(&none)
	if err := validateScenario(none); err == nil {
		t.Fatal("a size guard scenario with no failure code was accepted")
	}
	negative := Scenario{ID: "negative-cap", Prompt: "p", Category: categoryPrometheus, Expect: Expect{Datasources: []string{"prometheus"}, RebuildMetricShare: 0.25, RebuildMetricCap: -1}}
	applyDefaults(&negative)
	if err := validateScenario(negative); err == nil || !strings.Contains(err.Error(), "rebuild_metric_cap") {
		t.Fatalf("a negative cap: %v", err)
	}
}

func TestSizeGuardResultsStayOutOfThePassRate(t *testing.T) {
	results := []Result{
		{ID: "a", Kind: kindCreate, Category: "prometheus", Status: "succeeded", Passed: true},
		{ID: "b", Kind: kindCreate, Category: "prometheus", Status: "failed", FailureCode: "call_timeout"},
		{ID: "g1", Kind: kindSizeGuard, Category: "prometheus", Status: "not_started", FailureCode: failurePromptTooLong, Passed: true},
		{ID: "g2", Kind: kindSizeGuard, Category: "prometheus", Status: "not_started", FailureCode: failurePromptTooLong, Passed: true},
		{ID: "g3", Kind: kindSizeGuard, Category: "prometheus", Status: "succeeded"},
		{ID: "g4", Kind: kindSizeGuard, Skipped: true},
	}
	agg := aggregate(results)
	if agg.Scenarios.Passed != 1 || agg.Scenarios.Total != 2 || agg.Scenarios.Rate != 0.5 {
		t.Errorf("scenarios = %+v, want 1 of 2", agg.Scenarios)
	}
	if agg.SizeGuard.Passed != 2 || agg.SizeGuard.Total != 3 {
		t.Errorf("size guard = %+v, want 2 of 3", agg.SizeGuard)
	}
	if _, ok := agg.Kinds[kindSizeGuard]; ok || agg.Kinds[kindCreate].Total != 2 {
		t.Errorf("kinds = %+v", agg.Kinds)
	}
	if agg.Categories["prometheus"].Total != 2 || agg.FailureCodes[failurePromptTooLong] != 0 || agg.Skipped != 1 {
		t.Errorf("categories %+v, failure codes %v, skipped %d", agg.Categories, agg.FailureCodes, agg.Skipped)
	}
	payload, err := json.Marshal(agg)
	if err != nil || !strings.Contains(string(payload), `"size_guard":{"passed":2,"total":3,`) {
		t.Fatalf("aggregates JSON = %s, %v", payload, err)
	}
	markdown := renderMarkdown(Report{Aggregates: agg, Results: results}, gateOutcome{})
	if !strings.Contains(markdown, "\nSize guard: 2 of 3 rejected as expected\n") || !strings.Contains(markdown, "| Scenarios passed | 1 of 2 (50%) |") {
		t.Errorf("markdown:\n%s", markdown)
	}
	if without := renderMarkdown(Report{Aggregates: aggregate(results[:2]), Results: results[:2]}, gateOutcome{}); strings.Contains(without, "Size guard") {
		t.Errorf("a run without size guard scenarios must not print the line:\n%s", without)
	}

	passingOnly := aggregate([]Result{results[1], results[2], results[3]})
	if passingOnly.Scenarios.Rate != 0 {
		t.Errorf("size guard passes raised the pass rate to %v", passingOnly.Scenarios.Rate)
	}
}

func TestMatrixShowsTheSizeGuardPerModel(t *testing.T) {
	withGuard := Report{Provider: "p1", Model: "m1", Aggregates: aggregate([]Result{
		{ID: "a", Kind: kindCreate, Passed: true},
		{ID: "g1", Kind: kindSizeGuard, Passed: true},
		{ID: "g2", Kind: kindSizeGuard},
	})}
	without := Report{Provider: "p2", Model: "m2", Aggregates: aggregate([]Result{{ID: "a", Kind: kindCreate, Passed: true}})}
	table := renderMatrix([]matrixResult{{name: "alpha", report: withGuard}, {name: "beta", report: without}})
	if !strings.Contains(table, "| size_guard, rejected as expected | 1 of 2 | n/a |\n") || !strings.Contains(table, "| Entry | Provider | Model | Scenarios passed | Skipped |\n") {
		t.Errorf("matrix:\n%s", table)
	}
	if strings.Contains(table, "| size_guard |") {
		t.Errorf("size_guard must not be a kind row:\n%s", table)
	}
	if plain := renderMatrix([]matrixResult{{name: "beta", report: without}}); strings.Contains(plain, "size_guard") {
		t.Errorf("a matrix without size guard runs must not show the row:\n%s", plain)
	}
}

func TestParseFlagsTimeoutOutlastsTheServerRunLimit(t *testing.T) {
	o, err := parseFlags(nil, io.Discard)
	if err != nil || o.timeout != serverRunLimit+serverSettleMargin || o.timeout <= serverRunLimit {
		t.Fatalf("default timeout %s, %v", o.timeout, err)
	}
	if o.timeout != 125*time.Minute {
		t.Errorf("default timeout %s, want 125m", o.timeout)
	}
	for _, arg := range []string{"0", "60s", "4m", "64m59s", "124m59s"} {
		if _, err := parseFlags([]string{"--timeout", arg}, io.Discard); err == nil || !strings.Contains(err.Error(), "--timeout must be at least 2h5m0s") {
			t.Errorf("--timeout %s: %v", arg, err)
		}
	}
	if _, err := parseFlags([]string{"--timeout", "125m"}, io.Discard); err != nil {
		t.Errorf("--timeout 125m: %v", err)
	}
}

func TestRunnerTimeoutIsItsOwnOutcomeAndNotAServerCode(t *testing.T) {
	_, c := newFake(t, 3, time.Hour)
	res, abandoned := runScenario(t.Context(), c, Pricing{}, sampleScenarios(1)[0], options{timeout: 100 * time.Millisecond})
	if res.RunnerCode != runnerTimeout || res.FailureCode != "" || res.Uncoded || res.Passed || abandoned == nil {
		t.Fatalf("runner code %q, failure code %q, uncoded %t, passed %t, abandoned %v", res.RunnerCode, res.FailureCode, res.Uncoded, res.Passed, abandoned)
	}
	if !strings.Contains(res.Error, "still running on the server") {
		t.Errorf("error %q does not say where the request stands", res.Error)
	}

	results := []Result{res, {ID: "b", Category: "prometheus", Status: "failed", FailureCode: "run_timeout"}}
	agg := aggregate(results)
	if agg.RunnerCodes[runnerTimeout] != 1 || agg.FailureCodes[runnerTimeout] != 0 || agg.FailureCodes["run_timeout"] != 1 || agg.Uncoded != 0 {
		t.Fatalf("runner codes %v, failure codes %v, uncoded %d", agg.RunnerCodes, agg.FailureCodes, agg.Uncoded)
	}
	markdown := renderMarkdown(Report{Aggregates: agg, Results: results}, evaluateGates(agg, 0, 0, 0))
	for _, want := range []string{"| runner_timeout | 1 |", "| run_timeout | 1 |", "Runner outcome, not a server failure code"} {
		if !strings.Contains(markdown, want) {
			t.Errorf("markdown lacks %q:\n%s", want, markdown)
		}
	}
}

func TestServerFailureWithinTheWaitCarriesItsCodeNotARunnerOutcome(t *testing.T) {
	f, c := newFake(t, 3, 50*time.Millisecond)
	f.code = "run_timeout"
	res, abandoned := runScenario(t.Context(), c, Pricing{}, sampleScenarios(1)[0], options{timeout: 5 * time.Second})
	if res.FailureCode != "run_timeout" || res.RunnerCode != "" || res.Uncoded || abandoned != nil {
		t.Fatalf("failure code %q, runner code %q, uncoded %t, abandoned %v", res.FailureCode, res.RunnerCode, res.Uncoded, abandoned)
	}
}

func TestInterruptedWaitIsNotARunnerTimeout(t *testing.T) {
	_, c := newFake(t, 3, time.Hour)
	ctx, cancel := context.WithCancel(t.Context())
	time.AfterFunc(100*time.Millisecond, cancel)
	res, _ := runScenario(ctx, c, Pricing{}, sampleScenarios(1)[0], options{timeout: 5 * time.Second})
	if res.RunnerCode != "" || !strings.Contains(res.Error, "interrupted") {
		t.Fatalf("runner code %q, error %q", res.RunnerCode, res.Error)
	}
}

func TestResultCarriesWallClockTimesAndModelCalls(t *testing.T) {
	_, c := newFake(t, 3, 20*time.Millisecond)
	before := time.Now().UTC()
	res, _ := runScenario(t.Context(), c, Pricing{}, sampleScenarios(1)[0], options{timeout: 5 * time.Second})
	if res.StartedAt.Before(before) || res.FinishedAt.Before(res.StartedAt) || res.StartedAt.Location() != time.UTC || res.FinishedAt.Location() != time.UTC {
		t.Fatalf("started %s, finished %s, test began %s", res.StartedAt, res.FinishedAt, before)
	}
	payload, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"started_at", "finished_at"} {
		text, _ := fields[key].(string)
		if parsed, err := time.Parse(time.RFC3339, text); err != nil || !strings.HasSuffix(text, "Z") || parsed.IsZero() {
			t.Errorf("%s = %q, %v", key, text, err)
		}
	}
	if value, present := fields["model_calls"]; !present || value != nil {
		t.Errorf("model_calls = %v (present %t), want null without usage", value, present)
	}

	obs := observation{request: okRequest(withUsage(Usage{ModelCalls: 4}))}
	if got := finish(Scenario{Expect: Expect{FailureCode: "x"}}, Pricing{}, Result{}, obs); got.ModelCalls == nil || *got.ModelCalls != 4 {
		t.Errorf("model calls = %v, want 4", got.ModelCalls)
	}
	obs = observation{request: okRequest(withUsage(Usage{InputTokens: 9}))}
	if got := finish(Scenario{Expect: Expect{FailureCode: "x"}}, Pricing{}, Result{}, obs); got.ModelCalls != nil {
		t.Errorf("model calls = %d, want none when the server reports no calls", *got.ModelCalls)
	}
}

func TestLatencyOnlyFailuresAreMarkedAndCounted(t *testing.T) {
	good := observe(dashboard("now-1h", spec.Panel{ID: "p", Type: spec.Timeseries, Datasource: "prom", Queries: []spec.Query{{Query: "up"}}}))
	good.panels = map[string]panelOutcome{"p": {data: PanelData{Frames: []Frame{{Fields: []Field{{Name: "v", Type: "number", Values: []any{1.0}}}}}}}}
	good.request = okRequest()
	slow := Scenario{Expect: Expect{Datasources: []string{"prometheus"}, MinPanels: 1, MaxPanels: 12, MaxSeconds: 3}}
	fast := slow
	fast.Expect.MaxSeconds = 60
	overBudget := fast
	overBudget.Expect.MaxModelCalls = 1
	withCalls := good
	withCalls.request = okRequest(withUsage(Usage{ModelCalls: 5}))
	wrong := slow
	wrong.Expect.Datasources = []string{"clickhouse"}

	cases := map[string]struct {
		sc         Scenario
		obs        observation
		wantPassed bool
		wantOnly   bool
	}{
		"passes":                     {fast, good, true, false},
		"slow and otherwise correct": {slow, good, false, true},
		"over the call budget only":  {overBudget, withCalls, false, true},
		"slow and wrong":             {wrong, good, false, false},
		"wrong and fast":             {func() Scenario { w := wrong; w.Expect.MaxSeconds = 60; return w }(), good, false, false},
	}
	var results []Result
	for name, tc := range cases {
		got := finish(tc.sc, Pricing{}, Result{ID: name, Category: "prometheus", Status: "succeeded"}, tc.obs)
		if got.Passed != tc.wantPassed || got.LatencyOnly != tc.wantOnly {
			t.Errorf("%s: passed %t, latency only %t, want %t and %t: %+v", name, got.Passed, got.LatencyOnly, tc.wantPassed, tc.wantOnly, got.Checks)
		}
		results = append(results, got)
	}
	agg := aggregate(results)
	if agg.LatencyOnly != 2 || agg.Scenarios.Passed != 1 || agg.Scenarios.Total != 5 {
		t.Fatalf("latency only %d, scenarios %+v", agg.LatencyOnly, agg.Scenarios)
	}
	markdown := renderMarkdown(Report{Aggregates: agg, Results: results}, evaluateGates(agg, 0, 0, 0))
	for _, want := range []string{"Failed on latency or budget only, included in the failures | 2 |", "| slow and otherwise correct | n/a | n/a | n/a | n/a | FAIL (latency only) |", "| slow and wrong | n/a | n/a | n/a | n/a | FAIL |"} {
		if !strings.Contains(markdown, want) {
			t.Errorf("markdown lacks %q:\n%s", want, markdown)
		}
	}
	if payload, err := json.Marshal(agg); err != nil || !strings.Contains(string(payload), `"latency_only_failures":2`) {
		t.Errorf("aggregates JSON = %s, %v", payload, err)
	}
}
