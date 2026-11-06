package loki_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/loki"
)

type routedLoki struct {
	mu       sync.Mutex
	bodies   map[string]string
	requests []request
}

func (f *routedLoki) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, request{path: r.URL.Path, params: r.URL.Query()})
	body, ok := f.bodies[r.URL.Path]
	f.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	_, _ = w.Write([]byte(body))
}

func (f *routedLoki) to(path string) []request {
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

func serveRoutes(t *testing.T, bodies map[string]string) (*loki.Datasource, *routedLoki) {
	t.Helper()
	fake := &routedLoki{bodies: bodies}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	ds, err := loki.New(loki.Options{URL: srv.URL})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = ds.Close() })
	return ds, fake
}

func labelList(names ...string) string {
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = fmt.Sprintf("%q", n)
	}
	return `{"status":"success","data":[` + strings.Join(quoted, ",") + `]}`
}

const (
	labelsPath = "/loki/api/v1/labels"
	rangePath  = "/loki/api/v1/query_range"
	seriesPath = "/loki/api/v1/series"
)

func window(t *testing.T, r request) time.Duration {
	t.Helper()
	start, err := time.Parse(time.RFC3339Nano, r.params.Get("start"))
	if err != nil {
		t.Fatalf("start %q: %v", r.params.Get("start"), err)
	}
	end, err := time.Parse(time.RFC3339Nano, r.params.Get("end"))
	if err != nil {
		t.Fatalf("end %q: %v", r.params.Get("end"), err)
	}
	return end.Sub(start)
}

func TestDiscoverLabelsSendsTheSelector(t *testing.T) {
	ds, fake := serveRoutes(t, map[string]string{labelsPath: labelList("level", "service_name")})
	if _, err := ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverLabels, Metric: `{service_name="payments"}`}); err != nil {
		t.Fatal(err)
	}
	if got := fake.to(labelsPath)[0].params.Get("query"); got != `{service_name="payments"}` {
		t.Fatalf("query = %q", got)
	}
}

func TestDiscoverLabelsSendTheRange(t *testing.T) {
	const valuesPath = "/loki/api/v1/label/service_name/values"
	from := time.Date(2026, 11, 6, 6, 0, 0, 0, time.UTC)
	rng := datasource.TimeRange{From: from, To: from.Add(6 * time.Hour)}
	ds, fake := serveRoutes(t, map[string]string{labelsPath: labelList("service_name"), valuesPath: labelList("checkout")})
	for _, req := range []datasource.DiscoverRequest{
		{Kind: datasource.DiscoverLabels, Range: rng},
		{Kind: datasource.DiscoverLabelValues, Label: "service_name", Range: rng},
		{Kind: datasource.DiscoverLabels},
		{Kind: datasource.DiscoverLabelValues, Label: "service_name"},
	} {
		if _, err := ds.Discover(context.Background(), req); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{labelsPath, valuesPath} {
		sent := fake.to(path)
		if len(sent) != 2 {
			t.Fatalf("%s: %d requests, want 2", path, len(sent))
		}
		if got := sent[0].params; got.Get("start") != "2026-11-06T06:00:00Z" || got.Get("end") != "2026-11-06T12:00:00Z" {
			t.Fatalf("%s: ranged request sent start %q end %q", path, got.Get("start"), got.Get("end"))
		}
		if got := sent[1].params; got.Has("start") || got.Has("end") {
			t.Fatalf("%s: a request without a range sent start %q end %q", path, got.Get("start"), got.Get("end"))
		}
	}
}

func TestDiscoverSampleMergesStreamsNewestFirst(t *testing.T) {
	streams := success("streams", `[
		{"stream":{"service_name":"checkout"},"values":[["1793966400000000003","checkout line"],["1793966400000000001","older checkout line"]]},
		{"stream":{"service_name":"payments","level":"error","pod":"p-1"},"values":[["1793966400000000002","payment failed"]]}
	]`)
	ds, fake := serveRoutes(t, map[string]string{
		labelsPath: labelList("__stream_shard__", "job", "service_name"),
		rangePath:  streams,
	})
	got, err := ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverSampleRows, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	labelsReq := fake.to(labelsPath)
	if len(labelsReq) != 1 || window(t, labelsReq[0]) != 15*time.Minute {
		t.Fatalf("labels requests = %+v", labelsReq)
	}
	query := fake.to(rangePath)[0]
	if query.params.Get("query") != `{service_name=~".+"}` || query.params.Get("limit") != "20" || query.params.Get("direction") != "backward" || window(t, query) != 15*time.Minute {
		t.Fatalf("query params = %v", query.params)
	}
	var names []string
	for _, f := range got.Sample.Fields {
		names = append(names, f.Name)
	}
	if want := []string{"Time", "Body", "level", "level", "pod", "service_name"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("fields = %v, want %v", names, want)
	}
	values := func(i int) []any { return got.Sample.Fields[i].Values }
	if want := []any{"checkout line", "payment failed", "older checkout line"}; !reflect.DeepEqual(values(1), want) {
		t.Fatalf("bodies = %v, want %v", values(1), want)
	}
	if want := []any{nil, "error", nil}; !reflect.DeepEqual(values(2), want) {
		t.Fatalf("levels = %v, want %v", values(2), want)
	}
	if want := []any{nil, "p-1", nil}; !reflect.DeepEqual(values(4), want) {
		t.Fatalf("pods = %v, want %v", values(4), want)
	}
	if want := []any{"checkout", "payments", "checkout"}; !reflect.DeepEqual(values(5), want) {
		t.Fatalf("services = %v, want %v", values(5), want)
	}
	if first, ok := values(0)[0].(time.Time); !ok || first.UnixNano() != 1793966400000000003 {
		t.Fatalf("first time = %v", values(0)[0])
	}
}

func TestDiscoverSampleLimit(t *testing.T) {
	cases := []struct {
		limit int
		want  string
	}{{0, "5"}, {3, "3"}, {21, "20"}}
	for _, tc := range cases {
		ds, fake := serveRoutes(t, map[string]string{labelsPath: labelList("app"), rangePath: success("streams", "[]")})
		got, err := ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverSampleRows, Limit: tc.limit})
		if err != nil {
			t.Fatal(err)
		}
		if limit := fake.to(rangePath)[0].params.Get("limit"); limit != tc.want {
			t.Errorf("limit %d sent %s, want %s", tc.limit, limit, tc.want)
		}
		if len(got.Sample.Fields) != 2 || len(got.Sample.Fields[0].Values) != 0 {
			t.Errorf("no lines answered %+v", got.Sample)
		}
	}
}

func TestDiscoverSamplePicksTheServiceLabel(t *testing.T) {
	cases := []struct {
		labels []string
		want   string
	}{
		{[]string{"app", "job"}, `{job=~".+"}`},
		{[]string{"zone", "app", "__name__"}, `{app=~".+"}`},
	}
	for _, tc := range cases {
		ds, fake := serveRoutes(t, map[string]string{labelsPath: labelList(tc.labels...), rangePath: success("streams", "[]")})
		if _, err := ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverSampleRows}); err != nil {
			t.Fatal(err)
		}
		if got := fake.to(rangePath)[0].params.Get("query"); got != tc.want {
			t.Errorf("labels %v queried %s, want %s", tc.labels, got, tc.want)
		}
	}

	ds, fake := serveRoutes(t, map[string]string{labelsPath: labelList("__stream_shard__")})
	got, err := ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverSampleRows})
	if err != nil {
		t.Fatal(err)
	}
	if got.Sample == nil || len(got.Sample.Fields) != 0 || len(fake.to(rangePath)) != 0 {
		t.Fatalf("no labels answered %+v after %d queries", got.Sample, len(fake.to(rangePath)))
	}
}

func TestDiscoverSampleTakesASelector(t *testing.T) {
	ds, fake := serveRoutes(t, map[string]string{rangePath: success("streams", "[]")})
	if _, err := ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverSampleRows, Table: ` {service_name="payments"} `}); err != nil {
		t.Fatal(err)
	}
	if len(fake.to(labelsPath)) != 0 || fake.to(rangePath)[0].params.Get("query") != `{service_name="payments"}` {
		t.Fatalf("requests = %+v", fake.requests)
	}
	if _, err := ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverSampleRows, Table: "payments"}); !errors.Is(err, datasource.ErrInvalidQuery) {
		t.Fatalf("a table name: err = %v", err)
	}
}

func TestDiscoverStreamInventory(t *testing.T) {
	body := `{"status":"success","data":[
		{"service_name":"checkout","level":"info","__stream_shard__":"1"},
		{"service_name":"checkout","level":"error"},
		{"service_name":"payments"}
	]}`
	ds, fake := serveRoutes(t, map[string]string{seriesPath: body})
	got, err := ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverInventory, Scope: ` {namespace="shop"}`})
	if err != nil {
		t.Fatal(err)
	}
	want := []datasource.Family{{Name: "level", Type: "label", Series: 2}, {Name: "service_name", Type: "label", Series: 3}}
	if !reflect.DeepEqual(got.Families, want) || got.FamiliesTotal != 2 || got.Truncated {
		t.Fatalf("catalog = %+v", got)
	}
	req := fake.to(seriesPath)[0]
	if req.params.Get("match[]") != `{namespace="shop"}` || window(t, req) != time.Hour {
		t.Fatalf("series params = %v", req.params)
	}
	if _, err := ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverInventory, Scope: "checkout"}); !errors.Is(err, datasource.ErrInvalidQuery) {
		t.Fatalf("a scope without a selector: err = %v", err)
	}
}

func TestDiscoverStreamInventoryCapsStreams(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"status":"success","data":[`)
	for i := range 50_001 {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"a":"b"}`)
	}
	b.WriteString(`]}`)
	ds, _ := serveRoutes(t, map[string]string{seriesPath: b.String()})
	got, err := ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverInventory, Scope: `{a="b"}`})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Truncated || len(got.Families) != 1 || got.Families[0].Series != 50_000 {
		t.Fatalf("catalog = %+v", got)
	}
}
