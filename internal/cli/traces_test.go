package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const traceID = "4bf92f3577b34da6a3ce929d0e0e4736"

const traceFrames = `{"frames":[{"ref":"A","fields":[
	{"name":"TraceId","type":"string","values":["t","t","t","t","t"]},
	{"name":"SpanId","type":"string","values":["db","root","pay","cart","lost"]},
	{"name":"ParentSpanId","type":"string","values":["pay","","root","root","gone"]},
	{"name":"SpanName","type":"string","values":["SELECT orders","POST /checkout","charge","load cart","retry"]},
	{"name":"ServiceName","type":"string","values":["postgres","checkout","payments","cart","checkout"]},
	{"name":"StartTime","type":"time","values":[1762000000030,1762000000000,1762000000020,1762000000005,1762000000900]},
	{"name":"Duration","type":"number","values":[850000,1230000000,12400000,"3000000",400]},
	{"name":"StatusCode","type":"string","values":["Unset","Ok","STATUS_CODE_ERROR","Ok","Error"]}
]}],"meta":{}}`

type tracesFake struct {
	server  *httptest.Server
	kind    string
	answer  string
	queries []map[string]any
}

func newTracesFake(t *testing.T, kind string) *tracesFake {
	t.Helper()
	f := &tracesFake{kind: kind, answer: traceFrames}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/datasources":
			_, _ = fmt.Fprintf(w, `{"datasources":[{"uid":"traces","type":%q,"name":"T","healthy":true,"checked_at":"x","dashboard_count":0}]}`, f.kind)
		case "/api/v1/datasources/traces/query":
			var body map[string]any
			data, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(data, &body)
			f.queries = append(f.queries, body)
			_, _ = io.WriteString(w, f.answer)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":"not found","code":"not_found"}`)
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *tracesFake) run(args ...string) (string, string, int) {
	return runCLI(append([]string{"--server", f.server.URL, "--token", "k", "traces"}, args...)...)
}

func TestTracesTempoSendsTheID(t *testing.T) {
	f := newTracesFake(t, "tempo")
	if _, stderr, code := f.run(strings.ToUpper(traceID), "--datasource", "traces"); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	q := f.queries[0]
	if q["query"] != traceID || q["kind"] != "range" || q["from"] != "now-24h" || q["to"] != "now" || q["limit"] != float64(1000) {
		t.Fatalf("query %v", q)
	}
}

func TestTracesClickHouseSendsSQLWithTheID(t *testing.T) {
	f := newTracesFake(t, "clickhouse")
	if _, _, code := f.run(traceID[:16], "--datasource", "traces", "--from", "now-7d"); code != 0 {
		t.Fatalf("exit %d", code)
	}
	query, _ := f.queries[0]["query"].(string)
	want := "SELECT TraceId, SpanId, ParentSpanId, SpanName, ServiceName, Timestamp AS StartTime, Duration, StatusCode FROM otel_traces WHERE TraceId = '" + traceID[:16] + "' AND $__timeFilter(Timestamp) ORDER BY Timestamp LIMIT 1000"
	if query != want || f.queries[0]["from"] != "now-7d" {
		t.Fatalf("query %q", query)
	}
}

func TestTracesRefusesBadInputBeforeQuerying(t *testing.T) {
	cases := []struct {
		name string
		kind string
		args []string
		want string
	}{
		{"short id", "tempo", []string{"abc123", "--datasource", "traces"}, "is not a trace id"},
		{"quote in id", "clickhouse", []string{"4bf92f3577b34da6' OR 1=1", "--datasource", "traces"}, "is not a trace id"},
		{"no datasource", "tempo", []string{traceID}, "needs --datasource"},
		{"prometheus", "prometheus", []string{traceID, "--datasource", "traces"}, "traces reads Tempo or ClickHouse datasources; traces is prometheus"},
		{"unknown uid", "tempo", []string{traceID, "--datasource", "nope"}, `no datasource has uid "nope"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newTracesFake(t, tc.kind)
			_, stderr, code := f.run(tc.args...)
			if code != 2 || !strings.Contains(stderr, tc.want) || len(f.queries) != 0 {
				t.Fatalf("exit %d, stderr %q, queries %d", code, stderr, len(f.queries))
			}
		})
	}
}

func TestTracesPrintsTheTree(t *testing.T) {
	f := newTracesFake(t, "tempo")
	stdout, _, code := f.run(traceID, "--datasource", "traces")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if lines[0] != "Trace "+traceID+"  5 spans  1.23s  checkout POST /checkout" {
		t.Fatalf("header %q", lines[0])
	}
	header := lines[1]
	spanAt, startAt := strings.Index(header, "SPAN"), strings.Index(header, "START")
	var names, rest []string
	for _, line := range lines[2:] {
		names = append(names, strings.TrimRight(line[spanAt:startAt], " "))
		rest = append(rest, strings.Join(strings.Fields(line[:spanAt]+line[startAt:]), " "))
	}
	wantNames := []string{"POST /checkout", "  load cart", "  charge", "    SELECT orders", "retry"}
	wantRest := []string{
		"checkout +0µs 1.23s ok",
		"cart +5.0ms 3.0ms ok",
		"payments +20.0ms 12.4ms error",
		"postgres +30.0ms 850µs -",
		"checkout +900.0ms 0µs error",
	}
	if strings.Join(names, "|") != strings.Join(wantNames, "|") || strings.Join(rest, "|") != strings.Join(wantRest, "|") {
		t.Fatalf("spans %q, rows %q", names, rest)
	}
}

func TestSpanTreeOrdersDepthFirstByStart(t *testing.T) {
	at := func(ms int) time.Time { return time.UnixMilli(int64(ms)) }
	rows := spanTree([]span{
		{ID: "c2", ParentID: "r", Start: at(20)},
		{ID: "g", ParentID: "c1", Start: at(15)},
		{ID: "orphan", ParentID: "missing", Start: at(5)},
		{ID: "c1", ParentID: "r", Start: at(10)},
		{ID: "r", Start: at(0)},
	})
	var got []string
	for _, row := range rows {
		got = append(got, fmt.Sprintf("%s:%d", row.span.ID, row.depth))
	}
	if strings.Join(got, " ") != "r:0 c1:1 g:2 c2:1 orphan:0" {
		t.Fatalf("tree %v", got)
	}
}

func TestFormatSpanDuration(t *testing.T) {
	cases := map[time.Duration]string{
		850 * time.Microsecond:   "850µs",
		12400 * time.Microsecond: "12.4ms",
		1230 * time.Millisecond:  "1.23s",
		0:                        "0µs",
	}
	for d, want := range cases {
		if got := formatSpanDuration(d); got != want {
			t.Errorf("%v: %q, want %q", d, got, want)
		}
	}
}

func TestSpanRowsNamesAMissingColumn(t *testing.T) {
	body, err := decode[exploreResponse]([]byte(`{"frames":[{"ref":"A","fields":[{"name":"TraceId","type":"string","values":["t"]}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = spanRows(body.Frames)
	if err == nil || !strings.Contains(err.Error(), "without a SpanId column") || !strings.Contains(err.Error(), "TraceId, SpanId, ParentSpanId") {
		t.Fatalf("err %v", err)
	}
}

func TestTracesNotFoundExitsOne(t *testing.T) {
	f := newTracesFake(t, "tempo")
	f.answer = `{"frames":[],"meta":{}}`
	_, stderr, code := f.run(traceID, "--datasource", "traces")
	if code != 1 || !strings.Contains(stderr, "trace "+traceID+" was not found between now-24h and now") {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
}

func TestTracesJSONPrintsTheResponse(t *testing.T) {
	f := newTracesFake(t, "tempo")
	stdout, _, code := f.run(traceID, "--datasource", "traces", "--json")
	if code != 0 || strings.TrimSpace(stdout) != strings.TrimSpace(traceFrames) {
		t.Fatalf("exit %d, stdout %q", code, stdout)
	}
}

const traceSearchFrames = `{"frames":[{"ref":"A","fields":[
	{"name":"TraceId","type":"string","values":["t1","t2"]},
	{"name":"StartTime","type":"time","values":[1762000000000,1762000060000]},
	{"name":"ServiceName","type":"string","values":["checkout","checkout"]},
	{"name":"SpanName","type":"string","values":["POST /checkout","charge"]},
	{"name":"Duration","type":"number","values":[1230000000,"612000000"]},
	{"name":"Spans","type":"number","values":[5,3]}
]}],"meta":{}}`

func TestTracesAskRunsTheFiltersAsClickHouseSQL(t *testing.T) {
	f := newAskFake(t, "clickhouse", `{"query":"","language":"filters","trace_filters":{"service":"checkout","min":"500ms"},"explanation":"Slow checkout traces.","warnings":[]}`, traceSearchFrames)
	stdout, stderr, code := f.run("traces", "--ask", "slow checkout requests", "--from", "now-6h")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if draft := f.drafts[0]; draft["signal"] != "traces" || draft["question"] != "slow checkout requests" || draft["from"] != "now-6h" {
		t.Fatalf("draft %v", draft)
	}
	query := f.queries[0]
	want := clickhouseTraceSearch(traceFilters{Service: "checkout", Min: "500ms"})
	if len(f.queries) != 1 || query["query"] != want || query["kind"] != "instant" || query["limit"] != float64(100) || query["from"] != "now-6h" {
		t.Fatalf("queries %v", f.queries)
	}
	if stderr != "Filters: service=checkout min=500ms\nSlow checkout traces.\n" {
		t.Fatalf("stderr %q", stderr)
	}
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) != 3 || strings.Join(strings.Fields(lines[0]), " ") != "STARTED SERVICE SPAN DURATION SPANS ERRORS" {
		t.Fatalf("stdout %q", stdout)
	}
	if got := strings.Join(strings.Fields(lines[2]), " "); got != "2025-11-01T12:27:40Z checkout charge 612.0ms 3" {
		t.Fatalf("row %q", lines[2])
	}
}

func TestTracesAskOnTempo(t *testing.T) {
	cases := []struct {
		draft string
		want  string
	}{
		{`{"query":"","language":"filters","trace_filters":{"service":"checkout","errors":true},"explanation":"E.","warnings":[]}`, `{resource.service.name = "checkout" && status = error}`},
		{`{"query":"{ span.http.status_code = 500 }","language":"traceql","trace_filters":null,"explanation":"E.","warnings":[]}`, "{ span.http.status_code = 500 }"},
	}
	for _, c := range cases {
		f := newAskFake(t, "tempo", c.draft, traceSearchFrames)
		if _, stderr, code := f.run("traces", "--ask", "failed checkout"); code != 0 {
			t.Fatalf("exit %d, stderr %q", code, stderr)
		}
		if got := f.queries[0]["query"]; got != c.want {
			t.Errorf("query %q, want %q", got, c.want)
		}
	}
}

func TestTracesAskDryRunNeverSearches(t *testing.T) {
	f := newAskFake(t, "tempo", `{"query":"{ }","language":"traceql","trace_filters":null,"explanation":"E.","warnings":[]}`, traceSearchFrames)
	stdout, stderr, code := f.run("traces", "--ask", "all traces", "--dry-run")
	if code != 0 || len(f.queries) != 0 || stdout != "" || stderr != "Query: { }\nE.\n" {
		t.Fatalf("exit %d, queries %d, stdout %q, stderr %q", code, len(f.queries), stdout, stderr)
	}
}

func TestTracesAskUsageErrors(t *testing.T) {
	cases := []struct {
		kind string
		args []string
		want string
	}{
		{"tempo", []string{traceID, "--ask", "slow"}, "give a trace id or --ask, not both"},
		{"tempo", nil, "traces needs a trace id or --ask"},
		{"tempo", []string{traceID, "--dry-run"}, "--dry-run needs --ask"},
		{"tempo", []string{traceID, traceID}, "takes one trace id"},
		{"prometheus", []string{"--ask", "slow"}, "traces reads Tempo or ClickHouse datasources; ds is prometheus"},
	}
	for _, c := range cases {
		f := newAskFake(t, c.kind, `{}`, traceSearchFrames)
		_, stderr, code := f.run("traces", c.args...)
		if code != 2 || !strings.Contains(stderr, c.want) || len(f.drafts)+len(f.queries) != 0 {
			t.Errorf("%v: exit %d, stderr %q", c.args, code, stderr)
		}
	}
}
