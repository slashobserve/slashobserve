package prometheus_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/prometheus"
)

func servicesDS(t *testing.T, labels string, values map[string]string) (*prometheus.Datasource, *[]string) {
	t.Helper()
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path+"?"+r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/labels" {
			_, _ = w.Write([]byte(`{"status":"success","data":` + labels + `}`))
			return
		}
		name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/label/"), "/values")
		body, ok := values[name]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"status":"success","data":` + body + `}`))
	}))
	t.Cleanup(srv.Close)
	ds, err := prometheus.New(prometheus.Options{URL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ds.Close() })
	return ds, &paths
}

func servicesAsk() datasource.ServiceRequest {
	to := time.Unix(1_800_000_000, 0)
	return datasource.ServiceRequest{Range: datasource.TimeRange{From: to.Add(-15 * time.Minute), To: to}, Limit: 50}
}

func TestServicesReadsTheFirstServiceLabelInOrderOverTheWindow(t *testing.T) {
	tests := []struct {
		name   string
		labels string
		field  string
	}{
		{"service_name over the rest", `["app","job","service","service_name"]`, "service_name"},
		{"service over app and job", `["app","job","service"]`, "service"},
		{"app over job", `["app","job","pod"]`, "app"},
		{"job alone", `["instance","job"]`, "job"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values := map[string]string{tt.field: `["shop","checkout","shop"]`}
			ds, paths := servicesDS(t, tt.labels, values)
			got, err := ds.Services(context.Background(), servicesAsk())
			if err != nil {
				t.Fatal(err)
			}
			want := datasource.ServiceList{Signal: "metrics", Field: tt.field, Values: []string{"checkout", "shop"}}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("Services = %+v, want %+v", got, want)
			}
			if len(*paths) != 2 {
				t.Fatalf("requests = %v, want the label list and one label's values", *paths)
			}
			query, err := url.ParseQuery(strings.SplitN((*paths)[1], "?", 2)[1])
			if err != nil || query.Get("start") != "1799999100.000" || query.Get("end") != "1800000000.000" {
				t.Fatalf("query = %v, err = %v, want the window", query, err)
			}
		})
	}
}

func TestServicesSkipsAServiceLabelWithoutValues(t *testing.T) {
	ds, _ := servicesDS(t, `["job","service"]`, map[string]string{"service": `[]`, "job": `["shop"]`})
	got, err := ds.Services(context.Background(), servicesAsk())
	if err != nil {
		t.Fatal(err)
	}
	if got.Field != "job" || !reflect.DeepEqual(got.Values, []string{"shop"}) {
		t.Fatalf("Services = %+v, want job with shop", got)
	}
}

func TestServicesWithNoServiceLabelIsEmptyWithNoField(t *testing.T) {
	ds, paths := servicesDS(t, `["instance","pod"]`, nil)
	got, err := ds.Services(context.Background(), servicesAsk())
	if err != nil {
		t.Fatal(err)
	}
	if got.Field != "" || len(got.Values) != 0 || got.Signal != "metrics" || len(*paths) != 1 {
		t.Fatalf("Services = %+v after %v", got, *paths)
	}
}

func TestServicesCutsTheListAtTheLimit(t *testing.T) {
	ds, _ := servicesDS(t, `["job"]`, map[string]string{"job": `["a","b","c"]`})
	req := servicesAsk()
	req.Limit = 2
	got, err := ds.Services(context.Background(), req)
	if err != nil || !reflect.DeepEqual(got.Values, []string{"a", "b"}) {
		t.Fatalf("Services = %+v, err = %v", got, err)
	}
}

func TestServicesRefusesLogsAndTraces(t *testing.T) {
	ds, _ := servicesDS(t, `[]`, nil)
	for _, signal := range []string{"logs", "traces"} {
		req := servicesAsk()
		req.Signal = signal
		if _, err := ds.Services(context.Background(), req); !errors.Is(err, datasource.ErrInvalidQuery) {
			t.Fatalf("%s: err = %v, want ErrInvalidQuery", signal, err)
		}
	}
}
