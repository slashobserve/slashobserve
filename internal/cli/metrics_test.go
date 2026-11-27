package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const promFrames = `{"frames":[
	{"ref":"A","name":"http_requests_total","labels":{"service":"cart","code":"200"},"fields":[
		{"name":"Time","type":"time","values":[1762000000000,1762000015000,1762000030000]},
		{"name":"Value","type":"number","values":[1,4,10]}]},
	{"ref":"A","name":"","labels":{"service":"cart"},"fields":[
		{"name":"Time","type":"time","values":[1762000000000]},
		{"name":"Value","type":"number","values":[0.5]}]},
	{"ref":"A","name":"empty","fields":[
		{"name":"Time","type":"time","values":[]},
		{"name":"Value","type":"number","values":[]}]}
	],"meta":{}}`

type metricsFake struct {
	server   *httptest.Server
	mu       sync.Mutex
	requests []map[string]any
	answer   string
	status   int
}

func newMetricsFake(t *testing.T, answer string) *metricsFake {
	t.Helper()
	f := &metricsFake{answer: answer, status: http.StatusOK}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.URL.Path != "/api/v1/datasources/prom/query" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var body map[string]any
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &body)
		f.requests = append(f.requests, body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(f.status)
		_, _ = io.WriteString(w, f.answer)
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *metricsFake) run(args ...string) (string, string, int) {
	return runCLI(append([]string{"--server", f.server.URL, "--token", "k", "metrics"}, args...)...)
}

func TestMetricsQuerySendsARangeQueryAndPrintsOneLinePerSeries(t *testing.T) {
	f := newMetricsFake(t, promFrames)
	stdout, stderr, code := f.run("query", "up", "--datasource", "prom", "--from", "now-6h")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if got := f.requests[0]; got["query"] != "up" || got["kind"] != "range" || got["from"] != "now-6h" || got["to"] != "now" {
		t.Fatalf("request %v", got)
	}
	want := strings.Join([]string{
		`SERIES                                                POINTS  LAST  MIN  MAX  MEAN`,
		`http_requests_total{code="200", service="cart"}      3       10    1    10   5`,
		`{service="cart"}                                      1       0.5   0.5  0.5  0.5`,
	}, "\n")
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 3 {
		t.Fatalf("stdout %q", stdout)
	}
	for i, line := range strings.Split(want, "\n") {
		if strings.Join(strings.Fields(lines[i]), " ") != strings.Join(strings.Fields(line), " ") {
			t.Errorf("line %d: %q, want %q", i, lines[i], line)
		}
	}
}

func TestMetricsQueryInstantSendsAnInstantQueryAndPrintsValues(t *testing.T) {
	f := newMetricsFake(t, promFrames)
	stdout, _, code := f.run("query", "up", "--datasource", "prom", "--instant")
	if code != 0 || f.requests[0]["kind"] != "instant" {
		t.Fatalf("exit %d, request %v", code, f.requests[0])
	}
	if !strings.HasPrefix(stdout, "SERIES") || strings.Contains(stdout, "POINTS") || !strings.Contains(stdout, "VALUE") {
		t.Fatalf("stdout %q", stdout)
	}
}

func TestMetricsQueryWithNoSeriesSaysSoAndExitsZero(t *testing.T) {
	f := newMetricsFake(t, `{"frames":[],"meta":{}}`)
	stdout, stderr, code := f.run("query", "up", "--datasource", "prom", "--from", "now-5m")
	if code != 0 || stdout != "" || strings.TrimSpace(stderr) != "No series between now-5m and now" {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestMetricsQueryJSONPrintsTheResponse(t *testing.T) {
	f := newMetricsFake(t, promFrames)
	stdout, _, code := f.run("query", "up", "--datasource", "prom", "--json")
	if code != 0 || strings.TrimSpace(stdout) != strings.TrimSpace(promFrames) {
		t.Fatalf("exit %d, stdout %q", code, stdout)
	}
}

func TestMetricsQueryShowsTheServersLimitMessage(t *testing.T) {
	f := newMetricsFake(t, `{"error":"series limit exceeded: more than 200 series","code":"limit_exceeded"}`)
	f.status = http.StatusUnprocessableEntity
	_, stderr, code := f.run("query", "up", "--datasource", "prom")
	if code != 1 || !strings.Contains(stderr, "series limit exceeded: more than 200 series (limit_exceeded)") {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
}

func TestMetricsQueryRefusesAMissingDatasourceBeforeAnyRequest(t *testing.T) {
	f := newMetricsFake(t, promFrames)
	_, stderr, code := f.run("query", "up")
	if code != 2 || !strings.Contains(stderr, "needs --datasource") || len(f.requests) != 0 {
		t.Fatalf("exit %d, stderr %q, requests %d", code, stderr, len(f.requests))
	}
}

const promDraft = `{"query":"sum by (service) (rate(http_requests_total[5m]))","language":"promql","trace_filters":null,"explanation":"Requests per second by service.","warnings":[]}`

func runMetricsAsk(f *askFake, args ...string) (string, string, int) {
	return runCLI(append([]string{"--server", f.server.URL, "--token", "k", "metrics", "query", "--datasource", "ds"}, args...)...)
}

func TestMetricsAskDraftsTheQueryThenRunsIt(t *testing.T) {
	f := newAskFake(t, "prometheus", promDraft, promFrames)
	stdout, stderr, code := runMetricsAsk(f, "--ask", "requests per service", "--from", "now-6h", "--instant")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	draft := f.drafts[0]
	if len(f.drafts) != 1 || draft["question"] != "requests per service" || draft["from"] != "now-6h" || draft["signal"] != "metrics" {
		t.Fatalf("drafts %v", f.drafts)
	}
	query := f.queries[0]
	if len(f.queries) != 1 || query["query"] != "sum by (service) (rate(http_requests_total[5m]))" || query["kind"] != "instant" {
		t.Fatalf("queries %v", f.queries)
	}
	if stderr != "Query: sum by (service) (rate(http_requests_total[5m]))\nRequests per second by service.\n" || !strings.Contains(stdout, "http_requests_total") {
		t.Fatalf("stderr %q, stdout %q", stderr, stdout)
	}
}

func TestMetricsAskDryRunNeverRunsTheQuery(t *testing.T) {
	f := newAskFake(t, "prometheus", promDraft, promFrames)
	stdout, stderr, code := runMetricsAsk(f, "--ask", "requests per service", "--dry-run")
	if code != 0 || len(f.drafts) != 1 || len(f.queries) != 0 || stdout != "" || !strings.HasPrefix(stderr, "Query: ") {
		t.Fatalf("exit %d, drafts %d, queries %d, stdout %q, stderr %q", code, len(f.drafts), len(f.queries), stdout, stderr)
	}
}

func TestMetricsAskUsageErrors(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"up", "--ask", "targets"}, "give a PromQL query or --ask, not both"},
		{nil, "query needs a PromQL query or --ask"},
		{[]string{"up", "--dry-run"}, "--dry-run needs --ask"},
	}
	for _, c := range cases {
		f := newAskFake(t, "prometheus", promDraft, promFrames)
		_, stderr, code := runMetricsAsk(f, c.args...)
		if code != 2 || !strings.Contains(stderr, c.want) || len(f.drafts)+len(f.queries) != 0 {
			t.Errorf("%v: exit %d, stderr %q", c.args, code, stderr)
		}
	}
}
