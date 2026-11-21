package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type seenCall struct{ method, path, query, body string }

func recordingServer(t *testing.T, reply string) (*httptest.Server, *seenCall) {
	t.Helper()
	seen := &seenCall{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*seen = seenCall{r.Method, r.URL.Path, r.URL.RawQuery, string(body)}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(server.Close)
	return server, seen
}

func TestDatasourcesListPrintsEveryDatasource(t *testing.T) {
	server, seen := recordingServer(t, `{"datasources":[{"uid":"prom","type":"prometheus","name":"Prom","healthy":true,"message":null,"dashboard_count":3},{"uid":"lk","type":"loki","name":"Loki","healthy":false,"message":"refused","dashboard_count":0}]}`)
	stdout, stderr, code := runWith(server, "datasources", "list")
	if code != 0 || seen.path != "/api/v1/datasources" || !strings.Contains(stdout, "prom") || !strings.Contains(stdout, "unhealthy: refused") {
		t.Fatalf("exit %d, call %+v, stdout %q, stderr %q", code, seen, stdout, stderr)
	}
}

func TestDatasourcesListShowsTheAudienceOnlyWhenTheServerSendsIt(t *testing.T) {
	plain, _ := recordingServer(t, `{"datasources":[{"uid":"prom","type":"prometheus","name":"Prom","source":"app","healthy":true,"dashboard_count":3}]}`)
	stdout, _, code := runWith(plain, "datasources", "list")
	if code != 0 || strings.Contains(stdout, "AUDIENCE") {
		t.Fatalf("exit %d, stdout %q", code, stdout)
	}
	admin, _ := recordingServer(t, `{"datasources":[{"uid":"prom","type":"prometheus","name":"Prom","source":"app","audience":"everyone","healthy":true,"dashboard_count":3},{"uid":"pay","type":"loki","name":"Pay","source":"file","audience":"admins","healthy":true,"dashboard_count":0}]}`)
	stdout, _, code = runWith(admin, "datasources", "list")
	stdout = strings.Join(strings.Fields(stdout), " ")
	if code != 0 || !strings.Contains(stdout, "SOURCE AUDIENCE DASHBOARDS") || !strings.Contains(stdout, "app everyone 3") || !strings.Contains(stdout, "file admins 0") {
		t.Fatalf("exit %d, stdout %q", code, stdout)
	}
}

func TestMetricsSendsTheFiltersAndSaysWhenMoreExist(t *testing.T) {
	server, seen := recordingServer(t, `{"datasource":"prom","metrics":[{"name":"http_requests_total","type":"counter","help":"Requests"}],"truncated":true}`)
	stdout, stderr, code := runWith(server, "datasources", "metrics", "prom", "--search", "http", "--limit", "20")
	if code != 0 || seen.path != "/api/v1/datasources/prom/metrics" || seen.query != "limit=20&search=http" {
		t.Fatalf("exit %d, call %+v, stderr %q", code, seen, stderr)
	}
	if !strings.Contains(stdout, "http_requests_total") || !strings.Contains(stderr, "More exist beyond the first 20") {
		t.Fatalf("stdout %q, stderr %q", stdout, stderr)
	}
}

func TestLabelsAndValuesPassTheirScope(t *testing.T) {
	server, seen := recordingServer(t, `{"datasource":"ch","labels":["service.name"],"values":["checkout"],"label":"service.name","truncated":false}`)
	stdout, _, code := runWith(server, "datasources", "labels", "ch", "--table", "otel_logs", "--from", "now-6h", "--to", "now")
	if code != 0 || seen.path != "/api/v1/datasources/ch/labels" || seen.query != "from=now-6h&limit=100&table=otel_logs&to=now" || strings.TrimSpace(stdout) != "service.name" {
		t.Fatalf("exit %d, call %+v, stdout %q", code, seen, stdout)
	}
	stdout, _, code = runWith(server, "datasources", "label-values", "ch", "service.name", "--metric", "up")
	if code != 0 || seen.path != "/api/v1/datasources/ch/labels/service.name/values" || seen.query != "limit=100&metric=up" || strings.TrimSpace(stdout) != "checkout" {
		t.Fatalf("exit %d, call %+v, stdout %q", code, seen, stdout)
	}
}

func TestNameListsRefuseABadLimitOrHalfAWindow(t *testing.T) {
	for _, args := range [][]string{
		{"datasources", "metrics", "prom", "--limit", "501"},
		{"datasources", "labels", "prom", "--limit", "0"},
		{"datasources", "labels", "prom", "--from", "now-1h"},
		{"datasources", "label-values", "prom"},
	} {
		_, _, stderr, code := runAgainst(t, http.StatusOK, `{}`, args...)
		if code != 2 || stderr == "" {
			t.Fatalf("%v: exit %d, stderr %q", args, code, stderr)
		}
	}
}

func TestLintExitsOneOnlyForErrors(t *testing.T) {
	server, seen := recordingServer(t, `{"diagnostics":[{"severity":"error","message":"unknown metric","start":0,"end":3}]}`)
	stdout, stderr, code := runWith(server, "datasources", "lint", "prom", "rate(foo[5m])")
	if code != 1 || seen.method != http.MethodPost || seen.path != "/api/v1/datasources/prom/lint" || !strings.Contains(seen.body, `"query":"rate(foo[5m])"`) {
		t.Fatalf("exit %d, call %+v", code, seen)
	}
	if !strings.Contains(stdout, "error 0-3  unknown metric") || !strings.Contains(stderr, "the query has 1 errors") {
		t.Fatalf("stdout %q, stderr %q", stdout, stderr)
	}

	warn, _ := recordingServer(t, `{"diagnostics":[{"severity":"warning","message":"slow","start":0,"end":1}]}`)
	if _, _, code := runWith(warn, "datasources", "lint", "prom", "up"); code != 0 {
		t.Fatalf("a warning exited %d", code)
	}
	clean, _ := recordingServer(t, `{"diagnostics":[]}`)
	stdout, _, code = runWith(clean, "datasources", "lint", "prom", "up")
	if code != 0 || !strings.Contains(stdout, "No diagnostics") {
		t.Fatalf("exit %d, stdout %q", code, stdout)
	}
}

func TestLogVolumeAndTraceStatsPostTheRange(t *testing.T) {
	server, seen := recordingServer(t, `{"step_ms":60000,"total":5,"buckets":[{"start":"2026-01-01T00:00:00Z","error":2,"warning":1,"info":2,"other":0}]}`)
	stdout, _, code := runWith(server, "datasources", "log-volume", "lk", `{app="shop"}`, "--from", "now-2h", "--buckets", "10")
	if code != 0 || seen.path != "/api/v1/datasources/lk/log-volume" || seen.body != `{"query":"{app=\"shop\"}","from":"now-2h","to":"now","buckets":10}` || !strings.Contains(stdout, "Total 5") {
		t.Fatalf("exit %d, call %+v, stdout %q", code, seen, stdout)
	}

	stats, seenStats := recordingServer(t, `{"step_ms":1000,"partial":true,"spans":9,"errors":1,"buckets":[{"start":"2026-01-01T00:00:00Z","spans":9,"errors":1,"p50_ms":12.5,"p95_ms":40,"p99_ms":null}],"histogram":[]}`)
	stdout, stderr, code := runWith(stats, "datasources", "trace-stats", "tempo", "{}")
	if code != 0 || seenStats.path != "/api/v1/datasources/tempo/trace-stats" || !strings.Contains(seenStats.body, `"query":"{}"`) || !strings.Contains(stdout, "12.5ms") || !strings.Contains(stdout, "Spans 9, errors 1") || !strings.Contains(stderr, "lower bound") {
		t.Fatalf("exit %d, call %+v, stdout %q, stderr %q", code, seenStats, stdout, stderr)
	}
	cut, _ := recordingServer(t, `{"step_ms":3600000,"total":4,"partial":true,"counted_from":"2026-01-06T00:00:00Z","buckets":[{"start":"2026-01-06T00:00:00Z","error":0,"warning":0,"info":4,"other":0}]}`)
	if _, stderr, code := runWith(cut, "datasources", "log-volume", "lk", `{app="shop"}`, "--from", "now-7d"); code != 0 || !strings.Contains(stderr, "from 2026-01-06T00:00:00Z on were counted") {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if _, _, stderr, code := runAgainst(t, http.StatusOK, `{}`, "datasources", "log-volume", "lk", "x", "--buckets", "201"); code != 2 || stderr == "" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
}

func TestTraceLogsPrintsTheLinesOfATrace(t *testing.T) {
	reply := `{"frames":[{"ref":"A","name":"logs","labels":{},"fields":[{"name":"Timestamp","type":"time","values":[1762000000000]},{"name":"Body","type":"string","values":["payment failed"]}]}]}`
	server, seen := recordingServer(t, reply)
	stdout, stderr, code := runWith(server, "datasources", "trace-logs", "lk", "4bf92f3577b34da6a3ce929d0e0e4736", "--span", "00f067aa0ba902b7", "--service", "checkout")
	if code != 0 || seen.path != "/api/v1/datasources/lk/trace-logs" {
		t.Fatalf("exit %d, call %+v, stderr %q", code, seen, stderr)
	}
	for _, want := range []string{`"trace_id":"4bf92f3577b34da6a3ce929d0e0e4736"`, `"span_id":"00f067aa0ba902b7"`, `"service":"checkout"`, `"limit":100`} {
		if !strings.Contains(seen.body, want) {
			t.Fatalf("body %s lacks %s", seen.body, want)
		}
	}
	if !strings.Contains(stdout, "payment failed") {
		t.Fatalf("stdout %q", stdout)
	}
}

func TestTraceStatsWithoutAFilterSendsAnEmptySpansetToTempo(t *testing.T) {
	cases := []struct {
		kind  string
		query string
	}{
		{"tempo", `"query":"{}"`},
		{"clickhouse", `"query":""`},
	}
	for _, tc := range cases {
		var stats string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Path == "/api/v1/datasources" {
				_, _ = w.Write([]byte(`{"datasources":[{"uid":"ds1","type":"` + tc.kind + `","name":"T","healthy":true,"message":null,"dashboard_count":0}]}`))
				return
			}
			body, _ := io.ReadAll(r.Body)
			stats = string(body)
			_, _ = w.Write([]byte(`{"step_ms":1000,"partial":false,"spans":0,"errors":0,"buckets":[],"histogram":[]}`))
		}))
		t.Cleanup(server.Close)
		if _, stderr, code := runWith(server, "datasources", "trace-stats", "ds1"); code != 0 || !strings.Contains(stats, tc.query) {
			t.Fatalf("%s: exit %d, body %q, stderr %q", tc.kind, code, stats, stderr)
		}
	}
}

const unhealthyLoki = `{"uid":"loki","type":"loki","name":"Loki","source":"file","healthy":false,"message":"The datasource is not answering","detail":"The datasource answered HTTP 503","checked_at":"2026-11-19T10:00:00Z","dashboard_count":2}`

type datasourcesFake struct {
	server   *httptest.Server
	mu       sync.Mutex
	requests []recordedRequest
	recheck  string
}

func newDatasourcesFake(t *testing.T) *datasourcesFake {
	t.Helper()
	f := &datasourcesFake{recheck: unhealthyLoki}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		data, _ := io.ReadAll(r.Body)
		f.requests = append(f.requests, recordedRequest{method: r.Method, path: r.URL.Path, query: r.URL.RawQuery, body: string(data)})
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/datasources":
			_, _ = io.WriteString(w, `{"datasources":[{"uid":"prom","type":"prometheus","name":"Prometheus","source":"app","healthy":true,"checked_at":"2026-11-19T10:00:00Z","dashboard_count":3},`+unhealthyLoki+`]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/datasources/loki/recheck":
			_, _ = io.WriteString(w, f.recheck)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":"datasource not found","code":"not_found"}`)
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *datasourcesFake) run(args ...string) (string, string, int) {
	var stdout, stderr strings.Builder
	code := Main(append([]string{"--server", f.server.URL, "--token", "k"}, args...), IO{In: strings.NewReader(""), Out: &stdout, Err: &stderr}, "test")
	return stdout.String(), stderr.String(), code
}

func TestDatasourcesListShowsSourceAndWhyUnhealthy(t *testing.T) {
	f := newDatasourcesFake(t)
	stdout, stderr, code := f.run("datasources", "list")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	for _, want := range []string{"prom", "app", "healthy", "unhealthy: The datasource is not answering (The datasource answered HTTP 503)", "file"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("output lacks %q:\n%s", want, stdout)
		}
	}
}

func TestDatasourcesRecheckPostsAndExitsOneWhileUnhealthy(t *testing.T) {
	f := newDatasourcesFake(t)
	stdout, _, code := f.run("datasources", "recheck", "loki")
	if code != 1 || !strings.Contains(stdout, "answered HTTP 503") {
		t.Fatalf("exit %d, output %q", code, stdout)
	}
	if got := f.requests[len(f.requests)-1]; got.method != http.MethodPost || got.path != "/api/v1/datasources/loki/recheck" {
		t.Errorf("request = %+v", got)
	}

	f.recheck = strings.NewReplacer(`"healthy":false`, `"healthy":true`).Replace(unhealthyLoki)
	stdout, _, code = f.run("datasources", "recheck", "loki")
	if code != 0 || !strings.Contains(stdout, "healthy") {
		t.Fatalf("exit %d, output %q", code, stdout)
	}
}

func TestDatasourcesRecheckOfAnUnknownUIDFails(t *testing.T) {
	f := newDatasourcesFake(t)
	_, stderr, code := f.run("datasources", "recheck", "nope")
	if code != 1 || !strings.Contains(stderr, "datasource not found") {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
}
