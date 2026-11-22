package jaeger_test

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/jaeger"
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

type fakeJaeger struct {
	mu       sync.Mutex
	replies  map[string]reply
	requests []request
}

func (f *fakeJaeger) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, request{path: r.URL.Path, params: r.URL.Query(), header: r.Header.Clone()})
	rep, ok := f.replies[r.URL.Path]
	f.mu.Unlock()
	if !ok {
		rep = reply{http.StatusNotFound, "404 page not found"}
	}
	if rep.status == 0 {
		rep.status = http.StatusOK
	}
	w.WriteHeader(rep.status)
	_, _ = w.Write([]byte(rep.body))
}

func (f *fakeJaeger) last(t *testing.T) request {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.requests) == 0 {
		t.Fatal("no request reached the fake Jaeger")
	}
	return f.requests[len(f.requests)-1]
}

func serve(t *testing.T, replies map[string]reply, mutate ...func(*jaeger.Options)) (*jaeger.Datasource, *fakeJaeger) {
	t.Helper()
	fake := &fakeJaeger{replies: replies}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	opts := jaeger.Options{URL: srv.URL}
	for _, m := range mutate {
		m(&opts)
	}
	ds, err := jaeger.New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = ds.Close() })
	return ds, fake
}

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := fs.ReadFile(os.DirFS("testdata"), name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

const (
	traceID   = "0af7651916cd43dd8448eb211c80319c"
	tracePath = "/api/v3/traces/" + traceID
	searchURL = "/api/v3/trace-summaries"
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

func TestSearchSendsTheFiltersAndReturnsOneRowPerTrace(t *testing.T) {
	ds, fake := serve(t, map[string]reply{searchURL: {0, fixture(t, "summaries.json")}})
	q := datasource.Query{
		Text:   `service=checkout operation="POST /checkout" minDuration=10ms maxDuration=1s http.status_code=500`,
		Limits: datasource.Limits{MaxRows: 20},
	}
	frames, err := ds.QueryRange(context.Background(), q, lastHour, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	p := fake.last(t).params
	want := map[string]string{
		"query.serviceName":   "checkout",
		"query.operationName": "POST /checkout",
		"query.durationMin":   "10ms",
		"query.durationMax":   "1s",
		"query.attributes":    `{"http.status_code":"500"}`,
		"query.searchDepth":   "20",
		"query.startTimeMin":  "2026-11-06T11:00:00.5Z",
		"query.startTimeMax":  "2026-11-06T12:00:00.5Z",
	}
	for k, v := range want {
		if p.Get(k) != v {
			t.Errorf("%s = %q, want %q (params %v)", k, p.Get(k), v, p)
		}
	}
	if len(frames) != 1 {
		t.Fatalf("frames = %+v", frames)
	}
	f := frames[0]
	names := []string{"string:TraceId", "string:SpanId", "string:SpanName", "string:ServiceName", "time:StartTime", "number:Duration", "number:Spans", "number:Errors"}
	if got := fieldNames(f); !slices.Equal(got, names) {
		t.Fatalf("fields = %v, want %v", got, names)
	}
	ids := []any{"1b2c3d4e5f60718293a4b5c6d7e8f901", "00f7651916cd43dd8448eb211c80319c"}
	if got := fieldValues(t, f, "TraceId"); !slices.Equal(got, ids) {
		t.Fatalf("trace ids = %v, want newest first and padded to 32", got)
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
	if got := fieldValues(t, f, "Spans"); !slices.Equal(got, []any{datasource.Number(1), datasource.Number(8)}) {
		t.Fatalf("spans = %v", got)
	}
	if got := fieldValues(t, f, "Errors"); !slices.Equal(got, []any{datasource.Number(0), datasource.Number(3)}) {
		t.Fatalf("errors = %v", got)
	}
}

func TestSearchOmitsFiltersTheQueryDoesNotSet(t *testing.T) {
	ds, fake := serve(t, map[string]reply{searchURL: {0, `{"summaries":[]}`}})
	frames, err := ds.QueryInstant(context.Background(), datasource.Query{}, lastHour)
	if err != nil || len(frames) != 0 {
		t.Fatalf("frames = %+v, %v; want none", frames, err)
	}
	p := fake.last(t).params
	for _, k := range []string{"query.serviceName", "query.operationName", "query.attributes", "query.durationMin", "query.durationMax"} {
		if p.Has(k) {
			t.Errorf("%s was sent: %v", k, p)
		}
	}
}

func TestSearchReportsTruncationAtTheRowLimit(t *testing.T) {
	ds, _ := serve(t, map[string]reply{searchURL: {0, fixture(t, "summaries.json")}})
	frames, err := ds.QueryInstant(context.Background(), datasource.Query{Limits: datasource.Limits{MaxRows: 2}}, lastHour)
	if err != nil || len(frames) != 1 || frames[0].Meta == nil || !frames[0].Meta.Truncated {
		t.Fatalf("frames = %+v, %v; want a truncated frame", frames, err)
	}
}

func TestSearchRefusesARangeThatStartsInTheFuture(t *testing.T) {
	ds, _ := serve(t, nil)
	future := time.Now().Add(time.Hour)
	_, err := ds.QueryInstant(context.Background(), datasource.Query{}, datasource.TimeRange{From: future, To: future.Add(time.Hour)})
	if !errors.Is(err, datasource.ErrRangeInFuture) {
		t.Fatalf("err = %v", err)
	}
}

func TestInvalidSearchIsRefusedBeforeAnyRequest(t *testing.T) {
	ds, fake := serve(t, nil)
	_, err := ds.QueryInstant(context.Background(), datasource.Query{Text: `checkout`}, lastHour)
	if !errors.Is(err, datasource.ErrInvalidQuery) || !strings.Contains(err.Error(), "expected key=value") {
		t.Fatalf("err = %v", err)
	}
	if len(fake.requests) != 0 {
		t.Fatalf("requests = %+v", fake.requests)
	}
}

func TestTraceByIDReturnsOneRowPerSpan(t *testing.T) {
	ds, fake := serve(t, map[string]reply{tracePath: {0, fixture(t, "trace.json")}})
	frames, err := ds.QueryInstant(context.Background(), datasource.Query{Text: " " + strings.ToUpper(traceID) + " "}, lastHour)
	if err != nil || len(frames) != 1 {
		t.Fatalf("frames = %+v, %v", frames, err)
	}
	if p := fake.last(t).params; p.Get("startTime") != "2026-11-06T11:00:00.5Z" || p.Get("endTime") != "2026-11-06T12:00:00.5Z" {
		t.Fatalf("params = %v", p)
	}
	f := frames[0]
	if got := fieldValues(t, f, "SpanName"); !slices.Equal(got, []any{"POST /checkout", "charge", "authorize"}) {
		t.Fatalf("names = %v, want start order", got)
	}
	if got := fieldValues(t, f, "SpanId"); !slices.Equal(got, []any{"b7ad6b7169203331", "00f067aa0ba902b7", "00f067aa0ba902b8"}) {
		t.Fatalf("span ids = %v", got)
	}
	if got := fieldValues(t, f, "ParentSpanId"); !slices.Equal(got, []any{"", "b7ad6b7169203331", "00f067aa0ba902b7"}) {
		t.Fatalf("parents = %v, want base64 ids decoded to hex", got)
	}
	if got := fieldValues(t, f, "TraceId"); got[2] != traceID {
		t.Fatalf("trace ids = %v", got)
	}
	if got := fieldValues(t, f, "ServiceName"); !slices.Equal(got, []any{"checkout", "checkout", "payments"}) {
		t.Fatalf("services = %v", got)
	}
	if got := fieldValues(t, f, "Duration"); !slices.Equal(got, []any{int64(50 * time.Millisecond), int64(30 * time.Millisecond), int64(20 * time.Millisecond)}) {
		t.Fatalf("durations = %v", got)
	}
	if got := fieldValues(t, f, "StatusCode"); !slices.Equal(got, []any{"Error", "Unset", "Ok"}) {
		t.Fatalf("status = %v", got)
	}
	if got := fieldValues(t, f, "StatusMessage"); got[0] != "boom" {
		t.Fatalf("status messages = %v", got)
	}
	if got := fieldValues(t, f, "SpanKind"); !slices.Equal(got, []any{"Server", "Client", "Server"}) {
		t.Fatalf("kinds = %v", got)
	}
	attrs := fieldValues(t, f, "SpanAttributes")[0].(string)
	for _, want := range []string{`"http.method":"POST"`, `"http.status_code":500`, `"retry":true`, `"ratio":0.5`, `"tags":["a",2]`, `"nested":{"k":"v"}`} {
		if !strings.Contains(attrs, want) {
			t.Errorf("attributes %s lack %s", attrs, want)
		}
	}
	if got := fieldValues(t, f, "ResourceAttributes")[0].(string); !strings.Contains(got, `"k8s.pod.name":"checkout-0"`) || !strings.Contains(got, `"service.name":"checkout"`) {
		t.Fatalf("resource attributes = %s", got)
	}
	if got := fieldValues(t, f, "SpanEvents")[1].(string); !strings.Contains(got, `"name":"retry"`) || !strings.Contains(got, `"attempt":2`) {
		t.Fatalf("events = %s", got)
	}
	if got := fieldValues(t, f, "SpanLinks")[1].(string); !strings.Contains(got, "1b2c3d4e5f60718293a4b5c6d7e8f901") {
		t.Fatalf("links = %s", got)
	}
	if f.Meta == nil || f.Meta.Bytes == 0 || f.Meta.Truncated {
		t.Fatalf("meta = %+v", f.Meta)
	}
}

func TestTraceCapsSpansAtTheRowLimit(t *testing.T) {
	ds, _ := serve(t, map[string]reply{tracePath: {0, fixture(t, "trace.json")}})
	f, err := ds.Trace(context.Background(), traceID, lastHour, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(fieldValues(t, f, "SpanId")) != 2 || f.Meta == nil || !f.Meta.Truncated || f.Meta.RowsTotal != 3 {
		t.Fatalf("frame = %+v", f)
	}
}

func TestMissingTraceIsAnEmptyResultOnQueryAndNotFoundOnTrace(t *testing.T) {
	for name, rep := range map[string]reply{
		"json 404": {404, `{"error":{"httpCode":404,"message":"trace not found"}}`},
		"no spans": {200, `{"result":{"resourceSpans":[]}}`},
	} {
		t.Run(name, func(t *testing.T) {
			ds, _ := serve(t, map[string]reply{tracePath: rep})
			frames, err := ds.QueryInstant(context.Background(), datasource.Query{Text: traceID}, lastHour)
			if err != nil || len(frames) != 0 {
				t.Fatalf("frames = %+v, %v", frames, err)
			}
			if _, err := ds.Trace(context.Background(), traceID, lastHour, 10); !errors.Is(err, datasource.ErrNotFound) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestOlderJaegerWithoutTheV3APIIsNamed(t *testing.T) {
	ds, _ := serve(t, nil)
	err := ds.CheckHealth(context.Background())
	if !errors.Is(err, datasource.ErrUnavailable) || !strings.Contains(err.Error(), "Jaeger 2.21 or later") {
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
		{"bad request", 400, `{"error":{"httpCode":400,"message":"invalid duration"}}`, datasource.ErrInvalidQuery, "invalid duration"},
		{"search depth", 400, `{"error":{"httpCode":400,"message":"search depth above the maximum"}}`, datasource.ErrLimitExceeded, "search depth"},
		{"unauthorized", 401, "", datasource.ErrAuthFailed, "authentication failed (HTTP 401)"},
		{"forbidden", 403, "", datasource.ErrAuthFailed, "authentication failed (HTTP 403)"},
		{"rate limited", 429, "", datasource.ErrUnavailable, "rate limited by upstream"},
		{"gateway timeout", 504, "", datasource.ErrTimeout, "query timed out"},
		{"server error naming an address", 500, `{"error":{"message":"dial tcp 10.0.0.7:17271: connection refused"}}`, datasource.ErrUnavailable, "upstream returned HTTP 500"},
		{"unreadable success", 200, "not json", datasource.ErrUnavailable, "unreadable result"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ds, _ := serve(t, map[string]reply{searchURL: {tc.status, tc.body}, tracePath: {tc.status, tc.body}})
			for _, text := range []string{``, traceID} {
				_, err := ds.QueryInstant(context.Background(), datasource.Query{Text: text}, lastHour)
				if !errors.Is(err, tc.kind) || !strings.Contains(err.Error(), tc.message) || strings.Contains(err.Error(), "10.0.0.7") {
					t.Fatalf("%q: err = %v, want %v mentioning %q", text, err, tc.kind, tc.message)
				}
			}
		})
	}
}

func TestUnreadableSpanIDsAreRefused(t *testing.T) {
	ds, _ := serve(t, map[string]reply{tracePath: {0, `{"result":{"resourceSpans":[{"scopeSpans":[{"spans":[{"spanId":"%%%","startTimeUnixNano":"1"}]}]}]}}`}})
	if _, err := ds.QueryInstant(context.Background(), datasource.Query{Text: traceID}, lastHour); !errors.Is(err, datasource.ErrUnavailable) {
		t.Fatalf("err = %v, want unreadable", err)
	}
}

func TestRequestsCarryAuthAndHeaders(t *testing.T) {
	ds, fake := serve(t, map[string]reply{"/api/v3/services": {0, `{"services":[]}`}}, func(o *jaeger.Options) {
		o.BearerToken = datasource.NewSecret("tok")
		o.Headers = map[string]string{"X-Scope-OrgID": "acme"}
	})
	if err := ds.CheckHealth(context.Background()); err != nil {
		t.Fatal(err)
	}
	h := fake.last(t).header
	if h.Get("Authorization") != "Bearer tok" || h.Get("X-Scope-OrgID") != "acme" || h.Get("Accept") != "application/json" {
		t.Fatalf("headers = %v", h)
	}
	ds, fake = serve(t, map[string]reply{"/api/v3/services": {0, `{"services":[]}`}}, func(o *jaeger.Options) {
		o.BasicAuthUser, o.BasicAuthPassword = "u", datasource.NewSecret("p")
	})
	if err := ds.CheckHealth(context.Background()); err != nil {
		t.Fatal(err)
	}
	if u, p, ok := (&http.Request{Header: fake.last(t).header}).BasicAuth(); !ok || u != "u" || p != "p" {
		t.Fatalf("basic auth = %q %q %v", u, p, ok)
	}
}

func TestCheckHealthFailsWhenTheServerIsDown(t *testing.T) {
	ds, _ := serve(t, map[string]reply{"/api/v3/services": {503, ""}})
	if err := ds.CheckHealth(context.Background()); !errors.Is(err, datasource.ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestNewRefusesInvalidOptions(t *testing.T) {
	cases := map[string]jaeger.Options{
		"no scheme":             {URL: "jaeger:16686"},
		"no host":               {URL: "http://"},
		"user info":             {URL: "http://reader@jaeger"},
		"query":                 {URL: "http://jaeger?x=1"},
		"password without user": {URL: "http://jaeger", BasicAuthPassword: datasource.NewSecret("p")},
		"two auth methods":      {URL: "http://jaeger", BasicAuthUser: "u", BearerToken: datasource.NewSecret("t")},
		"authorization header":  {URL: "http://jaeger", Headers: map[string]string{"Authorization": "x"}},
		"reserved header":       {URL: "http://jaeger", Headers: map[string]string{"host": "x"}},
		"negative max bytes":    {URL: "http://jaeger", MaxResponseBytes: -1},
	}
	for name, opts := range cases {
		t.Run(name, func(t *testing.T) {
			var cfg *datasource.ConfigError
			if _, err := jaeger.New(opts); !errors.As(err, &cfg) {
				t.Fatalf("err = %v, want a config error", err)
			}
		})
	}
	if ds, err := jaeger.New(jaeger.Options{URL: "http://jaeger/prefix/"}); err != nil || ds.Type() != "jaeger" {
		t.Fatalf("ds = %v, err = %v", ds, err)
	}
}

func TestRedirectsAndLargeResponsesAreRefused(t *testing.T) {
	redirect := httptest.NewServer(http.RedirectHandler("http://127.0.0.1:1/", http.StatusFound))
	t.Cleanup(redirect.Close)
	ds, err := jaeger.New(jaeger.Options{URL: redirect.URL})
	if err != nil {
		t.Fatal(err)
	}
	if err := ds.CheckHealth(context.Background()); err == nil {
		t.Fatal("a redirect was followed")
	}
	big, _ := serve(t, map[string]reply{"/api/v3/services": {0, `{"services":["` + strings.Repeat("a", 200) + `"]}`}}, func(o *jaeger.Options) { o.MaxResponseBytes = 100 })
	if _, err := big.Services(context.Background(), datasource.ServiceRequest{}); !errors.Is(err, datasource.ErrLimitExceeded) {
		t.Fatalf("err = %v", err)
	}
}

func TestVariablesExpandIntoTheSearch(t *testing.T) {
	ds, fake := serve(t, map[string]reply{searchURL: {0, `{"summaries":[]}`}})
	q := datasource.Query{Text: `service=$svc`, Variables: []datasource.Variable{{Name: "svc", Values: []string{"checkout"}}}}
	if _, err := ds.QueryInstant(context.Background(), q, lastHour); err != nil {
		t.Fatal(err)
	}
	if got := fake.last(t).params.Get("query.serviceName"); got != "checkout" {
		t.Fatalf("serviceName = %q", got)
	}
}

func TestServicesAndOperations(t *testing.T) {
	ds, fake := serve(t, map[string]reply{
		"/api/v3/services":   {0, `{"services":["frontend","checkout","checkout"]}`},
		"/api/v3/operations": {0, `{"operations":[{"name":"POST /checkout","spanKind":"server"},{"name":"charge"}]}`},
	})
	ctx := context.Background()
	list, err := ds.Services(ctx, datasource.ServiceRequest{Limit: 10})
	if err != nil || list.Field != "service" || !slices.Equal(list.Values, []string{"checkout", "frontend"}) {
		t.Fatalf("services = %+v, %v", list, err)
	}
	labels, err := ds.Discover(ctx, datasource.DiscoverRequest{Kind: datasource.DiscoverLabels, Search: "SERV"})
	if err != nil || !slices.Equal(labels.Labels, []string{"service"}) {
		t.Fatalf("labels = %+v, %v", labels, err)
	}
	values, err := ds.Discover(ctx, datasource.DiscoverRequest{Kind: datasource.DiscoverLabelValues, Label: "operation", Metric: "service=checkout"})
	if err != nil || !slices.Equal(values.Values, []string{"POST /checkout", "charge"}) {
		t.Fatalf("operations = %+v, %v", values, err)
	}
	if got := fake.last(t).params.Get("service"); got != "checkout" {
		t.Fatalf("service = %q", got)
	}
	for _, req := range []datasource.DiscoverRequest{
		{Kind: datasource.DiscoverLabelValues, Label: "operation"},
		{Kind: datasource.DiscoverLabelValues, Label: "http.method"},
		{Kind: datasource.DiscoverMetrics},
	} {
		if _, err := ds.Discover(ctx, req); !errors.Is(err, datasource.ErrInvalidQuery) {
			t.Fatalf("%+v: err = %v, want invalid query", req, err)
		}
	}
}

func TestDiscoverTablesListsServicesWithTheirOperations(t *testing.T) {
	ds, _ := serve(t, map[string]reply{
		"/api/v3/services":   {0, `{"services":["frontend","checkout"]}`},
		"/api/v3/operations": {0, `{"operations":[{"name":"GET /"}]}`},
	})
	cat, err := ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverTables})
	if err != nil || cat.Sample == nil {
		t.Fatalf("catalog = %+v, %v", cat, err)
	}
	if got := fieldValues(t, *cat.Sample, "service"); !slices.Equal(got, []any{"checkout", "frontend"}) {
		t.Fatalf("services = %v", got)
	}
	if got := fieldValues(t, *cat.Sample, "span_name"); !slices.Equal(got, []any{"GET /", "GET /"}) {
		t.Fatalf("span names = %v", got)
	}
	if cat.Truncated || !slices.Equal(cat.Labels, []string{"operation", "service"}) {
		t.Fatalf("catalog = %+v", cat)
	}
}

func TestVariableValuesAreUnsupported(t *testing.T) {
	ds, _ := serve(t, nil)
	if _, err := ds.VariableValues(context.Background(), datasource.Query{Text: "x"}, lastHour); !errors.Is(err, datasource.ErrInvalidQuery) {
		t.Fatalf("err = %v", err)
	}
}

func TestQueryPageWindowsTheSearchByOffset(t *testing.T) {
	ds, fake := serve(t, map[string]reply{searchURL: {0, fixture(t, "summaries.json")}})
	if !ds.PagesByOffset() {
		t.Fatal("Jaeger pages by offset")
	}
	req := datasource.PageRequest{Query: datasource.Query{Limits: datasource.Limits{MaxRows: 100}}, Range: lastHour, Page: 1, PageSize: 1}
	frames, info, err := ds.QueryPage(context.Background(), req)
	if err != nil || len(frames) != 1 {
		t.Fatalf("frames = %+v, %v", frames, err)
	}
	if got := fakeDepth(t, fake); got != "2" {
		t.Fatalf("searchDepth = %s, want one page plus one row", got)
	}
	if !info.HasNext || info.Page != 1 || info.Window != 100 || len(fieldValues(t, frames[0], "TraceId")) != 1 {
		t.Fatalf("info = %+v", info)
	}
	req.Page = 2
	frames, info, err = ds.QueryPage(context.Background(), req)
	if err != nil || info.HasNext || fieldValues(t, frames[0], "TraceId")[0] != "00f7651916cd43dd8448eb211c80319c" {
		t.Fatalf("page 2 = %+v, %+v, %v", frames, info, err)
	}
	req.Page = 101
	if _, _, err := ds.QueryPage(context.Background(), req); !errors.Is(err, datasource.ErrInvalidQuery) || !strings.Contains(err.Error(), "pages past 100 traces") {
		t.Fatalf("past the window: err = %v", err)
	}
	req.Page, req.Query.Text = 1, traceID
	if _, _, err := ds.QueryPage(context.Background(), req); !errors.Is(err, datasource.ErrInvalidQuery) || !strings.Contains(err.Error(), "paging needs a Jaeger search") {
		t.Fatalf("trace id: err = %v", err)
	}
}

func fakeDepth(t *testing.T, fake *fakeJaeger) string {
	t.Helper()
	return fake.last(t).params.Get("query.searchDepth")
}

func TestDependencies(t *testing.T) {
	ds, fake := serve(t, map[string]reply{"/api/dependencies": {0, fixture(t, "dependencies.json")}})
	deps, err := ds.Dependencies(context.Background(), lastHour)
	if err != nil {
		t.Fatal(err)
	}
	want := []datasource.ServiceDependency{{Parent: "frontend", Child: "checkout", Calls: 12}, {Parent: "checkout", Child: "payments", Calls: 7}}
	if !slices.Equal(deps, want) {
		t.Fatalf("deps = %+v, want %+v", deps, want)
	}
	p := fake.last(t).params
	if p.Get("endTs") != "1793966400500" || p.Get("lookback") != "3600000" {
		t.Fatalf("params = %v", p)
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
		{"empty", ``, "", 0, 0, ""},
		{"trace id", " " + traceID + " ", "", 0, 0, ""},
		{"search", `service=checkout minDuration=1s http.status_code=500`, "", 0, 0, ""},
		{"bare word", `service=a checkout`, "expected key=value", 10, 18, datasource.SeverityError},
		{"bad duration", `maxDuration=soon`, "maxDuration must be a duration", 12, 16, datasource.SeverityError},
		{"inside a variable", `service=$svc minDuration=x`, "minDuration must be a duration", 25, 26, datasource.SeverityError},
		{"unknown macro", `service=$__nope`, "unknown macro", 0, 0, datasource.SeverityError},
	}
	ds, err := jaeger.New(jaeger.Options{URL: "http://jaeger.invalid"})
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
