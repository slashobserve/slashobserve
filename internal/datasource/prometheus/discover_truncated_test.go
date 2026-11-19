package prometheus_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/prometheus"
)

func namesDS(t *testing.T, count int) *prometheus.Datasource {
	t.Helper()
	names := make([]string, count)
	for i := range names {
		names[i] = fmt.Sprintf("name_%04d", i)
	}
	body, err := json.Marshal(map[string]any{"status": "success", "data": names})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/metadata" {
			_, _ = w.Write([]byte(`{"status":"success","data":{}}`))
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	ds, err := prometheus.New(prometheus.Options{URL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ds.Close() })
	return ds
}

func TestDiscoverSaysWhenItCutAListAtTheLimit(t *testing.T) {
	kinds := map[string]datasource.DiscoverRequest{
		"metrics": {Kind: datasource.DiscoverMetrics},
		"labels":  {Kind: datasource.DiscoverLabels},
		"values":  {Kind: datasource.DiscoverLabelValues, Label: "job"},
	}
	for name, req := range kinds {
		t.Run(name, func(t *testing.T) {
			ds := namesDS(t, 300)
			req.Limit = 200
			cat, err := ds.Discover(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if got := len(cat.Metrics) + len(cat.Labels) + len(cat.Values); got != 200 || !cat.Truncated {
				t.Fatalf("%d names, truncated = %v; want 200 and true", got, cat.Truncated)
			}
			req.Limit = 300
			cat, err = ds.Discover(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if got := len(cat.Metrics) + len(cat.Labels) + len(cat.Values); got != 300 || cat.Truncated {
				t.Fatalf("%d names, truncated = %v; want 300 and false", got, cat.Truncated)
			}
		})
	}
}
