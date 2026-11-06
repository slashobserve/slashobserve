package elasticsearch_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/elasticsearch"
)

func facetsFixture(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/facets.json")
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func fieldCapsOf(t *testing.T, types map[string]string) string {
	t.Helper()
	fields := map[string]any{}
	for name, typ := range types {
		fields[name] = map[string]any{typ: map[string]any{"type": typ, "metadata_field": false, "searchable": true, "aggregatable": typ != "text"}}
	}
	raw, err := json.Marshal(map[string]any{"fields": fields})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func facetsServer(t *testing.T, caps, reply string) (*fakeServer, *elasticsearch.Datasource) {
	t.Helper()
	f := healthyServer(t)
	if caps != "" {
		f.fixed(http.MethodPost, "/logs-*/_field_caps", http.StatusOK, caps)
	}
	f.fixed(http.MethodPost, searchPath, http.StatusOK, reply)
	return f, serve(t, f)
}

func facetRequest(text string, limit int, fields ...string) datasource.FacetRequest {
	return datasource.FacetRequest{Query: datasource.Query{Text: text}, Range: queryRange, Fields: fields, Limit: limit}
}

func aggregationsOf(t *testing.T, r request) map[string]any {
	t.Helper()
	aggs, ok := searchBody(t, r)["aggs"].(map[string]any)
	if !ok {
		t.Fatalf("body %s has no aggs", r.body)
	}
	return aggs
}

func termsOf(t *testing.T, aggs map[string]any, name string) map[string]any {
	t.Helper()
	agg, ok := aggs[name].(map[string]any)
	if !ok {
		t.Fatalf("no aggregation %q in %v", name, aggs)
	}
	terms, _ := agg["terms"].(map[string]any)
	return terms
}

func facetNames(facets datasource.Facets) []string {
	names := make([]string, len(facets.Fields))
	for i, f := range facets.Fields {
		names[i] = f.Name
	}
	return names
}

func facetByName(t *testing.T, facets datasource.Facets, name string) datasource.FacetField {
	t.Helper()
	for _, f := range facets.Fields {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("no facet %q in %v", name, facetNames(facets))
	return datasource.FacetField{}
}

func TestLogFacetsDefaultsPutTheLevelAndServiceFirstAndCountMessageThroughItsKeyword(t *testing.T) {
	f, ds := facetsServer(t, "", facetsFixture(t))
	facets, err := ds.LogFacets(context.Background(), facetRequest("service.name:checkout", 5))
	if err != nil {
		t.Fatal(err)
	}
	wantNames := []string{"log.level", "service.name", "host.ip", "http.response.status_code", "message", "span.id", "tags", "trace.id"}
	if got := facetNames(facets); !slices.Equal(got, wantNames) {
		t.Fatalf("fields = %v, want %v", got, wantNames)
	}
	sent := searchBody(t, f.last(t))
	if sent["size"] != 0.0 || sent["track_total_hits"] != false {
		t.Fatalf("body = %v, want size 0 and no hit count", sent)
	}
	assertJSONEqual(t, mustJSON(t, sent["query"]), `{"bool":{"filter":[`+rangeFilter+`,{"match":{"service.name":"checkout"}}]}}`)
	aggs := aggregationsOf(t, f.last(t))
	if len(aggs) != 2*len(wantNames) {
		t.Fatalf("%d aggregations, want %d", len(aggs), 2*len(wantNames))
	}
	for name, field := range map[string]string{"log.level": "log.level", "message": "message.keyword", "http.response.status_code": "http.response.status_code"} {
		terms := termsOf(t, aggs, name)
		if terms["field"] != field || terms["size"] != 5.0 {
			t.Errorf("terms of %s = %v, want field %s and size 5", name, terms, field)
		}
		card := aggs[name+"__card"].(map[string]any)["cardinality"].(map[string]any)
		if card["field"] != field {
			t.Errorf("cardinality of %s = %v", name, card)
		}
	}
	if facets.Sampled || facets.LinesSampled != 0 || facets.Truncated {
		t.Fatalf("facets = %+v, want exact and untruncated", facets)
	}
	level := facetByName(t, facets, "log.level")
	wantLevelValues := []datasource.FacetValue{{Value: "info", Count: 100}, {Value: "warn", Count: 50}, {Value: "error", Count: 50}}
	if level.Kind != "keyword" || !level.Exact || level.Cardinality != 3 || level.Truncated || !slices.Equal(level.Values, wantLevelValues) {
		t.Fatalf("log.level = %+v", level)
	}
	status := facetByName(t, facets, "http.response.status_code")
	if status.Kind != "number" || status.Values[0] != (datasource.FacetValue{Value: "200", Count: 50}) {
		t.Fatalf("status = %+v", status)
	}
	if ip := facetByName(t, facets, "host.ip"); ip.Kind != "ip" || !ip.Truncated || !ip.Exact || ip.Cardinality != 200 {
		t.Fatalf("host.ip = %+v", ip)
	}
	if message := facetByName(t, facets, "message"); message.Kind != "text" || message.Exact || !message.Truncated || message.Values[0].Value != "request 0 handled" {
		t.Fatalf("message = %+v", message)
	}
}

func TestLogFacetsOfRequestedFieldsSkipWhatCannotBeCounted(t *testing.T) {
	f, ds := facetsServer(t, "", facetsFixture(t))
	facets, err := ds.LogFacets(context.Background(), facetRequest("", 0, "error.stack_trace", "nosuch", "service.name", "http", "event.sequence", "message.keyword"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := facetNames(facets), []string{"error.stack_trace", "nosuch", "service.name", "http", "event.sequence", "message.keyword"}; !slices.Equal(got, want) {
		t.Fatalf("fields = %v, want %v", got, want)
	}
	for name, kind := range map[string]string{"error.stack_trace": "text", "nosuch": "unknown", "http": "object", "event.sequence": "keyword"} {
		field := facetByName(t, facets, name)
		if field.Kind != kind || field.Exact || len(field.Values) != 0 || field.Truncated || field.Values == nil {
			t.Errorf("%s = %+v, want kind %s and no values", name, field, kind)
		}
	}
	if service := facetByName(t, facets, "service.name"); !service.Exact || len(service.Values) != 3 {
		t.Fatalf("service.name = %+v", service)
	}
	aggs := aggregationsOf(t, f.last(t))
	if len(aggs) != 4 || termsOf(t, aggs, "service.name")["size"] != 10.0 || termsOf(t, aggs, "message.keyword")["field"] != "message.keyword" {
		t.Fatalf("aggs = %v, want counts for service.name and message.keyword only, size 10", aggs)
	}
}

func TestLogFacetsSendsNoSearchWhenNothingCanBeCounted(t *testing.T) {
	f, ds := facetsServer(t, "", facetsFixture(t))
	facets, err := ds.LogFacets(context.Background(), facetRequest("", 5, "error.stack_trace", "nosuch"))
	if err != nil || len(facets.Fields) != 2 {
		t.Fatalf("facets = %+v, err = %v", facets, err)
	}
	if f.count(searchPath) != 0 {
		t.Fatal("a request with nothing to count reached the search endpoint")
	}
}

func TestLogFacetsReadsBooleanFlattenedAndNumericKeys(t *testing.T) {
	caps := fieldCapsOf(t, map[string]string{"@timestamp": "date_nanos", "retry": "boolean", "labels": "flattened"})
	const reply = `{"timed_out":false,"_shards":{"failed":0},"aggregations":{
		"retry":{"doc_count_error_upper_bound":0,"sum_other_doc_count":0,"buckets":[{"key":1,"key_as_string":"true","doc_count":4},{"key":0,"key_as_string":"false","doc_count":1}]},
		"retry__card":{"value":2},
		"labels":{"doc_count_error_upper_bound":0,"sum_other_doc_count":0,"buckets":[{"key":"eu","doc_count":5}]},
		"labels__card":{"value":1}
	}}`
	f, ds := facetsServer(t, caps, reply)
	facets, err := ds.LogFacets(context.Background(), facetRequest("", 5, "retry", "labels"))
	if err != nil {
		t.Fatal(err)
	}
	retry := facetByName(t, facets, "retry")
	if retry.Kind != "boolean" || retry.Values[0] != (datasource.FacetValue{Value: "true", Count: 4}) || retry.Values[1].Value != "false" {
		t.Fatalf("retry = %+v", retry)
	}
	if labels := facetByName(t, facets, "labels"); labels.Kind != "flattened" || labels.Values[0].Value != "eu" {
		t.Fatalf("labels = %+v", labels)
	}
	if termsOf(t, aggregationsOf(t, f.last(t)), "labels")["field"] != "labels" {
		t.Fatalf("body = %s", f.last(t).body)
	}
	defaults, err := ds.LogFacets(context.Background(), facetRequest("", 5))
	if err != nil {
		t.Fatal(err)
	}
	if got := facetNames(defaults); !slices.Equal(got, []string{"retry"}) {
		t.Fatalf("default fields = %v, want the boolean only", got)
	}
}

func TestLogFacetsDefaultsAskForAtMostTwentyFields(t *testing.T) {
	types := map[string]string{"@timestamp": "date_nanos"}
	for i := range 30 {
		types[fmt.Sprintf("field%02d", i)] = "keyword"
	}
	f, ds := facetsServer(t, fieldCapsOf(t, types), `{"timed_out":false,"_shards":{"failed":0},"aggregations":{}}`)
	facets, err := ds.LogFacets(context.Background(), facetRequest("", 5))
	if err != nil {
		t.Fatal(err)
	}
	if len(facets.Fields) != 20 || !facets.Truncated || facets.Fields[0].Name != "field00" || facets.Fields[19].Name != "field19" {
		t.Fatalf("%d fields, truncated = %v, first = %s", len(facets.Fields), facets.Truncated, facets.Fields[0].Name)
	}
	if aggs := aggregationsOf(t, f.last(t)); len(aggs) != 40 {
		t.Fatalf("%d aggregations, want 40 (20 terms and 20 cardinalities)", len(aggs))
	}
}

func TestLogFacetsRefusesDSLAggregationsAndMarksPartialAnswersInexact(t *testing.T) {
	f, ds := facetsServer(t, "", facetsFixture(t))
	_, err := ds.LogFacets(context.Background(), facetRequest(`{"aggs":{"by":{"terms":{"field":"log.level"}}}}`, 5))
	if !errors.Is(err, datasource.ErrInvalidQuery) || !strings.Contains(err.Error(), "facets need a KQL, Lucene or filter-only query body") {
		t.Fatalf("error = %v", err)
	}
	if f.count(searchPath) != 0 {
		t.Fatal("a refused query reached the search endpoint")
	}
	partial := strings.Replace(facetsFixture(t), `"failed": 0`, `"failed": 1`, 1)
	_, ds = facetsServer(t, "", partial)
	facets, err := ds.LogFacets(context.Background(), facetRequest("", 5))
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range facets.Fields {
		if field.Exact {
			t.Errorf("%s is exact on a partial answer", field.Name)
		}
	}
}

func TestLogFacetsMapsUpstreamQueryErrors(t *testing.T) {
	f := healthyServer(t)
	f.fixed(http.MethodPost, searchPath, http.StatusBadRequest, esError("parsing_exception", "bad"))
	ds := serve(t, f)
	if _, err := ds.LogFacets(context.Background(), facetRequest("", 5)); !errors.Is(err, datasource.ErrInvalidQuery) {
		t.Fatalf("error = %v", err)
	}
}
