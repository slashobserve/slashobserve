package loki_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/loki"
)

type sampleCall struct {
	path       string
	start, end time.Time
	limit      int
}

type windowedFacetLoki struct {
	mu        sync.Mutex
	calls     []sampleCall
	perWindow int
	slowRange func(n int) bool
	slowFound bool
}

func (f *windowedFacetLoki) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	start, _ := time.Parse(time.RFC3339Nano, q.Get("start"))
	end, _ := time.Parse(time.RFC3339Nano, q.Get("end"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	f.mu.Lock()
	f.calls = append(f.calls, sampleCall{r.URL.Path, start, end, limit})
	ranges := 0
	for _, c := range f.calls {
		if c.path == "/loki/api/v1/query_range" {
			ranges++
		}
	}
	f.mu.Unlock()
	switch r.URL.Path {
	case "/loki/api/v1/labels":
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": []string{}})
	case "/loki/api/v1/detected_fields":
		if f.slowFound {
			<-r.Context().Done()
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"fields": []map[string]any{{"label": "route", "cardinality": 1, "parsers": []string{"json"}}}})
	case "/loki/api/v1/query_range":
		if f.slowRange != nil && f.slowRange(ranges) {
			<-r.Context().Done()
			return
		}
		var values [][2]string
		for i := range min(limit, f.perWindow) {
			at := end.Add(-time.Duration(i+1) * time.Second)
			values = append(values, [2]string{strconv.FormatInt(at.UnixNano(), 10), "line"})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{
			"resultType": "streams",
			"result":     []any{map[string]any{"stream": map[string]string{"service_name": "checkout", "route": "/pay"}, "values": values}},
		}})
	default:
		http.NotFound(w, r)
	}
}

func (f *windowedFacetLoki) to(path string) []sampleCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []sampleCall
	for _, c := range f.calls {
		if c.path == path {
			out = append(out, c)
		}
	}
	return out
}

func serveWindowedFacets(t *testing.T, fake *windowedFacetLoki) *loki.Datasource {
	t.Helper()
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	ds, err := loki.New(loki.Options{URL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ds.Close() })
	return ds
}

func broadFacets(days int, timeout time.Duration) datasource.FacetRequest {
	return datasource.FacetRequest{
		Query:  datasource.Query{Text: `{service_name="checkout"} | json`, Limits: datasource.Limits{Timeout: timeout}},
		Range:  datasource.TimeRange{From: rangeEnd.Add(-time.Duration(days) * 24 * time.Hour), To: rangeEnd},
		Limit:  10,
		Sample: 100,
	}
}

func TestFacetsOverAWeekReadTheNewestHourFirstWhenItFillsTheSample(t *testing.T) {
	fake := &windowedFacetLoki{perWindow: 500}
	ds := serveWindowedFacets(t, fake)
	got, err := ds.LogFacets(context.Background(), broadFacets(7, time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if got.LinesSampled != 100 {
		t.Fatalf("lines_sampled = %d, want the sample of 100", got.LinesSampled)
	}
	hour := rangeEnd.Add(-time.Hour)
	if calls := fake.to("/loki/api/v1/query_range"); len(calls) != 1 || !calls[0].start.Equal(hour) || calls[0].limit != 100 {
		t.Fatalf("sample requests = %+v, want one over the newest hour", calls)
	}
	if calls := fake.to("/loki/api/v1/detected_fields"); len(calls) != 1 || !calls[0].start.Equal(hour) || !calls[0].end.Equal(rangeEnd) {
		t.Fatalf("detected_fields requests = %+v, want one over the sampled hour", calls)
	}
	if route := fieldNamed(t, got, "route"); route.Values[0].Count != 100 {
		t.Fatalf("route = %+v, want 100 sampled lines", route)
	}
}

func TestFacetsWidenTheWindowUntilTheSampleIsFull(t *testing.T) {
	fake := &windowedFacetLoki{perWindow: 30}
	ds := serveWindowedFacets(t, fake)
	got, err := ds.LogFacets(context.Background(), broadFacets(1, time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	calls := fake.to("/loki/api/v1/query_range")
	var limits []int
	for _, c := range calls {
		limits = append(limits, c.limit)
	}
	if !slices.Equal(limits, []int{100, 70, 40, 10}) || got.LinesSampled != 100 {
		t.Fatalf("limits = %v, lines_sampled = %d, want four windows that fill the sample of 100", limits, got.LinesSampled)
	}
	if found := fake.to("/loki/api/v1/detected_fields"); len(found) != 1 || !found[0].start.Equal(rangeEnd.Add(-15*time.Hour)) {
		t.Fatalf("detected_fields requests = %+v, want one over the 15 sampled hours", found)
	}
}

func TestFacetsThatRunOutOfTimeCountTheLinesReadSoFar(t *testing.T) {
	fake := &windowedFacetLoki{perWindow: 30, slowRange: func(n int) bool { return n == 2 }}
	ds := serveWindowedFacets(t, fake)
	started := time.Now()
	got, err := ds.LogFacets(context.Background(), broadFacets(7, 2*time.Second))
	if err != nil {
		t.Fatalf("err = %v, want the fields of the lines read in time", err)
	}
	if took := time.Since(started); took > 1900*time.Millisecond {
		t.Fatalf("took %v, want an answer before the 2 s limit", took)
	}
	if !got.Sampled || got.LinesSampled != 30 {
		t.Fatalf("facets = %+v, want 30 sampled lines", got)
	}
	if route := fieldNamed(t, got, "route"); route.Values[0].Count != 30 {
		t.Fatalf("route = %+v", route)
	}
}

func TestFacetsCountTheSampleWhenDetectedFieldsRunsOutOfTime(t *testing.T) {
	fake := &windowedFacetLoki{perWindow: 500, slowFound: true}
	ds := serveWindowedFacets(t, fake)
	got, err := ds.LogFacets(context.Background(), broadFacets(7, 1500*time.Millisecond))
	if err != nil {
		t.Fatalf("err = %v, want the sample's own fields", err)
	}
	if route := fieldNamed(t, got, "route"); route.Kind != datasource.FacetKindField || route.Values[0].Count != 100 {
		t.Fatalf("route = %+v, want the field read from the sample's labels", route)
	}
}
