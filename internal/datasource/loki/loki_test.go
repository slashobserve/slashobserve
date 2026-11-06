package loki_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/loki"
)

type request struct {
	path   string
	params url.Values
	header http.Header
}

type fakeLoki struct {
	mu       sync.Mutex
	requests []request
	status   int
	body     string
}

func (f *fakeLoki) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, request{path: r.URL.Path, params: r.URL.Query(), header: r.Header.Clone()})
	status, body := f.status, f.body
	f.mu.Unlock()
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func (f *fakeLoki) last(t *testing.T) request {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.requests) == 0 {
		t.Fatal("no request reached the fake Loki")
	}
	return f.requests[len(f.requests)-1]
}

func serve(t *testing.T, status int, body string, mutate ...func(*loki.Options)) (*loki.Datasource, *fakeLoki) {
	t.Helper()
	fake := &fakeLoki{status: status, body: body}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	opts := loki.Options{URL: srv.URL}
	for _, m := range mutate {
		m(&opts)
	}
	ds, err := loki.New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = ds.Close() })
	return ds, fake
}

func success(resultType, result string) string {
	return fmt.Sprintf(`{"status":"success","data":{"resultType":%q,"result":%s,"stats":{}}}`, resultType, result)
}

var (
	rangeEnd   = time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	lastHour   = datasource.TimeRange{From: rangeEnd.Add(-time.Hour), To: rangeEnd}
	twoStreams = success("streams", `[
		{"stream":{"service_name":"frontend","detected_level":"warn"},"values":[["1793966400000000002","level=warn msg=slow"]]},
		{"stream":{"service_name":"checkout","level":"error","detected_level":"error"},"values":[["1793966400000000001","retrying"],["1793966400000000000","payment failed"]]},
		{"stream":{"service_name":"payments"},"values":[["1793966399000000000","charged"]]}
	]`)
)

func fieldValues(t *testing.T, f datasource.Frame, name string) []any {
	t.Helper()
	i := slices.IndexFunc(f.Fields, func(field datasource.Field) bool { return field.Name == name })
	if i < 0 {
		t.Fatalf("frame %v has no field %q", f.Labels, name)
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

func TestLogQueryReturnsOneFramePerStream(t *testing.T) {
	ds, fake := serve(t, 0, twoStreams)
	frames, err := ds.QueryInstant(context.Background(), datasource.Query{Text: `{service_name=~".+"}`}, lastHour)
	if err != nil {
		t.Fatal(err)
	}
	req := fake.last(t)
	if req.path != "/loki/api/v1/query_range" {
		t.Fatalf("path = %q, want the range endpoint because Loki refuses instant log queries", req.path)
	}
	want := url.Values{
		"query":     {`{service_name=~".+"}`},
		"start":     {"2026-11-06T11:00:00Z"},
		"end":       {"2026-11-06T12:00:00Z"},
		"limit":     {"5000"},
		"direction": {"backward"},
	}
	if fmt.Sprint(req.params) != fmt.Sprint(want) {
		t.Fatalf("params = %v, want %v", req.params, want)
	}
	if len(frames) != 3 {
		t.Fatalf("got %d frames, want 3", len(frames))
	}
	checkout, frontend, payments := frames[0], frames[1], frames[2]
	if checkout.Labels["service_name"] != "checkout" || frontend.Labels["service_name"] != "frontend" || payments.Labels["service_name"] != "payments" {
		t.Fatalf("frames are not ordered by labels: %v, %v, %v", checkout.Labels, frontend.Labels, payments.Labels)
	}
	if got, want := fieldNames(checkout), []string{"time:Time", "string:Body", "string:TimeNs", "string:level"}; !slices.Equal(got, want) {
		t.Fatalf("fields = %v, want %v", got, want)
	}
	if got := fieldValues(t, checkout, "Body"); !slices.Equal(got, []any{"retrying", "payment failed"}) {
		t.Fatalf("bodies = %v", got)
	}
	if got := fieldValues(t, checkout, "Time"); got[0] != time.Unix(0, 1793966400000000001).UTC() || got[1] != time.Unix(0, 1793966400000000000).UTC() {
		t.Fatalf("times = %v", got)
	}
	if got := fieldValues(t, checkout, "TimeNs"); !slices.Equal(got, []any{"1793966400000000001", "1793966400000000000"}) {
		t.Fatalf("nanosecond times = %v", got)
	}
	if got := fieldValues(t, checkout, "level"); !slices.Equal(got, []any{"error", "error"}) {
		t.Fatalf("levels = %v", got)
	}
	if got := fieldValues(t, frontend, "level"); !slices.Equal(got, []any{"warn"}) {
		t.Fatalf("detected level = %v", got)
	}
	if got, want := fieldNames(payments), []string{"time:Time", "string:Body", "string:TimeNs"}; !slices.Equal(got, want) {
		t.Fatalf("a stream without a level has fields %v, want %v", got, want)
	}
	if slices.ContainsFunc(frames, func(f datasource.Frame) bool { return f.Meta != nil }) {
		t.Fatalf("frames carry meta below the line limit: %+v", frames)
	}

	ds, _ = serve(t, 0, oneLineStreams(300))
	q := datasource.Query{Text: `{service_name=~".+"}`, Limits: datasource.Limits{MaxSeries: 200}}
	frames, err = ds.QueryInstant(context.Background(), q, lastHour)
	if err != nil {
		t.Fatalf("300 streams under MaxSeries 200: %v", err)
	}
	if len(frames) != 300 {
		t.Fatalf("got %d frames, want 300: a log query counts lines, not streams", len(frames))
	}
}

func oneLineStreams(n int) string {
	streams := make([]string, n)
	for i := range streams {
		streams[i] = fmt.Sprintf(`{"stream":{"service_name":"svc-%03d","trace_id":"%d"},"values":[["%d","line"]]}`, i, i, 1793966400000000000+i)
	}
	return success("streams", "["+strings.Join(streams, ",")+"]")
}

func TestLogQueryMarksTruncationAtTheLimit(t *testing.T) {
	cases := []struct {
		name      string
		body      string
		truncated bool
	}{
		{"as many lines as the limit", oneLineStreams(2), true},
		{"fewer lines than the limit", oneLineStreams(1), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ds, fake := serve(t, 0, tc.body)
			q := datasource.Query{Text: `{a="b"}`, Limits: datasource.Limits{MaxRows: 2}}
			frames, err := ds.QueryInstant(context.Background(), q, lastHour)
			if err != nil {
				t.Fatal(err)
			}
			if got := fake.last(t).params.Get("limit"); got != "2" {
				t.Fatalf("limit = %q, want 2", got)
			}
			if got := frames[0].Meta != nil && frames[0].Meta.Truncated; got != tc.truncated {
				t.Fatalf("frame 0 truncated = %v, want %v (meta %+v)", got, tc.truncated, frames[0].Meta)
			}
			for _, f := range frames[1:] {
				if f.Meta != nil {
					t.Fatalf("a later frame carries meta: %+v", f.Meta)
				}
			}
		})
	}
}

func TestLogLineLimitFollowsTheRowLimit(t *testing.T) {
	cases := []struct {
		name    string
		maxRows int
		want    string
	}{
		{"default rows capped at Loki's own limit", 0, "5000"},
		{"fewer rows than Loki's limit", 120, "120"},
		{"exactly Loki's limit", 5000, "5000"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ds, fake := serve(t, 0, success("streams", "[]"))
			q := datasource.Query{Text: `{a="b"} |= "x"`, Limits: datasource.Limits{MaxRows: tc.maxRows}}
			if _, err := ds.QueryRange(context.Background(), q, lastHour, time.Minute); err != nil {
				t.Fatal(err)
			}
			req := fake.last(t)
			if got := req.params.Get("limit"); got != tc.want {
				t.Fatalf("limit = %q, want %q", got, tc.want)
			}
			if req.params.Has("step") {
				t.Fatalf("a log query sent a step: %v", req.params)
			}
		})
	}
}

func TestMetricQueries(t *testing.T) {
	matrix := success("matrix", `[
		{"metric":{"service_name":"frontend"},"values":[[1793962800,"1"],[1793962860,"NaN"]]},
		{"metric":{"service_name":"checkout"},"values":[[1793962800.5,"2"]]}
	]`)
	ds, fake := serve(t, 0, matrix)
	query := `sum by (service_name) (count_over_time({service_name=~".+"}[1m]))`
	frames, err := ds.QueryRange(context.Background(), datasource.Query{Text: query}, lastHour, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	req := fake.last(t)
	if req.path != "/loki/api/v1/query_range" || req.params.Get("step") != "30" || req.params.Get("start") != "2026-11-06T11:00:00Z" || req.params.Has("limit") || req.params.Has("direction") {
		t.Fatalf("request = %s %v", req.path, req.params)
	}
	if len(frames) != 2 || frames[0].Labels["service_name"] != "checkout" {
		t.Fatalf("frames = %+v", frames)
	}
	if got, want := fieldNames(frames[0]), []string{"time:Time", "number:Value"}; !slices.Equal(got, want) {
		t.Fatalf("fields = %v, want %v", got, want)
	}
	if got := fieldValues(t, frames[0], "Time"); got[0] != time.UnixMilli(1793962800500).UTC() {
		t.Fatalf("times = %v", got)
	}
	if got := fieldValues(t, frames[1], "Value"); !slices.Equal(got, []any{1.0, nil}) {
		t.Fatalf("values = %v, want NaN as a gap", got)
	}

	ds, fake = serve(t, 0, success("vector", `[{"metric":{"service_name":"checkout"},"value":[1793966400,"7"]}]`))
	frames, err = ds.QueryInstant(context.Background(), datasource.Query{Text: query}, lastHour)
	if err != nil {
		t.Fatal(err)
	}
	req = fake.last(t)
	if req.path != "/loki/api/v1/query" || req.params.Get("time") != "2026-11-06T12:00:00Z" || req.params.Has("start") {
		t.Fatalf("instant request = %s %v", req.path, req.params)
	}
	if len(frames) != 1 || !slices.Equal(fieldValues(t, frames[0], "Value"), []any{7.0}) {
		t.Fatalf("frames = %+v", frames)
	}
}

func matrixSeries(id, samples int) string {
	values := make([]string, samples)
	for i := range values {
		values[i] = fmt.Sprintf(`[%d,"1"]`, 1793962800+60*i)
	}
	return fmt.Sprintf(`{"metric":{"a":"%d"},"values":[%s]}`, id, strings.Join(values, ","))
}

func TestLimitsAreEnforced(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		query  string
		limits datasource.Limits
		step   time.Duration
		want   string
	}{
		{"too many series", success("matrix", `[{"metric":{"a":"1"},"values":[]},{"metric":{"a":"2"},"values":[]}]`), `rate({a="b"}[1m])`, datasource.Limits{MaxSeries: 1}, time.Minute, "more than 1 series"},
		{"too many points", success("matrix", "["+matrixSeries(1, 40)+","+matrixSeries(2, 40)+"]"), `rate({a="b"}[1m])`, datasource.Limits{MaxPoints: 70}, time.Minute, "more than 70 points"},
		{"step finer than the resolution limit", success("matrix", "[]"), `rate({a="b"}[1m])`, datasource.Limits{}, 100 * time.Millisecond, "max 11000"},
		{"step that exceeds the points per series", success("matrix", "[]"), `rate({a="b"}[1m])`, datasource.Limits{MaxPoints: 60}, time.Minute, "61 points per series"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ds, _ := serve(t, 0, tc.body)
			_, err := ds.QueryRange(context.Background(), datasource.Query{Text: tc.query, Limits: tc.limits}, lastHour, tc.step)
			if !errors.Is(err, datasource.ErrLimitExceeded) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want a limit error mentioning %q", err, tc.want)
			}
		})
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
		{"parse error", 400, "parse error at line 1, col 15: syntax error: unexpected $end, expecting STRING", datasource.ErrInvalidQuery, "parse error at line 1, col 15"},
		{"entries limit", 400, "max entries limit per query exceeded, limit > max_entries_limit_per_query (100000 > 5000)", datasource.ErrLimitExceeded, "max entries limit"},
		{"json error body", 400, `{"code":400,"status":"error","message":"queries require at least one regexp or equality matcher"}`, datasource.ErrInvalidQuery, "queries require at least one"},
		{"unauthorized", 401, "no org id", datasource.ErrAuthFailed, "authentication failed (HTTP 401)"},
		{"rate limited", 429, "too many outstanding requests", datasource.ErrUnavailable, "rate limited by upstream"},
		{"gateway timeout", 504, "", datasource.ErrTimeout, "query timed out"},
		{"server error naming an address", 500, "rpc error: dial tcp 10.0.0.7:9095: connection refused", datasource.ErrUnavailable, "upstream returned HTTP 500"},
		{"unreadable success", 200, "not json", datasource.ErrUnavailable, "unreadable result"},
		{"unknown result type", 200, success("scalar", `[1,"1"]`), datasource.ErrUnavailable, `unsupported result type "scalar"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ds, _ := serve(t, tc.status, tc.body)
			_, err := ds.QueryInstant(context.Background(), datasource.Query{Text: `count_over_time({a="b"}[1m])`}, lastHour)
			if !errors.Is(err, tc.kind) || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("err = %v, want %v mentioning %q", err, tc.kind, tc.message)
			}
			if strings.Contains(err.Error(), "10.0.0.7") {
				t.Fatalf("error names an address: %v", err)
			}
		})
	}
}

func TestRequestsCarryAuthHeadersAndTenant(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*loki.Options)
		check  func(http.Header) bool
	}{
		{"bearer token", func(o *loki.Options) { o.BearerToken = datasource.NewSecret("tok") }, func(h http.Header) bool { return h.Get("Authorization") == "Bearer tok" }},
		{"basic auth", func(o *loki.Options) { o.BasicAuthUser, o.BasicAuthPassword = "u", datasource.NewSecret("p") }, func(h http.Header) bool { return h.Get("Authorization") == "Basic dTpw" }},
		{"tenant", func(o *loki.Options) { o.TenantID = "team-a" }, func(h http.Header) bool { return h.Get("X-Scope-OrgID") == "team-a" }},
		{"plain header", func(o *loki.Options) { o.Headers = map[string]string{"X-Extra": "v"} }, func(h http.Header) bool { return h.Get("X-Extra") == "v" && h.Get("Accept") == "application/json" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ds, fake := serve(t, 0, "ready", tc.mutate)
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
	ds, _ := serve(t, http.StatusServiceUnavailable, "Ingester not ready: waiting for 15s after being ready")
	if err := ds.CheckHealth(context.Background()); !errors.Is(err, datasource.ErrUnavailable) {
		t.Fatalf("err = %v, want unavailable", err)
	}
}

func TestNewRefusesInvalidOptions(t *testing.T) {
	cases := []struct {
		name string
		opts loki.Options
		want string
	}{
		{"scheme", loki.Options{URL: "ftp://loki"}, "url scheme must be http or https"},
		{"user info", loki.Options{URL: "http://reader@loki"}, "url must not contain user info"},
		{"query", loki.Options{URL: "http://loki?x=1"}, "url must not contain a query or fragment"},
		{"authorization header", loki.Options{URL: "http://loki", Headers: map[string]string{"authorization": "x"}}, "Authorization header is set through basic auth"},
		{"tenant header", loki.Options{URL: "http://loki", Headers: map[string]string{"x-scope-orgid": "x"}}, "X-Scope-OrgID header is set through tenantId"},
		{"managed header", loki.Options{URL: "http://loki", Headers: map[string]string{"accept": "x"}}, "Accept header is managed"},
		{"tenant with a newline", loki.Options{URL: "http://loki", TenantID: "a\nb"}, "invalid tenant id"},
		{"tenant with spaces around it", loki.Options{URL: "http://loki", TenantID: " a"}, "invalid tenant id"},
		{"basic and bearer", loki.Options{URL: "http://loki", BasicAuthUser: "u", BearerToken: datasource.NewSecret("t")}, "mutually exclusive"},
		{"password without user", loki.Options{URL: "http://loki", BasicAuthPassword: datasource.NewSecret("p")}, "requires a user"},
		{"negative response cap", loki.Options{URL: "http://loki", MaxResponseBytes: -1}, "must not be negative"},
		{"negative limit", loki.Options{URL: "http://loki", Limits: datasource.Limits{MaxRows: -1}}, "maxRows must be positive"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := loki.New(tc.opts)
			var cfg *datasource.ConfigError
			if !errors.As(err, &cfg) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want a config error mentioning %q", err, tc.want)
			}
		})
	}
}

func TestRedirectsAndLargeResponsesAreRefused(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(success("streams", "[]")))
	}))
	t.Cleanup(target.Close)
	redirect := httptest.NewServer(http.RedirectHandler(target.URL+"/loki/api/v1/query_range", http.StatusFound))
	t.Cleanup(redirect.Close)
	ds, err := loki.New(loki.Options{URL: redirect.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ds.QueryInstant(context.Background(), datasource.Query{Text: `{a="b"}`}, lastHour); !errors.Is(err, datasource.ErrUnavailable) {
		t.Fatalf("redirect err = %v, want unavailable", err)
	}

	big, _ := serve(t, 0, twoStreams, func(o *loki.Options) { o.MaxResponseBytes = 64 })
	if _, err := big.QueryInstant(context.Background(), datasource.Query{Text: `{a="b"}`}, lastHour); !errors.Is(err, datasource.ErrLimitExceeded) || !strings.Contains(err.Error(), "larger than 64 bytes") {
		t.Fatalf("large response err = %v", err)
	}
}

func TestVariablesAreEscapedForLogQL(t *testing.T) {
	cases := []struct {
		name  string
		query string
		vars  []datasource.Variable
		want  string
	}{
		{"quote in an equality matcher", `{service_name="$svc"}`, []datasource.Variable{{Name: "svc", Values: []string{`a"b`}}}, `{service_name="a\"b"}`},
		{"several values in a regex matcher", `{service_name=~"$svc"}`, []datasource.Variable{{Name: "svc", Values: []string{"a.b", "c"}}}, `{service_name=~"(a\\.b|c)"}`},
		{"all in a regex matcher", `{service_name=~"$svc"}`, []datasource.Variable{{Name: "svc", All: true}}, `{service_name=~".*"}`},
		{"line filter", `{a="b"} |= "${needle}"`, []datasource.Variable{{Name: "needle", Values: []string{`x\y`}}}, `{a="b"} |= "x\\y"`},
		{"range and interval macros", `count_over_time({a="b"}[$__range]) or count_over_time({a="b"}[$__interval])`, nil, `count_over_time({a="b"}[3600s]) or count_over_time({a="b"}[3s])`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ds, fake := serve(t, 0, success("streams", "[]"))
			if _, err := ds.QueryInstant(context.Background(), datasource.Query{Text: tc.query, Variables: tc.vars}, lastHour); err != nil {
				t.Fatal(err)
			}
			if got := fake.last(t).params.Get("query"); got != tc.want {
				t.Fatalf("query = %s, want %s", got, tc.want)
			}
		})
	}
	ds, _ := serve(t, 0, success("streams", "[]"))
	_, err := ds.QueryInstant(context.Background(), datasource.Query{Text: `{a=$v}`, Variables: []datasource.Variable{{Name: "v", Values: []string{`"x"} or {b="`}}}}, lastHour)
	if !errors.Is(err, datasource.ErrInvalidQuery) {
		t.Fatalf("a value that is not an identifier in code: err = %v", err)
	}
}

func TestDiscover(t *testing.T) {
	labels := `{"status":"success","data":["service_name","level","detected_level","pod"]}`
	ds, fake := serve(t, 0, labels)
	ctx := context.Background()
	got, err := ds.Discover(ctx, datasource.DiscoverRequest{Kind: datasource.DiscoverLabels, Search: "LEVEL", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.Labels, []string{"detected_level"}) || fake.last(t).path != "/loki/api/v1/labels" {
		t.Fatalf("labels = %v from %s", got.Labels, fake.last(t).path)
	}

	ds, fake = serve(t, 0, `{"status":"success","data":["payments","checkout","checkout"]}`)
	got, err = ds.Discover(ctx, datasource.DiscoverRequest{Kind: datasource.DiscoverLabelValues, Label: "service_name", Metric: `{level="error"}`})
	if err != nil {
		t.Fatal(err)
	}
	req := fake.last(t)
	if !slices.Equal(got.Values, []string{"checkout", "payments"}) || req.path != "/loki/api/v1/label/service_name/values" || req.params.Get("query") != `{level="error"}` {
		t.Fatalf("values = %v from %s %v", got.Values, req.path, req.params)
	}

	got, err = ds.Discover(ctx, datasource.DiscoverRequest{Kind: datasource.DiscoverMetrics})
	if err != nil || len(got.Metrics) != 0 {
		t.Fatalf("metrics = %+v, %v; want an empty catalog", got, err)
	}
	for _, req := range []datasource.DiscoverRequest{
		{Kind: datasource.DiscoverTables},
		{Kind: datasource.DiscoverLabelValues, Label: "bad-label"},
	} {
		if _, err := ds.Discover(ctx, req); !errors.Is(err, datasource.ErrInvalidQuery) {
			t.Errorf("%+v: err = %v, want invalid query", req, err)
		}
	}
	if _, err := ds.Discover(ctx, datasource.DiscoverRequest{Kind: datasource.DiscoverColumns}); err == nil || !strings.Contains(err.Error(), "not supported by loki") {
		t.Fatalf("columns err = %v", err)
	}
}

func TestVariableValues(t *testing.T) {
	cases := []struct {
		name     string
		query    string
		path     string
		selector string
	}{
		{"label only", "label_values(service_name)", "/loki/api/v1/label/service_name/values", ""},
		{"selector and label", `label_values({namespace="shop", pod=~"a,b"}, service_name)`, "/loki/api/v1/label/service_name/values", `{namespace="shop", pod=~"a,b"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ds, fake := serve(t, 0, `{"status":"success","data":["b","a","b"]}`)
			got, err := ds.VariableValues(context.Background(), datasource.Query{Text: tc.query}, lastHour)
			if err != nil {
				t.Fatal(err)
			}
			req := fake.last(t)
			if !slices.Equal(got, []string{"a", "b"}) || req.path != tc.path || req.params.Get("query") != tc.selector || req.params.Get("start") != "2026-11-06T11:00:00Z" {
				t.Fatalf("values = %v from %s %v", got, req.path, req.params)
			}
		})
	}
	ds, _ := serve(t, 0, `{"status":"success","data":[]}`)
	for _, query := range []string{`{a="b"}`, "label_values()", "label_values(bad-label)"} {
		if _, err := ds.VariableValues(context.Background(), datasource.Query{Text: query}, lastHour); !errors.Is(err, datasource.ErrInvalidQuery) {
			t.Errorf("%s: err = %v, want invalid query", query, err)
		}
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
		{"stream selector", `{service_name="checkout"}`, "", 0, 0, ""},
		{"pipeline with parsers and label filters", `{app="a"} |= "err" != "debug" |~ "x|y" | json | level="error" or level="warn" | line_format "{{.msg | ToUpper}}" | duration > 10s | __error__=""`, "", 0, 0, ""},
		{"metric query", `sum by (pod) (rate({app="a"} | logfmt | unwrap bytes [5m]))`, "", 0, 0, ""},
		{"comment", "{app=\"a\"} # tail\n|= \"x\"", "", 0, 0, ""},
		{"no stream selector", `rate(foo[5m])`, "a LogQL query needs a stream selector", 0, 13, datasource.SeverityError},
		{"unclosed selector", `{app="a"`, `unclosed "{"`, 0, 1, datasource.SeverityError},
		{"unterminated string", `{app="a}`, "unterminated string at offset 5", 0, 0, datasource.SeverityError},
		{"mismatched bracket", `rate({app="a"}[5m)`, `unexpected ")"`, 17, 18, datasource.SeverityError},
		{"unknown stage", `{app="a"} | jsn`, `unknown pipeline stage "jsn"`, 12, 15, datasource.SeverityError},
		{"empty stage", `{app="a"} |`, `expected a parser, formatter or label filter after "|"`, 10, 11, datasource.SeverityError},
		{"problem after a variable keeps original offsets", `{app="$app"} | jsn`, `unknown pipeline stage "jsn"`, 15, 18, datasource.SeverityError},
		{"unknown macro", `{app="a"} |= "$__nope"`, "unknown macro", 0, 0, datasource.SeverityError},
	}
	ds, err := loki.New(loki.Options{URL: "http://loki.invalid"})
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
