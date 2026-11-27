package clickhouse_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/clickhouse"
)

const analyticsDB = "otel_fa"

func provisionAnalytics(ctx context.Context, t *testing.T, rows ...string) *clickhouse.Datasource {
	t.Helper()
	if testsSkipped {
		t.Skip("integration tests need Docker")
	}
	admin, err := rawConn("admin", adminPass)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close() }()
	script := `CREATE DATABASE IF NOT EXISTS ` + analyticsDB + `;
CREATE TABLE IF NOT EXISTS ` + analyticsDB + `.otel_traces
(
    Timestamp DateTime64(9),
    TraceId String,
    SpanId String,
    ParentSpanId String,
    SpanName LowCardinality(String),
    SpanKind LowCardinality(String),
    ServiceName LowCardinality(String),
    Duration UInt64,
    StatusCode LowCardinality(String),
    StatusMessage String,
    SpanAttributes Map(LowCardinality(String), String),
    ResourceAttributes Map(LowCardinality(String), String),
    Events Nested(Timestamp DateTime64(9), Name LowCardinality(String), Attributes Map(LowCardinality(String), String)),
    Links Nested(TraceId String, SpanId String, TraceState String, Attributes Map(LowCardinality(String), String))
)
ENGINE = MergeTree
PARTITION BY toDate(Timestamp)
ORDER BY (ServiceName, SpanName, Timestamp);
TRUNCATE TABLE ` + analyticsDB + `.otel_traces;
GRANT SELECT ON ` + analyticsDB + `.* TO ` + roUser
	for _, stmt := range statements(script) {
		if err := admin.Exec(ctx, stmt); err != nil {
			t.Fatalf("%v in %q", err, stmt)
		}
	}
	for _, row := range rows {
		if err := admin.Exec(ctx, "INSERT INTO "+analyticsDB+".otel_traces "+row); err != nil {
			t.Fatalf("%v in %q", err, row)
		}
	}
	ds, err := clickhouse.New(clickhouse.Options{Addr: host, Database: analyticsDB, Username: roUser, Password: datasource.NewSecret(roPass)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ds.Close() })
	return ds
}

func TestListTracesFindsFailingTracesWithTheirRoute(t *testing.T) {
	ctx := context.Background()
	ds := provisionAnalytics(ctx, t,
		`(Timestamp, TraceId, SpanId, ParentSpanId, SpanName, ServiceName, Duration, StatusCode, StatusMessage) VALUES
		 (now64(9) - toIntervalSecond(30), 'aaaa', 'a1', '', 'GET /checkout', 'frontend', 90000000, 'STATUS_CODE_ERROR', ''),
		 (now64(9) - toIntervalSecond(30), 'aaaa', 'a2', 'a1', 'Charge', 'payment', 40000000, 'STATUS_CODE_ERROR', 'card declined'),
		 (now64(9) - toIntervalSecond(20), 'bbbb', 'b1', '', 'GET /cart', 'frontend', 5000000, 'STATUS_CODE_OK', ''),
		 (now64(9) - toIntervalSecond(10), 'cccc', 'c1', '', 'GET /cart', 'frontend', 7000000, 'STATUS_CODE_UNSET', ''),
		 (now64(9) - toIntervalSecond(10), 'cccc', 'c2', 'c1', 'GetCart', 'cart', 6000000, 'STATUS_CODE_ERROR', 'redis down')`)
	got, err := ds.ListTraces(ctx, datasource.TraceListRequest{Range: lastHour(), Errors: true})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Complete || len(got.Traces) != 2 || got.Traces[0].ID != "cccc" || got.Traces[0].Route != "GET /cart" || got.Traces[1].Route != "GET /checkout" || got.Traces[1].Duration != 90*time.Millisecond {
		t.Fatalf("failing traces = %+v", got)
	}
	got, err = ds.ListTraces(ctx, datasource.TraceListRequest{Range: lastHour(), Filter: datasource.TraceFilter{Service: "payment"}, Errors: true})
	if err != nil || len(got.Traces) != 1 || got.Traces[0].ID != "aaaa" {
		t.Fatalf("payment traces = %+v, %v", got, err)
	}
	got, err = ds.ListTraces(ctx, datasource.TraceListRequest{Range: lastHour(), Filter: datasource.TraceFilter{Route: "GET /cart"}, MinDuration: 6 * time.Millisecond, Limit: 1})
	if err != nil || len(got.Traces) != 1 || got.Traces[0].ID != "cccc" || got.Complete {
		t.Fatalf("slow cart traces = %+v, %v", got, err)
	}
	f, err := ds.Trace(ctx, "aaaa", lastHour(), datasource.TraceSpanBudget)
	if err != nil || f.Rows() != 2 {
		t.Fatalf("trace = %d rows, %v", f.Rows(), err)
	}
}

func TestTraceDurationsMatchTheExactQuantileOfEverySpan(t *testing.T) {
	ctx := context.Background()
	ds := provisionAnalytics(ctx, t,
		`(Timestamp, TraceId, SpanId, ParentSpanId, SpanName, ServiceName, Duration, StatusCode)
		 SELECT now64(9) - toIntervalSecond(number), lower(hex(number)), lower(hex(number)), '', 'GET /cart', if(number % 4 = 0, 'cart', 'shop'),
		        (number * 7919 % 211 + 1) * 1000000 + number, if(number % 10 = 0, 'STATUS_CODE_ERROR', 'STATUS_CODE_OK')
		 FROM numbers(203)`)
	got, err := ds.TraceDurations(ctx, datasource.TraceDurationRequest{Query: "", GroupBy: "resource.service.name", Range: lastHour()})
	if err != nil {
		t.Fatal(err)
	}
	var all, cart []int64
	for n := range int64(203) {
		d := (n*7919%211+1)*1_000_000 + n
		all = append(all, d)
		if n%4 == 0 {
			cart = append(cart, d)
		}
	}
	slices.Sort(all)
	slices.Sort(cart)
	want := func(values []int64, q float64) float64 { return float64(datasource.ExactQuantile(values, q)) / 1e6 }
	if !got.Exact || got.Window.Spans != 203 || got.Window.Errors != 21 {
		t.Fatalf("window = %+v, exact %v", got.Window, got.Exact)
	}
	for _, c := range []struct {
		name      string
		got       [3]float64
		values    []int64
		wantSpans int64
		spans     int64
	}{
		{"window", [3]float64{got.Window.P50, got.Window.P95, got.Window.P99}, all, 203, got.Window.Spans},
		{"cart", [3]float64{got.Groups[1].P50, got.Groups[1].P95, got.Groups[1].P99}, cart, 51, got.Groups[1].Spans},
	} {
		if c.spans != c.wantSpans {
			t.Errorf("%s spans = %d, want %d", c.name, c.spans, c.wantSpans)
		}
		for i, q := range []float64{0.5, 0.95, 0.99} {
			if w := want(c.values, q); c.got[i] != w {
				t.Errorf("%s p%v = %v, want %v from the Go position", c.name, q*100, c.got[i], w)
			}
		}
	}
	if got.Groups[0].Value != "shop" || got.Groups[1].Value != "cart" {
		t.Fatalf("groups = %+v", got.Groups)
	}
	if _, err := ds.TraceDurations(ctx, datasource.TraceDurationRequest{Query: "", GroupBy: "span.a'b", Range: lastHour()}); err == nil {
		t.Fatal("a quote in group_by was accepted")
	}
}
