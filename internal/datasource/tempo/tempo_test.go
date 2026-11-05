package tempo_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/tempo"
)

type reply struct {
	status int
	body   string
}

type request struct {
	path   string
	params url.Values
	header http.Header
}

type fakeTempo struct {
	mu       sync.Mutex
	replies  map[string]reply
	requests []request
}

func (f *fakeTempo) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, request{path: r.URL.Path, params: r.URL.Query(), header: r.Header.Clone()})
	rep, ok := f.replies[r.URL.Path]
	f.mu.Unlock()
	if !ok {
		rep = reply{http.StatusNotFound, ""}
	}
	if rep.status == 0 {
		rep.status = http.StatusOK
	}
	w.WriteHeader(rep.status)
	_, _ = w.Write([]byte(rep.body))
}

func (f *fakeTempo) last(t *testing.T) request {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.requests) == 0 {
		t.Fatal("no request reached the fake Tempo")
	}
	return f.requests[len(f.requests)-1]
}

func serve(t *testing.T, replies map[string]reply, mutate ...func(*tempo.Options)) (*tempo.Datasource, *fakeTempo) {
	t.Helper()
	fake := &fakeTempo{replies: replies}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	opts := tempo.Options{URL: srv.URL}
	for _, m := range mutate {
		m(&opts)
	}
	ds, err := tempo.New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = ds.Close() })
	return ds, fake
}

const (
	traceID   = "0af7651916cd43dd8448eb211c80319c"
	tracePath = "/api/traces/" + traceID
	traceBody = `{"batches":[{
		"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"checkout"}},{"key":"k8s.pod.name","value":{"stringValue":"checkout-0"}}]},
		"scopeSpans":[{"scope":{"name":"shop"},"spans":[
			{"traceId":"CvdlGRbNQ92ESOshHIAxnA==","spanId":"APBnqgupArc=","parentSpanId":"t61rcWkgMzE=","name":"charge","kind":"SPAN_KIND_CLIENT",
			 "startTimeUnixNano":"1793966399960000000","endTimeUnixNano":"1793966399990000000","status":{}},
			{"traceId":"CvdlGRbNQ92ESOshHIAxnA==","spanId":"t61rcWkgMzE=","name":"POST /checkout","kind":"SPAN_KIND_SERVER",
			 "startTimeUnixNano":"1793966399950000000","endTimeUnixNano":"1793966400000000000",
			 "attributes":[{"key":"http.method","value":{"stringValue":"POST"}},{"key":"http.status_code","value":{"intValue":"500"}},
			   {"key":"retry","value":{"boolValue":true}},{"key":"ratio","value":{"doubleValue":0.5}},{"key":"weird","value":{"doubleValue":"NaN"}},
			   {"key":"tags","value":{"arrayValue":{"values":[{"stringValue":"a"},{"intValue":"2"}]}}},
			   {"key":"nested","value":{"kvlistValue":{"values":[{"key":"k","value":{"stringValue":"v"}}]}}}],
			 "status":{"code":"STATUS_CODE_ERROR","message":"boom"}}
		]}]
	}]}`
	searchBody = `{"traces":[
		{"traceID":"f7651916cd43dd8448eb211c80319c","rootServiceName":"checkout","rootTraceName":"POST /checkout","startTimeUnixNano":"1793966399000000000","durationMs":50,
		 "spanSet":{"spans":[{"spanID":"b7ad6b7169203331"}],"matched":1}},
		{"traceID":"1b2c3d4e5f60718293a4b5c6d7e8f901","rootServiceName":"frontend","rootTraceName":"GET /","startTimeUnixNano":"1793966400000000000"}
	],"metrics":{"inspectedTraces":2}}`
)

var (
	rangeEnd = time.Date(2026, 11, 6, 12, 0, 0, 500_000_000, time.UTC)
	lastHour = datasource.TimeRange{From: rangeEnd.Add(-time.Hour), To: rangeEnd}
)

func fieldValues(t *testing.T, f datasource.Frame, name string) []any {
	t.Helper()
	i := slices.IndexFunc(f.Fields, func(field datasource.Field) bool { return field.Name == name })
	if i < 0 {
		t.Fatalf("frame has no field %q", name)
	}
	return f.Fields[i].Values
}

func fieldNames(f datasource.Frame) []string {
	names := make([]string, len(f.Fields))
	for i, field := range f.Fields {
		names[i] = string(field.Type) + ":" + field.Name
	}
	return names
}

func TestTraceIDReturnsOneRowPerSpan(t *testing.T) {
	ds, fake := serve(t, map[string]reply{tracePath: {0, traceBody}})
	frames, err := ds.QueryInstant(context.Background(), datasource.Query{Text: " " + strings.ToUpper(traceID) + "\n"}, lastHour)
	if err != nil {
		t.Fatal(err)
	}
	if got := fake.last(t).path; got != tracePath {
		t.Fatalf("path = %s, want the lowercased trace id", got)
	}
	if len(frames) != 1 {
		t.Fatalf("frames = %+v, want one", frames)
	}
	f := frames[0]
	want := []string{
		"string:TraceId", "string:SpanId", "string:ParentSpanId", "string:SpanName", "string:ServiceName",
		"time:StartTime", "number:Duration", "string:StatusCode", "string:SpanAttributes", "string:ResourceAttributes",
	}
	if got := fieldNames(f); !slices.Equal(got, want) {
		t.Fatalf("fields = %v, want %v", got, want)
	}
	checks := map[string][]any{
		"TraceId":      {traceID, traceID},
		"SpanId":       {"b7ad6b7169203331", "00f067aa0ba902b7"},
		"ParentSpanId": {"", "b7ad6b7169203331"},
		"SpanName":     {"POST /checkout", "charge"},
		"ServiceName":  {"checkout", "checkout"},
		"StartTime":    {time.Unix(0, 1793966399950000000).UTC(), time.Unix(0, 1793966399960000000).UTC()},
		"Duration":     {int64(50 * time.Millisecond), int64(30 * time.Millisecond)},
		"StatusCode":   {"Error", "Unset"},
		"SpanAttributes": {
			`{"http.method":"POST","http.status_code":500,"nested":{"k":"v"},"ratio":0.5,"retry":true,"tags":["a",2],"weird":"NaN"}`,
			`{}`,
		},
		"ResourceAttributes": {`{"k8s.pod.name":"checkout-0","service.name":"checkout"}`, `{"k8s.pod.name":"checkout-0","service.name":"checkout"}`},
	}
	for name, values := range checks {
		if got := fieldValues(t, f, name); !slices.Equal(got, values) {
			t.Errorf("%s = %v, want %v", name, got, values)
		}
	}
}

func TestTraceIDRowsAreLimited(t *testing.T) {
	ds, _ := serve(t, map[string]reply{tracePath: {0, traceBody}})
	_, err := ds.QueryRange(context.Background(), datasource.Query{Text: traceID, Limits: datasource.Limits{MaxRows: 1}}, lastHour, time.Minute)
	if !errors.Is(err, datasource.ErrLimitExceeded) || !strings.Contains(err.Error(), "more than 1 spans") {
		t.Fatalf("err = %v, want a row limit error", err)
	}
}

func TestUnknownTraceIsEmpty(t *testing.T) {
	ds, _ := serve(t, nil)
	frames, err := ds.QueryInstant(context.Background(), datasource.Query{Text: "b7ad6b7169203331"}, lastHour)
	if err != nil || len(frames) != 0 || frames == nil {
		t.Fatalf("frames = %#v, %v, want an empty list", frames, err)
	}
}

func TestSearchReturnsOneRowPerTrace(t *testing.T) {
	ds, fake := serve(t, map[string]reply{"/api/search": {0, searchBody}})
	q := datasource.Query{Text: `{ resource.service.name = "checkout" }`, Limits: datasource.Limits{MaxRows: 20}}
	frames, err := ds.QueryRange(context.Background(), q, lastHour, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	params := fake.last(t).params
	if params.Get("q") != q.Text || params.Get("limit") != "20" || params.Get("start") != "1793962800" || params.Get("end") != "1793966401" {
		t.Fatalf("params = %v", params)
	}
	if len(frames) != 1 {
		t.Fatalf("frames = %+v", frames)
	}
	f := frames[0]
	want := []string{"string:TraceId", "string:SpanId", "string:SpanName", "string:ServiceName", "time:StartTime", "number:Duration"}
	if got := fieldNames(f); !slices.Equal(got, want) {
		t.Fatalf("fields = %v, want %v", got, want)
	}
	ids := []any{"1b2c3d4e5f60718293a4b5c6d7e8f901", "00f7651916cd43dd8448eb211c80319c"}
	if got := fieldValues(t, f, "TraceId"); !slices.Equal(got, ids) {
		t.Fatalf("trace ids = %v, want newest first and padded to 32", got)
	}
	if got := fieldValues(t, f, "SpanId"); !slices.Equal(got, ids) {
		t.Fatalf("span ids = %v, want the trace ids", got)
	}
	if got := fieldValues(t, f, "SpanName"); !slices.Equal(got, []any{"GET /", "POST /checkout"}) {
		t.Fatalf("names = %v", got)
	}
	if got := fieldValues(t, f, "ServiceName"); !slices.Equal(got, []any{"frontend", "checkout"}) {
		t.Fatalf("services = %v", got)
	}
	if got := fieldValues(t, f, "Duration"); !slices.Equal(got, []any{int64(0), int64(50 * time.Millisecond)}) {
		t.Fatalf("durations = %v", got)
	}
}

func TestSearchWithNoTracesIsEmpty(t *testing.T) {
	ds, _ := serve(t, map[string]reply{"/api/search": {0, `{"traces":[],"metrics":{}}`}})
	frames, err := ds.QueryInstant(context.Background(), datasource.Query{Text: `{}`}, lastHour)
	if err != nil || len(frames) != 0 {
		t.Fatalf("frames = %+v, %v", frames, err)
	}
}

func TestSearchRefusesMoreTracesThanTheRowLimit(t *testing.T) {
	ds, _ := serve(t, map[string]reply{"/api/search": {0, searchBody}})
	_, err := ds.QueryInstant(context.Background(), datasource.Query{Text: `{}`, Limits: datasource.Limits{MaxRows: 1}}, lastHour)
	if !errors.Is(err, datasource.ErrLimitExceeded) {
		t.Fatalf("err = %v, want a row limit error", err)
	}
}

func TestEmptyQueryIsRefused(t *testing.T) {
	ds, _ := serve(t, nil)
	_, err := ds.QueryInstant(context.Background(), datasource.Query{Text: "  "}, lastHour)
	if !errors.Is(err, datasource.ErrInvalidQuery) || !strings.Contains(err.Error(), "needs a trace id or a TraceQL query") {
		t.Fatalf("err = %v", err)
	}
}

func TestUpstreamErrorsKeepTheirKind(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		kind    error
		message string
	}{
		{"parse error", 400, "invalid TraceQL query: parse error at line 1, col 26: syntax error: unexpected $end", datasource.ErrInvalidQuery, "invalid TraceQL query"},
		{"range too long", 400, "range specified by start and end exceeds 168h0m0s. received start=1 end=2", datasource.ErrLimitExceeded, "exceeds 168h"},
		{"unauthorized", 401, "", datasource.ErrAuthFailed, "authentication failed (HTTP 401)"},
		{"rate limited", 429, "", datasource.ErrUnavailable, "rate limited by upstream"},
		{"gateway timeout", 504, "", datasource.ErrTimeout, "query timed out"},
		{"server error naming an address", 500, "dial tcp 10.0.0.7:9095: connection refused", datasource.ErrUnavailable, "upstream returned HTTP 500"},
		{"unreadable success", 200, "not json", datasource.ErrUnavailable, "unreadable result"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ds, _ := serve(t, map[string]reply{"/api/search": {tc.status, tc.body}, tracePath: {tc.status, tc.body}})
			for _, text := range []string{`{}`, traceID} {
				_, err := ds.QueryInstant(context.Background(), datasource.Query{Text: text}, lastHour)
				if !errors.Is(err, tc.kind) || !strings.Contains(err.Error(), tc.message) || strings.Contains(err.Error(), "10.0.0.7") {
					t.Fatalf("%s: err = %v, want %v mentioning %q", text, err, tc.kind, tc.message)
				}
			}
		})
	}
}

func TestUnreadableSpanIDsAreRefused(t *testing.T) {
	ds, _ := serve(t, map[string]reply{tracePath: {0, `{"batches":[{"scopeSpans":[{"spans":[{"spanId":"%%%","startTimeUnixNano":"1"}]}]}]}`}})
	if _, err := ds.QueryInstant(context.Background(), datasource.Query{Text: traceID}, lastHour); !errors.Is(err, datasource.ErrUnavailable) {
		t.Fatalf("err = %v, want unreadable", err)
	}
}

func TestRequestsCarryAuthAndHeaders(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*tempo.Options)
		check  func(http.Header) bool
	}{
		{"bearer token", func(o *tempo.Options) { o.BearerToken = datasource.NewSecret("tok") }, func(h http.Header) bool { return h.Get("Authorization") == "Bearer tok" }},
		{"basic auth", func(o *tempo.Options) { o.BasicAuthUser, o.BasicAuthPassword = "u", datasource.NewSecret("p") }, func(h http.Header) bool { return h.Get("Authorization") == "Basic dTpw" }},
		{"tenant header", func(o *tempo.Options) { o.Headers = map[string]string{"X-Scope-OrgID": "team-a"} }, func(h http.Header) bool {
			return h.Get("X-Scope-OrgID") == "team-a" && h.Get("Accept") == "application/json"
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ds, fake := serve(t, map[string]reply{"/ready": {0, "ready"}}, tc.mutate)
			if err := ds.CheckHealth(context.Background()); err != nil {
				t.Fatal(err)
			}
			req := fake.last(t)
			if req.path != "/ready" || !tc.check(req.header) {
				t.Fatalf("request %s carried %v", req.path, req.header)
			}
		})
	}
}

func TestCheckHealthFailsWhenNotReady(t *testing.T) {
	ds, _ := serve(t, map[string]reply{"/ready": {http.StatusServiceUnavailable, "Ingester not ready"}})
	if err := ds.CheckHealth(context.Background()); !errors.Is(err, datasource.ErrUnavailable) {
		t.Fatalf("err = %v, want unavailable", err)
	}
}

func TestNewRefusesInvalidOptions(t *testing.T) {
	cases := []struct {
		name string
		opts tempo.Options
		want string
	}{
		{"scheme", tempo.Options{URL: "ftp://tempo"}, "url scheme must be http or https"},
		{"no host", tempo.Options{URL: "http://"}, "url requires a host"},
		{"user info", tempo.Options{URL: "http://reader@tempo"}, "url must not contain user info"},
		{"fragment", tempo.Options{URL: "http://tempo#x"}, "url must not contain a query or fragment"},
		{"authorization header", tempo.Options{URL: "http://tempo", Headers: map[string]string{"authorization": "x"}}, "Authorization header is set through basic auth"},
		{"managed header", tempo.Options{URL: "http://tempo", Headers: map[string]string{"host": "x"}}, "Host header is managed"},
		{"invalid header", tempo.Options{URL: "http://tempo", Headers: map[string]string{"X-A": "a\nb"}}, "invalid header"},
		{"basic and bearer", tempo.Options{URL: "http://tempo", BasicAuthUser: "u", BearerToken: datasource.NewSecret("t")}, "mutually exclusive"},
		{"password without user", tempo.Options{URL: "http://tempo", BasicAuthPassword: datasource.NewSecret("p")}, "requires a user"},
		{"huge response cap", tempo.Options{URL: "http://tempo", MaxResponseBytes: 2 << 30}, "must not exceed"},
		{"negative limit", tempo.Options{URL: "http://tempo", Limits: datasource.Limits{MaxRows: -1}}, "maxRows must be positive"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tempo.New(tc.opts)
			var cfg *datasource.ConfigError
			if !errors.As(err, &cfg) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want a config error mentioning %q", err, tc.want)
			}
		})
	}
}

func TestRedirectsAndLargeResponsesAreRefused(t *testing.T) {
	redirect := httptest.NewServer(http.RedirectHandler("http://127.0.0.1:1/api/search", http.StatusFound))
	t.Cleanup(redirect.Close)
	ds, err := tempo.New(tempo.Options{URL: redirect.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ds.QueryInstant(context.Background(), datasource.Query{Text: `{}`}, lastHour); !errors.Is(err, datasource.ErrUnavailable) {
		t.Fatalf("redirect err = %v, want unavailable", err)
	}
	big, _ := serve(t, map[string]reply{tracePath: {0, traceBody}}, func(o *tempo.Options) { o.MaxResponseBytes = 64 })
	if _, err := big.QueryInstant(context.Background(), datasource.Query{Text: traceID}, lastHour); !errors.Is(err, datasource.ErrLimitExceeded) || !strings.Contains(err.Error(), "larger than 64 bytes") {
		t.Fatalf("large response err = %v", err)
	}
}

func TestVariablesExpandIntoTraceQL(t *testing.T) {
	ds, fake := serve(t, map[string]reply{"/api/search": {0, `{"traces":[]}`}, tracePath: {0, traceBody}})
	vars := []datasource.Variable{{Name: "svc", Values: []string{`a"b`}}, {Name: "trace", Values: []string{traceID}}}
	if _, err := ds.QueryInstant(context.Background(), datasource.Query{Text: `{ resource.service.name = "$svc" }`, Variables: vars}, lastHour); err != nil {
		t.Fatal(err)
	}
	if got := fake.last(t).params.Get("q"); got != `{ resource.service.name = "a\"b" }` {
		t.Fatalf("q = %s", got)
	}
	if _, err := ds.QueryInstant(context.Background(), datasource.Query{Text: `{ duration > $__interval && span.window = "$__range" }`}, lastHour); err != nil {
		t.Fatal(err)
	}
	if got := fake.last(t).params.Get("q"); got != `{ duration > 3s && span.window = "3600s" }` {
		t.Fatalf("q with macros = %s", got)
	}
	frames, err := ds.QueryInstant(context.Background(), datasource.Query{Text: `"${trace}"`, Variables: vars}, lastHour)
	if err != nil || len(frames) != 1 || fake.last(t).path != tracePath {
		t.Fatalf("trace by variable = %+v, %v, path %s", frames, err, fake.last(t).path)
	}
}

func TestDiscover(t *testing.T) {
	ds, fake := serve(t, map[string]reply{
		"/api/v2/search/tags":                             {0, `{"scopes":[{"name":"intrinsic","tags":["duration","name","status"]},{"name":"resource","tags":["service.name","k8s.pod.name"]},{"name":"span","tags":["http.method"]}]}`},
		"/api/v2/search/tag/resource.service.name/values": {0, `{"tagValues":[{"type":"string","value":"frontend"},{"type":"string","value":"checkout"}]}`},
	})
	ctx := context.Background()
	labels, err := ds.Discover(ctx, datasource.DiscoverRequest{Kind: datasource.DiscoverLabels})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"duration", "name", "resource.k8s.pod.name", "resource.service.name", "span.http.method", "status"}
	if !slices.Equal(labels.Labels, want) {
		t.Fatalf("labels = %v, want %v", labels.Labels, want)
	}
	searched, err := ds.Discover(ctx, datasource.DiscoverRequest{Kind: datasource.DiscoverLabels, Search: "RESOURCE", Limit: 1})
	if err != nil || !slices.Equal(searched.Labels, []string{"resource.k8s.pod.name"}) {
		t.Fatalf("searched labels = %+v, %v", searched, err)
	}
	values, err := ds.Discover(ctx, datasource.DiscoverRequest{Kind: datasource.DiscoverLabelValues, Label: "resource.service.name", Metric: `{ span.http.method = "POST" }`})
	if err != nil || !slices.Equal(values.Values, []string{"checkout", "frontend"}) {
		t.Fatalf("values = %+v, %v", values, err)
	}
	if got := fake.last(t).params.Get("q"); got != `{ span.http.method = "POST" }` {
		t.Fatalf("q = %s", got)
	}
	for _, req := range []datasource.DiscoverRequest{
		{Kind: datasource.DiscoverLabelValues, Label: "../x"},
		{Kind: datasource.DiscoverLabelValues},
		{Kind: datasource.DiscoverMetrics},
		{Kind: datasource.DiscoverTables},
	} {
		if _, err := ds.Discover(ctx, req); !errors.Is(err, datasource.ErrInvalidQuery) {
			t.Fatalf("%+v: err = %v, want invalid query", req, err)
		}
	}
}

func TestVariableValuesAreUnsupported(t *testing.T) {
	ds, _ := serve(t, nil)
	if _, err := ds.VariableValues(context.Background(), datasource.Query{Text: "x"}, lastHour); !errors.Is(err, datasource.ErrInvalidQuery) {
		t.Fatalf("err = %v", err)
	}
}

func TestLint(t *testing.T) {
	cases := []struct {
		name       string
		query      string
		message    string
		start, end int
		severity   datasource.Severity
	}{
		{"trace id", " " + traceID + " ", "", 0, 0, ""},
		{"short trace id", "b7ad6b7169203331", "", 0, 0, ""},
		{"quoted trace id variable", `"$trace"`, "could not lint around a variable", 0, 8, datasource.SeverityWarning},
		{"bare trace id variable", `$trace`, "needs a trace id or a TraceQL query", 0, 6, datasource.SeverityWarning},
		{"spanset", `{ resource.service.name = "checkout" && status = error }`, "", 0, 0, ""},
		{"pipeline", `{ span.http.status_code >= 500 } | count() > 2`, "", 0, 0, ""},
		{"brace inside a string", "{ name = `a}b` }", "", 0, 0, ""},
		{"no spanset", `count() > 2`, "needs a trace id or a TraceQL query", 0, 11, datasource.SeverityError},
		{"empty", ``, "needs a trace id or a TraceQL query", 0, 0, datasource.SeverityError},
		{"unclosed spanset", `{ name = "x"`, `unclosed "{"`, 0, 1, datasource.SeverityError},
		{"unexpected close", `{ name = "x" }}`, `unexpected "}"`, 14, 15, datasource.SeverityError},
		{"mismatched bracket", `({ name = "x" ]`, `unexpected "]"`, 14, 15, datasource.SeverityError},
		{"problem after a variable keeps original offsets", `{ name = "$op" }}`, `unexpected "}"`, 16, 17, datasource.SeverityError},
		{"unknown macro", `{ name = "$__nope" }`, "unknown macro", 0, 0, datasource.SeverityError},
	}
	ds, err := tempo.New(tempo.Options{URL: "http://tempo.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diags, err := ds.Lint(context.Background(), tc.query)
			if err != nil {
				t.Fatal(err)
			}
			if tc.message == "" {
				if len(diags) != 0 {
					t.Fatalf("diagnostics = %+v, want none", diags)
				}
				return
			}
			if len(diags) != 1 {
				t.Fatalf("diagnostics = %+v, want one", diags)
			}
			d := diags[0]
			if !strings.Contains(d.Message, tc.message) || d.Severity != tc.severity || d.Start != tc.start || d.End != tc.end {
				t.Fatalf("diagnostic = %+v, want %q at %d-%d", d, tc.message, tc.start, tc.end)
			}
		})
	}
}
