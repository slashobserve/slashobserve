package proxyhttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	window       = "from=1760000000123&to=1760021600456"
	upstreamSlot = 32
)

func init() { gin.SetMode(gin.TestMode) }

type fakeSpecs struct {
	mu    sync.Mutex
	known map[specKey]*spec.Dashboard
	err   error
	calls int
}

func (f *fakeSpecs) VersionSpec(_ context.Context, id pgtype.UUID, version int) (*spec.Dashboard, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	d, ok := f.known[specKey{id: id, version: version}]
	if !ok {
		return nil, fmt.Errorf("%w: version %d", dashboards.ErrNotFound, version)
	}
	return d, nil
}

func (f *fakeSpecs) lookups() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type fakeDatasource struct {
	onRange    func(ctx context.Context, r datasource.TimeRange, step time.Duration) ([]datasource.Frame, error)
	onInstant  func(ctx context.Context, r datasource.TimeRange) ([]datasource.Frame, error)
	onVariable func(ctx context.Context, r datasource.TimeRange) ([]string, error)
	onLint     func(ctx context.Context, query string) []datasource.Diagnostic

	mu            sync.Mutex
	rangeCalls    int
	instantCalls  int
	variableCalls int
	lintCalls     int
	variableRange datasource.TimeRange
	rangeRange    datasource.TimeRange
}

func (f *fakeDatasource) counts() (ranges, instants, variables, lints int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.rangeCalls, f.instantCalls, f.variableCalls, f.lintCalls
}

func (f *fakeDatasource) Type() string                      { return string(spec.Prometheus) }
func (f *fakeDatasource) CheckHealth(context.Context) error { return nil }
func (f *fakeDatasource) Close() error                      { return nil }
func (f *fakeDatasource) Discover(context.Context, datasource.DiscoverRequest) (*datasource.Catalog, error) {
	return &datasource.Catalog{}, nil
}

func (f *fakeDatasource) Lint(ctx context.Context, query string) []datasource.Diagnostic {
	f.mu.Lock()
	f.lintCalls++
	f.mu.Unlock()
	if f.onLint != nil {
		return f.onLint(ctx, query)
	}
	return nil
}

func (f *fakeDatasource) VariableValues(ctx context.Context, _ datasource.Query, r datasource.TimeRange) ([]string, error) {
	f.mu.Lock()
	f.variableCalls++
	f.variableRange = r
	f.mu.Unlock()
	if f.onVariable != nil {
		return f.onVariable(ctx, r)
	}
	return []string{"api", "web"}, nil
}

func (f *fakeDatasource) QueryRange(ctx context.Context, _ datasource.Query, r datasource.TimeRange, step time.Duration) ([]datasource.Frame, error) {
	f.mu.Lock()
	f.rangeCalls++
	f.rangeRange = r
	f.mu.Unlock()
	if f.onRange != nil {
		return f.onRange(ctx, r, step)
	}
	return sampleFrames(), nil
}

func (f *fakeDatasource) QueryInstant(ctx context.Context, _ datasource.Query, r datasource.TimeRange) ([]datasource.Frame, error) {
	f.mu.Lock()
	f.instantCalls++
	f.mu.Unlock()
	if f.onInstant != nil {
		return f.onInstant(ctx, r)
	}
	return sampleFrames(), nil
}

func sampleFrames() []datasource.Frame {
	return []datasource.Frame{{
		Name:   "up",
		Labels: map[string]string{"pod": "a"},
		Fields: []datasource.Field{
			{Name: "Time", Type: datasource.FieldTime, Values: []any{time.UnixMilli(1760000000000).UTC()}},
			{Name: "Value", Type: datasource.FieldNumber, Values: []any{1.0}},
		},
	}}
}

func bigFrames(bytes int) []datasource.Frame {
	return []datasource.Frame{{Fields: []datasource.Field{{Name: "blob", Type: datasource.FieldString, Values: []any{strings.Repeat("x", bytes)}}}}}
}

func testDashboard() *spec.Dashboard {
	ref := spec.DatasourceRef{Type: spec.Prometheus, UID: "prom"}
	return &spec.Dashboard{
		SpecVersion: spec.Version,
		Title:       "test",
		TimeRange:   spec.TimeRange{From: "now-6h", To: "now"},
		Variables: []spec.Variable{
			{Name: "job", Type: spec.QueryVariable, Datasource: &ref, Query: "label_values(up, job)"},
			{Name: "env", Type: spec.CustomVariable, Options: []string{"prod", "dev"}},
		},
		Panels: []spec.Panel{
			{ID: "p_ts", Type: spec.Timeseries, Title: "ts", Datasource: ref, Query: `up{job="$job"}`},
			{ID: "p_tbl", Type: spec.Table, Title: "tbl", Datasource: ref, Query: `count(up{job="$job"})`},
		},
	}
}

type harness struct {
	t      *testing.T
	h      *Handler
	router *gin.Engine
	ds     *fakeDatasource
	specs  *fakeSpecs
	runner *queryproxy.Runner
	id     pgtype.UUID
}

func newHarness(t *testing.T, opts ...queryproxy.RunnerOption) *harness {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ds := &fakeDatasource{}
	registry, err := datasource.NewRegistry(logger, datasource.Instance{UID: "prom", Datasource: ds})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	id := pgtype.UUID{Bytes: [16]byte{9, 9, 9}, Valid: true}
	specs := &fakeSpecs{known: map[specKey]*spec.Dashboard{{id: id, version: 1}: testDashboard()}}
	runner := queryproxy.NewRunner(registry, logger, opts...)
	h := NewHandler(runner, specs, registry, logger)

	router := gin.New()
	api := router.Group("/api/v1")
	api.Use(func(c *gin.Context) {
		principal := c.GetHeader("X-Principal")
		if principal == "" {
			principal = "alice"
		}
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), auth.Principal{ID: principal}))
	})
	h.Register(api)
	return &harness{t: t, h: h, router: router, ds: ds, specs: specs, runner: runner, id: id}
}

func (hn *harness) dashboardPath(suffix string) string {
	return "/api/v1/dashboards/" + hn.id.String() + "/versions/1" + suffix
}

func (hn *harness) dataPath(panel, query string) string {
	return hn.dashboardPath("/panels/" + panel + "/data?" + query)
}

func (hn *harness) valuesPath(name, query string) string {
	return hn.dashboardPath("/variables/" + name + "/values?" + query)
}

func (hn *harness) serve(req *http.Request, principal string) *httptest.ResponseRecorder {
	if principal != "" {
		req.Header.Set("X-Principal", principal)
	}
	rec := httptest.NewRecorder()
	hn.router.ServeHTTP(rec, req)
	return rec
}

func (hn *harness) get(path, principal string) *httptest.ResponseRecorder {
	return hn.serve(httptest.NewRequest(http.MethodGet, path, nil), principal)
}

func (hn *harness) lint(uid, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/datasources/"+uid+"/lint", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return hn.serve(req, "")
}

func (hn *harness) heldSlots() int {
	hn.h.slots.mu.Lock()
	defer hn.h.slots.mu.Unlock()
	total := 0
	for _, n := range hn.h.slots.held {
		total += n
	}
	return total
}

type errorBody struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) errorBody {
	t.Helper()
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("error body %q: %v", rec.Body.String(), err)
	}
	return body
}

func expectError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d (body %s)", rec.Code, status, rec.Body.String())
	}
	if got := decodeError(t, rec).Code; got != code {
		t.Fatalf("code = %q, want %q (body %s)", got, code, rec.Body.String())
	}
}

func decodeData(t *testing.T, rec *httptest.ResponseRecorder) dataResponse {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	var body dataResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("data body: %v", err)
	}
	return body
}

func decodeValues(t *testing.T, rec *httptest.ResponseRecorder) valuesResponse {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	var body valuesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("values body: %v", err)
	}
	return body
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func holdPrincipalSlots(t *testing.T, hn *harness, principal string) {
	t.Helper()
	for range maxInFlightPerPrincipal {
		release, ok := hn.h.slots.acquire(principal)
		if !ok {
			t.Fatal("could not fill the principal's slots")
		}
		t.Cleanup(release)
	}
}

func TestForbiddenParameterIsRefusedBeforeAnythingElse(t *testing.T) {
	hn := newHarness(t)
	holdPrincipalSlots(t, hn, "alice")
	missingDashboard := "/api/v1/dashboards/" + pgtype.UUID{Bytes: [16]byte{1}, Valid: true}.String() + "/versions/7"

	for _, query := range []string{"query=up", "expr=up", "sql=select+1", "q=x", "query=%zz", "expr=%", "from=%zz&q=up", "%71=up", "window=1&" + window + "&query=up"} {
		paths := []string{
			missingDashboard + "/panels/p_ts/data?" + query,
			hn.dataPath("nope", query),
			hn.valuesPath("nope", query),
			hn.valuesPath("job", query),
		}
		for _, path := range paths {
			expectError(t, hn.get(path, "alice"), http.StatusForbidden, "query_not_in_spec")
		}
	}
	if hn.specs.lookups() != 0 {
		t.Fatalf("%d dashboard lookups before the 403", hn.specs.lookups())
	}
	if r, i, v, _ := hn.ds.counts(); r+i+v != 0 {
		t.Fatalf("upstream was called %d times", r+i+v)
	}

	rec := hn.get(hn.dataPath("p_ts", window), "alice")
	expectError(t, rec, http.StatusTooManyRequests, "too_many_requests")
	if got := rec.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q, want 1", got)
	}
}

func TestMissingDashboardAndPanelAreNotFound(t *testing.T) {
	hn := newHarness(t)
	missing := "/api/v1/dashboards/" + pgtype.UUID{Bytes: [16]byte{1}, Valid: true}.String() + "/versions/1"
	expectError(t, hn.get(missing+"/panels/p_ts/data", ""), http.StatusNotFound, "not_found")
	expectError(t, hn.get(hn.dashboardPath("/panels/nope/data"), ""), http.StatusNotFound, "panel_not_found")
	expectError(t, hn.get(hn.dashboardPath("/variables/nope/values"), ""), http.StatusBadRequest, "invalid_request")
	expectError(t, hn.get(hn.dashboardPath("/panels/p_ts/data?var-nope=1"), ""), http.StatusBadRequest, "invalid_request")
	expectError(t, hn.get("/api/v1/dashboards/not-a-uuid/versions/1/panels/p_ts/data", ""), http.StatusBadRequest, "invalid_request")
	expectError(t, hn.get("/api/v1/dashboards/"+hn.id.String()+"/versions/0/panels/p_ts/data", ""), http.StatusBadRequest, "invalid_request")
}

func TestMalformedQueryStringIsABadRequest(t *testing.T) {
	hn := newHarness(t)
	for _, path := range []string{
		hn.dataPath("p_ts", "from=%zz"),
		hn.dataPath("p_ts", "var-job=%zz"),
		hn.dataPath("p_ts", "max_data_points=%1"),
		hn.valuesPath("job", "to=%zz"),
	} {
		expectError(t, hn.get(path, ""), http.StatusBadRequest, "invalid_request")
	}
	if hn.specs.lookups() != 0 {
		t.Fatal("a malformed request reached the dashboard store")
	}
}

func TestUnknownAndRepeatedParametersAreBadRequests(t *testing.T) {
	hn := newHarness(t)
	for _, path := range []string{
		hn.dataPath("p_ts", "whatever=1"),
		hn.dataPath("p_ts", "from=now-1h&from=now-2h"),
		hn.dataPath("p_ts", "max_data_points=9"),
		hn.valuesPath("job", "max_data_points=100"),
		hn.dataPath("p_ts", "from=now-401d"),
	} {
		expectError(t, hn.get(path, ""), http.StatusBadRequest, "invalid_request")
	}
}

func TestRangePanelDataIsAlignedAndCachedWithItsOriginalExecutionTime(t *testing.T) {
	hn := newHarness(t)
	path := hn.dataPath("p_ts", window+"&var-job=api")

	first := decodeData(t, hn.get(path, ""))
	if first.Meta.Cached {
		t.Fatal("the first response claims to be cached")
	}
	if first.Meta.Kind != queryproxy.KindRange || first.Meta.StepMS != 22000 {
		t.Fatalf("meta = %+v, want a range panel with a 22s step", first.Meta)
	}
	from, err := time.Parse(time.RFC3339Nano, first.Meta.From)
	if err != nil || from.UnixMilli()%22000 != 0 {
		t.Fatalf("meta.from = %q (%v) is not aligned to the step", first.Meta.From, err)
	}
	to, err := time.Parse(time.RFC3339Nano, first.Meta.To)
	if err != nil || to.UnixMilli()%22000 != 0 {
		t.Fatalf("meta.to = %q (%v) is not aligned to the step", first.Meta.To, err)
	}
	if want := `[{"name":"up","labels":{"pod":"a"},"fields":[{"name":"Time","type":"time","values":[1760000000000]},{"name":"Value","type":"number","values":[1]}]}]`; string(first.Frames) != want {
		t.Fatalf("frames = %s", first.Frames)
	}

	second := decodeData(t, hn.get(path, ""))
	if !second.Meta.Cached {
		t.Fatal("the repeated response is not cached")
	}
	if second.Meta.ExecutedAt != first.Meta.ExecutedAt {
		t.Fatalf("executed_at changed from %s to %s on a cache hit", first.Meta.ExecutedAt, second.Meta.ExecutedAt)
	}
	if string(second.Frames) != string(first.Frames) || second.Meta.From != first.Meta.From || second.Meta.To != first.Meta.To || second.Meta.StepMS != first.Meta.StepMS {
		t.Fatalf("cached response differs: %+v vs %+v", second, first)
	}
	if ranges, _, _, _ := hn.ds.counts(); ranges != 1 {
		t.Fatalf("%d range queries, want 1", ranges)
	}
	if hn.specs.lookups() != 1 {
		t.Fatalf("%d dashboard lookups, want 1 (the spec cache)", hn.specs.lookups())
	}

	other := decodeData(t, hn.get(hn.dataPath("p_ts", window+"&var-job=web"), ""))
	if other.Meta.Cached {
		t.Fatal("a different variable selection was served from the cache")
	}
}

func TestInstantPanelAlignsToFiveSeconds(t *testing.T) {
	hn := newHarness(t)
	body := decodeData(t, hn.get(hn.dataPath("p_tbl", window), ""))
	if body.Meta.Kind != queryproxy.KindInstant || body.Meta.StepMS != 0 {
		t.Fatalf("meta = %+v", body.Meta)
	}
	to, err := time.Parse(time.RFC3339Nano, body.Meta.To)
	if err != nil || to.UnixMilli()%5000 != 0 {
		t.Fatalf("meta.to = %q is not aligned to 5s", body.Meta.To)
	}
}

func TestConcurrentIdenticalRequestsMakeOneUpstreamCall(t *testing.T) {
	const requests = 20
	hn := newHarness(t)
	entered := make(chan struct{}, requests)
	release := make(chan struct{})
	hn.ds.onRange = func(ctx context.Context, _ datasource.TimeRange, _ time.Duration) ([]datasource.Frame, error) {
		entered <- struct{}{}
		select {
		case <-release:
			return sampleFrames(), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	path := hn.dataPath("p_ts", window+"&var-job=api")
	recorders := make([]*httptest.ResponseRecorder, requests)
	var wg sync.WaitGroup
	for i := range requests {
		wg.Go(func() { recorders[i] = hn.get(path, fmt.Sprintf("user-%d", i)) })
	}
	<-entered
	waitUntil(t, "every request to be in flight", func() bool { return hn.heldSlots() == requests })
	close(release)
	wg.Wait()

	if ranges, _, _, _ := hn.ds.counts(); ranges != 1 {
		t.Fatalf("%d upstream range queries for %d identical requests, want 1", ranges, requests)
	}
	fresh := 0
	var frames string
	for i, rec := range recorders {
		body := decodeData(t, rec)
		if !body.Meta.Cached {
			fresh++
		}
		if i == 0 {
			frames = string(body.Frames)
		} else if string(body.Frames) != frames {
			t.Fatalf("request %d got different frames", i)
		}
	}
	if fresh == 0 {
		t.Fatal("no request reported a fresh execution")
	}
}

func TestNinthConcurrentRequestIsRefusedWithRetryAfter(t *testing.T) {
	hn := newHarness(t)
	release := make(chan struct{})
	hn.ds.onRange = func(ctx context.Context, _ datasource.TimeRange, _ time.Duration) ([]datasource.Frame, error) {
		select {
		case <-release:
			return sampleFrames(), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	recorders := make([]*httptest.ResponseRecorder, maxInFlightPerPrincipal)
	var wg sync.WaitGroup
	for i := range maxInFlightPerPrincipal {
		wg.Go(func() {
			recorders[i] = hn.get(hn.dataPath("p_ts", fmt.Sprintf("%s&var-job=api&max_data_points=%d", window, 100+i)), "alice")
		})
	}
	waitUntil(t, "eight upstream calls", func() bool { r, _, _, _ := hn.ds.counts(); return r == maxInFlightPerPrincipal })

	rec := hn.get(hn.dataPath("p_ts", window+"&var-job=api&max_data_points=500"), "alice")
	expectError(t, rec, http.StatusTooManyRequests, "too_many_requests")
	if got := rec.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q, want 1", got)
	}
	expectError(t, hn.get(hn.valuesPath("job", window), "alice"), http.StatusTooManyRequests, "too_many_requests")
	expectError(t, hn.lintAs("alice", "prom", `{"query":"up"}`), http.StatusTooManyRequests, "too_many_requests")

	close(release)
	wg.Wait()
	for i, rec := range recorders {
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: status %d", i, rec.Code)
		}
	}
	if held := hn.heldSlots(); held != 0 {
		t.Fatalf("%d slots still held", held)
	}
	if rec := hn.get(hn.dataPath("p_ts", window+"&var-job=api"), "alice"); rec.Code != http.StatusOK {
		t.Fatalf("a freed principal was refused: %d", rec.Code)
	}
}

func (hn *harness) lintAs(principal, uid, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/datasources/"+uid+"/lint", strings.NewReader(body))
	return hn.serve(req, principal)
}

func TestOversizedResponseIsRefusedAndNeverCached(t *testing.T) {
	hn := newHarness(t)
	hn.ds.onRange = func(context.Context, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
		return bigFrames(maxResponseBytes + 1), nil
	}
	rec := hn.get(hn.dataPath("p_ts", window+"&var-job=api"), "")
	expectError(t, rec, http.StatusUnprocessableEntity, "limit_exceeded")
	if msg := decodeError(t, rec).Error; msg != "response too large" {
		t.Fatalf("message = %q", msg)
	}
	hn.get(hn.dataPath("p_ts", window+"&var-job=api"), "")
	if ranges, _, _, _ := hn.ds.counts(); ranges != 2 {
		t.Fatalf("%d range queries, want 2 (a refused response is not cached)", ranges)
	}
}

func TestEntryOverTheCacheCapIsServedButNotCached(t *testing.T) {
	hn := newHarness(t)
	hn.ds.onRange = func(context.Context, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
		return bigFrames(resultMaxEntry + 1), nil
	}
	path := hn.dataPath("p_ts", window+"&var-job=api")
	for range 2 {
		if body := decodeData(t, hn.get(path, "")); body.Meta.Cached {
			t.Fatal("an entry over the per-entry cap was cached")
		}
	}
	if ranges, _, _, _ := hn.ds.counts(); ranges != 2 {
		t.Fatalf("%d range queries, want 2", ranges)
	}
}

func TestErrorBodiesCarryNoAddressesOrSecrets(t *testing.T) {
	secrets := []string{"10.9.8.7", "prometheus.internal", "9090", "postgres://", "hunter2", "goroutine", ".go:"}
	leak := errors.New("dial tcp 10.9.8.7:9090: connect: connection refused (http://prometheus.internal:9090/api/v1/query_range)")

	type scenario struct {
		name   string
		setup  func(hn *harness)
		do     func(hn *harness) *httptest.ResponseRecorder
		status int
		code   string
	}
	rangeRequest := func(hn *harness) *httptest.ResponseRecorder {
		return hn.get(hn.dataPath("p_ts", window+"&var-job=api"), "")
	}
	scenarios := []scenario{
		{
			name: "raw upstream error",
			setup: func(hn *harness) {
				hn.ds.onRange = func(context.Context, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
					return nil, leak
				}
			},
			do: rangeRequest, status: http.StatusInternalServerError, code: "internal",
		},
		{
			name: "panicking datasource",
			setup: func(hn *harness) {
				hn.ds.onRange = func(context.Context, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
					panic(leak)
				}
			},
			do: rangeRequest, status: http.StatusInternalServerError, code: "internal",
		},
		{
			name: "raw variable error",
			setup: func(hn *harness) {
				hn.ds.onVariable = func(context.Context, datasource.TimeRange) ([]string, error) { return nil, leak }
			},
			do:     func(hn *harness) *httptest.ResponseRecorder { return hn.get(hn.valuesPath("job", window), "") },
			status: http.StatusInternalServerError, code: "internal",
		},
		{
			name: "dashboard store failure",
			setup: func(hn *harness) {
				hn.specs.err = errors.New("pq: password authentication failed for postgres://user:hunter2@db:5432")
			},
			do: rangeRequest, status: http.StatusInternalServerError, code: "internal",
		},
		{
			name: "panicking lint",
			setup: func(hn *harness) {
				hn.ds.onLint = func(context.Context, string) []datasource.Diagnostic { panic(leak) }
			},
			do:     func(hn *harness) *httptest.ResponseRecorder { return hn.lint("prom", `{"query":"up"}`) },
			status: http.StatusInternalServerError, code: "internal",
		},
		{
			name: "upstream error from an adapter keeps its own safe message",
			setup: func(hn *harness) {
				hn.ds.onRange = func(context.Context, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
					return nil, datasource.Errorf(datasource.ErrUnavailable, "datasource is unreachable")
				}
			},
			do: rangeRequest, status: http.StatusBadGateway, code: "datasource_unavailable",
		},
	}
	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			hn := newHarness(t)
			s.setup(hn)
			rec := s.do(hn)
			expectError(t, rec, s.status, s.code)
			for _, secret := range secrets {
				if strings.Contains(rec.Body.String(), secret) {
					t.Fatalf("body %s leaks %q", rec.Body.String(), secret)
				}
			}
		})
	}
}

func TestValuesFollowTheSameAlignedRangeAsTheDataDefaults(t *testing.T) {
	hn := newHarness(t)
	decodeData(t, hn.get(hn.dataPath("p_ts", window), ""))
	hn.ds.mu.Lock()
	fromData := hn.ds.variableRange
	hn.ds.mu.Unlock()

	decodeValues(t, hn.get(hn.valuesPath("job", window), ""))
	hn.ds.mu.Lock()
	fromValues := hn.ds.variableRange
	hn.ds.mu.Unlock()

	if !fromValues.From.Equal(fromData.From) || !fromValues.To.Equal(fromData.To) {
		t.Fatalf("the picker lists values for %v but the panel default resolved them for %v", fromValues, fromData)
	}
	if fromValues.From.UnixMilli()%22000 != 0 || fromValues.To.UnixMilli()%22000 != 0 {
		t.Fatalf("values range %v is not aligned to the 22s default step", fromValues)
	}
}

func TestVariableValuesAreCachedForQueryVariablesOnly(t *testing.T) {
	hn := newHarness(t)
	path := hn.valuesPath("job", window)
	first := decodeValues(t, hn.get(path, ""))
	second := decodeValues(t, hn.get(path, ""))
	if first.Cached || !second.Cached {
		t.Fatalf("cached flags = %v, %v; want false, true", first.Cached, second.Cached)
	}
	if strings.Join(second.Values, ",") != "api,web" || second.Truncated {
		t.Fatalf("values = %+v", second)
	}
	if _, _, variables, _ := hn.ds.counts(); variables != 1 {
		t.Fatalf("%d variable lookups, want 1", variables)
	}

	custom := hn.valuesPath("env", window)
	for range 2 {
		body := decodeValues(t, hn.get(custom, ""))
		if body.Cached || strings.Join(body.Values, ",") != "prod,dev" {
			t.Fatalf("custom values = %+v", body)
		}
	}
}

func TestVariableValuesOverTheEntryCapAreNotCached(t *testing.T) {
	hn := newHarness(t)
	hn.ds.onVariable = func(context.Context, datasource.TimeRange) ([]string, error) {
		values := make([]string, 1000)
		for i := range values {
			values[i] = fmt.Sprintf("%04d%s", i, strings.Repeat("v", 1100))
		}
		return values, nil
	}
	path := hn.valuesPath("job", window)
	for range 2 {
		if body := decodeValues(t, hn.get(path, "")); body.Cached || len(body.Values) != 1000 {
			t.Fatalf("cached %v with %d values", body.Cached, len(body.Values))
		}
	}
	if _, _, variables, _ := hn.ds.counts(); variables != 2 {
		t.Fatalf("%d variable lookups, want 2", variables)
	}
}

func TestEmptyVariableListIsAnEmptyArray(t *testing.T) {
	hn := newHarness(t)
	hn.ds.onVariable = func(context.Context, datasource.TimeRange) ([]string, error) { return nil, nil }
	rec := hn.get(hn.valuesPath("job", window), "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"values":[]`) {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestLintReturnsDiagnosticsAndValidatesTheBody(t *testing.T) {
	hn := newHarness(t)
	hn.ds.onLint = func(_ context.Context, query string) []datasource.Diagnostic {
		return []datasource.Diagnostic{{Severity: datasource.SeverityError, Message: "bad " + query, Start: 0, End: 1}}
	}
	rec := hn.lint("prom", `{"query":"sum(rate("}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"severity":"error"`) {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}

	hn.ds.onLint = nil
	if rec := hn.lint("prom", `{"query":"up"}`); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"diagnostics":[]`) {
		t.Fatalf("clean lint: status %d body %s", rec.Code, rec.Body.String())
	}

	expectError(t, hn.lint("nope", `{"query":"up"}`), http.StatusNotFound, "not_found")
	for _, body := range []string{
		`{"query":""}`,
		`{}`,
		`{"query":"up","extra":1}`,
		`{"query":"up"} {}`,
		`not json`,
		`{"query":"` + strings.Repeat("a", maxLintQueryBytes+1) + `"}`,
	} {
		expectError(t, hn.lint("prom", body), http.StatusBadRequest, "invalid_request")
	}
}

func holdUpstreamSlots(t *testing.T, hn *harness) {
	t.Helper()
	for range upstreamSlot {
		release, err := hn.runner.AcquireUpstream(context.Background())
		if err != nil {
			t.Fatalf("AcquireUpstream: %v", err)
		}
		t.Cleanup(release)
	}
}

func TestLintTakesAProcessWideUpstreamSlot(t *testing.T) {
	hn := newHarness(t)
	hn.h.lintTimeout = 30 * time.Millisecond

	releases := make([]func(), upstreamSlot)
	for i := range releases {
		release, err := hn.runner.AcquireUpstream(context.Background())
		if err != nil {
			t.Fatalf("AcquireUpstream: %v", err)
		}
		releases[i] = release
	}

	rec := hn.lint("prom", `{"query":"up"}`)
	expectError(t, rec, http.StatusTooManyRequests, "too_many_requests")
	if rec.Header().Get("Retry-After") != "1" {
		t.Fatalf("Retry-After = %q", rec.Header().Get("Retry-After"))
	}
	if _, _, _, lints := hn.ds.counts(); lints != 0 {
		t.Fatal("lint ran without an upstream slot")
	}
	expectError(t, hn.lint("nope", `{"query":"up"}`), http.StatusNotFound, "not_found")
	expectError(t, hn.lint("prom", `{"query":""}`), http.StatusBadRequest, "invalid_request")
	if held := hn.heldSlots(); held != 0 {
		t.Fatalf("%d principal slots leaked", held)
	}

	for _, release := range releases {
		release()
	}
	if rec := hn.lint("prom", `{"query":"up"}`); rec.Code != http.StatusOK {
		t.Fatalf("lint after the slots were freed: %d %s", rec.Code, rec.Body.String())
	}
}

func TestLintReleasesItsUpstreamSlot(t *testing.T) {
	hn := newHarness(t)
	for range 2 * upstreamSlot {
		if rec := hn.lint("prom", `{"query":"up"}`); rec.Code != http.StatusOK {
			t.Fatalf("lint: %d %s", rec.Code, rec.Body.String())
		}
	}
	holdUpstreamSlots(t, hn)
}

func TestClientHangUpCancelsAbandonedPanelQuery(t *testing.T) {
	hn := newHarness(t)
	entered := make(chan struct{}, 1)
	canceled := make(chan struct{})
	hn.ds.onRange = func(ctx context.Context, _ datasource.TimeRange, _ time.Duration) ([]datasource.Frame, error) {
		entered <- struct{}{}
		<-ctx.Done()
		close(canceled)
		return nil, ctx.Err()
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		req := httptest.NewRequest(http.MethodGet, hn.dataPath("p_ts", window+"&var-job=api"), nil).WithContext(ctx)
		done <- hn.serve(req, "")
	}()
	<-entered
	cancel()

	select {
	case <-canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("the upstream query kept running after its only client went away")
	}
	rec := <-done
	if rec.Body.Len() != 0 {
		t.Fatalf("a response was written to a client that went away: %s", rec.Body.String())
	}
	waitUntil(t, "the principal slot to be released", func() bool { return hn.heldSlots() == 0 })
	holdUpstreamSlots(t, hn)
}

func TestClientHangUpCancelsAbandonedVariableLookup(t *testing.T) {
	hn := newHarness(t, queryproxy.WithValueCache())
	entered := make(chan struct{}, 1)
	canceled := make(chan struct{})
	hn.ds.onVariable = func(ctx context.Context, _ datasource.TimeRange) ([]string, error) {
		entered <- struct{}{}
		<-ctx.Done()
		close(canceled)
		return nil, ctx.Err()
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		hn.serve(httptest.NewRequest(http.MethodGet, hn.valuesPath("job", window), nil).WithContext(ctx), "")
	}()
	<-entered
	cancel()

	select {
	case <-canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("the variable lookup kept running after its only client went away")
	}
	<-done
	holdUpstreamSlots(t, hn)
}

func TestOneClientHangingUpDoesNotFailAnIdenticalRequest(t *testing.T) {
	hn := newHarness(t)
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	hn.ds.onRange = func(ctx context.Context, _ datasource.TimeRange, _ time.Duration) ([]datasource.Frame, error) {
		entered <- struct{}{}
		select {
		case <-release:
			return sampleFrames(), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	path := hn.dataPath("p_ts", window+"&var-job=api")

	ctx, cancel := context.WithCancel(context.Background())
	leaderDone := make(chan struct{})
	go func() {
		defer close(leaderDone)
		hn.serve(httptest.NewRequest(http.MethodGet, path, nil).WithContext(ctx), "leader")
	}()
	<-entered
	follower := make(chan *httptest.ResponseRecorder, 1)
	go func() { follower <- hn.get(path, "follower") }()
	waitUntil(t, "both requests to be in flight", func() bool { return hn.heldSlots() == 2 })

	cancel()
	<-leaderDone
	close(release)
	if body := decodeData(t, <-follower); len(body.Frames) == 0 {
		t.Fatal("the follower got no frames")
	}
}
