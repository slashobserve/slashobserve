package clickhouse

import (
	"context"
	"slices"
	"time"

	ch "github.com/ClickHouse/clickhouse-go/v2"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/macros"
)

const (
	variableMaxRows = 10000
	maxResultBytes  = 32 << 20
)

func (d *Datasource) QueryRange(ctx context.Context, q datasource.Query, r datasource.TimeRange, step time.Duration) ([]datasource.Frame, error) {
	if step < time.Second {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "step must be at least 1s")
	}
	rs, lim, err := d.run(ctx, q, r, step, d.limits.Tighten(q.Limits))
	if err != nil {
		return nil, err
	}
	if q.Format == "table" || q.Format == "logs" {
		return rawFrames(rs, lim)
	}
	frames := rs.seriesFrames()
	if len(frames) > lim.MaxSeries {
		return nil, datasource.Errorf(datasource.ErrLimitExceeded, "result exceeds %d series", lim.MaxSeries)
	}
	return checkPoints(frames, lim)
}

func (d *Datasource) QueryInstant(ctx context.Context, q datasource.Query, r datasource.TimeRange) ([]datasource.Frame, error) {
	rs, lim, err := d.run(ctx, q, r, max(instantInterval(r), q.MinInterval), d.limits.Tighten(q.Limits))
	if err != nil {
		return nil, err
	}
	return rawFrames(rs, lim)
}

func (d *Datasource) VariableValues(ctx context.Context, q datasource.Query, r datasource.TimeRange) ([]string, error) {
	lim := d.limits.Tighten(q.Limits).Tighten(datasource.Limits{MaxRows: variableMaxRows})
	rs, _, err := d.run(ctx, q, r, instantInterval(r), lim)
	if err != nil {
		return nil, err
	}
	if len(rs.columns) == 0 {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "variable query returned no columns")
	}
	var values []string
	for _, row := range rs.rows {
		if row[0] == nil {
			continue
		}
		v := variableString(row[0])
		if !slices.Contains(values, v) {
			values = append(values, v)
		}
	}
	return values, nil
}

func variableString(v any) string {
	if t, ok := v.(time.Time); ok {
		return t.UTC().Format(time.RFC3339Nano)
	}
	return stringOf(v)
}

func checkPoints(frames []datasource.Frame, lim datasource.Limits) ([]datasource.Frame, error) {
	if datasource.Points(frames) > lim.MaxPoints {
		return nil, datasource.Errorf(datasource.ErrLimitExceeded, "result exceeds %d points", lim.MaxPoints)
	}
	return frames, nil
}

func rawFrames(rs *resultSet, lim datasource.Limits) ([]datasource.Frame, error) {
	frame := rs.tableFrame()
	if frame.Rows() > lim.MaxPoints {
		return nil, datasource.Errorf(datasource.ErrLimitExceeded, "result exceeds %d points", lim.MaxPoints)
	}
	return []datasource.Frame{frame}, nil
}

func instantInterval(r datasource.TimeRange) time.Duration {
	return max(time.Second, r.To.Sub(r.From)/1000)
}

func (d *Datasource) run(ctx context.Context, q datasource.Query, r datasource.TimeRange, interval time.Duration, lim datasource.Limits) (*resultSet, datasource.Limits, error) {
	if err := r.Validate(); err != nil {
		return nil, lim, err
	}
	if err := d.readOnly(ctx); err != nil {
		return nil, lim, err
	}
	exp, err := macros.Expand(macros.ClickHouse, q.Text, macros.Env{Range: r, Interval: interval, Variables: q.Variables, Dashboard: q.Dashboard, User: q.User})
	if err != nil {
		return nil, lim, err
	}
	text, err := d.guard(exp.Text)
	if err != nil {
		return nil, lim, err
	}
	lim.Timeout = d.clampTimeout(lim.Timeout)
	timeout := effectiveTimeout(ctx, lim.Timeout)
	ctx, cancel := context.WithTimeout(ctx, lim.Timeout)
	defer cancel()
	rs, err := d.fetch(ctx, text, lim, nil)
	if err != nil {
		return nil, lim, mapError(ctx, err, timeout)
	}
	return rs, lim, nil
}

func (d *Datasource) fetch(ctx context.Context, text string, lim datasource.Limits, params ch.Parameters) (*resultSet, error) {
	ctx = ch.Context(ctx, queryOptions(lim, params)...)
	rows, err := d.conn.Query(ctx, text)
	if err != nil {
		return nil, err
	}
	return scanRows(rows, lim.MaxRows)
}

func queryOptions(lim datasource.Limits, params ch.Parameters) []ch.QueryOption {
	opts := []ch.QueryOption{ch.WithSettings(ch.Settings{
		"max_execution_time":   int(max(1, (lim.Timeout+time.Second-1)/time.Second)),
		"max_result_rows":      lim.MaxRows,
		"max_result_bytes":     maxResultBytes,
		"result_overflow_mode": "throw",
	})}
	if len(params) > 0 {
		opts = append(opts, ch.WithParameters(params))
	}
	return opts
}

func quoteIdent(name string) string {
	out := make([]byte, 0, len(name)+2)
	out = append(out, '`')
	for i := 0; i < len(name); i++ {
		if name[i] == '`' || name[i] == '\\' {
			out = append(out, '\\')
		}
		out = append(out, name[i])
	}
	return string(append(out, '`'))
}
