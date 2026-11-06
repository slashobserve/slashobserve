package proxyhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

type compareDatasource struct {
	*traceDatasource
	onAttributes func(ctx context.Context, req datasource.TraceAttributesRequest) ([]datasource.TraceAttributeSet, error)
	attributes   []datasource.TraceAttributesRequest
}

func (d *compareDatasource) TraceAttributes(ctx context.Context, req datasource.TraceAttributesRequest) ([]datasource.TraceAttributeSet, error) {
	d.mu.Lock()
	d.attributes = append(d.attributes, req)
	d.mu.Unlock()
	return d.onAttributes(ctx, req)
}

func (d *compareDatasource) attributeCalls() []datasource.TraceAttributesRequest {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]datasource.TraceAttributesRequest(nil), d.attributes...)
}

func attributeSetsFor(ids []string, pairs func(i int) []datasource.TraceAttribute) []datasource.TraceAttributeSet {
	sets := make([]datasource.TraceAttributeSet, len(ids))
	for i, id := range ids {
		sets[i] = datasource.TraceAttributeSet{ID: id, Attributes: pairs(i)}
	}
	return sets
}

func newCompareHarness(t *testing.T) (*harness, *compareDatasource) {
	t.Helper()
	hn, tempoDS := newTraceHarness(t)
	ds := &compareDatasource{traceDatasource: tempoDS}
	ds.onAttributes = func(_ context.Context, req datasource.TraceAttributesRequest) ([]datasource.TraceAttributeSet, error) {
		return attributeSetsFor(req.IDs, func(int) []datasource.TraceAttribute { return nil }), nil
	}
	registry, err := datasource.NewRegistry(hn.h.logger,
		datasource.Instance{UID: "prom", Datasource: hn.ds},
		datasource.Instance{UID: "tempo", Datasource: ds},
	)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	hn.runner = queryproxy.NewRunner(registry, hn.h.logger)
	hn.h.runner, hn.h.registry = hn.runner, registry
	return hn, ds
}

func (hn *harness) traceCompare(uid string, body map[string]any) *httptest.ResponseRecorder {
	hn.t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		hn.t.Fatalf("marshal trace compare body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/datasources/"+uid+"/trace-compare", bytes.NewReader(encoded))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Kind", string(auth.KindServiceAccount))
	return hn.serve(req, "alice")
}

func compareIDs(prefix byte, n int) []string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("%c%031x", prefix, i+1)
	}
	return ids
}

func traceCompareBody(selected, baseline []string) map[string]any {
	return map[string]any{"selected": selected, "baseline": baseline, "from": volumeFrom, "to": volumeTo}
}

func TestTraceCompareRanksDifferences(t *testing.T) {
	hn, ds := newCompareHarness(t)
	ds.onAttributes = func(_ context.Context, req datasource.TraceAttributesRequest) ([]datasource.TraceAttributeSet, error) {
		return attributeSetsFor(req.IDs, func(i int) []datasource.TraceAttribute {
			pairs := []datasource.TraceAttribute{{Key: "kind", Value: "Server"}, {Key: "resource.service.name", Value: "checkout"}}
			if req.IDs[0][0] == 'a' || i < 1 {
				pairs = append(pairs, datasource.TraceAttribute{Key: "span.db.system", Value: "postgres"})
			}
			if req.IDs[0][0] == 'a' && i < 2 {
				pairs = append(pairs, datasource.TraceAttribute{Key: "status", Value: "Error"})
			}
			return pairs
		}), nil
	}
	selected := compareIDs('a', 4)
	baseline := compareIDs('b', 10)
	body := traceCompareBody(append(selected, strings.ToUpper(selected[0])), baseline)
	body["limit"] = 1
	body["timezone"] = "Asia/Kolkata"
	rec := hn.traceCompare("tempo", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	want := `{"selected":{"requested":4,"read":4},"baseline":{"requested":10,"read":10},"attributes":4,"partial":false,` +
		`"differences":[{"attribute":"span.db.system","value":"postgres","selected_share":1,"baseline_share":0.1,"selected_count":4,"baseline_count":1}],"truncated":true}`
	if got := strings.TrimSpace(rec.Body.String()); got != want {
		t.Fatalf("body =\n%s\nwant\n%s", got, want)
	}
	calls := ds.attributeCalls()
	if len(calls) != 2 || fmt.Sprint(calls[0].IDs) != fmt.Sprint(selected) || fmt.Sprint(calls[1].IDs) != fmt.Sprint(baseline) {
		t.Fatalf("calls = %+v, want the lowercased selected ids then the baseline ids", calls)
	}
	for _, call := range calls {
		if call.MaxSpans != datasource.CompareSpanBudget || !call.Range.From.Equal(volumeStart) || call.Range.To.Sub(call.Range.From) != time.Hour {
			t.Fatalf("call = %+v", call)
		}
	}
	if hn.heldSlots() != 0 {
		t.Fatal("a slot stayed held")
	}
}

func TestTraceCompareReportsPartialWhenATraceIsMissing(t *testing.T) {
	hn, ds := newCompareHarness(t)
	ds.onAttributes = func(_ context.Context, req datasource.TraceAttributesRequest) ([]datasource.TraceAttributeSet, error) {
		return attributeSetsFor(req.IDs[:len(req.IDs)-1], func(int) []datasource.TraceAttribute { return nil }), nil
	}
	rec := hn.traceCompare("tempo", traceCompareBody(compareIDs('a', 2), compareIDs('b', 3)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	want := `{"selected":{"requested":2,"read":1},"baseline":{"requested":3,"read":2},"attributes":0,"partial":true,"differences":[],"truncated":false}`
	if got := strings.TrimSpace(rec.Body.String()); got != want {
		t.Fatalf("body =\n%s\nwant\n%s", got, want)
	}
}

func TestTraceCompareRejectsBadRequests(t *testing.T) {
	hn, ds := newCompareHarness(t)
	with := func(edit func(map[string]any)) map[string]any {
		body := traceCompareBody(compareIDs('a', 2), compareIDs('b', 2))
		edit(body)
		return body
	}
	tests := []struct {
		name    string
		uid     string
		body    map[string]any
		status  int
		message string
	}{
		{"empty selected", "tempo", with(func(b map[string]any) { b["selected"] = []string{} }), http.StatusBadRequest, "selected must hold from 1 to 50 trace ids"},
		{"missing baseline", "tempo", with(func(b map[string]any) { delete(b, "baseline") }), http.StatusBadRequest, "baseline must hold from 1 to 150 trace ids"},
		{"51 selected", "tempo", with(func(b map[string]any) { b["selected"] = compareIDs('a', 51) }), http.StatusBadRequest, "selected must hold from 1 to 50 trace ids"},
		{"151 baseline", "tempo", with(func(b map[string]any) { b["baseline"] = compareIDs('b', 151) }), http.StatusBadRequest, "baseline must hold from 1 to 150 trace ids"},
		{"overlap", "tempo", with(func(b map[string]any) { b["baseline"] = []string{strings.ToUpper(compareIDs('a', 1)[0])} }), http.StatusBadRequest, "a trace id cannot be in both selected and baseline"},
		{"bad id", "tempo", with(func(b map[string]any) { b["baseline"] = []string{"xyz"} }), http.StatusBadRequest, "baseline must hold trace ids of 16 or 32 hex characters"},
		{"limit zero", "tempo", with(func(b map[string]any) { b["limit"] = 0 }), http.StatusBadRequest, "limit must be from 1 to 100"},
		{"limit over 100", "tempo", with(func(b map[string]any) { b["limit"] = 101 }), http.StatusBadRequest, "limit must be from 1 to 100"},
		{"missing to", "tempo", with(func(b map[string]any) { delete(b, "to") }), http.StatusBadRequest, "from and to are required"},
		{"bad time", "tempo", with(func(b map[string]any) { b["from"] = "yesterday-ish" }), http.StatusBadRequest, ""},
		{"unknown field", "tempo", with(func(b map[string]any) { b["query"] = "{}" }), http.StatusBadRequest, ""},
		{"a Prometheus datasource", "prom", with(func(map[string]any) {}), http.StatusBadRequest, traceCompareSourcesMessage},
		{"an unknown datasource", "missing", with(func(map[string]any) {}), http.StatusNotFound, "datasource not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := hn.traceCompare(tt.uid, tt.body)
			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.status, rec.Body.String())
			}
			body := decodeError(t, rec)
			if body.Ref != "" {
				t.Fatalf("error carries ref %q", body.Ref)
			}
			if tt.message != "" && body.Error != tt.message {
				t.Fatalf("error = %q, want %q", body.Error, tt.message)
			}
		})
	}
	if calls := len(ds.attributeCalls()); calls != 0 {
		t.Fatalf("%d upstream calls for invalid requests", calls)
	}
	if hn.heldSlots() != 0 {
		t.Fatal("a slot stayed held")
	}
}

func TestTraceCompareMapsUpstreamErrorsLikeExplore(t *testing.T) {
	hn, ds := newCompareHarness(t)
	tests := []struct {
		kind   error
		status int
		code   string
	}{
		{datasource.ErrUnavailable, http.StatusBadGateway, "datasource_unavailable"},
		{datasource.ErrTimeout, http.StatusGatewayTimeout, "query_timeout"},
	}
	for _, tc := range tests {
		ds.onAttributes = func(context.Context, datasource.TraceAttributesRequest) ([]datasource.TraceAttributeSet, error) {
			return nil, datasource.Errorf(tc.kind, "upstream said no")
		}
		rec := hn.traceCompare("tempo", traceCompareBody(compareIDs('a', 1), compareIDs('b', 1)))
		expectError(t, rec, tc.status, tc.code)
		if ref := decodeError(t, rec).Ref; ref != "" {
			t.Fatalf("error carries ref %q", ref)
		}
	}
}

func TestTraceCompareAnswersTooManyRequestsWhenEverySlotIsHeld(t *testing.T) {
	hn, ds := newCompareHarness(t)
	holdPrincipalSlots(t, hn, "alice")
	expectError(t, hn.traceCompare("tempo", traceCompareBody(compareIDs('a', 1), compareIDs('b', 1))), http.StatusTooManyRequests, "too_many_requests")
	if calls := len(ds.attributeCalls()); calls != 0 {
		t.Fatalf("%d upstream calls while every slot is held", calls)
	}
}
