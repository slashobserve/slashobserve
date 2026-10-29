package proxyhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
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
	valuesFor  func(q datasource.Query) []string
	onLint     func(ctx context.Context, query string) ([]datasource.Diagnostic, error)
	onDiscover func(ctx context.Context, req datasource.DiscoverRequest) (*datasource.Catalog, error)

	mu            sync.Mutex
	discovered    []datasource.DiscoverRequest
	rangeCalls    int
	instantCalls  int
	variableCalls int
	lintCalls     int
	variableRange datasource.TimeRange
	rangeRange    datasource.TimeRange
	rangeSteps    map[string]time.Duration
	seen          []datasource.Query
	failText      map[string]error
	panicText     map[string]bool
	before        func(ctx context.Context, text string)
	after         func(text string)
}

func (f *fakeDatasource) lastQuery() datasource.Query {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.seen[len(f.seen)-1]
}

func (f *fakeDatasource) stepFor(text string) time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.rangeSteps[text]
}

func (f *fakeDatasource) counts() (ranges, instants, variables, lints int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.rangeCalls, f.instantCalls, f.variableCalls, f.lintCalls
}

func (f *fakeDatasource) Type() string                      { return string(spec.Prometheus) }
func (f *fakeDatasource) CheckHealth(context.Context) error { return nil }
func (f *fakeDatasource) Close() error                      { return nil }
func (f *fakeDatasource) Discover(ctx context.Context, req datasource.DiscoverRequest) (*datasource.Catalog, error) {
	f.mu.Lock()
	f.discovered = append(f.discovered, req)
	f.mu.Unlock()
	if f.onDiscover != nil {
		return f.onDiscover(ctx, req)
	}
	return &datasource.Catalog{}, nil
}

func (f *fakeDatasource) discoverRequests() []datasource.DiscoverRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]datasource.DiscoverRequest(nil), f.discovered...)
}

func (f *fakeDatasource) Lint(ctx context.Context, query string) ([]datasource.Diagnostic, error) {
	f.mu.Lock()
	f.lintCalls++
	f.mu.Unlock()
	if f.onLint != nil {
		return f.onLint(ctx, query)
	}
	return nil, nil
}

func (f *fakeDatasource) VariableValues(ctx context.Context, q datasource.Query, r datasource.TimeRange) ([]string, error) {
	f.mu.Lock()
	f.seen = append(f.seen, q)
	f.variableCalls++
	f.variableRange = r
	f.mu.Unlock()
	if f.valuesFor != nil {
		return f.valuesFor(q), nil
	}
	if f.onVariable != nil {
		return f.onVariable(ctx, r)
	}
	return []string{"api", "web"}, nil
}

func (f *fakeDatasource) QueryRange(ctx context.Context, q datasource.Query, r datasource.TimeRange, step time.Duration) ([]datasource.Frame, error) {
	if f.after != nil {
		defer f.after(q.Text)
	}
	if f.before != nil {
		f.before(ctx, q.Text)
	}
	f.mu.Lock()
	f.rangeCalls++
	f.seen = append(f.seen, q)
	f.rangeRange = r
	if f.rangeSteps == nil {
		f.rangeSteps = map[string]time.Duration{}
	}
	f.rangeSteps[q.Text] = step
	failure := f.failText[q.Text]
	panics := f.panicText[q.Text]
	f.mu.Unlock()
	if panics {
		panic("datasource exploded")
	}
	if failure != nil {
		return nil, failure
	}
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
	return &spec.Dashboard{
		SpecVersion: spec.Version,
		Title:       "test",
		Time:        spec.TimeRange{From: "now-6h", To: "now"},
		Variables: []spec.Variable{
			{Name: "job", Type: spec.QueryVariable, Datasource: "prom", Query: "label_values(up, job)"},
			{Name: "env", Type: spec.CustomVariable, Options: []string{"prod", "dev"}},
		},
		Panels: []spec.Panel{
			{ID: "p_ts", Type: spec.Timeseries, Title: "ts", Datasource: "prom", Queries: []spec.Query{{Query: `up{job="$job"}`}}},
			{ID: "p_tbl", Type: spec.Table, Title: "tbl", Datasource: "prom", Queries: []spec.Query{{Query: `count(up{job="$job"})`}}},
			{ID: "p_note", Type: spec.Text},
			{ID: "r_row", Type: spec.Row, Panels: []spec.Panel{
				{ID: "p_child", Type: spec.Table, Title: "child", Datasource: "prom", Queries: []spec.Query{{Query: `count(up)`}}},
			}},
		},
	}
}

type harness struct {
	t      *testing.T
	h      *Handler
	router *gin.Engine
	ds     *fakeDatasource
	other  *fakeDatasource
	specs  *fakeSpecs
	runner *queryproxy.Runner
	id     pgtype.UUID
	logs   *bytes.Buffer
}

func newHarness(t *testing.T, opts ...queryproxy.RunnerOption) *harness {
	t.Helper()
	return buildHarness(t, nil, opts...)
}

func newPreviewHarness(t *testing.T, opts ...queryproxy.RunnerOption) *harness {
	t.Helper()
	hn := buildHarness(t, &fakeDatasource{}, opts...)
	hn.specs.known[specKey{id: hn.id, version: 1}] = previewDashboard()
	return hn
}

func previewDashboard() *spec.Dashboard {
	return &spec.Dashboard{
		SpecVersion: spec.Version,
		Title:       "preview",
		Time:        spec.TimeRange{From: "now-6h", To: "now"},
		Variables:   []spec.Variable{{Name: "job", Type: spec.QueryVariable, Datasource: "prom", Query: "label_values(up, job)"}},
		Panels: []spec.Panel{
			{ID: "p_ts", Type: spec.Timeseries, Title: "ts", Datasource: "prom", GridPos: spec.GridPos{W: 12, H: 8}, Queries: []spec.Query{{Query: `up{job="$job"}`}}},
		},
	}
}

func buildHarness(t *testing.T, other *fakeDatasource, opts ...queryproxy.RunnerOption) *harness {
	t.Helper()
	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewTextHandler(logs, nil))
	ds := &fakeDatasource{}
	instances := []datasource.Instance{{UID: "prom", Datasource: ds}}
	if other != nil {
		instances = append(instances, datasource.Instance{UID: "other", Datasource: other})
	}
	registry, err := datasource.NewRegistry(logger, instances...)
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
		email := c.GetHeader("X-Email")
		if email == "" {
			email = principal + "@example.com"
		}
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), auth.Principal{Kind: auth.PrincipalKind(c.GetHeader("X-Kind")), ID: principal, Email: email, Name: principal, Role: auth.RoleAdmin}))
	})
	h.Register(auth.NewRoutes(api))
	return &harness{t: t, h: h, router: router, ds: ds, other: other, specs: specs, runner: runner, id: id, logs: logs}
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

func (hn *harness) previewAs(kind auth.PrincipalKind, uid string, edit func(*spec.Panel)) *httptest.ResponseRecorder {
	hn.t.Helper()
	panel := previewDashboard().Panels[0]
	if edit != nil {
		edit(&panel)
	}
	body, err := json.Marshal(map[string]any{
		"dashboard_id": hn.id.String(),
		"version":      1,
		"panel_id":     "p_ts",
		"panel":        panel,
		"from":         "1760000000123",
		"to":           "1760021600456",
		"variables":    map[string][]string{"job": {"api"}},
	})
	if err != nil {
		hn.t.Fatalf("marshal preview body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/datasources/"+uid+"/preview", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Kind", string(kind))
	return hn.serve(req, "alice")
}

type errorBody struct {
	Error string `json:"error"`
	Code  string `json:"code"`
	Ref   string `json:"ref"`
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

func TestPreviewRefusesAQueryOverridingTheDatasource(t *testing.T) {
	hn := newPreviewHarness(t)
	rec := hn.previewAs(auth.KindUser, "prom", func(p *spec.Panel) {
		p.Queries = append(p.Queries, spec.Query{Query: "up", Datasource: "other"})
	})
	expectError(t, rec, http.StatusBadRequest, "invalid_request")
	if got := decodeError(t, rec).Error; got != "every query of the panel must use datasource prom" {
		t.Fatalf("message = %q", got)
	}
	if ranges, _, _, _ := hn.other.counts(); ranges != 0 {
		t.Fatalf("the overriding datasource ran %d queries", ranges)
	}
	if ranges, _, _, _ := hn.ds.counts(); ranges != 0 {
		t.Fatalf("the path's datasource ran %d queries", ranges)
	}
	if held := hn.heldSlots(); held != 0 {
		t.Fatalf("%d slots still held", held)
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
	if want := `[{"ref":"A","name":"up","labels":{"pod":"a"},"fields":[{"name":"Time","type":"time","values":[1760000000000]},{"name":"Value","type":"number","values":[1]}]}]`; string(first.Frames) != want {
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

func TestRowChildReturnsDataAndOtherKindsAreNotFound(t *testing.T) {
	hn := newHarness(t)
	body := decodeData(t, hn.get(hn.dataPath("p_child", window+"&var-job=api"), ""))
	if body.Meta.Kind != queryproxy.KindInstant {
		t.Fatalf("meta = %+v, want an instant panel", body.Meta)
	}
	expectError(t, hn.get(hn.dataPath("p_note", window), ""), http.StatusNotFound, "panel_not_found")
	expectError(t, hn.get(hn.dataPath("r_row", window), ""), http.StatusNotFound, "panel_not_found")
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

func TestClientGoingAwayMidQueryEndsWithClientClosedStatusAndNoBody(t *testing.T) {
	hn := newHarness(t)
	entered := make(chan struct{})
	hn.ds.onRange = func(ctx context.Context, _ datasource.TimeRange, _ time.Duration) ([]datasource.Frame, error) {
		close(entered)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, hn.dataPath("p_ts", window+"&var-job=api"), nil)
	rec := make(chan *httptest.ResponseRecorder, 1)
	go func() { rec <- hn.serve(req, "") }()
	<-entered
	cancel()
	got := <-rec
	if got.Code != clientgone.Status || got.Body.Len() != 0 {
		t.Fatalf("status %d body %q, want %d and no body", got.Code, got.Body.String(), clientgone.Status)
	}
}

func TestCancellationWhileTheClientIsStillConnectedIsAServerError(t *testing.T) {
	hn := newHarness(t)
	hn.ds.onRange = func(context.Context, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
		return nil, fmt.Errorf("upstream gave up: %w", context.Canceled)
	}
	got := hn.serve(httptest.NewRequest(http.MethodGet, hn.dataPath("p_ts", window+"&var-job=api"), nil), "")
	if got.Code != http.StatusInternalServerError || !strings.Contains(got.Body.String(), `"code":"internal"`) {
		t.Fatalf("status %d body %q, want 500 internal", got.Code, got.Body.String())
	}
	if !strings.Contains(hn.logs.String(), "level=ERROR") {
		t.Fatalf("a server side cancellation was not logged as an error: %s", hn.logs.String())
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
	rec = hn.previewAs(auth.KindUser, "prom", nil)
	expectError(t, rec, http.StatusTooManyRequests, "too_many_requests")
	if got := rec.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("preview Retry-After = %q, want 1", got)
	}

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

func TestResponseLimitCountsEveryQueryOfThePanel(t *testing.T) {
	hn := newHarness(t)
	hn.ds.onRange = func(context.Context, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
		return bigFrames(maxResponseBytes/2 + 1), nil
	}
	hn.addPanels(multiQueryPanel("p_multi", spec.Query{Query: "first"}, spec.Query{Query: "second"}))
	path := hn.dataPath("p_multi", window)
	rec := hn.get(path, "")
	expectError(t, rec, http.StatusUnprocessableEntity, "limit_exceeded")
	if msg := decodeError(t, rec).Error; msg != "response too large" {
		t.Fatalf("message = %q", msg)
	}
	hn.get(path, "")
	if ranges, _, _, _ := hn.ds.counts(); ranges != 4 {
		t.Fatalf("%d range queries, want 4 (a refused response is not cached)", ranges)
	}
}

func TestSizeLimitOutranksAQueryErrorWhicheverFinishesFirst(t *testing.T) {
	for _, invalidFirst := range []bool{true, false} {
		t.Run(fmt.Sprintf("invalid query finishes first=%t", invalidFirst), func(t *testing.T) {
			hn := newHarness(t)
			invalidDone := make(chan struct{})
			hn.ds.failText = map[string]error{"bad": datasource.Errorf(datasource.ErrInvalidQuery, "nope")}
			hn.ds.onRange = func(context.Context, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
				return bigFrames(maxResponseBytes/2 + 1), nil
			}
			hn.ds.before = func(ctx context.Context, text string) {
				wait := func(ch <-chan struct{}) {
					select {
					case <-ch:
					case <-time.After(5 * time.Second):
					}
				}
				switch {
				case text == "bad" && !invalidFirst:
					wait(ctx.Done())
				case text != "bad" && invalidFirst:
					wait(invalidDone)
				}
			}
			hn.ds.after = func(text string) {
				if text == "bad" {
					close(invalidDone)
				}
			}
			hn.addPanels(multiQueryPanel("p_multi", spec.Query{Query: "bad"}, spec.Query{Query: "big1"}, spec.Query{Query: "big2"}))
			rec := hn.get(hn.dataPath("p_multi", window), "")
			expectError(t, rec, http.StatusUnprocessableEntity, "limit_exceeded")
			if body := decodeError(t, rec); body.Error != "response too large" || body.Ref != "" {
				t.Fatalf("body = %s, want response too large without a ref", rec.Body.String())
			}
		})
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
				hn.ds.onLint = func(context.Context, string) ([]datasource.Diagnostic, error) { panic(leak) }
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

func TestInternalFailureLogsCarryOnlyTheCodeAndType(t *testing.T) {
	tests := []struct {
		name  string
		msg   string
		setup func(hn *harness)
		extra string
	}{
		{
			name: "frame conversion",
			msg:  "convert panel frames",
			setup: func(hn *harness) {
				hn.ds.onRange = func(context.Context, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
					return []datasource.Frame{{Fields: []datasource.Field{{Name: "UNIQUE-COLUMN", Type: datasource.FieldTime, Values: []any{"UNIQUE-VALUE"}}}}}, nil
				}
			},
		},
		{
			name: "version load",
			msg:  "load dashboard version",
			setup: func(hn *harness) {
				hn.specs.err = fmt.Errorf("load: %w", &pgconn.PgError{Code: "08006", Message: "UNIQUE-MESSAGE postgres://user:hunter2@db:5432", Detail: "UNIQUE-DETAIL"})
			},
			extra: "sqlstate=08006",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hn := newHarness(t)
			tc.setup(hn)
			expectError(t, hn.get(hn.dataPath("p_ts", window+"&var-job=api"), ""), http.StatusInternalServerError, "internal")
			logs := hn.logs.String()
			for _, leak := range []string{"UNIQUE-COLUMN", "UNIQUE-VALUE", "UNIQUE-MESSAGE", "UNIQUE-DETAIL", "hunter2", "holds a"} {
				if strings.Contains(logs, leak) {
					t.Fatalf("log leaks %q: %s", leak, logs)
				}
			}
			want := []string{"level=ERROR", "msg=\"" + tc.msg + "\"", "error_code=internal", "error_type="}
			if tc.extra != "" {
				want = append(want, tc.extra)
			}
			for _, w := range want {
				if !strings.Contains(logs, w) {
					t.Fatalf("log lacks %q: %s", w, logs)
				}
			}
			if strings.Contains(logs, " error=") {
				t.Fatalf("log carries the error text: %s", logs)
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
	hn.ds.onLint = func(_ context.Context, query string) ([]datasource.Diagnostic, error) {
		return []datasource.Diagnostic{{Severity: datasource.SeverityError, Message: "bad " + query, Start: 0, End: 1}}, nil
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
	} {
		expectError(t, hn.lint("prom", body), http.StatusBadRequest, "invalid_request")
	}
}

func TestLintNamesAQueryOrBodyOverTheLimitsWithAnExactCode(t *testing.T) {
	hn := newHarness(t)
	if rec := hn.lint("prom", `{"query":"`+strings.Repeat("a", maxLintQueryBytes)+`"}`); rec.Code != http.StatusOK {
		t.Fatalf("a query of exactly the limit: status %d body %s", rec.Code, rec.Body.String())
	}
	expectError(t, hn.lint("prom", `{"query":"`+strings.Repeat("a", maxLintQueryBytes+1)+`"}`), http.StatusBadRequest, "query_too_long")
	expectError(t, hn.lint("prom", `{"query":"`+strings.Repeat("a", maxLintBodyBytes)+`"}`), http.StatusRequestEntityTooLarge, "too_large")
}

func TestLintMapsDatasourceFailuresToGatewayErrors(t *testing.T) {
	for _, c := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"unavailable", datasource.Errorf(datasource.ErrUnavailable, "connection failed"), http.StatusBadGateway, "datasource_unavailable"},
		{"timeout", datasource.Errorf(datasource.ErrTimeout, "query timed out"), http.StatusGatewayTimeout, "query_timeout"},
	} {
		t.Run(c.name, func(t *testing.T) {
			hn := newHarness(t)
			hn.ds.onLint = func(context.Context, string) ([]datasource.Diagnostic, error) { return nil, c.err }
			expectError(t, hn.lint("prom", `{"query":"up"}`), c.status, c.code)
		})
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

func (hn *harness) addPanels(panels ...spec.Panel) {
	d := hn.specs.known[specKey{id: hn.id, version: 1}]
	d.Panels = append(d.Panels, panels...)
}

func multiQueryPanel(id string, queries ...spec.Query) spec.Panel {
	return spec.Panel{ID: id, Type: spec.Timeseries, Title: id, Datasource: "prom", Queries: queries}
}

func frameRefs(t *testing.T, body dataResponse) []string {
	t.Helper()
	var frames []wireFrame
	if err := json.Unmarshal(body.Frames, &frames); err != nil {
		t.Fatalf("frames: %v", err)
	}
	refs := make([]string, len(frames))
	for i, f := range frames {
		refs[i] = f.Ref
	}
	return refs
}

func TestPreviewRunsWithTheDraftLimitsAndIsNeverCached(t *testing.T) {
	hn := newPreviewHarness(t)
	first := decodeData(t, hn.previewAs(auth.KindUser, "prom", nil))
	if first.Meta.Cached {
		t.Fatal("the first preview reports cached")
	}
	if got := hn.ds.lastQuery().Limits; got != queryproxy.DraftLimits {
		t.Fatalf("limits = %+v, want %+v", got, queryproxy.DraftLimits)
	}
	second := decodeData(t, hn.previewAs(auth.KindUser, "prom", nil))
	if second.Meta.Cached {
		t.Fatal("the second preview reports cached")
	}
	if ranges, _, _, _ := hn.ds.counts(); ranges != 2 {
		t.Fatalf("%d upstream range queries for 2 identical previews, want 2", ranges)
	}
}

func TestPreviewIsRefusedForNonUsersWithoutTakingASlot(t *testing.T) {
	hn := newPreviewHarness(t)
	holdPrincipalSlots(t, hn, "alice")
	for _, kind := range []auth.PrincipalKind{auth.KindServiceAccount, ""} {
		expectError(t, hn.previewAs(kind, "prom", nil), http.StatusForbidden, "user_session_required")
	}
	if ranges, _, _, _ := hn.ds.counts(); ranges != 0 {
		t.Fatalf("%d upstream range queries for refused previews", ranges)
	}
	if held := hn.heldSlots(); held != maxInFlightPerPrincipal {
		t.Fatalf("%d slots held, want only the %d the test holds", held, maxInFlightPerPrincipal)
	}
}

func TestPreviewRejectsMalformedIdentifiersAndVariableNames(t *testing.T) {
	hn := newPreviewHarness(t)
	valid := hn.id.String()
	tests := []struct {
		name    string
		body    string
		message string
	}{
		{"misplaced dashes", `"dashboard_id":"` + valid[:7] + "-" + valid[8:9] + valid[7:8] + valid[10:] + `","version":1`, "dashboard_id must be a UUID"},
		{"no dashes", `"dashboard_id":"` + strings.ReplaceAll(valid, "-", "") + `","version":1`, "dashboard_id must be a UUID"},
		{"empty variable name", `"dashboard_id":"` + valid + `","version":1,"variables":{"":["a"],"zz":[]}`, "variables needs non-empty names"},
		{"first empty variable by name", `"dashboard_id":"` + valid + `","version":1,"variables":{"b":[],"a":[]}`, "variables.a needs at least one value"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body := `{` + tc.body + `,"panel_id":"p_ts","panel":{}}`
			req := httptest.NewRequest(http.MethodPost, "/api/v1/datasources/prom/preview", strings.NewReader(body))
			req.Header.Set("X-Kind", string(auth.KindUser))
			rec := hn.serve(req, "alice")
			expectError(t, rec, http.StatusBadRequest, "invalid_request")
			if got := decodeError(t, rec).Error; got != tc.message {
				t.Fatalf("message = %q, want %q", got, tc.message)
			}
		})
	}
}

func TestPreviewLogsEachServerFailureOnce(t *testing.T) {
	tests := []struct {
		name   string
		status int
		msg    string
		setup  func(hn *harness)
	}{
		{
			name:   "frame conversion",
			status: http.StatusInternalServerError,
			msg:    "convert panel frames",
			setup: func(hn *harness) {
				hn.ds.onRange = func(context.Context, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
					return []datasource.Frame{{Fields: []datasource.Field{{Name: "Time", Type: datasource.FieldTime, Values: []any{"not a time"}}}}}, nil
				}
			},
		},
		{
			name:   "datasource panic",
			status: http.StatusInternalServerError,
			msg:    "datasource call panicked",
			setup:  func(hn *harness) { hn.ds.panicText = map[string]bool{"boom": true} },
		},
		{
			name:   "version load",
			status: http.StatusInternalServerError,
			msg:    "load dashboard version",
			setup:  func(hn *harness) { hn.specs.err = errors.New("storage broke") },
		},
		{
			name:   "unavailable datasource",
			status: http.StatusBadGateway,
			msg:    "draft preview failed",
			setup:  func(hn *harness) { hn.ds.failText = map[string]error{"boom": datasource.ErrUnavailable} },
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hn := newPreviewHarness(t)
			tc.setup(hn)
			rec := hn.previewAs(auth.KindUser, "prom", func(p *spec.Panel) { p.Queries[0].Query = "boom" })
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tc.status, rec.Body.String())
			}
			logs := hn.logs.String()
			if got := strings.Count(logs, "level=ERROR"); got != 1 {
				t.Fatalf("%d error log lines, want 1: %s", got, logs)
			}
			if !strings.Contains(logs, "msg=\""+tc.msg+"\"") {
				t.Fatalf("log lacks %q: %s", tc.msg, logs)
			}
		})
	}
}

func TestMultiQueryPanelReturnsFramesTaggedByQueryRefInPanelOrder(t *testing.T) {
	hn := newHarness(t)
	hn.addPanels(multiQueryPanel("p_multi", spec.Query{Query: "first"}, spec.Query{Query: "second"}))
	body := decodeData(t, hn.get(hn.dataPath("p_multi", window), ""))
	if got := frameRefs(t, body); !slices.Equal(got, []string{"A", "B"}) {
		t.Fatalf("frame refs = %v, want [A B]", got)
	}
	wantMeta := []queryMeta{{Ref: "A", Kind: queryproxy.KindRange, StepMS: 22000}, {Ref: "B", Kind: queryproxy.KindRange, StepMS: 22000}}
	if !slices.Equal(body.Meta.Queries, wantMeta) {
		t.Fatalf("meta.queries = %+v, want %+v", body.Meta.Queries, wantMeta)
	}
}

func TestMultiQueryPanelUsesAnExplicitRef(t *testing.T) {
	hn := newHarness(t)
	hn.addPanels(multiQueryPanel("p_multi", spec.Query{Ref: "errors", Query: "first"}, spec.Query{Query: "second"}))
	body := decodeData(t, hn.get(hn.dataPath("p_multi", window), ""))
	if got := frameRefs(t, body); !slices.Equal(got, []string{"errors", "B"}) {
		t.Fatalf("frame refs = %v, want [errors B]", got)
	}
}

func TestHiddenQueryNeverRunsAndIsAbsentFromTheResponse(t *testing.T) {
	hn := newHarness(t)
	hn.addPanels(multiQueryPanel("p_multi", spec.Query{Query: "shown"}, spec.Query{Query: "hidden", Hide: true}, spec.Query{Query: "last"}))
	body := decodeData(t, hn.get(hn.dataPath("p_multi", window), ""))
	if got := frameRefs(t, body); !slices.Equal(got, []string{"A", "C"}) {
		t.Fatalf("frame refs = %v, want [A C]", got)
	}
	if len(body.Meta.Queries) != 2 || body.Meta.Queries[1].Ref != "C" {
		t.Fatalf("meta.queries = %+v, want A and C", body.Meta.Queries)
	}
	if ranges, _, _, _ := hn.ds.counts(); ranges != 2 {
		t.Fatalf("%d range queries, want 2: the hidden query must not run", ranges)
	}
}

func TestAllHiddenPanelReturnsNoFramesAndNoQueries(t *testing.T) {
	hn := newHarness(t)
	hn.addPanels(multiQueryPanel("p_multi", spec.Query{Query: "hidden", Hide: true}))
	rec := hn.get(hn.dataPath("p_multi", window), "")
	body := decodeData(t, rec)
	if string(body.Frames) != "[]" || body.Meta.Queries == nil || len(body.Meta.Queries) != 0 {
		t.Fatalf("body = %s, want empty frames and queries", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"queries":[]`) || !strings.Contains(rec.Body.String(), `"frames":[]`) {
		t.Fatalf("body = %s, want \"frames\":[] and \"queries\":[]", rec.Body.String())
	}
	if body.Meta.Kind != queryproxy.KindInstant {
		t.Fatalf("kind = %q, want instant", body.Meta.Kind)
	}
}

func TestFailingQueryFailsThePanelRequestAndNamesItsRef(t *testing.T) {
	hn := newHarness(t)
	hn.ds.failText = map[string]error{"second": datasource.Errorf(datasource.ErrLimitExceeded, "too many series")}
	hn.addPanels(multiQueryPanel("p_multi", spec.Query{Query: "first"}, spec.Query{Query: "second"}))
	rec := hn.get(hn.dataPath("p_multi", window), "")
	expectError(t, rec, http.StatusUnprocessableEntity, "limit_exceeded")
	if ref := decodeError(t, rec).Ref; ref != "B" {
		t.Fatalf("ref = %q, want B (body %s)", ref, rec.Body.String())
	}
}

func TestPanickingQueryFailsThePanelRequestWithAnInternalErrorAndItsRef(t *testing.T) {
	hn := newHarness(t)
	hn.ds.panicText = map[string]bool{"second": true}
	hn.addPanels(multiQueryPanel("p_multi", spec.Query{Query: "first"}, spec.Query{Query: "second"}))
	rec := hn.get(hn.dataPath("p_multi", window), "")
	expectError(t, rec, http.StatusInternalServerError, "internal")
	if ref := decodeError(t, rec).Ref; ref != "B" {
		t.Fatalf("ref = %q, want B (body %s)", ref, rec.Body.String())
	}
}

func TestOtherErrorsCarryNoRef(t *testing.T) {
	hn := newHarness(t)
	rec := hn.get(hn.dataPath("p_missing", window), "")
	expectError(t, rec, http.StatusNotFound, "panel_not_found")
	if strings.Contains(rec.Body.String(), `"ref"`) {
		t.Fatalf("body = %s, want no ref", rec.Body.String())
	}
}

func TestPanelMaxDataPointsCapsTheRequest(t *testing.T) {
	hn := newHarness(t)
	capped := multiQueryPanel("p_capped", spec.Query{Query: "capped"})
	capped.MaxDataPoints = 50
	hn.addPanels(capped, multiQueryPanel("p_plain", spec.Query{Query: "plain"}))
	cappedBody := decodeData(t, hn.get(hn.dataPath("p_capped", window+"&max_data_points=1000"), ""))
	plainBody := decodeData(t, hn.get(hn.dataPath("p_plain", window+"&max_data_points=1000"), ""))
	if plainBody.Meta.StepMS != 22000 {
		t.Fatalf("uncapped step_ms = %d, want 22000", plainBody.Meta.StepMS)
	}
	if cappedBody.Meta.StepMS != 433000 || hn.ds.stepFor("capped") != 433*time.Second {
		t.Fatalf("capped step_ms = %d, upstream step = %v, want 433s (50 points)", cappedBody.Meta.StepMS, hn.ds.stepFor("capped"))
	}
}

func TestQueryIntervalRaisesThatQueryStepOnly(t *testing.T) {
	hn := newHarness(t)
	hn.addPanels(multiQueryPanel("p_multi", spec.Query{Query: "plain"}, spec.Query{Query: "slow", Interval: "5m"}))
	body := decodeData(t, hn.get(hn.dataPath("p_multi", window), ""))
	if body.Meta.StepMS != 22000 || len(body.Meta.Queries) != 2 {
		t.Fatalf("meta = %+v", body.Meta)
	}
	if body.Meta.Queries[0].StepMS != 22000 || body.Meta.Queries[1].StepMS != 300000 {
		t.Fatalf("meta.queries = %+v, want 22000 and 300000", body.Meta.Queries)
	}
	if hn.ds.stepFor("slow") != 5*time.Minute || hn.ds.stepFor("plain") != 22*time.Second {
		t.Fatalf("upstream steps slow=%v plain=%v", hn.ds.stepFor("slow"), hn.ds.stepFor("plain"))
	}
}

const (
	userPanelVersion    = 2
	userVariableVersion = 3
)

func (hn *harness) withUserVersions() {
	panelDashboard := testDashboard()
	panelDashboard.Panels = append(panelDashboard.Panels, spec.Panel{
		ID: "p_user", Type: spec.Timeseries, Title: "user", Datasource: "prom",
		Queries: []spec.Query{{Query: `up{who="${__user.login}", dash="$__dashboard"}`}},
	})
	variableDashboard := testDashboard()
	variableDashboard.Variables[0].Query = `label_values(up{who="${__user.login}"}, job)`
	hn.specs.known[specKey{id: hn.id, version: userPanelVersion}] = panelDashboard
	hn.specs.known[specKey{id: hn.id, version: userVariableVersion}] = variableDashboard
}

func (hn *harness) versionPath(version int, suffix string) string {
	return "/api/v1/dashboards/" + hn.id.String() + "/versions/" + strconv.Itoa(version) + suffix
}

func TestPanelDataIsCachedPerPrincipalWhenTheVersionReferencesTheUser(t *testing.T) {
	hn := newHarness(t)
	hn.withUserVersions()
	path := hn.versionPath(userPanelVersion, "/panels/p_user/data?"+window+"&var-job=api")

	alice := decodeData(t, hn.get(path, "alice"))
	bob := decodeData(t, hn.get(path, "bob"))
	if alice.Meta.Cached || bob.Meta.Cached {
		t.Fatalf("cached flags = %v, %v; want false for each principal's first request", alice.Meta.Cached, bob.Meta.Cached)
	}
	if ranges, _, _, _ := hn.ds.counts(); ranges != 2 {
		t.Fatalf("%d range queries, want one per principal", ranges)
	}
	if query := hn.ds.lastQuery(); query.User.Login != "bob@example.com" || query.User.ID != "bob" || query.Dashboard != "test" {
		t.Fatalf("the datasource saw dashboard %q and user %+v", query.Dashboard, query.User)
	}
	if again := decodeData(t, hn.get(path, "alice")); !again.Meta.Cached {
		t.Fatal("the same principal's repeated request was not served from the cache")
	}
	if ranges, _, _, _ := hn.ds.counts(); ranges != 2 {
		t.Fatalf("%d range queries after a repeat, want 2", ranges)
	}
}

func TestPanelDataIsSharedAcrossPrincipalsWhenNoQueryReferencesTheUser(t *testing.T) {
	hn := newHarness(t)
	path := hn.dataPath("p_ts", window+"&var-job=api")

	if first := decodeData(t, hn.get(path, "alice")); first.Meta.Cached {
		t.Fatal("the first request claims to be cached")
	}
	if second := decodeData(t, hn.get(path, "bob")); !second.Meta.Cached {
		t.Fatal("a second principal did not share the cached panel data")
	}
	if ranges, _, _, _ := hn.ds.counts(); ranges != 1 {
		t.Fatalf("%d range queries, want 1", ranges)
	}
}

func TestVariableValuesAreCachedPerPrincipalWhenAQueryReferencesTheUser(t *testing.T) {
	hn := newHarness(t)
	hn.withUserVersions()
	path := hn.versionPath(userVariableVersion, "/variables/job/values?"+window)

	if alice, bob := decodeValues(t, hn.get(path, "alice")), decodeValues(t, hn.get(path, "bob")); alice.Cached || bob.Cached {
		t.Fatalf("cached flags = %v, %v; want false for each principal's first request", alice.Cached, bob.Cached)
	}
	if query := hn.ds.lastQuery(); query.User.Login != "bob@example.com" || query.Dashboard != "test" {
		t.Fatalf("the datasource saw dashboard %q and user %+v", query.Dashboard, query.User)
	}
	if again := decodeValues(t, hn.get(path, "alice")); !again.Cached {
		t.Fatal("the same principal's repeated request was not served from the cache")
	}
	if _, _, variables, _ := hn.ds.counts(); variables != 2 {
		t.Fatalf("%d variable lookups, want one per principal", variables)
	}
}

func (hn *harness) getWithEmail(path, principal, email string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("X-Email", email)
	return hn.serve(req, principal)
}

func TestPanelDataIsNotSharedWhenThePrincipalsEmailChanges(t *testing.T) {
	hn := newHarness(t)
	hn.withUserVersions()
	path := hn.versionPath(userPanelVersion, "/panels/p_user/data?"+window+"&var-job=api")

	if first := decodeData(t, hn.getWithEmail(path, "alice", "alice@old.example")); first.Meta.Cached {
		t.Fatal("the first request claims to be cached")
	}
	if repeat := decodeData(t, hn.getWithEmail(path, "alice", "alice@old.example")); !repeat.Meta.Cached {
		t.Fatal("an unchanged principal was not served from the cache")
	}
	if changed := decodeData(t, hn.getWithEmail(path, "alice", "alice@new.example")); changed.Meta.Cached {
		t.Fatal("a principal with a new email was served data built for the old email")
	}
	if query := hn.ds.lastQuery(); query.User.ID != "alice" || query.User.Email != "alice@new.example" || query.User.Login != "alice@new.example" {
		t.Fatalf("the datasource saw user %+v", query.User)
	}
	if ranges, _, _, _ := hn.ds.counts(); ranges != 2 {
		t.Fatalf("%d range queries, want 2", ranges)
	}
}

func TestVariableValuesAreNotSharedWhenThePrincipalsEmailChanges(t *testing.T) {
	hn := newHarness(t)
	hn.withUserVersions()
	path := hn.versionPath(userVariableVersion, "/variables/job/values?"+window)

	if first := decodeValues(t, hn.getWithEmail(path, "alice", "alice@old.example")); first.Cached {
		t.Fatal("the first request claims to be cached")
	}
	if changed := decodeValues(t, hn.getWithEmail(path, "alice", "alice@new.example")); changed.Cached {
		t.Fatal("a principal with a new email was served values built for the old email")
	}
	if _, _, variables, _ := hn.ds.counts(); variables != 2 {
		t.Fatalf("%d variable lookups, want 2", variables)
	}
}

const userEmailVariableVersion = 4

func (hn *harness) withUserEmailVariableOnly() {
	d := testDashboard()
	d.Variables[0].Query = `label_values(up{who="${__user.email}"}, job)`
	hn.specs.known[specKey{id: hn.id, version: userEmailVariableVersion}] = d
	hn.ds.valuesFor = func(q datasource.Query) []string { return []string{"job-of-" + q.User.Email} }
}

func TestAVariableQueryThatReferencesTheUserKeepsPrincipalsApartForValuesAndPanelData(t *testing.T) {
	hn := newHarness(t)
	hn.withUserEmailVariableOnly()
	valuesPath := hn.versionPath(userEmailVariableVersion, "/variables/job/values?"+window)
	dataPath := hn.versionPath(userEmailVariableVersion, "/panels/p_ts/data?"+window)

	alice := decodeValues(t, hn.get(valuesPath, "alice"))
	bob := decodeValues(t, hn.get(valuesPath, "bob"))
	if !slices.Equal(alice.Values, []string{"job-of-alice@example.com"}) || !slices.Equal(bob.Values, []string{"job-of-bob@example.com"}) {
		t.Fatalf("values alice=%v bob=%v, want each principal's own list", alice.Values, bob.Values)
	}
	if alice.Cached || bob.Cached {
		t.Fatal("a principal was served another principal's cached values")
	}

	jobOf := func(q datasource.Query) []string {
		for _, v := range q.Variables {
			if v.Name == "job" {
				return v.Values
			}
		}
		return nil
	}
	if first := decodeData(t, hn.get(dataPath, "alice")); first.Meta.Cached {
		t.Fatal("the first panel request claims to be cached")
	}
	if got := jobOf(hn.ds.lastQuery()); !slices.Equal(got, []string{"job-of-alice@example.com"}) {
		t.Fatalf("alice's panel query ran with job=%v", got)
	}
	if second := decodeData(t, hn.get(dataPath, "bob")); second.Meta.Cached {
		t.Fatal("bob was served the panel data built from alice's variable values")
	}
	if got := jobOf(hn.ds.lastQuery()); !slices.Equal(got, []string{"job-of-bob@example.com"}) {
		t.Fatalf("bob's panel query ran with job=%v", got)
	}
	if ranges, _, _, _ := hn.ds.counts(); ranges != 2 {
		t.Fatalf("%d range queries, want one per principal", ranges)
	}
}

func TestPerPrincipalScopeSurvivesAnEvictedAndReloadedVersion(t *testing.T) {
	hn := newHarness(t)
	hn.withUserVersions()
	path := hn.versionPath(userPanelVersion, "/panels/p_user/data?"+window+"&var-job=api")
	decodeData(t, hn.get(path, "alice"))
	loads := hn.specs.lookups()

	for i := range specCacheEntries + 1 {
		hn.h.caches.specs.Put(specKey{id: hn.id, version: 1000 + i}, loadedVersion{}, 1)
	}
	if bob := decodeData(t, hn.get(path, "bob")); bob.Meta.Cached {
		t.Fatal("a reloaded version was served from another principal's cache")
	}
	if hn.specs.lookups() == loads {
		t.Fatal("the version was not reloaded, so the test did not evict it")
	}
}

func (hn *harness) addVariables(variables ...spec.Variable) {
	d := hn.specs.known[specKey{id: hn.id, version: 1}]
	d.Variables = append(d.Variables, variables...)
}

func TestValuesOfTheNewVariableTypes(t *testing.T) {
	hn := newHarness(t)
	hn.addVariables(
		spec.Variable{Name: "const", Type: spec.ConstantVariable, Value: "shop"},
		spec.Variable{Name: "iv", Type: spec.IntervalVariable, Options: []string{"1m", "5m"}, Auto: true},
		spec.Variable{Name: "dsv", Type: spec.DatasourceVariable, DatasourceType: spec.Prometheus},
		spec.Variable{Name: "tb", Type: spec.TextboxVariable},
		spec.Variable{Name: "pf", Type: spec.AdhocVariable, Datasource: "prom"},
	)
	for name, want := range map[string]string{
		"const": "shop",
		"iv":    "$__auto,1m,5m",
		"dsv":   "prom",
		"tb":    "",
	} {
		rec := hn.get(hn.valuesPath(name, window), "")
		body := decodeValues(t, rec)
		if got := strings.Join(body.Values, ","); got != want || body.Cached {
			t.Errorf("%s values = %+v (%s), want %q", name, body, rec.Body.String(), want)
		}
	}
	if rec := hn.get(hn.valuesPath("tb", window), ""); !strings.Contains(rec.Body.String(), `"values":[]`) {
		t.Errorf("textbox body = %s, want an empty array", rec.Body.String())
	}
	rec := hn.get(hn.valuesPath("pf", window), "")
	expectError(t, rec, http.StatusBadRequest, "invalid_request")
	if !strings.Contains(rec.Body.String(), "has no value list") {
		t.Errorf("adhoc body = %s", rec.Body.String())
	}
}

func TestPanelDataWithTheNewVariableTypes(t *testing.T) {
	hn := newHarness(t)
	hn.addVariables(
		spec.Variable{Name: "const", Type: spec.ConstantVariable, Value: "shop"},
		spec.Variable{Name: "iv", Type: spec.IntervalVariable, Options: []string{"1m", "5m"}, Auto: true},
		spec.Variable{Name: "tb", Type: spec.TextboxVariable},
		spec.Variable{Name: "pf", Type: spec.AdhocVariable, Datasource: "prom"},
	)
	hn.addPanels(spec.Panel{ID: "p_vars", Type: spec.Timeseries, Title: "vars", Datasource: "prom", Queries: []spec.Query{{Query: `up{app="$const", i="$iv", t="$tb"}`}}})
	path := func(query string) string { return hn.dataPath("p_vars", window+"&"+query) }

	if rec := hn.get(path("var-job=api"), ""); rec.Code != http.StatusOK {
		t.Fatalf("defaults: status %d body %s", rec.Code, rec.Body.String())
	}
	if text := hn.ds.lastQuery().Text; text != `up{app="$const", i="$iv", t="$tb"}` || len(hn.ds.lastQuery().Filters) != 0 {
		t.Fatalf("default query = %+v", hn.ds.lastQuery())
	}

	rec := hn.get(path("var-job=api&var-iv=%24__auto&var-tb=&var-pf=job%7C%3D%7Capi&var-pf=pod%7C!~%7Cweb-.*"), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("selections: status %d body %s", rec.Code, rec.Body.String())
	}
	want := []datasource.AdhocFilter{{Key: "job", Op: "=", Value: "api"}, {Key: "pod", Op: "!~", Value: "web-.*"}}
	if got := hn.ds.lastQuery().Filters; !reflect.DeepEqual(got, want) {
		t.Fatalf("filters = %+v, want %+v", got, want)
	}

	for name, query := range map[string]string{
		"constant other value": "var-const=other",
		"interval not offered": "var-iv=10m",
		"textbox twice":        "var-tb=a&var-tb=b",
		"adhoc bad key":        "var-pf=Bad%20Key%7C%3D%7Cx",
		"adhoc bad operator":   "var-pf=job%7C%3C%7Cx",
		"adhoc All":            "var-pf=%24__all",
	} {
		t.Run(name, func(t *testing.T) {
			expectError(t, hn.get(path("var-job=api&"+query), ""), http.StatusBadRequest, "invalid_request")
		})
	}
}
