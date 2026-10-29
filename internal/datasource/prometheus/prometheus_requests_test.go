package prometheus_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/prometheus"
)

type recordedRequest struct {
	method string
	path   string
	form   url.Values
}

type recorder struct {
	mu          sync.Mutex
	requests    []recordedRequest
	idleClosers atomic.Int32
}

func (r *recorder) RoundTrip(req *http.Request) (*http.Response, error) {
	form := req.URL.Query()
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, err
		}
		raw, err := io.ReadAll(body)
		if err != nil {
			return nil, err
		}
		if parsed, err := url.ParseQuery(string(raw)); err == nil {
			for k, v := range parsed {
				form[k] = append(form[k], v...)
			}
		}
	}
	r.mu.Lock()
	r.requests = append(r.requests, recordedRequest{method: req.Method, path: req.URL.Path, form: form})
	r.mu.Unlock()
	return http.DefaultTransport.RoundTrip(req)
}

func (r *recorder) CloseIdleConnections() { r.idleClosers.Add(1) }

func (r *recorder) to(path string) []recordedRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []recordedRequest
	for _, q := range r.requests {
		if q.path == path {
			out = append(out, q)
		}
	}
	return out
}

func (r *recorder) all() []recordedRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recordedRequest(nil), r.requests...)
}

func recordingDS(t *testing.T, mutate ...func(*prometheus.Options)) (*prometheus.Datasource, *recorder) {
	t.Helper()
	rec := &recorder{}
	ds := newDS(t, append([]func(*prometheus.Options){func(o *prometheus.Options) {
		o.HTTPClient = &http.Client{Transport: rec}
	}}, mutate...)...)
	return ds, rec
}

func TestPre1970Range(t *testing.T) {
	ds, rec := recordingDS(t)
	ctx := context.Background()
	to := time.Date(1969, 12, 31, 23, 59, 58, 500_000_000, time.UTC)
	r := datasource.TimeRange{From: to.Add(-time.Hour), To: to}
	if _, err := ds.QueryInstant(ctx, datasource.Query{Text: "up"}, r); err != nil {
		t.Fatalf("QueryInstant: %v", err)
	}
	if _, err := ds.QueryRange(ctx, datasource.Query{Text: "up"}, r, time.Minute); err != nil {
		t.Fatalf("QueryRange: %v", err)
	}
	if got := rec.to("/api/v1/query")[0].form.Get("time"); got != "-1.500" {
		t.Errorf("instant time = %q, want -1.500", got)
	}
	rng := rec.to("/api/v1/query_range")[0].form
	if rng.Get("start") != "-3601.500" || rng.Get("end") != "-1.500" {
		t.Errorf("range start/end = %q/%q, want -3601.500/-1.500", rng.Get("start"), rng.Get("end"))
	}
	epoch := datasource.TimeRange{From: time.Unix(-1, 0), To: time.Unix(1, 0)}
	if _, err := ds.QueryInstant(ctx, datasource.Query{Text: "up"}, epoch); err != nil {
		t.Fatalf("QueryInstant across epoch: %v", err)
	}
	if got := rec.to("/api/v1/query")[1].form.Get("time"); got != "1.000" {
		t.Errorf("epoch time = %q, want 1.000", got)
	}
}

func TestTimeRangeBounds(t *testing.T) {
	ds, rec := recordingDS(t)
	ctx := context.Background()
	ranges := map[string]datasource.TimeRange{
		"year 0 from":    {From: time.Date(0, 1, 1, 0, 0, 0, 0, time.UTC), To: time.Now()},
		"year 10000 to":  {From: time.Now(), To: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)},
		"max time to":    {From: time.Now(), To: time.Unix(math.MaxInt64/1000, 0)},
		"zero from":      {To: time.Now()},
		"negative range": {From: time.Now(), To: time.Now().Add(-time.Hour)},
	}
	for name, r := range ranges {
		t.Run(name, func(t *testing.T) {
			if _, err := ds.QueryInstant(ctx, datasource.Query{Text: "up"}, r); !errors.Is(err, datasource.ErrInvalidQuery) {
				t.Errorf("QueryInstant: want ErrInvalidQuery, got %v", err)
			}
			if _, err := ds.QueryRange(ctx, datasource.Query{Text: "up"}, r, time.Minute); !errors.Is(err, datasource.ErrInvalidQuery) {
				t.Errorf("QueryRange: want ErrInvalidQuery, got %v", err)
			}
			if _, err := ds.VariableValues(ctx, datasource.Query{Text: "label_values(job)"}, r); !errors.Is(err, datasource.ErrInvalidQuery) {
				t.Errorf("VariableValues: want ErrInvalidQuery, got %v", err)
			}
		})
	}
	if n := len(rec.all()); n != 0 {
		t.Errorf("%d requests sent for invalid ranges", n)
	}
}

func TestNewRejectsUnsafeConfig(t *testing.T) {
	base := func(mutate func(*prometheus.Options)) error {
		opts := prometheus.Options{URL: "http://127.0.0.1:9090"}
		mutate(&opts)
		_, err := prometheus.New(opts)
		return err
	}
	rejected := map[string]func(*prometheus.Options){
		"content-type":   func(o *prometheus.Options) { o.Headers = map[string]string{"Content-Type": "text/plain"} },
		"accept lower":   func(o *prometheus.Options) { o.Headers = map[string]string{"accept": "text/html"} },
		"host":           func(o *prometheus.Options) { o.Headers = map[string]string{"HOST": "evil"} },
		"content-length": func(o *prometheus.Options) { o.Headers = map[string]string{"content-length": "1"} },
		"authorization":  func(o *prometheus.Options) { o.Headers = map[string]string{"authorization": "x"} },
		"authorization basic": func(o *prometheus.Options) {
			o.Headers = map[string]string{"Authorization": "x"}
			o.BasicAuthUser = "u"
		},
		"space in name":        func(o *prometheus.Options) { o.Headers = map[string]string{"X Y": "v"} },
		"colon in name":        func(o *prometheus.Options) { o.Headers = map[string]string{"X:Y": "v"} },
		"non-ascii name":       func(o *prometheus.Options) { o.Headers = map[string]string{"X-Ünï": "v"} },
		"nul in value":         func(o *prometheus.Options) { o.Headers = map[string]string{"X-A": "a\x00b"} },
		"del in value":         func(o *prometheus.Options) { o.Headers = map[string]string{"X-A": "a\x7fb"} },
		"newline in value":     func(o *prometheus.Options) { o.Headers = map[string]string{"X-A": "a\nb"} },
		"max response int64":   func(o *prometheus.Options) { o.MaxResponseBytes = math.MaxInt64 },
		"max response 1GiB+1":  func(o *prometheus.Options) { o.MaxResponseBytes = 1<<30 + 1 },
		"negative max":         func(o *prometheus.Options) { o.MaxResponseBytes = -1 },
		"empty header name":    func(o *prometheus.Options) { o.Headers = map[string]string{"": "v"} },
		"tab-only name":        func(o *prometheus.Options) { o.Headers = map[string]string{"\t": "v"} },
		"carriage return name": func(o *prometheus.Options) { o.Headers = map[string]string{"X-A\r": "v"} },
	}
	for name, mutate := range rejected {
		t.Run(name, func(t *testing.T) {
			if err := base(mutate); err == nil {
				t.Fatal("want error")
			}
		})
	}
	accepted := map[string]func(*prometheus.Options){
		"mimir tenant": func(o *prometheus.Options) { o.Headers = map[string]string{"X-Scope-OrgID": "tenant a"} },
		"1GiB":         func(o *prometheus.Options) { o.MaxResponseBytes = 1 << 30 },
		"tab in value": func(o *prometheus.Options) { o.Headers = map[string]string{"X-A": "a\tb"} },
	}
	for name, mutate := range accepted {
		t.Run(name, func(t *testing.T) {
			if err := base(mutate); err != nil {
				t.Fatalf("want ok, got %v", err)
			}
		})
	}
}

func TestRedirectsAreRefused(t *testing.T) {
	var leaked atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked.Add(1)
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", target.URL+"/api/v1/query")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer source.Close()

	callerClient := &http.Client{Transport: http.DefaultTransport.(*http.Transport).Clone()}
	ds, err := prometheus.New(prometheus.Options{
		URL:         source.URL,
		HTTPClient:  callerClient,
		Headers:     map[string]string{"X-Scope-OrgID": "tenant"},
		BearerToken: datasource.NewSecret("secret-token"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := ds.CheckHealth(context.Background()); !errors.Is(err, datasource.ErrUnavailable) {
		t.Fatalf("want ErrUnavailable, got %v", err)
	}
	if leaked.Load() != 0 {
		t.Fatalf("redirect target received %d requests", leaked.Load())
	}
	if callerClient.CheckRedirect != nil {
		t.Error("caller's client was mutated")
	}
}

func TestCloseLeavesCallerTransportAlone(t *testing.T) {
	rec := &recorder{}
	ds, err := prometheus.New(prometheus.Options{URL: "http://127.0.0.1:9090", HTTPClient: &http.Client{Transport: rec}})
	if err != nil {
		t.Fatal(err)
	}
	if err := ds.Close(); err != nil {
		t.Fatal(err)
	}
	if n := rec.idleClosers.Load(); n != 0 {
		t.Errorf("CloseIdleConnections called %d times on the caller's transport", n)
	}
}

func TestTimeoutMessage(t *testing.T) {
	ctx := context.Background()
	t.Run("adapter deadline fired", func(t *testing.T) {
		ds := newDS(t)
		_, err := ds.QueryInstant(ctx, datasource.Query{Text: "up", Limits: datasource.Limits{Timeout: time.Nanosecond}}, lastMinutes(1))
		if !errors.Is(err, datasource.ErrTimeout) || !strings.Contains(err.Error(), "after 1ns") {
			t.Fatalf("want ErrTimeout after 1ns, got %v", err)
		}
	})
	t.Run("parent deadline fired", func(t *testing.T) {
		ds := newDS(t)
		parent, cancel := context.WithTimeout(ctx, time.Nanosecond)
		defer cancel()
		<-parent.Done()
		for name, call := range map[string]func() error{
			"health": func() error { return ds.CheckHealth(parent) },
			"instant": func() error {
				_, err := ds.QueryInstant(parent, datasource.Query{Text: "up"}, lastMinutes(1))
				return err
			},
			"discover": func() error {
				_, err := ds.Discover(parent, datasource.DiscoverRequest{Kind: datasource.DiscoverLabels})
				return err
			},
		} {
			err := call()
			if !errors.Is(err, datasource.ErrTimeout) {
				t.Fatalf("%s: want ErrTimeout, got %v", name, err)
			}
			if strings.Contains(err.Error(), "after") {
				t.Errorf("%s: message %q reports the adapter's duration although the parent's deadline fired", name, err)
			}
		}
	})
}

func TestLintReportsAnUnavailableDatasourceAsAnError(t *testing.T) {
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(down.Close)
	ds, err := prometheus.New(prometheus.Options{URL: down.URL})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ds.Close() })

	diags, err := ds.Lint(context.Background(), "up")
	if !errors.Is(err, datasource.ErrUnavailable) || diags != nil {
		t.Fatalf("Lint = %+v, %v; want ErrUnavailable and no diagnostics", diags, err)
	}
	diags, err = ds.Lint(context.Background(), "sum(rate(")
	if err != nil || len(diags) != 1 || diags[0].Severity != datasource.SeverityError {
		t.Fatalf("Lint of a parse error = %+v, %v; want one error without asking the datasource", diags, err)
	}
}

func TestLintVariableInMatchers(t *testing.T) {
	ds := newDS(t)
	query := `rate(nope_total{job=~"$job"}[5m])`
	diags := lint(context.Background(), t, ds, query)
	if len(diags) != 1 || diags[0].Severity != datasource.SeverityError || !strings.Contains(diags[0].Message, "nope_total") {
		t.Fatalf("want one error naming the metric, got %+v", diags)
	}
	if got := query[diags[0].Start:diags[0].End]; got != "nope_total" {
		t.Errorf("diagnostic covers %q, want the metric name only", got)
	}
}

func TestLintQuotedMetricNames(t *testing.T) {
	ds := newDS(t)
	cases := []struct{ query, covers, metric string }{
		{`rate({"nope.x", job=~"$job"}[5m])`, `"nope.x"`, "nope.x"},
		{`{"nope_m", job="$job"}`, `"nope_m"`, "nope_m"},
		{`rate({"nope.y"}[5m])`, `"nope.y"`, "nope.y"},
		{`sum({ "nope.z" , job="a,}b"})`, `"nope.z"`, "nope.z"},
		{`{job="x", "nope.w"}`, `"nope.w"`, "nope.w"},
		{`{__name__="nope_n", job=~"$job"}`, `"nope_n"`, "nope_n"},
		{`{job=~"$job", "nope.a"}`, `"nope.a"`, "nope.a"},
		{`{"__name__"="nope_f", job=~"$job"}`, `"nope_f"`, "nope_f"},
		{`{job="x", "__name__" = 'nope_g'}`, `'nope_g'`, "nope_g"},
	}
	for _, c := range cases {
		t.Run(c.query, func(t *testing.T) {
			diags := lint(context.Background(), t, ds, c.query)
			if len(diags) != 1 || diags[0].Severity != datasource.SeverityError || !strings.Contains(diags[0].Message, c.metric) {
				t.Fatalf("want one error naming %s, got %+v", c.metric, diags)
			}
			if got := c.query[diags[0].Start:diags[0].End]; got != c.covers {
				t.Errorf("diagnostic covers %q, want %q", got, c.covers)
			}
		})
	}
}

func TestLintExistenceUsesOneRequest(t *testing.T) {
	ds, rec := recordingDS(t)
	query := `rate(nope_one_total[5m]) + rate(nope_two_total[5m]) + up + sum(prometheus_http_requests_total)`
	diags := lint(context.Background(), t, ds, query)
	if len(diags) != 2 {
		t.Fatalf("want 2 diagnostics, got %+v", diags)
	}
	for i, name := range []string{"nope_one_total", "nope_two_total"} {
		if !strings.Contains(diags[i].Message, name) || query[diags[i].Start:diags[i].End] != name {
			t.Errorf("diagnostic %d = %+v, want metric %s", i, diags[i], name)
		}
	}
	if n := len(rec.all()); n != 1 {
		t.Fatalf("lint sent %d requests, want 1", n)
	}
	if got := len(rec.all()[0].form["match[]"]); got != 4 {
		t.Errorf("match[] count = %d, want 4", got)
	}
}

func TestVariableValuesUsesQueryInterval(t *testing.T) {
	ds, rec := recordingDS(t)
	from := time.Now().Add(-5 * time.Hour)
	r := datasource.TimeRange{From: from, To: from.Add(5*time.Hour + 1500*time.Millisecond)}
	_, err := ds.VariableValues(context.Background(), datasource.Query{Text: `label_values(up{job!="$__interval"}, job)`}, r)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ds.QueryInstant(context.Background(), datasource.Query{Text: `up{job!="$__interval"}`}, r); err != nil {
		t.Fatal(err)
	}
	variable := rec.to("/api/v1/label/job/values")[0].form.Get("match[]")
	instant := rec.to("/api/v1/query")[0].form.Get("query")
	if want := `up{job!="18s"}`; variable != want || instant != want {
		t.Errorf("variable match[] = %q, instant query = %q, want both %q", variable, instant, want)
	}
}

func TestDiscoverMetricsDoesNotDownloadWholeMetadata(t *testing.T) {
	ds, rec := recordingDS(t)
	for _, search := range []string{"", "http_requests", requestsMetric} {
		cat, err := ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverMetrics, Search: search})
		if err != nil {
			t.Fatal(err)
		}
		if search == "" {
			continue
		}
		var found bool
		for _, m := range cat.Metrics {
			if m.Name == requestsMetric {
				found = true
				if m.Type != "counter" || m.Help == "" {
					t.Errorf("search %q: metadata missing for %s: %+v", search, m.Name, m)
				}
			}
		}
		if !found {
			t.Errorf("search %q: %s missing", search, requestsMetric)
		}
	}
	for _, q := range rec.to("/api/v1/metadata") {
		if q.form.Get("limit") == "" && q.form.Get("metric") == "" {
			t.Errorf("unbounded metadata request %+v", q)
		}
	}
	if len(rec.to("/api/v1/metadata")) == 0 {
		t.Error("no metadata request sent")
	}
}

type clampMetadataLimit struct{ next http.RoundTripper }

func (c clampMetadataLimit) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Path == "/api/v1/metadata" && req.URL.Query().Has("limit") {
		clone := req.Clone(req.Context())
		q := clone.URL.Query()
		q.Set("limit", "1")
		clone.URL.RawQuery = q.Encode()
		req = clone
	}
	return c.next.RoundTrip(req)
}

func TestDiscoverMetricsTargetsFirstMissingMetadata(t *testing.T) {
	rec := &recorder{}
	ds := newDS(t, func(o *prometheus.Options) {
		o.HTTPClient = &http.Client{Transport: clampMetadataLimit{next: rec}}
	})
	cat, err := ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverMetrics})
	if err != nil {
		t.Fatal(err)
	}
	if len(cat.Metrics) < 30 {
		t.Fatalf("want at least 30 metrics, got %d", len(cat.Metrics))
	}
	var targeted []string
	for _, q := range rec.to("/api/v1/metadata") {
		if m := q.form.Get("metric"); m != "" {
			targeted = append(targeted, m)
		}
	}
	if len(targeted) != 10 {
		t.Fatalf("want 10 targeted metadata requests, got %v", targeted)
	}
	first := map[string]bool{}
	for _, m := range cat.Metrics[:11] {
		first[m.Name] = true
	}
	for _, name := range targeted {
		if !first[name] {
			t.Errorf("targeted %q is not among the first missing names", name)
		}
	}
	typed := 0
	for _, m := range cat.Metrics {
		if m.Type != "" {
			typed++
		}
	}
	if typed < 5 || typed > 11 {
		t.Errorf("%d metrics carry metadata, want 5 to 11 (series such as up have none)", typed)
	}
}

func TestAdhocFiltersReachEverySelector(t *testing.T) {
	ds, rec := recordingDS(t)
	ctx := context.Background()
	now := time.Now()
	r := datasource.TimeRange{From: now.Add(-time.Hour), To: now}
	filters := []datasource.AdhocFilter{{Key: "job", Op: "=", Value: "api"}, {Key: "pod", Op: "!~", Value: "web-.*"}}
	text := `sum by (job) (rate(x[5m])) / on() group_left count(y)`
	if _, err := ds.QueryInstant(ctx, datasource.Query{Text: text, Filters: filters}, r); err != nil {
		t.Fatalf("QueryInstant: %v", err)
	}
	got := rec.to("/api/v1/query")[0].form.Get("query")
	for _, want := range []string{`x{job="api",pod!~"web-.*"}[5m]`, `y{job="api",pod!~"web-.*"}`} {
		if !strings.Contains(got, want) {
			t.Errorf("query = %q, want it to contain %q", got, want)
		}
	}
	if _, err := ds.QueryRange(ctx, datasource.Query{Text: "up", Filters: filters[:1]}, r, time.Minute); err != nil {
		t.Fatalf("QueryRange: %v", err)
	}
	if got := rec.to("/api/v1/query_range")[0].form.Get("query"); got != `up{job="api"}` {
		t.Errorf("range query = %q, want up{job=\"api\"}", got)
	}
}

func TestQueryWithoutFiltersIsSentUnchanged(t *testing.T) {
	ds, rec := recordingDS(t)
	now := time.Now()
	r := datasource.TimeRange{From: now.Add(-time.Hour), To: now}
	if _, err := ds.QueryInstant(context.Background(), datasource.Query{Text: `up{job="a"}   `}, r); err != nil {
		t.Fatal(err)
	}
	if got := rec.to("/api/v1/query")[0].form.Get("query"); got != `up{job="a"}   ` {
		t.Errorf("query = %q, want it unchanged", got)
	}
}

func TestAdhocFilterWithABadRegexIsInvalid(t *testing.T) {
	ds, rec := recordingDS(t)
	now := time.Now()
	r := datasource.TimeRange{From: now.Add(-time.Hour), To: now}
	filters := []datasource.AdhocFilter{{Key: "job", Op: "=~", Value: "a("}}
	_, err := ds.QueryInstant(context.Background(), datasource.Query{Text: "up", Filters: filters}, r)
	if !errors.Is(err, datasource.ErrInvalidQuery) {
		t.Fatalf("err = %v, want ErrInvalidQuery", err)
	}
	if n := len(rec.all()); n != 0 {
		t.Errorf("%d upstream requests, want none", n)
	}
}

func TestUpstreamErrorTextNeverNamesAnAddress(t *testing.T) {
	tests := []struct {
		name        string
		errorType   string
		text        string
		wantKind    error
		wantMessage string
	}{
		{name: "parse error is shown", errorType: "bad_data", text: `1:5: parse error: unexpected end of input`, wantKind: datasource.ErrInvalidQuery, wantMessage: `1:5: parse error: unexpected end of input`},
		{name: "timeout text is shown", errorType: "timeout", text: "query timed out in expression evaluation", wantKind: datasource.ErrTimeout, wantMessage: "query timed out in expression evaluation"},
		{name: "limit text is shown", errorType: "execution", text: "query processing would load too many samples into memory", wantKind: datasource.ErrLimitExceeded, wantMessage: "query processing would load too many samples into memory"},
		{name: "other errors without an address are shown", errorType: "internal", text: "storage is closing", wantKind: datasource.ErrUnavailable, wantMessage: "storage is closing"},
		{name: "address in a bad query error", errorType: "bad_data", text: "dial tcp 10.1.2.3:9095: bad", wantKind: datasource.ErrInvalidQuery, wantMessage: "query rejected by the datasource"},
		{name: "address in a timeout", errorType: "timeout", text: "store 10.1.2.3:10901 did not answer", wantKind: datasource.ErrTimeout, wantMessage: "query timed out"},
		{name: "address in a limit error", errorType: "execution", text: "store-gateway-0.mimir.svc:9095 exceeded the series limit", wantKind: datasource.ErrLimitExceeded, wantMessage: "query limit exceeded"},
		{name: "address in an internal error", errorType: "internal", text: "rpc error: dial tcp [fd00::5]:9095: connection refused", wantKind: datasource.ErrUnavailable, wantMessage: "upstream error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				body, _ := json.Marshal(map[string]string{"status": "error", "errorType": tt.errorType, "error": tt.text})
				_, _ = w.Write(body)
			}))
			t.Cleanup(server.Close)
			ds, err := prometheus.New(prometheus.Options{URL: server.URL})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = ds.Close() })

			_, err = ds.QueryInstant(context.Background(), datasource.Query{Text: "up"}, lastMinutes(1))
			if !errors.Is(err, tt.wantKind) {
				t.Fatalf("error = %v, want kind %v", err, tt.wantKind)
			}
			var ue *datasource.UpstreamError
			if !errors.As(err, &ue) || ue.Message != tt.wantMessage {
				t.Fatalf("error = %v, want message %q", err, tt.wantMessage)
			}
		})
	}
}
