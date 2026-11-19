package loki_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/loki"
)

type structuredLoki struct {
	mu       sync.Mutex
	requests []request
	detected string
}

func (f *structuredLoki) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, request{path: r.URL.Path, params: r.URL.Query()})
	f.mu.Unlock()
	switch r.URL.Path {
	case labelsPath:
		_, _ = w.Write([]byte(labelList("service_name", "k8s_pod_name")))
	case "/loki/api/v1/label/detected_level/values":
		_, _ = w.Write([]byte(`{"status":"success"}`))
	case "/loki/api/v1/detected_field/detected_level/values":
		if f.detected == "" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(f.detected))
	default:
		http.NotFound(w, r)
	}
}

func structuredDatasource(t *testing.T, detected string) (*loki.Datasource, *structuredLoki) {
	t.Helper()
	fake := &structuredLoki{detected: detected}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	ds, err := loki.New(loki.Options{URL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	return ds, fake
}

func TestDiscoverReadsStructuredMetadataValuesWhenTheLabelAPIHasNone(t *testing.T) {
	ds, fake := structuredDatasource(t, `{"values":["warn","info"],"limit":50}`)
	got, err := ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverLabelValues, Label: "detected_level"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.Values, []string{"info", "warn"}) {
		t.Fatalf("values = %v, want the detected field values", got.Values)
	}
	var detected []request
	for _, r := range fake.requests {
		if r.path == "/loki/api/v1/detected_field/detected_level/values" {
			detected = append(detected, r)
		}
	}
	if len(detected) != 1 || detected[0].params.Get("query") != `{service_name=~".+"}` {
		t.Fatalf("detected field requests = %+v, want one over a service_name selector", detected)
	}
}

func TestDiscoverKeepsTheSelectorOfAStructuredMetadataRead(t *testing.T) {
	ds, fake := structuredDatasource(t, `{"values":["error"]}`)
	_, err := ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverLabelValues, Label: "detected_level", Metric: `{service_name="checkout"}`})
	if err != nil {
		t.Fatal(err)
	}
	last := fake.requests[len(fake.requests)-1]
	if last.params.Get("query") != `{service_name="checkout"}` {
		t.Fatalf("query = %q", last.params.Get("query"))
	}
}

func TestDiscoverAnswersNoValuesWhenLokiHasNoDetectedFieldEndpoint(t *testing.T) {
	ds, _ := structuredDatasource(t, "")
	got, err := ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverLabelValues, Label: "detected_level"})
	if err != nil || len(got.Values) != 0 {
		t.Fatalf("values = %+v, %v; want none", got, err)
	}
}
