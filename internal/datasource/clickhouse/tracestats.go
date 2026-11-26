package clickhouse

import (
	"context"
	"errors"
	"math"
	"regexp"
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

	unsupportedWindowNote = "ClickHouse trace stats give p50, p95 and p99 per time bucket only; whole-window and group_by quantiles are not available here, so run a SQL query with quantiles(0.5, 0.95, 0.99)(Duration) and GROUP BY ServiceName instead."
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

var conditionExample = "ServiceName = 'checkout' AND Duration > 1000000"

func checkTraceCondition(expr string) error {
	tokens, err := macros.TokenizeClickHouse(expr)
	if err != nil {
		return datasource.Errorf(datasource.ErrInvalidQuery, "the query must be a condition over otel_traces, such as %s", conditionExample)
	}
	depth := 0
	for _, t := range significantAndComments(tokens) {
		text := expr[t.Start:t.End]
		switch {
		case t.Kind == macros.TokenComment, isPunct(expr, t, ";"):
			return datasource.Errorf(datasource.ErrInvalidQuery, "the query must be one condition without comments or statements, such as %s", conditionExample)
		case isPunct(expr, t, "("):
			depth++
		case isPunct(expr, t, ")"):
			depth--
			if depth < 0 {
				return datasource.Errorf(datasource.ErrInvalidQuery, "the query has an unmatched closing parenthesis; write one condition such as %s", conditionExample)
			}
		case t.Kind == macros.TokenWord && depth == 0 && (strings.EqualFold(text, "SELECT") || strings.EqualFold(text, "WITH")):
			return datasource.Errorf(datasource.ErrInvalidQuery, "the query is a condition over otel_traces, not a SELECT statement; write something like %s", conditionExample)
		}
	}
	if depth != 0 {
		return datasource.Errorf(datasource.ErrInvalidQuery, "the query has an unclosed parenthesis; write one condition such as %s", conditionExample)
	}
	return nil
}

func significantAndComments(tokens []macros.Token) []macros.Token {
	kept := make([]macros.Token, 0, len(tokens))
	for _, t := range tokens {
		if t.Kind != macros.TokenWhitespace {
			kept = append(kept, t)
		}
	}
	return kept
}

var generatedSQL = regexp.MustCompile(`(?s)(SELECT (?:toStartOfInterval|exp2)\(|\(line \d+, col \d+\): ).*?GROUP BY (?:Time ORDER BY Time|Le ORDER BY Le)`)

func trimGeneratedSQL(err error) error {
	var ue *datasource.UpstreamError
	if !errors.As(err, &ue) || !errors.Is(ue.Kind, datasource.ErrInvalidQuery) {
		return err
	}
	trimmed := generatedSQL.ReplaceAllStringFunc(ue.Message, func(m string) string {
		if strings.HasPrefix(m, "(line ") {
			return m[:strings.Index(m, "): ")+3] + "..."
		}
		return "the query"
	})
	return &datasource.UpstreamError{Kind: ue.Kind, Message: trimmed + "; the query is a condition over otel_traces, such as " + conditionExample}
}

func (d *Datasource) TraceStats(ctx context.Context, req datasource.TraceStatsRequest) (datasource.TraceStats, error) {
	if err := req.Range.Validate(); err != nil {
		return datasource.TraceStats{}, err
	}
	if err := checkTraceCondition(req.Query); err != nil {
		return datasource.TraceStats{}, err
	}
	if !macros.WrapsExactly("(" + strings.TrimSpace(req.Query) + ")") {
		return datasource.TraceStats{}, datasource.Errorf(datasource.ErrInvalidQuery, "the trace filter must have balanced parentheses and no comment that reaches the end of it")
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
		return datasource.TraceStats{}, trimGeneratedSQL(err)
	}
	stats := foldTraceStats(req.Range, req.Step, results[0], results[1])
	stats.Note = unsupportedWindowNote
	return stats, nil
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
