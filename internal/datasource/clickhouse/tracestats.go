package clickhouse

import (
	"context"
	"math"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/macros"
)

const (
	traceStatsFrom = "FROM otel_traces WHERE $__timeFilter(Timestamp)"
	errorStatus    = "lower(StatusCode) IN ('error','status_code_error')"
	nanosPerMilli  = 1e6
)

func TraceStatsQuery(expr string, step time.Duration) string {
	seconds := max(int64(step/time.Second), 1)
	return "SELECT toStartOfInterval(Timestamp, INTERVAL " + strconv.FormatInt(seconds, 10) + " SECOND) AS Time, count() AS Spans, countIf(" + errorStatus + ") AS Errors, quantiles(0.5,0.95,0.99)(Duration) AS Q\n" +
		traceStatsFrom + traceStatsFilter(expr) + "\nGROUP BY Time ORDER BY Time"
}

func DurationHistogramQuery(expr string) string {
	return "SELECT exp2(ceil(log2(greatest(Duration, 1)))) AS Le, count() AS Count\n" + traceStatsFrom + traceStatsFilter(expr) + "\nGROUP BY Le ORDER BY Le"
}

func traceStatsFilter(expr string) string {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return ""
	}
	return " AND (" + expr + ")"
}

func (d *Datasource) TraceStats(ctx context.Context, req datasource.TraceStatsRequest) (datasource.TraceStats, error) {
	if err := req.Range.Validate(); err != nil {
		return datasource.TraceStats{}, err
	}
	lim := d.limits.Tighten(req.Limits)
	texts := [2]string{TraceStatsQuery(req.Query, req.Step), DurationHistogramQuery(req.Query)}
	for i, text := range texts {
		exp, err := macros.Expand(macros.ClickHouse, text, macros.Env{Range: req.Range, Interval: req.Step, User: req.User})
		if err != nil {
			return datasource.TraceStats{}, err
		}
		if texts[i], err = d.guard(exp.Text); err != nil {
			return datasource.TraceStats{}, err
		}
	}
	var results [2]*resultSet
	err := d.read(ctx, lim, func(ctx context.Context) error {
		group, groupCtx := errgroup.WithContext(ctx)
		for i, text := range texts {
			group.Go(func() error {
				rs, err := d.fetch(groupCtx, text, lim, nil)
				results[i] = rs
				return err
			})
		}
		return group.Wait()
	})
	if err != nil {
		return datasource.TraceStats{}, err
	}
	return foldTraceStats(req.Range, req.Step, results[0], results[1]), nil
}

func foldTraceStats(r datasource.TimeRange, step time.Duration, grouped, histogram *resultSet) datasource.TraceStats {
	n := int(r.To.Sub(r.From) / step)
	if r.To.Sub(r.From)%step != 0 {
		n++
	}
	stats := datasource.TraceStats{Step: step, Buckets: make([]datasource.TraceBucket, max(n, 1)), Histogram: []datasource.DurationBucket{}}
	for i := range stats.Buckets {
		stats.Buckets[i].Start = r.From.Add(time.Duration(i) * step)
	}
	columns := map[string]int{}
	for i, c := range grouped.columns {
		columns[c.name] = i
	}
	for _, row := range grouped.rows {
		at, ok := cellAt(row, columns, "Time").(time.Time)
		if !ok {
			continue
		}
		i := int(at.Sub(r.From) / step)
		if i < 0 || i >= len(stats.Buckets) {
			continue
		}
		b := &stats.Buckets[i]
		b.Spans += cellInt(cellAt(row, columns, "Spans"))
		b.Errors += cellInt(cellAt(row, columns, "Errors"))
		if q := elements(cellAt(row, columns, "Q")); b.Spans > 0 && len(q) == 3 {
			b.P50, b.P95, b.P99 = quantileMillis(q[0]), quantileMillis(q[1]), quantileMillis(q[2])
		}
	}
	for _, b := range stats.Buckets {
		stats.Spans += b.Spans
		stats.Errors += b.Errors
	}
	for _, row := range histogram.rows {
		if len(row) < 2 {
			continue
		}
		le, ok := toFloat(row[0])
		count := cellInt(row[1])
		if ok && count > 0 {
			stats.Histogram = append(stats.Histogram, datasource.DurationBucket{LeMS: le / nanosPerMilli, Count: count})
		}
	}
	return stats
}

func cellAt(row []any, columns map[string]int, name string) any {
	if i, ok := columns[name]; ok && i < len(row) {
		return row[i]
	}
	return nil
}

func cellInt(v any) int64 {
	f, ok := toFloat(v)
	if !ok {
		return 0
	}
	return int64(math.Round(f))
}

func quantileMillis(v any) *float64 {
	f, ok := toFloat(v)
	if !ok || math.IsNaN(f) || math.IsInf(f, 0) {
		return nil
	}
	ms := f / nanosPerMilli
	return &ms
}
