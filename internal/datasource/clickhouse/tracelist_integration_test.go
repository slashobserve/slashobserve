package clickhouse_test

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/clickhouse"
)

//go:embed testdata/trace_list_unfiltered.sql
var traceListUnfiltered string

//go:embed testdata/trace_list_filtered.sql
var traceListFiltered string

const (
	traceListDB    = "otel_tl"
	traceSlowAsync = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	traceQuick     = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	traceNoRoot    = "cccccccccccccccccccccccccccccccc"
)

type traceListRow struct {
	service, span string
	duration      float64
	spans, errors float64
	matching      *float64
}

func provisionTraceList(ctx context.Context, t *testing.T) {
	t.Helper()
	if testsSkipped {
		t.Skip("integration tests need Docker")
	}
	admin, err := rawConn("admin", adminPass)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close() }()
	script := `CREATE DATABASE IF NOT EXISTS ` + traceListDB + `;
CREATE TABLE IF NOT EXISTS ` + traceListDB + `.otel_traces
(
    Timestamp DateTime64(9),
    TraceId String,
    SpanId String,
    ParentSpanId String,
    SpanName LowCardinality(String),
    ServiceName LowCardinality(String),
    Duration UInt64,
    StatusCode LowCardinality(String),
    SpanAttributes Map(LowCardinality(String), String),
    ResourceAttributes Map(LowCardinality(String), String)
)
ENGINE = MergeTree
PARTITION BY toDate(Timestamp)
ORDER BY (ServiceName, SpanName, Timestamp);
TRUNCATE TABLE ` + traceListDB + `.otel_traces;
GRANT SELECT ON ` + traceListDB + `.* TO ` + roUser
	for _, stmt := range statements(script) {
		if err := admin.Exec(ctx, stmt); err != nil {
			t.Fatalf("%v in %q", err, stmt)
		}
	}
	base := time.Now().Add(-10 * time.Minute).Truncate(time.Millisecond)
	span := func(trace, id, parent, name, service string, offset, duration time.Duration, status string) string {
		at := base.Add(offset).UTC().Format("2006-01-02 15:04:05.000000000")
		return fmt.Sprintf("('%s', '%s', '%s', '%s', '%s', '%s', %d, '%s')", at, trace, id, parent, name, service, duration.Nanoseconds(), status)
	}
	values := []string{
		span(traceSlowAsync, "a1", "", "GET /checkout", "tl-front", 0, 100*time.Millisecond, "STATUS_CODE_OK"),
		span(traceSlowAsync, "a2", "a1", "charge", "tl-pay", 10*time.Millisecond, 60*time.Millisecond, "STATUS_CODE_ERROR"),
		span(traceSlowAsync, "a3", "a1", "process_order", "tl-back", 150*time.Millisecond, 1250*time.Millisecond, "STATUS_CODE_OK"),
		span(traceQuick, "b1", "", "GET /cart", "tl-front", time.Second, 20*time.Millisecond, "STATUS_CODE_OK"),
		span(traceNoRoot, "c1", "c0", "db.query", "tl-db", 2*time.Second, 5*time.Millisecond, "STATUS_CODE_OK"),
		span(traceNoRoot, "c2", "c0", "db.commit", "tl-db", 2*time.Second+7*time.Millisecond, 3*time.Millisecond, "STATUS_CODE_OK"),
	}
	insert := "INSERT INTO " + traceListDB + ".otel_traces (Timestamp, TraceId, SpanId, ParentSpanId, SpanName, ServiceName, Duration, StatusCode) VALUES " + strings.Join(values, ", ")
	if err := admin.Exec(ctx, insert); err != nil {
		t.Fatal(err)
	}
}

func traceListDS(t *testing.T) *clickhouse.Datasource {
	t.Helper()
	if testsSkipped {
		t.Skip("integration tests need Docker")
	}
	ds, err := clickhouse.New(clickhouse.Options{Addr: host, Database: traceListDB, Username: roUser, Password: datasource.NewSecret(roPass)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ds.Close() })
	return ds
}

func runTraceList(ctx context.Context, t *testing.T, ds *clickhouse.Datasource, sql string, sort *datasource.PageSort) map[string]traceListRow {
	t.Helper()
	frames, _, err := ds.QueryPage(ctx, datasource.PageRequest{Query: query(sql), Range: lastHour(), Page: 1, PageSize: 50, Sort: sort})
	if err != nil {
		t.Fatalf("%v\n%s", err, sql)
	}
	if len(frames) != 1 {
		t.Fatalf("frames = %d, want 1", len(frames))
	}
	f := frames[0]
	ids, services, spanNames := column(t, f, "TraceId"), column(t, f, "ServiceName"), column(t, f, "SpanName")
	durations, spans, errs := column(t, f, "Duration"), column(t, f, "Spans"), column(t, f, "Errors")
	var matching []any
	for _, field := range f.Fields {
		if field.Name == "Matching" {
			matching = field.Values
		}
	}
	rows := map[string]traceListRow{}
	order := make([]string, 0, f.Rows())
	for i := range f.Rows() {
		id := ids.Values[i].(string)
		row := traceListRow{service: services.Values[i].(string), span: spanNames.Values[i].(string), duration: durations.Values[i].(float64), spans: spans.Values[i].(float64), errors: errs.Values[i].(float64)}
		if matching != nil {
			n := matching[i].(float64)
			row.matching = &n
		}
		rows[id] = row
		order = append(order, id)
	}
	t.Logf("trace order: %v", order)
	return rows
}

func TestTraceListShowsTheWholeTraceForEachMatch(t *testing.T) {
	ctx := context.Background()
	provisionTraceList(ctx, t)
	ds := traceListDS(t)

	t.Run("without a filter every trace reports its own root, span count and length", func(t *testing.T) {
		rows := runTraceList(ctx, t, ds, traceListUnfiltered, nil)
		want := map[string]traceListRow{
			traceSlowAsync: {service: "tl-front", span: "GET /checkout", duration: 1.4e9, spans: 3, errors: 1},
			traceQuick:     {service: "tl-front", span: "GET /cart", duration: 20e6, spans: 1},
			traceNoRoot:    {service: "tl-db", span: "db.query", duration: 10e6, spans: 2},
		}
		for id, w := range want {
			if got := rows[id]; got != w {
				t.Errorf("trace %s = %+v, want %+v", id[:4], got, w)
			}
		}
	})

	t.Run("a filter keeps the traces with a matching span and shows them whole", func(t *testing.T) {
		rows := runTraceList(ctx, t, ds, traceListFiltered, nil)
		if _, ok := rows[traceQuick]; ok {
			t.Fatal("the 20 ms trace has no span of 50 ms and must not be listed")
		}
		got := rows[traceSlowAsync]
		one := 1.0
		want := traceListRow{service: "tl-front", span: "GET /checkout", duration: 1.4e9, spans: 3, errors: 1, matching: &one}
		if got.matching == nil || *got.matching != 1 || got.service != want.service || got.span != want.span || got.duration != want.duration || got.spans != want.spans || got.errors != want.errors {
			t.Fatalf("trace = %+v, want %+v", got, want)
		}
	})

	t.Run("a match on a late async span still shows the trace root", func(t *testing.T) {
		sql := strings.ReplaceAll(traceListFiltered, "'tl-front'", "'tl-back'")
		rows := runTraceList(ctx, t, ds, sql, nil)
		got, ok := rows[traceSlowAsync]
		if !ok || got.service != "tl-front" || got.span != "GET /checkout" || got.spans != 3 || got.matching == nil || *got.matching != 1 {
			t.Fatalf("trace = %+v, want the frontend root with 3 spans and 1 matching", got)
		}
	})

	t.Run("sorting orders the whole result", func(t *testing.T) {
		frames, _, err := ds.QueryPage(ctx, datasource.PageRequest{Query: query(traceListUnfiltered), Range: lastHour(), Page: 1, PageSize: 50, Sort: &datasource.PageSort{Column: "Duration", Descending: true}})
		if err != nil {
			t.Fatal(err)
		}
		ids := column(t, frames[0], "TraceId").Values
		if len(ids) < 3 || ids[0] != traceSlowAsync || ids[1] != traceQuick || ids[2] != traceNoRoot {
			t.Fatalf("trace ids = %v, want slowest first", ids)
		}
		frames, _, err = ds.QueryPage(ctx, datasource.PageRequest{Query: query(traceListUnfiltered), Range: lastHour(), Page: 1, PageSize: 50, Sort: &datasource.PageSort{Column: "Spans"}})
		if err != nil {
			t.Fatal(err)
		}
		if ids := column(t, frames[0], "TraceId").Values; ids[0] != traceQuick {
			t.Fatalf("trace ids = %v, want the one-span trace first", ids)
		}
	})
}

func TestTraceStatsRefusesABadConditionWithoutTheGeneratedSQL(t *testing.T) {
	ctx := context.Background()
	provisionTraceList(ctx, t)
	ds := traceListDS(t)
	for _, condition := range []string{"service = checkout", "ServiceName = ", "SELECT 1"} {
		_, err := ds.TraceStats(ctx, datasource.TraceStatsRequest{Query: condition, Range: lastHour(), Step: time.Minute})
		if err == nil {
			t.Fatalf("condition %q was accepted", condition)
		}
		t.Logf("%q -> %v", condition, err)
		for _, leak := range []string{"toStartOfInterval", "countIf", "GROUP BY", "exp2"} {
			if strings.Contains(err.Error(), leak) {
				t.Errorf("condition %q: error names %q: %v", condition, leak, err)
			}
		}
	}
	stats, err := ds.TraceStats(ctx, datasource.TraceStatsRequest{Query: "ServiceName = 'tl-front'", Range: lastHour(), Step: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Spans != 2 {
		t.Errorf("spans = %d, want 2", stats.Spans)
	}
}
