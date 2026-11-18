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

func TestMetricsQuerySendsARangeQuery(t *testing.T) {
	f := newMetricsFake(t, promFrames)
	if _, stderr, code := f.run("query", "up", "--datasource", "prom", "--from", "now-6h"); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	got := f.requests[0]
	if got["query"] != "up" || got["kind"] != "range" || got["from"] != "now-6h" || got["to"] != "now" {
		t.Fatalf("request %v", got)
	}
}

func TestMetricsQueryInstantSendsAnInstantQuery(t *testing.T) {
	f := newMetricsFake(t, promFrames)
	if _, _, code := f.run("query", "up", "--datasource", "prom", "--instant"); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if f.requests[0]["kind"] != "instant" {
		t.Fatalf("request %v", f.requests[0])
	}
}

func TestMetricsQueryPrintsOneLinePerSeries(t *testing.T) {
	f := newMetricsFake(t, promFrames)
	stdout, _, code := f.run("query", "up", "--datasource", "prom")
	want := strings.Join([]string{
		`SERIES                                                POINTS  LAST  MIN  MAX  MEAN`,
		`http_requests_total{code="200", service="cart"}      3       10    1    10   5`,
		`{service="cart"}                                      1       0.5   0.5  0.5  0.5`,
	}, "\n")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
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

func TestMetricsQueryInstantPrintsValues(t *testing.T) {
	f := newMetricsFake(t, promFrames)
	stdout, _, _ := f.run("query", "up", "--datasource", "prom", "--instant")
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
