package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

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
