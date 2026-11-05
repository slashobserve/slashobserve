package clickhouse_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

var tracesFamily = datasource.Family{
	Name:   "otel_traces",
	Type:   "traces",
	Labels: []string{"Duration", "ResourceAttributes", "ServiceName", "SpanAttributes", "SpanId", "SpanName", "StatusCode", "Timestamp", "TraceId"},
	Series: 5,
	Table:  "otel_traces",
}

func TestInventoryListsEveryTableAsAFamily(t *testing.T) {
	ds := roDS(t)
	cat, err := ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverInventory})
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]datasource.Family{}
	for _, f := range cat.Families {
		byName[f.Name] = f
	}
	for name, kind := range map[string]string{"otel_logs": "logs", "otel_metrics_gauge": "metrics_gauge", "otel_metrics_sum": "metrics_sum", "otel_traces": "traces"} {
		f, ok := byName[name]
		if !ok || f.Type != kind || f.Table != name || f.Series == 0 || len(f.Labels) == 0 {
			t.Errorf("family %s = %+v, want kind %s with columns and rows", name, f, kind)
		}
	}
	if got := byName["otel_traces"]; !reflect.DeepEqual(got, tracesFamily) {
		t.Errorf("otel_traces = %+v, want %+v", got, tracesFamily)
	}
	if cat.FamiliesTotal != len(cat.Families) || cat.Truncated {
		t.Errorf("families_total = %d for %d families, truncated = %v", cat.FamiliesTotal, len(cat.Families), cat.Truncated)
	}
}

func TestInventoryScopeNarrowsToOneTable(t *testing.T) {
	ds := roDS(t)
	cat, err := ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverInventory, Scope: "otel_traces"})
	if err != nil {
		t.Fatal(err)
	}
	if len(cat.Families) != 1 || !reflect.DeepEqual(cat.Families[0], tracesFamily) || cat.FamiliesTotal != 1 {
		t.Fatalf("catalog = %+v, want only otel_traces", cat)
	}
	_, err = ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverInventory, Scope: "missing"})
	if !errors.Is(err, datasource.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound for a missing table", err)
	}
}
