package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

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
	if len(scenarios) != 30 || countDoneWhen(scenarios) != 10 {
		t.Fatalf("got %d scenarios with %d done_when, want 30 and 10", len(scenarios), countDoneWhen(scenarios))
	}
}

func TestScenarioValidation(t *testing.T) {
	cases := map[string]struct {
		extra string
		want  string
		mixed bool
	}{
		"valid": {
			extra: "expect:\n  datasources: [prometheus]\n  metrics: [up]\n  units: [reqps, \"µs\"]\n  threshold_at: {percent: 1}\n  time_range_from: [now-1h]\n  query_contains_any:\n    - [offset 1d, offset 24h]\n",
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
	if r := checkQueries("tables", []string{"otel_logs"}, spec.ClickHouse, obs); !r.Passed {
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
	res, _ := runScenario(t.Context(), c, Pricing{}, sampleScenarios(1)[0], 5*time.Second)
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
	res, abandoned := runScenario(t.Context(), c, Pricing{}, sampleScenarios(1)[0], 100*time.Millisecond)
	if !strings.Contains(res.Error, "no request slot within 100ms") || abandoned != nil {
		t.Fatalf("error %q, abandoned %v", res.Error, abandoned)
	}
}

func TestPollRetriesServerAndClientTimeoutErrors(t *testing.T) {
	f, c := newFake(t, 3, 20*time.Millisecond)
	f.getFails, f.getHangs = 2, 1
	c.http.Timeout = 100 * time.Millisecond
	res, abandoned := runScenario(t.Context(), c, Pricing{}, sampleScenarios(1)[0], 5*time.Second)
	if res.Error != "boom" || res.Status != "failed" || abandoned != nil {
		t.Fatalf("error %q, status %q, abandoned %v", res.Error, res.Status, abandoned)
	}
}

func TestPerCallTimeoutIsNotReportedAsScenarioTimeout(t *testing.T) {
	f, c := newFake(t, 3, 20*time.Millisecond)
	f.getHangs = 1
	c.http.Timeout = 100 * time.Millisecond
	res, _ := runScenario(t.Context(), c, Pricing{}, sampleScenarios(1)[0], 5*time.Second)
	if strings.Contains(res.Error, "timed out after") {
		t.Fatalf("error %q", res.Error)
	}
}

func TestUndecodablePollResponseFailsFast(t *testing.T) {
	f, c := newFake(t, 3, 20*time.Millisecond)
	f.getBad = true
	started := time.Now()
	res, abandoned := runScenario(t.Context(), c, Pricing{}, sampleScenarios(1)[0], 5*time.Second)
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
	res, _ := runScenario(t.Context(), c, Pricing{}, sampleScenarios(1)[0], 600*time.Millisecond)
	if !strings.Contains(res.Error, "timed out after 600ms") || !strings.Contains(res.Error, "waiting for a request slot)") {
		t.Fatalf("error %q", res.Error)
	}
}

func TestDrainIsAnnounced(t *testing.T) {
	_, c := newFake(t, 3, 300*time.Millisecond)
	var stderr strings.Builder
	runAll(t.Context(), c, Pricing{}, sampleScenarios(1), options{parallel: 1, timeout: 50 * time.Millisecond}, &stderr)
	if !strings.Contains(stderr.String(), "draining req-1 (up to 10s)") {
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
