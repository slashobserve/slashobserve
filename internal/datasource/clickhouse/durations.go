package clickhouse

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/macros"
)

const groupByColumnHint = "group_by must be a TraceQL-style attribute such as resource.service.name, span.http.route or name, status, kind and statusMessage, or a column such as ServiceName or SpanName"

var _ datasource.TraceDurationSource = (*Datasource)(nil)

var (
	groupColumns = map[string]string{
		"resource.service.name": "ServiceName", "service.name": "ServiceName", "ServiceName": "ServiceName",
		"name": "SpanName", "SpanName": "SpanName",
		"status": "StatusCode", "StatusCode": "StatusCode",
		"kind": "SpanKind", "SpanKind": "SpanKind",
		"statusMessage": "StatusMessage", "StatusMessage": "StatusMessage",
	}
	groupAttribute = regexp.MustCompile(`^(resource|span)?\.([A-Za-z_][A-Za-z0-9_.-]{0,120})$`)
)

func GroupColumn(attribute string) (string, bool) {
	if column, ok := groupColumns[attribute]; ok {
		return column, true
	}
	m := groupAttribute.FindStringSubmatch(attribute)
	if m == nil {
		return "", false
	}
	return attributeColumn(m[1], m[2]), true
}

func attributeColumn(scope, key string) string {
	if scope == "resource" {
		return "ResourceAttributes[" + quoteString(key) + "]"
	}
	return "SpanAttributes[" + quoteString(key) + "]"
}

func DurationQuantilesQuery(expr, column string) string {
	sel := "SELECT "
	if column != "" {
		sel += column + " AS Value, "
	}
	text := sel + "count() AS Spans, countIf(" + errorStatus + ") AS Errors, quantilesExact(0.5,0.95,0.99)(Duration) AS Q\n" + traceStatsFrom + traceStatsFilter(expr)
	if column != "" {
		text += "\nGROUP BY Value HAVING Value != '' ORDER BY Spans DESC, Value LIMIT " + strconv.Itoa(datasource.MaxDurationGroups+1)
	}
	return text
}

func (d *Datasource) TraceDurations(ctx context.Context, req datasource.TraceDurationRequest) (datasource.TraceDurations, error) {
	if err := req.Range.Validate(); err != nil {
		return datasource.TraceDurations{}, err
	}
	if err := checkTraceCondition(req.Query); err != nil {
		return datasource.TraceDurations{}, err
	}
	if !macros.WrapsExactly("(" + strings.TrimSpace(req.Query) + ")") {
		return datasource.TraceDurations{}, datasource.Errorf(datasource.ErrInvalidQuery, "the trace filter must have balanced parentheses and no comment that reaches the end of it")
	}
	column := ""
	if groupBy := strings.TrimSpace(req.GroupBy); groupBy != "" {
		var ok bool
		if column, ok = GroupColumn(groupBy); !ok {
			return datasource.TraceDurations{}, datasource.Errorf(datasource.ErrInvalidQuery, "%s", groupByColumnHint)
		}
	}
	lim := d.limits.Tighten(req.Limits)
	texts := []string{DurationQuantilesQuery(req.Query, "")}
	if column != "" {
		texts = append(texts, DurationQuantilesQuery(req.Query, column))
	}
	for i, text := range texts {
		exp, err := macros.Expand(macros.ClickHouse, text, macros.Env{Range: req.Range, Interval: max(req.Range.To.Sub(req.Range.From), time.Second), User: req.User})
		if err != nil {
			return datasource.TraceDurations{}, err
		}
		if texts[i], err = d.guard(exp.Text); err != nil {
			return datasource.TraceDurations{}, err
		}
	}
	results := make([]*resultSet, len(texts))
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
		return datasource.TraceDurations{}, trimGeneratedSQL(err)
	}
	out := datasource.TraceDurations{Exact: true}
	if rows := durationRows(results[0]); len(rows) > 0 {
		out.Window = rows[0]
	}
	if column != "" {
		out.Groups = durationRows(results[1])
		if len(out.Groups) > datasource.MaxDurationGroups {
			out.Groups, out.GroupsTruncated = out.Groups[:datasource.MaxDurationGroups], true
		}
	}
	return out, nil
}

func durationRows(rs *resultSet) []datasource.DurationQuantiles {
	columns := map[string]int{}
	for i, c := range rs.columns {
		columns[c.name] = i
	}
	out := make([]datasource.DurationQuantiles, 0, len(rs.rows))
	for _, row := range rs.rows {
		q := datasource.DurationQuantiles{Spans: cellInt(cellAt(row, columns, "Spans")), Errors: cellInt(cellAt(row, columns, "Errors"))}
		if v, ok := cellAt(row, columns, "Value").(string); ok {
			q.Value = v
		}
		if values := elements(cellAt(row, columns, "Q")); q.Spans > 0 && len(values) == 3 {
			for i, slot := range []*float64{&q.P50, &q.P95, &q.P99} {
				if ms := quantileMillis(values[i]); ms != nil {
					*slot = *ms
				}
			}
		}
		out = append(out, q)
	}
	return out
}
