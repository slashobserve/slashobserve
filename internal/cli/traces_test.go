package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/queryproxy"
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
	want := "SELECT TraceId, SpanId, ParentSpanId, SpanName, ServiceName, Timestamp AS StartTime, Duration, StatusCode, StatusMessage FROM otel_traces WHERE TraceId = '" + traceID[:16] + "' AND $__timeFilter(Timestamp) ORDER BY Timestamp LIMIT 1000"
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
		{"prometheus", "prometheus", []string{traceID, "--datasource", "traces"}, "traces reads Tempo, Jaeger or ClickHouse datasources; traces is prometheus"},
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

func TestTracesTreeShowsTheStatusMessageBesideTheError(t *testing.T) {
	f := newTracesFake(t, "jaeger")
	f.answer = strings.Replace(traceFrames, `"Error"]}
]}]`, `"Error"]},
	{"name":"StatusMessage","type":"string","values":["","","card declined","",""]}
]}]`, 1)
	stdout, _, code := f.run(traceID, "--datasource", "traces")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(strings.Join(strings.Fields(stdout), " "), "12.4ms error: card declined") || strings.Contains(stdout, "ok: ") {
		t.Fatalf("stdout %q", stdout)
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
	if len(f.queries) != 1 || query["query"] != want || query["kind"] != "instant" || query["limit"] != nil || query["page_size"] != float64(100) || query["page"] != float64(1) || query["sort"] != nil || query["from"] != "now-6h" {
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

func TestTracesAskSortsAllClickHouseTracesOnTheServer(t *testing.T) {
	f := newAskFake(t, "clickhouse", `{"query":"","language":"filters","trace_filters":{"errors":true},"explanation":"E.","warnings":[]}`, traceSearchFrames)
	if _, stderr, code := f.run("traces", "--ask", "failed requests", "--sort", "-duration"); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if query := f.queries[0]; query["sort"] != "-Duration" || query["page_size"] != float64(100) {
		t.Fatalf("query %v", query)
	}
}

func TestTracesAskSortsTempoRowsOnTheClient(t *testing.T) {
	f := newAskFake(t, "tempo", `{"query":"{ }","language":"traceql","trace_filters":null,"explanation":"E.","warnings":[]}`, traceSearchFrames)
	stdout, stderr, code := f.run("traces", "--ask", "all", "--sort", "-duration")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if query := f.queries[0]; query["sort"] != nil || query["limit"] != float64(100) {
		t.Fatalf("query %v", query)
	}
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if !strings.Contains(lines[1], "POST /checkout") {
		t.Fatalf("stdout %q", stdout)
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
		{"tempo", []string{traceID, "--sort", "duration"}, "--sort needs --ask"},
		{"tempo", []string{"--ask", "slow", "--sort", "Duration"}, "--sort takes start"},
		{"prometheus", []string{"--ask", "slow"}, "traces reads Tempo, Jaeger or ClickHouse datasources; ds is prometheus"},
	}
	for _, c := range cases {
		f := newAskFake(t, c.kind, `{}`, traceSearchFrames)
		_, stderr, code := f.run("traces", c.args...)
		if code != 2 || !strings.Contains(stderr, c.want) || len(f.drafts)+len(f.queries) != 0 {
			t.Errorf("%v: exit %d, stderr %q", c.args, code, stderr)
		}
	}
}

func TestTracesAskPrintsWhyTheListIsIncompleteAndHowToGetMore(t *testing.T) {
	cases := map[string]string{
		queryproxy.FullPageNote:                                 "the page is full and more rows exist; narrow --from and --to or the question to see the rest",
		queryproxy.FullPageNote + ". " + queryproxy.PartialNote: "the page is full and more rows exist; narrow --from and --to or the question to see the rest. " + queryproxy.PartialNote,
		queryproxy.PartialNote:                                  queryproxy.PartialNote,
	}
	for note, want := range cases {
		body := strings.Replace(traceSearchFrames, `{"frames"`, `{"meta":{"note":`+strconv.Quote(note)+`},"frames"`, 1)
		f := newAskFake(t, "tempo", `{"query":"{ }","language":"traceql","trace_filters":null,"explanation":"E.","warnings":[]}`, body)
		stdout, stderr, code := f.run("traces", "--ask", "all")
		if code != 0 || !strings.Contains(stderr, "Note: "+want) || strings.Contains(stderr, "cursor") || strings.Contains(stdout, "Note:") {
			t.Fatalf("%q: exit %d, stdout %q, stderr %q", note, code, stdout, stderr)
		}
	}
}

func TestTheCLIRewritesTheWalkSentenceItAlsoReadsFromTheServer(t *testing.T) {
	if !strings.Contains(queryproxy.FullPageNote, serverWalkSentence) {
		t.Fatalf("%q no longer holds %q", queryproxy.FullPageNote, serverWalkSentence)
	}
}

const sqlRows = `{"frames":[{"ref":"A","fields":[
	{"name":"TraceId","type":"string","values":["` + traceID + `","5c1d7a2e9f8b4a3d8e6f0b1c2d3e4f50"]},
	{"name":"ServiceName","type":"string","values":["checkout","cart\tweb"]},
	{"name":"Timestamp","type":"time","values":[1762000000030,null]},
	{"name":"Spans","type":"number","values":[12,3]}
]}],"meta":{"page":{"page":1,"page_size":100,"has_next":true,"has_prev":false,"total_relation":"unknown","plain":true}}}`

const traceSQL = "SELECT TraceId, ServiceName, Timestamp, count() AS Spans FROM otel_traces WHERE $__timeFilter(Timestamp) GROUP BY TraceId, ServiceName, Timestamp"

func TestTracesQueryRunsTheSQLAsOnePageOfTheExploreQuery(t *testing.T) {
	f := newTracesFake(t, "clickhouse")
	f.answer = sqlRows
	stdout, stderr, code := f.run("--query", traceSQL, "--datasource", "traces", "--from", "now-6h")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	q := f.queries[0]
	if q["query"] != traceSQL || q["kind"] != "instant" || q["from"] != "now-6h" || q["to"] != "now" || q["page_size"] != float64(100) || q["page"] != float64(1) || q["as_written"] != true {
		t.Fatalf("query %v", q)
	}
	if _, ok := q["limit"]; ok {
		t.Fatalf("a page carries no limit: %v", q)
	}
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) != 3 || strings.Fields(lines[0])[0] != "TraceId" || !strings.Contains(lines[1], "2025-11-01T12:26:40.03Z") || !strings.Contains(lines[2], "cart web") {
		t.Fatalf("stdout %q", stdout)
	}
	if !strings.Contains(stderr, "Showing the first 100 rows") {
		t.Fatalf("stderr %q", stderr)
	}
}

func TestTracesQueryPrintsDurationInAHumanUnitAndJSONKeepsNanoseconds(t *testing.T) {
	rows := `{"frames":[{"ref":"A","fields":[
	{"name":"TraceId","type":"string","values":["` + traceID + `"]},
	{"name":"Duration","type":"number","values":[17277411]},
	{"name":"Spans","type":"number","values":[2500000000]}
]}],"meta":{"page":{"page":1,"page_size":100,"has_next":false,"has_prev":false,"total_relation":"unknown","plain":true}}}`
	f := newTracesFake(t, "clickhouse")
	f.answer = rows
	stdout, stderr, code := f.run("--query", traceSQL, "--datasource", "traces")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) != 2 || !strings.Contains(lines[1], "17.3ms") || !strings.Contains(lines[1], "2500000000") || strings.Contains(lines[1], "17277411") {
		t.Fatalf("stdout %q", stdout)
	}
	stdout, _, code = f.run("--query", traceSQL, "--datasource", "traces", "--json")
	if code != 0 || !strings.Contains(stdout, "17277411") {
		t.Fatalf("exit %d, stdout %q", code, stdout)
	}
}

func TestTracesQueryJSONPrintsTheResponse(t *testing.T) {
	f := newTracesFake(t, "clickhouse")
	f.answer = sqlRows
	stdout, _, code := f.run("--query", traceSQL, "--datasource", "traces", "--json")
	if code != 0 || !strings.Contains(stdout, `"has_next":true`) {
		t.Fatalf("exit %d, stdout %q", code, stdout)
	}
}

func TestTracesQueryUsageErrors(t *testing.T) {
	cases := []struct {
		name string
		kind string
		args []string
		want string
	}{
		{"prometheus", "prometheus", []string{"--query", "up", "--datasource", "traces"}, "traces reads Tempo, Jaeger or ClickHouse datasources; traces is prometheus"},
		{"empty", "clickhouse", []string{"--query", " ", "--datasource", "traces"}, "--query needs a query"},
		{"with an id", "clickhouse", []string{traceID, "--query", traceSQL, "--datasource", "traces"}, "give a trace id or --query, not both"},
		{"with ask", "clickhouse", []string{"--query", traceSQL, "--ask", "slow", "--datasource", "traces"}, "give --ask or --query, not both"},
		{"with dry run", "clickhouse", []string{"--query", traceSQL, "--dry-run", "--datasource", "traces"}, "--dry-run needs --ask"},
		{"with a bad sort", "jaeger", []string{"--query", "service=checkout", "--sort", "Duration", "--datasource", "traces"}, "--sort takes start"},
		{"sort with an id", "tempo", []string{traceID, "--sort", "duration", "--datasource", "traces"}, "--sort needs --ask or --query"},
		{"with compare", "clickhouse", []string{"--query", traceSQL, "--compare", traceID, "--datasource", "traces"}, "give --compare or --query, not both"},
		{"no datasource", "clickhouse", []string{"--query", traceSQL}, "needs --datasource"},
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

func TestTracesQueryRefusesSortOnClickHouseSQL(t *testing.T) {
	f := newTracesFake(t, "clickhouse")
	_, stderr, code := f.run("--query", traceSQL, "--sort", "-duration", "--datasource", "traces")
	if code != 2 || !strings.Contains(stderr, "--sort does not apply to SQL") || len(f.queries) != 0 {
		t.Fatalf("exit %d, stderr %q, queries %d", code, stderr, len(f.queries))
	}
}

func TestTracesQueryRunsAJaegerOrTempoSearchAsTheAskResultDoes(t *testing.T) {
	cases := []struct {
		kind  string
		query string
	}{
		{"jaeger", "service=checkout error=true"},
		{"tempo", `{ resource.service.name = "checkout" && status = error }`},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			f := newTracesFake(t, tc.kind)
			f.answer = traceSearchFrames
			stdout, stderr, code := f.run("--query", tc.query, "--datasource", "traces", "--from", "now-6h", "--sort", "duration")
			if code != 0 {
				t.Fatalf("exit %d, stderr %q", code, stderr)
			}
			q := f.queries[0]
			if len(f.queries) != 1 || q["query"] != tc.query || q["kind"] != "instant" || q["from"] != "now-6h" || q["to"] != "now" || q["limit"] != float64(100) || q["sort"] != nil || q["page_size"] != nil || q["as_written"] != nil {
				t.Fatalf("query %v", q)
			}
			lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
			if len(lines) != 3 || strings.Fields(lines[0])[0] != "STARTED" || !strings.Contains(lines[1], "charge") || !strings.Contains(lines[2], "POST /checkout") {
				t.Fatalf("stdout %q", stdout)
			}
		})
	}
}

func TestTracesQueryOnJaegerPrintsTheNoteAndJSON(t *testing.T) {
	f := newTracesFake(t, "jaeger")
	f.answer = `{"frames":[],"meta":{}}`
	stdout, stderr, code := f.run("--query", "service=nope", "--datasource", "traces")
	if code != 0 || stdout != "" || !strings.Contains(stderr, "No traces between now-24h and now") {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	f.answer = traceSearchFrames
	stdout, _, code = f.run("--query", "service=checkout", "--datasource", "traces", "--json")
	if code != 0 || strings.TrimSpace(stdout) != strings.TrimSpace(traceSearchFrames) {
		t.Fatalf("exit %d, stdout %q", code, stdout)
	}
}

func TestTracesQuerySaysWhenNothingMatched(t *testing.T) {
	f := newTracesFake(t, "clickhouse")
	f.answer = `{"frames":[],"meta":{}}`
	stdout, stderr, code := f.run("--query", traceSQL, "--datasource", "traces")
	if code != 0 || stdout != "" || !strings.Contains(stderr, "No rows between now-24h and now") {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}
