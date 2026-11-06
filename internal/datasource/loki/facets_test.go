package loki_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/loki"
)

const (
	volumePath         = "/loki/api/v1/index/volume"
	detectedFieldsPath = "/loki/api/v1/detected_fields"
)

type facetLoki struct {
	mu       sync.Mutex
	requests []request
	detected int
	volumes  map[string]string
	streams  string
}

func (f *facetLoki) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, request{path: r.URL.Path, params: r.URL.Query()})
	f.mu.Unlock()
	switch r.URL.Path {
	case labelsPath:
		_, _ = w.Write([]byte(labelList("__stream_shard__", "service_name", "k8s_pod_name")))
	case volumePath:
		_, _ = w.Write([]byte(f.volumes[r.URL.Query().Get("targetLabels")]))
	case detectedFieldsPath:
		if f.detected == http.StatusNotFound {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"fields":[{"label":"http_request_method","type":"string","cardinality":2,"parsers":null},{"label":"detected_level","type":"string","cardinality":3,"parsers":null}],"limit":50}`))
	case rangePath:
		_, _ = w.Write([]byte(f.streams))
	default:
		http.NotFound(w, r)
	}
}

func (f *facetLoki) to(path string) []request {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []request
	for _, r := range f.requests {
		if r.path == path {
			out = append(out, r)
		}
	}
	return out
}

type volumeEntry struct{ value, bytes string }

func volumeVector(label string, smallestFirst ...volumeEntry) string {
	var entries []string
	for _, e := range smallestFirst {
		entries = append(entries, `{"metric":{"`+label+`":"`+e.value+`"},"value":[1793984001,"`+e.bytes+`"]}`)
	}
	return success("vector", "["+strings.Join(entries, ",")+"]")
}

func serveFacets(t *testing.T) (*loki.Datasource, *facetLoki) {
	t.Helper()
	fake := &facetLoki{
		volumes: map[string]string{
			"service_name": volumeVector("service_name", volumeEntry{"checkout", "9931"}, volumeEntry{"frontend", "3218239"}),
			"k8s_pod_name": volumeVector("k8s_pod_name", volumeEntry{"frontend-0", "3218239"}),
		},
		streams: success("streams", `[
			{"stream":{"service_name":"frontend","http_request_method":"POST","detected_level":"info"},"values":[["1793966400000000003","a"],["1793966400000000002","b"],["1793966400000000001","c"]]},
			{"stream":{"service_name":"checkout","http_request_method":"GET"},"values":[["1793966400000000004","d"]]},
			{"stream":{"service_name":"checkout","http_request_method":"POST","detected_level":"error"},"values":[["1793966400000000006","e"],["1793966400000000005","f"]]}
		]`),
	}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	ds, err := loki.New(loki.Options{URL: srv.URL})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = ds.Close() })
	return ds, fake
}

func facetRequest(query string, fields []string, limit int) datasource.FacetRequest {
	return datasource.FacetRequest{Query: datasource.Query{Text: query}, Range: lastHour, Fields: fields, Limit: limit, Sample: 100}
}

func fieldNamed(t *testing.T, f datasource.Facets, name string) datasource.FacetField {
	t.Helper()
	for _, field := range f.Fields {
		if field.Name == name {
			return field
		}
	}
	t.Fatalf("no field %q in %+v", name, f.Fields)
	return datasource.FacetField{}
}

func TestLogFacetsCountsIndexLabelsFromTheVolumeAPI(t *testing.T) {
	ds, fake := serveFacets(t)
	got, err := ds.LogFacets(context.Background(), facetRequest(`{service_name=~".+"} |= "x"`, []string{"service_name"}, 1))
	if err != nil {
		t.Fatal(err)
	}
	if got.Sampled || got.LinesSampled != 0 || got.Truncated || len(got.Fields) != 1 {
		t.Fatalf("facets = %+v, want one unsampled label field", got)
	}
	want := datasource.FacetField{Name: "service_name", Kind: "label", Exact: true, Unit: "bytes", Cardinality: 2, Truncated: true, Values: []datasource.FacetValue{{Value: "frontend", Count: 3218239}}}
	if !reflect.DeepEqual(got.Fields[0], want) {
		t.Fatalf("field = %+v, want %+v", got.Fields[0], want)
	}
	labels := fake.to(labelsPath)
	if len(labels) != 1 || labels[0].params.Get("query") != `{service_name=~".+"}` || window(t, labels[0]) != lastHour.To.Sub(lastHour.From) {
		t.Fatalf("labels requests = %+v", labels)
	}
	volumes := fake.to(volumePath)
	if len(volumes) != 1 {
		t.Fatalf("%d volume requests, want 1 for the one requested label", len(volumes))
	}
	wantParams := url.Values{"query": {`{service_name=~".+"}`}, "targetLabels": {"service_name"}, "limit": {"2"}, "start": {"2026-11-06T11:00:00Z"}, "end": {"2026-11-06T12:00:00Z"}}
	if !reflect.DeepEqual(volumes[0].params, wantParams) {
		t.Fatalf("volume params = %v, want %v", volumes[0].params, wantParams)
	}
	if n := len(fake.to(rangePath)) + len(fake.to(detectedFieldsPath)); n != 0 {
		t.Fatalf("%d sample requests for a request that asked only for index labels", n)
	}
}

func TestLogFacetsSamplesDetectedFields(t *testing.T) {
	ds, fake := serveFacets(t)
	query := `{service_name=~".+"} | json`
	got, err := ds.LogFacets(context.Background(), facetRequest(query, nil, 10))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Sampled || got.LinesSampled != 6 || got.Truncated {
		t.Fatalf("sampled %v, lines_sampled %d, truncated %v", got.Sampled, got.LinesSampled, got.Truncated)
	}
	var names []string
	for _, f := range got.Fields {
		names = append(names, f.Name)
	}
	if want := []string{"k8s_pod_name", "service_name", "http_request_method", "detected_level"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("fields = %v, want %v", names, want)
	}
	method := fieldNamed(t, got, "http_request_method")
	wantMethod := datasource.FacetField{Name: "http_request_method", Kind: "field", Cardinality: 2, Values: []datasource.FacetValue{{Value: "POST", Count: 5}, {Value: "GET", Count: 1}}}
	if !reflect.DeepEqual(method, wantMethod) {
		t.Fatalf("http_request_method = %+v, want %+v", method, wantMethod)
	}
	level := fieldNamed(t, got, "detected_level")
	if level.Exact || level.Cardinality != 3 || !reflect.DeepEqual(level.Values, []datasource.FacetValue{{Value: "info", Count: 3}, {Value: "error", Count: 2}}) {
		t.Fatalf("detected_level = %+v, want the cardinality of detected_fields and the counts of the sample", level)
	}
	pod := fieldNamed(t, got, "k8s_pod_name")
	if !pod.Exact || pod.Unit != "bytes" || pod.Cardinality != 1 || pod.Truncated {
		t.Fatalf("k8s_pod_name = %+v", pod)
	}
	detected := fake.to(detectedFieldsPath)
	if len(detected) != 1 || detected[0].params.Get("query") != query || detected[0].params.Get("limit") != "50" {
		t.Fatalf("detected_fields requests = %+v", detected)
	}
	sample := fake.to(rangePath)
	if len(sample) != 1 || sample[0].params.Get("query") != query || sample[0].params.Get("limit") != "100" || sample[0].params.Get("direction") != "backward" {
		t.Fatalf("sample requests = %+v", sample)
	}
	if n := len(fake.to(volumePath)); n != 2 {
		t.Fatalf("%d volume requests, want one per index label", n)
	}
}

func TestLogFacetsFallsBackWhenDetectedFieldsIs404(t *testing.T) {
	ds, fake := serveFacets(t)
	fake.detected = http.StatusNotFound
	got, err := ds.LogFacets(context.Background(), facetRequest(`{service_name=~".+"}`, nil, 1))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range got.Fields {
		names = append(names, f.Name)
	}
	if want := []string{"k8s_pod_name", "service_name", "detected_level", "http_request_method"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("fields = %v, want the sample's labels minus the index labels", names)
	}
	method := fieldNamed(t, got, "http_request_method")
	if method.Cardinality != 2 || !method.Truncated || !reflect.DeepEqual(method.Values, []datasource.FacetValue{{Value: "POST", Count: 5}}) {
		t.Fatalf("http_request_method = %+v, want the sample's cardinality cut to the limit", method)
	}
}

func TestLogFacetsRefusesAMetricQuery(t *testing.T) {
	ds, fake := serveFacets(t)
	_, err := ds.LogFacets(context.Background(), facetRequest(`rate({service_name="checkout"}[1m])`, nil, 10))
	if !errors.Is(err, datasource.ErrInvalidQuery) || !strings.Contains(err.Error(), "facets need a log query that starts with a stream selector") {
		t.Fatalf("err = %v", err)
	}
	if len(fake.requests) != 0 {
		t.Fatalf("%d requests reached Loki for a metric query", len(fake.requests))
	}
}

func TestSelectorOfStopsAtTheFirstPipe(t *testing.T) {
	ds, fake := serveFacets(t)
	if _, err := ds.LogFacets(context.Background(), facetRequest(`{a="b"} |= "x | y" | json`, []string{"service_name"}, 10)); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{labelsPath, volumePath} {
		if got := fake.to(path)[0].params.Get("query"); got != `{a="b"}` {
			t.Fatalf("%s query = %q, want the selector before the first pipe outside a string", path, got)
		}
	}
}
