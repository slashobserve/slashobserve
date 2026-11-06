package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	t0ms       = 1762000000000
	lokiFrames = `{"frames":[
	{"ref":"A","labels":{"app":"shop","pod":"checkout-1"},"fields":[
		{"name":"Time","type":"time","values":[1762000002000,1762000000000]},
		{"name":"Body","type":"string","values":["payment declined","cart loaded"]},
		{"name":"level","type":"string","values":["error","info"]}]},
	{"ref":"A","labels":{"app":"shop","pod":"checkout-0"},"fields":[
		{"name":"Time","type":"time","values":[1762000001000]},
		{"name":"Body","type":"string","values":["order placed"]},
		{"name":"level","type":"string","values":["info"]}]}
	],"meta":{}}`
	clickhouseFrames = `{"frames":[{"ref":"A","fields":[
		{"name":"Timestamp","type":"time","values":[1762000001000,1762000000000]},
		{"name":"ServiceName","type":"string","values":["checkout","checkout"]},
		{"name":"SeverityText","type":"string","values":["WARN",""]},
		{"name":"Body","type":"string","values":["slow query","started"]}]}],"meta":{}}`
)

type logsFake struct {
	server   *httptest.Server
	mu       sync.Mutex
	requests []map[string]any
	answers  []string
	status   int
}

func newLogsFake(t *testing.T, answers ...string) *logsFake {
	t.Helper()
	f := &logsFake{answers: answers, status: http.StatusOK}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.URL.Path != "/api/v1/datasources/loki/query" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var body map[string]any
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &body)
		f.requests = append(f.requests, body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(f.status)
		answer := f.answers[min(len(f.requests), len(f.answers))-1]
		_, _ = io.WriteString(w, answer)
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *logsFake) run(args ...string) (string, string, int) {
	return runCLI(append([]string{"--server", f.server.URL, "--token", "k", "logs"}, args...)...)
}

func TestLogsSendsARangeQuery(t *testing.T) {
	f := newLogsFake(t, lokiFrames)
	if _, stderr, code := f.run(`{app="shop"}`, "--datasource", "loki", "--from", "now-15m", "--limit", "50"); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	got := f.requests[0]
	if got["query"] != `{app="shop"}` || got["kind"] != "range" || got["from"] != "now-15m" || got["to"] != "now" || got["limit"] != float64(50) {
		t.Fatalf("request %v", got)
	}
}

func TestLogsPrintsLokiLinesOldestFirstWithVaryingLabels(t *testing.T) {
	f := newLogsFake(t, lokiFrames)
	stdout, _, code := f.run(`{app="shop"}`, "--datasource", "loki")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	want := strings.Join([]string{
		`2025-11-01T12:26:40.000Z  info   {pod="checkout-1"}  cart loaded`,
		`2025-11-01T12:26:41.000Z  info   {pod="checkout-0"}  order placed`,
		`2025-11-01T12:26:42.000Z  error  {pod="checkout-1"}  payment declined`,
	}, "\n") + "\n"
	if stdout != want {
		t.Fatalf("stdout\n%s\nwant\n%s", stdout, want)
	}
}

func TestLogsReadsClickHouseColumns(t *testing.T) {
	f := newLogsFake(t, clickhouseFrames)
	stdout, _, code := f.run("SELECT 1", "--datasource", "loki")
	want := "2025-11-01T12:26:40.000Z  -     started\n2025-11-01T12:26:41.000Z  WARN  slow query\n"
	if code != 0 || stdout != want {
		t.Fatalf("exit %d, stdout %q", code, stdout)
	}
}

func TestLogsTrimsToTheNewestLines(t *testing.T) {
	f := newLogsFake(t, lokiFrames)
	stdout, _, _ := f.run("q", "--datasource", "loki", "--limit", "1")
	if strings.Count(stdout, "\n") != 1 || !strings.Contains(stdout, "payment declined") {
		t.Fatalf("stdout %q", stdout)
	}
}

func TestLogsWithNoLinesSaysSoAndExitsZero(t *testing.T) {
	f := newLogsFake(t, `{"frames":[],"meta":{}}`)
	stdout, stderr, code := f.run("q", "--datasource", "loki", "--from", "now-5m")
	if code != 0 || stdout != "" || strings.TrimSpace(stderr) != "No log lines between now-5m and now" {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestLogsJSONPrintsTheResponse(t *testing.T) {
	f := newLogsFake(t, lokiFrames)
	stdout, _, code := f.run("q", "--datasource", "loki", "--json")
	if code != 0 || strings.TrimSpace(stdout) != strings.TrimSpace(lokiFrames) {
		t.Fatalf("exit %d, stdout %q", code, stdout)
	}
}

func TestLogsInvalidQueryExitsOneWithTheServersMessage(t *testing.T) {
	f := newLogsFake(t, `{"error":"parse error at line 1, col 2: syntax error","code":"query_invalid"}`)
	f.status = http.StatusUnprocessableEntity
	_, stderr, code := f.run("{", "--datasource", "loki")
	if code != 1 || !strings.Contains(stderr, "parse error at line 1, col 2: syntax error (query_invalid)") {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
}

func TestLogsRefusesBadFlagsBeforeAnyRequest(t *testing.T) {
	for _, args := range [][]string{{"q"}, {"q", "--datasource", "loki", "--limit", "0"}, {"q", "--datasource", "loki", "--limit", "1001"}, {"--datasource", "loki"}} {
		f := newLogsFake(t, lokiFrames)
		_, stderr, code := f.run(args...)
		if code != 2 || stderr == "" || len(f.requests) != 0 {
			t.Errorf("%v: exit %d, stderr %q, requests %d", args, code, stderr, len(f.requests))
		}
	}
}

func TestVaryingLabelsKeepsOnlyKeysThatDiffer(t *testing.T) {
	lines := []logLine{
		{Labels: map[string]string{"app": "shop", "pod": "a", "zone": "x"}},
		{Labels: map[string]string{"app": "shop", "pod": "b"}},
	}
	if got := varyingLabels(lines); fmt.Sprint(got) != "[pod zone]" {
		t.Fatalf("varying %v", got)
	}
	if got := varyingLabels(lines[:1]); len(got) != 0 {
		t.Fatalf("one line varies %v", got)
	}
}

func tailFrame(lines ...[2]any) string {
	var times, bodies []string
	for _, line := range lines {
		times = append(times, fmt.Sprint(line[0]))
		bodies = append(bodies, fmt.Sprintf("%q", line[1]))
	}
	return `{"frames":[{"ref":"A","labels":{"app":"shop"},"fields":[{"name":"Time","type":"time","values":[` + strings.Join(times, ",") + `]},{"name":"Body","type":"string","values":[` + strings.Join(bodies, ",") + `]}]}],"meta":{}}`
}

func TestLogsTailPrintsOnlyNewLinesAndStopsCleanly(t *testing.T) {
	first := tailFrame([2]any{t0ms, "a"}, [2]any{t0ms + 1000, "b"})
	second := tailFrame([2]any{t0ms + 1000, "b"}, [2]any{t0ms + 2000, "c"})
	f := newLogsFake(t, second, second, second)
	base, err := url.Parse(f.server.URL)
	if err != nil {
		t.Fatal(err)
	}
	c, err := (&app{version: "test"}).clientFor(base, "k")
	if err != nil {
		t.Fatal(err)
	}
	body, err := decode[exploreResponse]([]byte(first))
	if err != nil {
		t.Fatal(err)
	}
	var stdout strings.Builder
	tail := &logTail{out: &output{out: &stdout, err: io.Discard}}
	tail.print(logLines(body.Frames))
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- tail.follow(ctx, c, "loki", exploreRequest{Query: "q", Kind: "range", From: "now-1h", To: "now", Limit: 100}, 10*time.Millisecond)
	}()
	deadline := time.After(5 * time.Second)
	for {
		f.mu.Lock()
		n := len(f.requests)
		f.mu.Unlock()
		if n >= 3 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("the tail never asked three times")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("follow: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 3 || !strings.HasSuffix(lines[0], "  a") || !strings.HasSuffix(lines[1], "  b") || !strings.HasSuffix(lines[2], `{app="shop"}  c`) {
		t.Fatalf("lines %q", lines)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.requests[0]["from"] != "2025-11-01T12:26:41Z" || f.requests[0]["to"] != "now" {
		t.Fatalf("first tail request %v", f.requests[0])
	}
}

func TestLogsTailJSONPrintsOneObjectPerLine(t *testing.T) {
	var stdout strings.Builder
	tail := &logTail{out: &output{out: &stdout, err: io.Discard}, json: true}
	tail.print([]logLine{{At: time.UnixMilli(t0ms).UTC(), Level: "info", Labels: map[string]string{"app": "shop"}, Body: "hello"}})
	if got := strings.TrimSpace(stdout.String()); got != `{"time":"2025-11-01T12:26:40.000Z","level":"info","labels":{"app":"shop"},"line":"hello"}` {
		t.Fatalf("stdout %q", got)
	}
}

const lokiDraft = `{"query":"{service_name=\"checkout\"} |= \"error\"","language":"logql","trace_filters":null,"explanation":"Checkout error lines.","warnings":["No matches in the last 15 minutes."]}`

func TestLogsAskDraftsTheQueryThenRunsIt(t *testing.T) {
	f := newAskFake(t, "loki", lokiDraft, lokiFrames)
	stdout, stderr, code := f.run("logs", "--ask", "checkout errors", "--from", "now-6h", "--limit", "50")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	draft := f.drafts[0]
	if len(f.drafts) != 1 || draft["question"] != "checkout errors" || draft["from"] != "now-6h" || draft["to"] != "now" || draft["signal"] != "logs" {
		t.Fatalf("drafts %v", f.drafts)
	}
	query := f.queries[0]
	if len(f.queries) != 1 || query["query"] != `{service_name="checkout"} |= "error"` || query["kind"] != "range" || query["from"] != "now-6h" || query["limit"] != float64(50) {
		t.Fatalf("queries %v", f.queries)
	}
	if stderr != "Query: {service_name=\"checkout\"} |= \"error\"\nCheckout error lines.\nWarning: No matches in the last 15 minutes.\n" {
		t.Fatalf("stderr %q", stderr)
	}
	if !strings.Contains(stdout, "payment declined") {
		t.Fatalf("stdout %q", stdout)
	}
}

func TestLogsAskDryRunNeverRunsTheQuery(t *testing.T) {
	f := newAskFake(t, "loki", lokiDraft, lokiFrames)
	stdout, stderr, code := f.run("logs", "--ask", "checkout errors", "--dry-run")
	if code != 0 || len(f.drafts) != 1 || len(f.queries) != 0 || stdout != "" || !strings.HasPrefix(stderr, "Query: ") {
		t.Fatalf("exit %d, drafts %d, queries %d, stdout %q, stderr %q", code, len(f.drafts), len(f.queries), stdout, stderr)
	}
}

func TestLogsAskUsageErrors(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{`{app="shop"}`, "--ask", "checkout errors"}, "give a log query or --ask, not both"},
		{nil, "logs needs a log query or --ask"},
		{[]string{`{app="shop"}`, "--dry-run"}, "--dry-run needs --ask"},
		{[]string{"--dry-run"}, "--dry-run needs --ask"},
	}
	for _, c := range cases {
		f := newAskFake(t, "loki", lokiDraft, lokiFrames)
		_, stderr, code := f.run("logs", c.args...)
		if code != 2 || !strings.Contains(stderr, c.want) || len(f.drafts)+len(f.queries) != 0 {
			t.Errorf("%v: exit %d, stderr %q", c.args, code, stderr)
		}
	}
}
