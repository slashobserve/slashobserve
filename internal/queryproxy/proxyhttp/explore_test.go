package proxyhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

const (
	exploreFrom = "1760000000123"
	exploreTo   = "1760000060456"
)

type instantCall struct {
	query datasource.Query
	rng   datasource.TimeRange
}

type recordingDatasource struct {
	*fakeDatasource
	mu    sync.Mutex
	calls []instantCall
}

func (r *recordingDatasource) QueryInstant(ctx context.Context, q datasource.Query, rng datasource.TimeRange) ([]datasource.Frame, error) {
	r.mu.Lock()
	r.calls = append(r.calls, instantCall{query: q, rng: rng})
	r.mu.Unlock()
	return r.fakeDatasource.QueryInstant(ctx, q, rng)
}

func (r *recordingDatasource) instants() []instantCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]instantCall(nil), r.calls...)
}

func newExploreHarness(t *testing.T) (*harness, *recordingDatasource) {
	t.Helper()
	hn := newHarness(t)
	recorder := &recordingDatasource{fakeDatasource: hn.ds}
	registry, err := datasource.NewRegistry(hn.h.logger, datasource.Instance{UID: "prom", Datasource: recorder})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	hn.runner = queryproxy.NewRunner(registry, hn.h.logger)
	hn.h.runner, hn.h.registry = hn.runner, registry
	return hn, recorder
}

func (hn *harness) explore(kind auth.PrincipalKind, uid string, body map[string]any) *httptest.ResponseRecorder {
	hn.t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		hn.t.Fatalf("marshal explore body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/datasources/"+uid+"/query", bytes.NewReader(encoded))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Kind", string(kind))
	return hn.serve(req, "alice")
}

func instantBody(query string) map[string]any {
	return map[string]any{"query": query, "kind": "instant", "from": exploreFrom, "to": exploreTo}
}

func TestExploreRunsAnInstantQueryOverTheExactRangeWithDraftLimits(t *testing.T) {
	hn, recorder := newExploreHarness(t)
	body := decodeData(t, hn.explore(auth.KindUser, "prom", instantBody(`{service_name="checkout"}`)))
	if body.Meta.Kind != queryproxy.KindInstant {
		t.Fatalf("kind = %q, want instant", body.Meta.Kind)
	}
	calls := recorder.instants()
	if len(calls) != 1 {
		t.Fatalf("%d instant calls, want 1", len(calls))
	}
	got := calls[0]
	if got.query.Text != `{service_name="checkout"}` {
		t.Fatalf("query text = %q", got.query.Text)
	}
	if got.query.Limits != queryproxy.DraftLimits {
		t.Fatalf("limits = %+v, want %+v", got.query.Limits, queryproxy.DraftLimits)
	}
	if got.rng.From.UnixMilli() != 1760000000123 || got.rng.To.UnixMilli() != 1760000060456 {
		t.Fatalf("range = %d to %d, want the exact bounds", got.rng.From.UnixMilli(), got.rng.To.UnixMilli())
	}
	if ranges, _, _, _ := hn.ds.counts(); ranges != 0 {
		t.Fatalf("%d range calls for an instant query", ranges)
	}
}

func TestExploreLimitLowersOnlyTheRowLimit(t *testing.T) {
	hn, recorder := newExploreHarness(t)
	body := instantBody("up")
	body["limit"] = 50
	decodeData(t, hn.explore(auth.KindUser, "prom", body))
	want := queryproxy.DraftLimits
	want.MaxRows = 50
	if got := recorder.instants()[0].query.Limits; got != want {
		t.Fatalf("limits = %+v, want %+v", got, want)
	}
}

func TestExploreRunsARangeQueryWithAStepOverAnAlignedRange(t *testing.T) {
	hn, _ := newExploreHarness(t)
	body := decodeData(t, hn.explore(auth.KindUser, "prom", map[string]any{
		"query": "rate(http_requests_total[5m])", "kind": "range", "from": "1760000000123", "to": "1760021600456", "max_data_points": 500,
	}))
	if body.Meta.Kind != queryproxy.KindRange || body.Meta.StepMS <= 0 {
		t.Fatalf("meta = %+v, want a range with a step", body.Meta)
	}
	step := hn.ds.stepFor("rate(http_requests_total[5m])")
	if step <= 0 {
		t.Fatalf("step = %v, want a positive step", step)
	}
	if step.Milliseconds() != body.Meta.StepMS {
		t.Fatalf("upstream step %v, response step %d ms", step, body.Meta.StepMS)
	}
	hn.ds.mu.Lock()
	rng := hn.ds.rangeRange
	hn.ds.mu.Unlock()
	if rng.From.UnixMilli()%step.Milliseconds() != 0 || rng.To.UnixMilli()%step.Milliseconds() != 0 {
		t.Fatalf("range %v to %v is not aligned to %v", rng.From, rng.To, step)
	}
	if ranges, instants, _, _ := hn.ds.counts(); ranges != 1 || instants != 0 {
		t.Fatalf("calls = %d range, %d instant; want 1 range", ranges, instants)
	}
}

func TestExploreIsNeverCachedOrCoalesced(t *testing.T) {
	hn, recorder := newExploreHarness(t)
	for i := range 2 {
		if body := decodeData(t, hn.explore(auth.KindUser, "prom", instantBody("up"))); body.Meta.Cached {
			t.Fatalf("request %d reports cached", i+1)
		}
	}
	if calls := len(recorder.instants()); calls != 2 {
		t.Fatalf("%d upstream calls for 2 identical requests, want 2", calls)
	}
}

func TestExploreAcceptsUsersAndServiceAccountKeys(t *testing.T) {
	hn, _ := newExploreHarness(t)
	for _, kind := range []auth.PrincipalKind{auth.KindUser, auth.KindServiceAccount} {
		if rec := hn.explore(kind, "prom", instantBody("up")); rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, body %s", kind, rec.Code, rec.Body.String())
		}
	}
}

func TestExploreAnswersNotFoundForAnUnknownDatasource(t *testing.T) {
	hn, recorder := newExploreHarness(t)
	expectError(t, hn.explore(auth.KindUser, "missing", instantBody("up")), http.StatusNotFound, "not_found")
	if calls := len(recorder.instants()); calls != 0 {
		t.Fatalf("%d upstream calls for an unknown datasource", calls)
	}
	if held := hn.heldSlots(); held != 0 {
		t.Fatalf("%d slots still held", held)
	}
}

func TestExploreRejectsInvalidRequests(t *testing.T) {
	hn, recorder := newExploreHarness(t)
	with := func(edit func(map[string]any)) map[string]any {
		body := instantBody("up")
		edit(body)
		return body
	}
	tests := []struct {
		name string
		body map[string]any
		code string
	}{
		{"missing query", with(func(b map[string]any) { delete(b, "query") }), "invalid_request"},
		{"empty query", with(func(b map[string]any) { b["query"] = "" }), "invalid_request"},
		{"query over 64 KiB", with(func(b map[string]any) { b["query"] = strings.Repeat("x", 64<<10+1) }), "query_too_long"},
		{"unknown kind", with(func(b map[string]any) { b["kind"] = "table" }), "invalid_request"},
		{"missing to", with(func(b map[string]any) { delete(b, "to") }), "invalid_request"},
		{"from after to", with(func(b map[string]any) { b["from"], b["to"] = exploreTo, exploreFrom }), "invalid_request"},
		{"limit zero", with(func(b map[string]any) { b["limit"] = 0 }), "invalid_request"},
		{"limit over the row limit", with(func(b map[string]any) { b["limit"] = 1001 }), "invalid_request"},
		{"max_data_points on an instant query", with(func(b map[string]any) { b["max_data_points"] = 100 }), "invalid_request"},
		{"unknown field", with(func(b map[string]any) { b["expr"] = "up" }), "invalid_request"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expectError(t, hn.explore(auth.KindUser, "prom", tt.body), http.StatusBadRequest, tt.code)
		})
	}
	if calls := len(recorder.instants()); calls != 0 {
		t.Fatalf("%d upstream calls for invalid requests", calls)
	}
}

func TestExploreReportsAQueryErrorWithItsRef(t *testing.T) {
	hn, _ := newExploreHarness(t)
	hn.ds.onInstant = func(context.Context, datasource.TimeRange) ([]datasource.Frame, error) {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "parse error at line 1")
	}
	rec := hn.explore(auth.KindUser, "prom", instantBody("{"))
	expectError(t, rec, http.StatusUnprocessableEntity, "query_invalid")
	body := decodeError(t, rec)
	if !strings.Contains(body.Error, "parse error at line 1") || body.Ref != "A" {
		t.Fatalf("error = %+v, want the datasource message and ref A", body)
	}
}

func TestExploreAnswersTooManyRequestsWhenEverySlotIsHeld(t *testing.T) {
	hn, recorder := newExploreHarness(t)
	holdPrincipalSlots(t, hn, "alice")
	rec := hn.explore(auth.KindUser, "prom", instantBody("up"))
	expectError(t, rec, http.StatusTooManyRequests, "too_many_requests")
	if got := rec.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q, want 1", got)
	}
	if calls := len(recorder.instants()); calls != 0 {
		t.Fatalf("%d upstream calls while every slot is held", calls)
	}
}

func TestExploreLogsAnUpstreamFailureWithoutTheQueryText(t *testing.T) {
	hn, _ := newExploreHarness(t)
	hn.ds.onInstant = func(context.Context, datasource.TimeRange) ([]datasource.Frame, error) {
		return nil, datasource.Errorf(datasource.ErrUnavailable, "connection refused")
	}
	text := `{service_name="secret-checkout-query"}`
	expectError(t, hn.explore(auth.KindUser, "prom", instantBody(text)), http.StatusBadGateway, "datasource_unavailable")
	logs := hn.logs.String()
	if n := strings.Count(logs, "explore query failed"); n != 1 {
		t.Fatalf("%d explore failure lines, want 1:\n%s", n, logs)
	}
	if strings.Contains(logs, "secret-checkout-query") {
		t.Fatalf("the log holds the query text:\n%s", logs)
	}
}
