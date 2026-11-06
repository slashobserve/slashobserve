package proxyhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

type facetDatasource struct {
	*fakeDatasource
	kind     spec.DatasourceType
	onFacets func(ctx context.Context, req datasource.FacetRequest) (datasource.Facets, error)

	mu       sync.Mutex
	requests []datasource.FacetRequest
}

func (d *facetDatasource) Type() string { return string(d.kind) }

func (d *facetDatasource) LogFacets(ctx context.Context, req datasource.FacetRequest) (datasource.Facets, error) {
	d.mu.Lock()
	d.requests = append(d.requests, req)
	d.mu.Unlock()
	if d.onFacets != nil {
		return d.onFacets(ctx, req)
	}
	return datasource.Facets{}, nil
}

func (d *facetDatasource) facetRequests() []datasource.FacetRequest {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.requests)
}

func newFacetsHarness(t *testing.T) (*harness, *facetDatasource) {
	t.Helper()
	hn := newHarness(t)
	lokiDS := &facetDatasource{fakeDatasource: &fakeDatasource{}, kind: spec.Loki}
	registry, err := datasource.NewRegistry(hn.h.logger,
		datasource.Instance{UID: "prom", Datasource: hn.ds},
		datasource.Instance{UID: "logs", Datasource: lokiDS},
	)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	hn.runner = queryproxy.NewRunner(registry, hn.h.logger)
	hn.h.runner, hn.h.registry = hn.runner, registry
	return hn, lokiDS
}

func (hn *harness) facets(uid string, body map[string]any) *httptest.ResponseRecorder {
	hn.t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		hn.t.Fatalf("marshal facets body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/datasources/"+uid+"/log-facets", bytes.NewReader(encoded))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Kind", string(auth.KindUser))
	return hn.serve(req, "alice")
}

func facetsBody(query string) map[string]any {
	return map[string]any{"query": query, "from": volumeFrom, "to": volumeTo}
}

func decodeFacets(t *testing.T, rec *httptest.ResponseRecorder) datasource.Facets {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	var body datasource.Facets
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("facets body: %v", err)
	}
	return body
}

func TestFacetsReturnTheSortedFieldsOfTheDatasource(t *testing.T) {
	hn, lokiDS := newFacetsHarness(t)
	lokiDS.onFacets = func(context.Context, datasource.FacetRequest) (datasource.Facets, error) {
		return datasource.Facets{Sampled: true, LinesSampled: 1000, Fields: []datasource.FacetField{
			{Name: "http_request_method", Kind: "field", Cardinality: 2, Values: []datasource.FacetValue{{Value: "GET", Count: 388}, {Value: "POST", Count: 612}}},
			{Name: "service_name", Kind: "label", Exact: true, Unit: "bytes", Cardinality: 3, Values: []datasource.FacetValue{{Value: "checkout", Count: 9931}, {Value: "frontend", Count: 48210}}},
		}}, nil
	}
	body := facetsBody(`{app="shop"} |= "x"`)
	body["fields"] = []string{"service_name", "http_request_method"}
	body["limit"] = 1
	body["sample"] = 500
	got := decodeFacets(t, hn.facets("logs", body))
	if !got.Sampled || got.LinesSampled != 1000 || got.Truncated || len(got.Fields) != 2 {
		t.Fatalf("facets = %+v", got)
	}
	service := got.Fields[0]
	if service.Name != "service_name" || !service.Exact || service.Unit != "bytes" || !service.Truncated || !slices.Equal(service.Values, []datasource.FacetValue{{Value: "frontend", Count: 48210}}) {
		t.Fatalf("first field = %+v, want the exact label with its top value", service)
	}
	if method := got.Fields[1]; method.Name != "http_request_method" || !slices.Equal(method.Values, []datasource.FacetValue{{Value: "POST", Count: 612}}) {
		t.Fatalf("second field = %+v", method)
	}
	calls := lokiDS.facetRequests()
	if len(calls) != 1 {
		t.Fatalf("%d facet calls, want 1", len(calls))
	}
	req := calls[0]
	if req.Query.Text != `{app="shop"} |= "x"` || req.Query.Limits != queryproxy.DraftLimits || req.Query.User.ID != "alice" {
		t.Fatalf("query = %+v", req.Query)
	}
	if !req.Range.From.Equal(volumeStart) || !req.Range.To.Equal(volumeStart.Add(time.Hour)) {
		t.Fatalf("range = %v to %v", req.Range.From, req.Range.To)
	}
	if !slices.Equal(req.Fields, []string{"service_name", "http_request_method"}) || req.Limit != 1 || req.Sample != 500 {
		t.Fatalf("fields %v, limit %d, sample %d", req.Fields, req.Limit, req.Sample)
	}
}

func TestFacetsDefaultsAndJSONKeys(t *testing.T) {
	hn, lokiDS := newFacetsHarness(t)
	lokiDS.onFacets = func(context.Context, datasource.FacetRequest) (datasource.Facets, error) {
		return datasource.Facets{Fields: []datasource.FacetField{{Name: "level", Kind: "label", Exact: true, Values: []datasource.FacetValue{{Value: "error", Count: 1}}}}}, nil
	}
	rec := hn.facets("logs", facetsBody(`{app="shop"}`))
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("body %s: %v", rec.Body.String(), err)
	}
	if got := slices.Sorted(maps.Keys(raw)); !slices.Equal(got, []string{"fields", "lines_sampled", "sampled", "truncated"}) {
		t.Fatalf("keys = %v", got)
	}
	var fields []map[string]json.RawMessage
	if err := json.Unmarshal(raw["fields"], &fields); err != nil || len(fields) != 1 {
		t.Fatalf("fields %s: %v", raw["fields"], err)
	}
	if got := slices.Sorted(maps.Keys(fields[0])); !slices.Equal(got, []string{"exact", "kind", "name", "truncated", "values"}) {
		t.Fatalf("field keys = %v", got)
	}
	req := lokiDS.facetRequests()[0]
	if req.Fields != nil || req.Limit != 10 || req.Sample != 1000 {
		t.Fatalf("defaults: fields %v, limit %d, sample %d", req.Fields, req.Limit, req.Sample)
	}
	lokiDS.onFacets = nil
	if got := decodeFacets(t, hn.facets("logs", facetsBody(`{app="shop"}`))); got.Fields == nil || len(got.Fields) != 0 {
		t.Fatalf("an empty answer has fields %v, want []", got.Fields)
	}
}

func TestFacetsRejectsInvalidRequests(t *testing.T) {
	hn, lokiDS := newFacetsHarness(t)
	with := func(edit func(map[string]any)) map[string]any {
		body := facetsBody(`{app="shop"}`)
		edit(body)
		return body
	}
	tests := []struct {
		name string
		body map[string]any
		code string
	}{
		{"missing query", with(func(b map[string]any) { delete(b, "query") }), "invalid_request"},
		{"query over 64 KiB", with(func(b map[string]any) { b["query"] = strings.Repeat("x", 64<<10+1) }), "query_too_long"},
		{"missing to", with(func(b map[string]any) { delete(b, "to") }), "invalid_request"},
		{"from after to", with(func(b map[string]any) { b["from"], b["to"] = volumeTo, volumeFrom }), "invalid_request"},
		{"empty fields", with(func(b map[string]any) { b["fields"] = []string{} }), "invalid_request"},
		{"51 fields", with(func(b map[string]any) { b["fields"] = slices.Repeat([]string{"a"}, 51) }), "invalid_request"},
		{"empty field name", with(func(b map[string]any) { b["fields"] = []string{""} }), "invalid_request"},
		{"field over 200 bytes", with(func(b map[string]any) { b["fields"] = []string{strings.Repeat("a", 201)} }), "invalid_request"},
		{"limit zero", with(func(b map[string]any) { b["limit"] = 0 }), "invalid_request"},
		{"limit over 50", with(func(b map[string]any) { b["limit"] = 51 }), "invalid_request"},
		{"sample under 100", with(func(b map[string]any) { b["sample"] = 99 }), "invalid_request"},
		{"sample over 5000", with(func(b map[string]any) { b["sample"] = 5001 }), "invalid_request"},
		{"unknown field", with(func(b map[string]any) { b["buckets"] = 10 }), "invalid_request"},
		{"unknown timezone", with(func(b map[string]any) { b["timezone"] = "Mars/Olympus" }), "invalid_request"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expectError(t, hn.facets("logs", tt.body), http.StatusBadRequest, tt.code)
		})
	}
	if n := len(lokiDS.facetRequests()); n != 0 {
		t.Fatalf("%d facet calls for invalid requests", n)
	}
}

func TestFacetsRefusesOtherDatasourceTypesAndUnknownUIDs(t *testing.T) {
	hn, _ := newFacetsHarness(t)
	rec := hn.facets("prom", facetsBody("up"))
	expectError(t, rec, http.StatusBadRequest, "invalid_request")
	if got := decodeError(t, rec).Error; got != "facets read Loki and ClickHouse datasources" {
		t.Fatalf("error = %q", got)
	}
	expectError(t, hn.facets("missing", facetsBody("up")), http.StatusNotFound, "not_found")
	if held := hn.heldSlots(); held != 0 {
		t.Fatalf("%d slots still held", held)
	}
}

func TestFacetsErrorsCarryNoRef(t *testing.T) {
	hn, lokiDS := newFacetsHarness(t)
	lokiDS.onFacets = func(context.Context, datasource.FacetRequest) (datasource.Facets, error) {
		return datasource.Facets{}, datasource.Errorf(datasource.ErrInvalidQuery, "facets need a log query that starts with a stream selector")
	}
	rec := hn.facets("logs", facetsBody(`rate({app="shop"}[1m])`))
	expectError(t, rec, http.StatusUnprocessableEntity, "query_invalid")
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if _, has := raw["ref"]; has {
		t.Fatalf("error body carries ref: %s", rec.Body.String())
	}
	lokiDS.onFacets = func(context.Context, datasource.FacetRequest) (datasource.Facets, error) {
		return datasource.Facets{}, datasource.Errorf(datasource.ErrUnavailable, "connection failed")
	}
	expectError(t, hn.facets("logs", facetsBody(`{app="shop"}`)), http.StatusBadGateway, "datasource_unavailable")
}

func TestFacetsAnswersTooManyRequestsWhenEverySlotIsHeld(t *testing.T) {
	hn, lokiDS := newFacetsHarness(t)
	holdPrincipalSlots(t, hn, "alice")
	rec := hn.facets("logs", facetsBody(`{app="shop"}`))
	expectError(t, rec, http.StatusTooManyRequests, "too_many_requests")
	if got := rec.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q, want 1", got)
	}
	if n := len(lokiDS.facetRequests()); n != 0 {
		t.Fatalf("%d facet calls while every slot is held", n)
	}
}
