package clickhouse

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/macros"
)

const checkoutFilter = "ServiceName = 'checkout' AND Duration >= 1000000"

func TestTraceStatsQueryText(t *testing.T) {
	cases := []struct {
		name          string
		expr          string
		step          time.Duration
		wantGrouped   string
		wantHistogram string
	}{
		{
			name: "every span", expr: "  ", step: time.Minute,
			wantGrouped: "SELECT toStartOfInterval(Timestamp, INTERVAL 60 SECOND) AS Time, count() AS Spans, countIf(lower(StatusCode) IN ('error','status_code_error')) AS Errors, quantiles(0.5,0.95,0.99)(Duration) AS Q\n" +
				"FROM otel_traces WHERE $__timeFilter(Timestamp)\nGROUP BY Time ORDER BY Time",
			wantHistogram: "SELECT exp2(ceil(log2(greatest(Duration, 1)))) AS Le, count() AS Count\nFROM otel_traces WHERE $__timeFilter(Timestamp)\nGROUP BY Le ORDER BY Le",
		},
		{
			name: "a filter and a sub-second step", expr: checkoutFilter, step: 500 * time.Millisecond,
			wantGrouped: "SELECT toStartOfInterval(Timestamp, INTERVAL 1 SECOND) AS Time, count() AS Spans, countIf(lower(StatusCode) IN ('error','status_code_error')) AS Errors, quantiles(0.5,0.95,0.99)(Duration) AS Q\n" +
				"FROM otel_traces WHERE $__timeFilter(Timestamp) AND (" + checkoutFilter + ")\nGROUP BY Time ORDER BY Time",
			wantHistogram: "SELECT exp2(ceil(log2(greatest(Duration, 1)))) AS Le, count() AS Count\nFROM otel_traces WHERE $__timeFilter(Timestamp) AND (" + checkoutFilter + ")\nGROUP BY Le ORDER BY Le",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := TraceStatsQuery(tc.expr, tc.step); got != tc.wantGrouped {
				t.Errorf("grouped =\n%s\nwant\n%s", got, tc.wantGrouped)
			}
			if got := DurationHistogramQuery(tc.expr); got != tc.wantHistogram {
				t.Errorf("histogram =\n%s\nwant\n%s", got, tc.wantHistogram)
			}
		})
	}
}

func TestGuardAcceptsTheTraceStatsQueries(t *testing.T) {
	for _, q := range []string{TraceStatsQuery(checkoutFilter, time.Minute), DurationHistogramQuery(checkoutFilter), TraceStatsQuery("", time.Minute), DurationHistogramQuery("")} {
		exp, err := macros.ExpandForLint(macros.ClickHouse, q)
		if err != nil {
			t.Fatalf("expand %q: %v", q, err)
		}
		if _, err := guardWith(exp.Text, loadedTableFunctions()); err != nil {
			t.Errorf("guard refused %q: %v", exp.Text, err)
		}
	}
}

func TestGuardRefusesASecondStatementInTheFilter(t *testing.T) {
	for _, expr := range []string{"1=1; DROP TABLE x", "1=1) UNION ALL SELECT * FROM url('http://x') SETTINGS max_threads=1 --"} {
		exp, err := macros.ExpandForLint(macros.ClickHouse, TraceStatsQuery(expr, time.Minute))
		if err != nil {
			t.Fatalf("expand: %v", err)
		}
		if _, err := guard(exp.Text); !errors.Is(err, datasource.ErrForbiddenQuery) {
			t.Errorf("%q: err = %v, want a forbidden query", expr, err)
		}
	}
}

func TestFoldTraceStatsFillsBucketsAndConvertsNanoseconds(t *testing.T) {
	from := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	rng := datasource.TimeRange{From: from, To: from.Add(150 * time.Second)}
	grouped := &resultSet{
		columns: []column{{name: "Time", typ: datasource.FieldTime}, {name: "Spans", typ: datasource.FieldNumber}, {name: "Errors", typ: datasource.FieldNumber}, {name: "Q", typ: datasource.FieldOther}},
		rows: [][]any{
			{from, uint64(40), uint64(1), []float64{33_100_000, 120_400_000, 410_000_000}},
			{from.Add(2 * time.Minute), uint64(2), uint64(0), []float64{1_000_000, 2_000_000, 3_000_000}},
			{from.Add(-time.Minute), uint64(99), uint64(99), []float64{1, 1, 1}},
		},
	}
	histogram := &resultSet{
		columns: []column{{name: "Le", typ: datasource.FieldNumber}, {name: "Count", typ: datasource.FieldNumber}},
		rows:    [][]any{{float64(131072), uint64(5)}, {float64(262144), uint64(11)}, {float64(524288), uint64(0)}},
	}
	stats := foldTraceStats(rng, time.Minute, grouped, histogram)
	if stats.Step != time.Minute || stats.Partial || stats.Spans != 42 || stats.Errors != 1 || len(stats.Buckets) != 3 {
		t.Fatalf("stats = %+v", stats)
	}
	first := stats.Buckets[0]
	if !first.Start.Equal(from) || first.Spans != 40 || first.Errors != 1 || first.P50 == nil || *first.P50 != 33.1 || first.P95 == nil || *first.P95 != 120.4 || first.P99 == nil || *first.P99 != 410 {
		t.Fatalf("bucket 0 = %+v", first)
	}
	if second := stats.Buckets[1]; !second.Start.Equal(from.Add(time.Minute)) || second.Spans != 0 || second.P50 != nil || second.P95 != nil || second.P99 != nil {
		t.Fatalf("bucket 1 = %+v, want zeros and null quantiles", second)
	}
	if third := stats.Buckets[2]; third.Spans != 2 || third.P50 == nil || *third.P50 != 1 {
		t.Fatalf("bucket 2 = %+v", third)
	}
	want := []datasource.DurationBucket{{LeMS: 0.131072, Count: 5}, {LeMS: 0.262144, Count: 11}}
	if !slices.Equal(stats.Histogram, want) {
		t.Fatalf("histogram = %+v, want %+v", stats.Histogram, want)
	}
}

func TestFoldTraceStatsWithNoRowsIsEmptyButFilled(t *testing.T) {
	from := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	stats := foldTraceStats(datasource.TimeRange{From: from, To: from.Add(time.Hour)}, time.Minute, &resultSet{}, &resultSet{})
	if len(stats.Buckets) != 60 || stats.Spans != 0 || stats.Histogram == nil || len(stats.Histogram) != 0 {
		t.Fatalf("stats = %+v", stats)
	}
	if !strings.HasPrefix(TraceStatsQuery("", time.Minute), "SELECT ") {
		t.Fatal("the grouped query is not a SELECT")
	}
}
