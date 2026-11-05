package prometheus_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/prometheus"
)

type inventoryServer struct {
	mu       sync.Mutex
	requests []recordedRequest
}

func (s *inventoryServer) to(path string) []recordedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []recordedRequest
	for _, r := range s.requests {
		if r.path == path {
			out = append(out, r)
		}
	}
	return out
}

func inventoryDS(t *testing.T, series []map[string]string, metadata string) (*prometheus.Datasource, *inventoryServer) {
	t.Helper()
	seen := &inventoryServer{}
	seriesBody, err := json.Marshal(map[string]any{"status": "success", "data": series})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.mu.Lock()
		seen.requests = append(seen.requests, recordedRequest{method: r.Method, path: r.URL.Path, form: r.URL.Query()})
		seen.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/series":
			_, _ = w.Write(seriesBody)
		case "/api/v1/metadata":
			_, _ = w.Write([]byte(`{"status":"success","data":` + metadata + `}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	ds, err := prometheus.New(prometheus.Options{URL: srv.URL})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = ds.Close() })
	return ds, seen
}

const shopMetadata = `{
  "http_requests_total":[{"type":"counter","help":"HTTP requests.","unit":""}],
  "http_request_duration_seconds":[{"type":"histogram","help":"Request duration.","unit":"seconds"}],
  "go_gc_duration_seconds":[{"type":"summary","help":"GC pauses.","unit":""}]
}`

func shopSeries() []map[string]string {
	return []map[string]string{
		{"__name__": "http_requests_total", "job": "shop", "pod": "checkout-0", "status": "200"},
		{"__name__": "http_requests_total", "job": "shop", "pod": "checkout-1", "status": "500"},
		{"__name__": "http_request_duration_seconds_bucket", "job": "shop", "pod": "checkout-0", "le": "0.1"},
		{"__name__": "http_request_duration_seconds_bucket", "job": "shop", "pod": "checkout-0", "le": "+Inf"},
		{"__name__": "http_request_duration_seconds_sum", "job": "shop", "pod": "checkout-0"},
		{"__name__": "http_request_duration_seconds_count", "job": "shop", "pod": "checkout-0"},
		{"__name__": "go_gc_duration_seconds", "job": "shop", "quantile": "0.5"},
		{"__name__": "go_gc_duration_seconds_count", "job": "shop"},
		{"__name__": "up", "job": "shop", "instance": "checkout-0:8080"},
		{"__name__": "process_open_fds_count", "job": "shop"},
	}
}

func TestInventoryGroupsSeriesIntoFamiliesInOneSeriesCall(t *testing.T) {
	ds, seen := inventoryDS(t, shopSeries(), shopMetadata)
	cat, err := ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverInventory, Scope: `{__name__=~".+",job="shop"}`})
	if err != nil {
		t.Fatal(err)
	}
	want := []datasource.Family{
		{Name: "go_gc_duration_seconds", Type: "summary", Help: "GC pauses.", Labels: []string{"job", "quantile"}, Series: 2},
		{Name: "http_request_duration_seconds", Type: "histogram", Help: "Request duration.", Unit: "seconds", Labels: []string{"job", "le", "pod"}, Series: 4},
		{Name: "http_requests_total", Type: "counter", Help: "HTTP requests.", Labels: []string{"job", "pod", "status"}, Series: 2},
		{Name: "process_open_fds_count", Labels: []string{"job"}, Series: 1},
		{Name: "up", Labels: []string{"instance", "job"}, Series: 1},
	}
	if !reflect.DeepEqual(cat.Families, want) {
		t.Fatalf("families =\n%+v\nwant\n%+v", cat.Families, want)
	}
	if cat.FamiliesTotal != 5 || cat.Truncated {
		t.Fatalf("families_total = %d, truncated = %v; want 5 and false", cat.FamiliesTotal, cat.Truncated)
	}
	calls := seen.to("/api/v1/series")
	if len(calls) != 1 {
		t.Fatalf("series requests = %d, want 1", len(calls))
	}
	if got := calls[0].form["match[]"]; len(got) != 1 || got[0] != `{__name__=~".+",job="shop"}` {
		t.Fatalf("match[] = %v, want the scope", got)
	}
	if got := calls[0].form.Get("limit"); got != fmt.Sprint(datasource.MaxDiscoverLimit*50+1) {
		t.Fatalf("limit = %q, want one above the scan cap", got)
	}
	if calls[0].form.Get("start") == "" || calls[0].form.Get("end") == "" {
		t.Fatalf("series request is not time-bounded: %v", calls[0].form)
	}
	metadata := seen.to("/api/v1/metadata")
	if len(metadata) != 1 || metadata[0].form.Has("metric") {
		t.Fatalf("metadata requests = %+v, want one request for all families", metadata)
	}
}

func TestInventoryCountsRuntimeFamiliesApartUnlessTheScopeNamesAMetric(t *testing.T) {
	series := append(shopSeries(),
		map[string]string{"__name__": "promhttp_metric_handler_requests_total", "job": "shop"},
		map[string]string{"__name__": "scrape_duration_seconds", "job": "shop"},
		map[string]string{"__name__": "upstream_latency_seconds", "job": "shop"},
	)
	every := []string{"go_gc_duration_seconds", "http_request_duration_seconds", "http_requests_total", "process_open_fds_count", "promhttp_metric_handler_requests_total", "scrape_duration_seconds", "up", "upstream_latency_seconds"}
	business := []string{"http_request_duration_seconds", "http_requests_total", "upstream_latency_seconds"}
	tests := []struct {
		name        string
		scope       string
		wantNames   []string
		wantRuntime int
	}{
		{"a label scope counts runtime families apart", `{job="shop"}`, business, 5},
		{"no scope counts runtime families apart", "", business, 5},
		{"a __name__ matcher keeps every family", `{__name__=~"go_.+|http_.+",job="shop"}`, every, 0},
		{"a metric name keeps every family", `go_gc_duration_seconds{job="shop"}`, every, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds, _ := inventoryDS(t, series, shopMetadata)
			cat, err := ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverInventory, Scope: tt.scope})
			if err != nil {
				t.Fatal(err)
			}
			names := make([]string, len(cat.Families))
			for i, f := range cat.Families {
				names[i] = f.Name
			}
			if !slices.Equal(names, tt.wantNames) || cat.FamiliesTotal != len(tt.wantNames) || cat.RuntimeFamilies != tt.wantRuntime {
				t.Fatalf("families = %v, total = %d, runtime = %d; want %v, %d, %d", names, cat.FamiliesTotal, cat.RuntimeFamilies, tt.wantNames, len(tt.wantNames), tt.wantRuntime)
			}
		})
	}
}

func TestInventoryScope(t *testing.T) {
	tests := []struct {
		name  string
		scope string
		match string
	}{
		{"empty scope covers every series", "", `{__name__=~".+"}`},
		{"blank scope covers every series", "  ", `{__name__=~".+"}`},
		{"metric name selector", `up{job="shop"}`, `up{job="shop"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds, seen := inventoryDS(t, nil, `{}`)
			cat, err := ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverInventory, Scope: tt.scope})
			if err != nil {
				t.Fatal(err)
			}
			if len(cat.Families) != 0 || cat.FamiliesTotal != 0 {
				t.Fatalf("catalog = %+v, want no families", cat)
			}
			if got := seen.to("/api/v1/series")[0].form.Get("match[]"); got != tt.match {
				t.Fatalf("match[] = %q, want %q", got, tt.match)
			}
		})
	}
}

func TestInventoryRejectsAScopeThatIsNotASelector(t *testing.T) {
	for _, scope := range []string{`job="shop"`, `rate(up[5m])`, `{job="shop"`} {
		t.Run(scope, func(t *testing.T) {
			ds, seen := inventoryDS(t, nil, `{}`)
			_, err := ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverInventory, Scope: scope})
			if !errors.Is(err, datasource.ErrInvalidQuery) || !strings.Contains(err.Error(), "scope must be a series selector") {
				t.Fatalf("err = %v, want an invalid scope", err)
			}
			if got := len(seen.to("/api/v1/series")); got != 0 {
				t.Fatalf("%d series requests for an invalid scope", got)
			}
		})
	}
}

func TestInventoryTruncatesBeyondTheCaps(t *testing.T) {
	scanCap := datasource.MaxDiscoverLimit * 50
	tests := []struct {
		name          string
		series        int
		families      int
		wantFamilies  int
		wantTotal     int
		wantTruncated bool
	}{
		{"series at the scan cap", scanCap, 10, 10, 10, false},
		{"series over the scan cap", scanCap + 1, 10, 10, 10, true},
		{"families at the list cap", datasource.MaxDiscoverLimit, datasource.MaxDiscoverLimit, datasource.MaxDiscoverLimit, datasource.MaxDiscoverLimit, false},
		{"families over the list cap", datasource.MaxDiscoverLimit + 1, datasource.MaxDiscoverLimit + 1, datasource.MaxDiscoverLimit, datasource.MaxDiscoverLimit + 1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			series := make([]map[string]string, tt.series)
			for i := range series {
				series[i] = map[string]string{"__name__": fmt.Sprintf("m%04d", i%tt.families), "i": fmt.Sprint(i)}
			}
			ds, _ := inventoryDS(t, series, `{}`)
			cat, err := ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverInventory, Scope: `{job="shop"}`})
			if err != nil {
				t.Fatal(err)
			}
			if len(cat.Families) != tt.wantFamilies || cat.FamiliesTotal != tt.wantTotal || cat.Truncated != tt.wantTruncated {
				t.Fatalf("families = %d, total = %d, truncated = %v; want %d, %d, %v",
					len(cat.Families), cat.FamiliesTotal, cat.Truncated, tt.wantFamilies, tt.wantTotal, tt.wantTruncated)
			}
		})
	}
}

func TestInventoryReportsAnUnreadableSeriesResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/series" {
			_, _ = w.Write([]byte(`{"status":"success","data":{"not":"a list"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{}}`))
	}))
	t.Cleanup(srv.Close)
	ds, err := prometheus.New(prometheus.Options{URL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ds.Close() })
	_, err = ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverInventory})
	if !errors.Is(err, datasource.ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}
