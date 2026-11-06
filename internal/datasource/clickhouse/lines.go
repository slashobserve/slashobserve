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
	var frames []datasource.Frame
	err := d.readLines(ctx, req.Query, req.Range, func(ctx context.Context, inner string, lim datasource.Limits, extra ch.Settings) error {
		lim.MaxRows = min(max(req.Limit, 1), lim.MaxRows)
		rs, err := d.fetchWith(ctx, linesQuery(inner, req.Direction), lim, linesParameters(req.Range, lim.MaxRows, 0), extra)
		if err != nil {
			return err
		}
		frames = markTruncated([]datasource.Frame{rs.tableFrame()}, rs, lim)
		return nil
	})
	return frames, err
}

type linesReader func(ctx context.Context, inner string, lim datasource.Limits, extra ch.Settings) error

func (d *Datasource) readLines(ctx context.Context, q datasource.Query, r datasource.TimeRange, read linesReader) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if err := d.readOnly(ctx); err != nil {
		return err
	}
	lim := d.limits.Tighten(q.Limits)
	exp, err := macros.Expand(macros.ClickHouse, q.Text, macros.Env{Range: wholeMilliseconds(r), Interval: instantInterval(r), Variables: q.Variables, Dashboard: q.Dashboard, User: q.User})
	if err != nil {
		return err
	}
	text, err := d.guard(exp.Text)
	if err != nil {
		return err
	}
	inner, err := volumeInner(text)
	if err != nil {
		return err
	}
	lim.Timeout = d.clampTimeout(lim.Timeout)
	timeout := effectiveTimeout(ctx, lim.Timeout)
	ctx, cancel := context.WithTimeout(ctx, lim.Timeout)
	defer cancel()
	extra, err := d.tableFilters(ctx, text, q.Filters)
	if err != nil {
		return err
	}
	if err := d.requireTimestamp(ctx, inner, lim, extra); err != nil {
		return mapError(ctx, err, timeout)
	}
	if err := read(ctx, inner, lim, extra); err != nil {
		return mapError(ctx, err, timeout)
	}
	return nil
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
	return "SELECT * FROM (\n" + inner + "\n)\n" + rangeFilter() +
		"ORDER BY " + ts + " " + order + "\nLIMIT {limit:UInt64}\nOFFSET {offset:UInt64}"
}

func countQuery(inner string) string {
	return "SELECT count() FROM (\n" + inner + "\n)\n" + rangeFilter()
}

func rangeFilter() string {
	ts := quoteIdent(timestampColumn)
	return "WHERE " + ts + " >= {from:DateTime64(9, 'UTC')} AND " + ts + " < {to:DateTime64(9, 'UTC')}\n"
}

func linesParameters(r datasource.TimeRange, limit, offset int) ch.Parameters {
	params := rangeParameters(r)
	params["limit"] = strconv.Itoa(limit)
	params["offset"] = strconv.Itoa(offset)
	return params
}

func rangeParameters(r datasource.TimeRange) ch.Parameters {
	return ch.Parameters{
		"from": parameterTime(r.From),
		"to":   parameterTime(r.To),
	}
}

func parameterTime(t time.Time) string {
	return t.UTC().Format(parameterTimeFormat)
}
