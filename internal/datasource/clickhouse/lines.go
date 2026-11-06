package clickhouse

import (
	"context"
	"strconv"
	"time"

	ch "github.com/ClickHouse/clickhouse-go/v2"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/macros"
)

const (
	linesNeedTimestamp  = "the query must select Timestamp"
	parameterTimeFormat = "2006-01-02 15:04:05.000000000"
)

var _ datasource.LineSource = (*Datasource)(nil)

func (d *Datasource) Lines(ctx context.Context, req datasource.LinesRequest) ([]datasource.Frame, error) {
	if err := req.Range.Validate(); err != nil {
		return nil, err
	}
	if err := d.readOnly(ctx); err != nil {
		return nil, err
	}
	lim := d.limits.Tighten(req.Query.Limits)
	q := req.Query
	exp, err := macros.Expand(macros.ClickHouse, q.Text, macros.Env{Range: wholeMilliseconds(req.Range), Interval: instantInterval(req.Range), Variables: q.Variables, Dashboard: q.Dashboard, User: q.User})
	if err != nil {
		return nil, err
	}
	text, err := d.guard(exp.Text)
	if err != nil {
		return nil, err
	}
	inner, err := volumeInner(text)
	if err != nil {
		return nil, err
	}
	lim.Timeout = d.clampTimeout(lim.Timeout)
	lim.MaxRows = min(max(req.Limit, 1), lim.MaxRows)
	timeout := effectiveTimeout(ctx, lim.Timeout)
	ctx, cancel := context.WithTimeout(ctx, lim.Timeout)
	defer cancel()
	extra, err := d.tableFilters(ctx, text, q.Filters)
	if err != nil {
		return nil, err
	}
	if err := d.requireTimestamp(ctx, inner, lim, extra); err != nil {
		return nil, mapError(ctx, err, timeout)
	}
	rs, err := d.fetchWith(ctx, linesQuery(inner, req.Direction), lim, linesParameters(req.Range, lim.MaxRows), extra)
	if err != nil {
		return nil, mapError(ctx, err, timeout)
	}
	return markTruncated([]datasource.Frame{rs.tableFrame()}, rs, lim), nil
}

func wholeMilliseconds(r datasource.TimeRange) datasource.TimeRange {
	from := r.From.Truncate(time.Millisecond)
	to := r.To.Truncate(time.Millisecond)
	if to.Before(r.To) {
		to = to.Add(time.Millisecond)
	}
	return datasource.TimeRange{From: from, To: to}
}

func (d *Datasource) requireTimestamp(ctx context.Context, inner string, lim datasource.Limits, extra ch.Settings) error {
	rs, err := d.fetchWith(ctx, "SELECT * FROM (\n"+inner+"\n) LIMIT 0", lim, nil, extra)
	if err != nil {
		return err
	}
	for _, c := range rs.columns {
		if c.name == timestampColumn {
			return nil
		}
	}
	return datasource.Errorf(datasource.ErrInvalidQuery, "%s", linesNeedTimestamp)
}

func linesQuery(inner string, direction datasource.LineDirection) string {
	order := "DESC"
	if direction == datasource.Forward {
		order = "ASC"
	}
	ts := quoteIdent(timestampColumn)
	return "SELECT * FROM (\n" + inner + "\n)\nWHERE " + ts + " >= {from:DateTime64(9, 'UTC')} AND " + ts + " < {to:DateTime64(9, 'UTC')}\n" +
		"ORDER BY " + ts + " " + order + "\nLIMIT {limit:UInt64}"
}

func linesParameters(r datasource.TimeRange, limit int) ch.Parameters {
	return ch.Parameters{
		"from":  parameterTime(r.From),
		"to":    parameterTime(r.To),
		"limit": strconv.Itoa(limit),
	}
}

func parameterTime(t time.Time) string {
	return t.UTC().Format(parameterTimeFormat)
}
