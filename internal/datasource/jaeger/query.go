package jaeger

import (
	"cmp"
	"context"
	"errors"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/macros"
)

const (
	traceIDHexLen    = 32
	maxSearchDepth   = 10000
	futureRange      = "the time range starts in the future, so no trace can match; pick a range that has already begun"
	summariesPath    = "/api/v3/trace-summaries"
	timeParamLayout  = time.RFC3339Nano
	searchDepthParam = "query.searchDepth"
)

type unixNanos int64

func (n *unixNanos) UnmarshalJSON(b []byte) error {
	v, err := strconv.ParseInt(strings.Trim(string(b), `"`), 10, 64)
	if err != nil {
		return err
	}
	*n = unixNanos(v)
	return nil
}

type summaryResponse struct {
	Summaries []struct {
		TraceID    string    `json:"traceId"`
		RootSvc    string    `json:"rootServiceName"`
		RootOp     string    `json:"rootOperationName"`
		Start      unixNanos `json:"minStartTimeUnixNano"`
		End        unixNanos `json:"maxEndTimeUnixNano"`
		SpanCount  int64     `json:"spanCount"`
		ErrorCount int64     `json:"errorSpanCount"`
	} `json:"summaries"`
}

type traceRow struct {
	traceID, name, service string
	start                  time.Time
	duration               int64
	spans, errors          int64
}

func (d *Datasource) QueryRange(ctx context.Context, q datasource.Query, r datasource.TimeRange, _ time.Duration) ([]datasource.Frame, error) {
	return d.QueryInstant(ctx, q, r)
}

func (d *Datasource) QueryInstant(ctx context.Context, q datasource.Query, r datasource.TimeRange) ([]datasource.Frame, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	lim := d.opts.Limits.Tighten(q.Limits)
	ctx, cancel := bound(ctx, lim.Timeout)
	defer cancel()
	text, err := expandQuery(q, r)
	if err != nil {
		return nil, err
	}
	q.Expanded(text)
	if id, ok := traceIDOf(text); ok {
		f, err := d.trace(ctx, id, r, lim.MaxRows)
		if errors.Is(err, datasource.ErrNotFound) {
			return []datasource.Frame{}, nil
		}
		if err != nil {
			return nil, err
		}
		return []datasource.Frame{f}, nil
	}
	s, err := searchOf(text)
	if err != nil {
		return nil, err
	}
	rows, err := d.searchRows(ctx, s, r, lim.MaxRows)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []datasource.Frame{}, nil
	}
	f := traceFrame(rows)
	if len(rows) == lim.MaxRows {
		f.Meta = &datasource.FrameMeta{Truncated: true}
	}
	return []datasource.Frame{f}, nil
}

func traceIDOf(text string) (string, bool) {
	text = strings.TrimSpace(text)
	if len(text) >= 2 && (text[0] == '"' || text[0] == '`') && text[len(text)-1] == text[0] {
		text = text[1 : len(text)-1]
	}
	if !datasource.ValidTraceID(text) {
		return "", false
	}
	return strings.ToLower(text), true
}

func expandQuery(q datasource.Query, r datasource.TimeRange) (string, error) {
	exp, err := macros.Expand(macros.Jaeger, q.Text, macros.Env{
		Range:     r,
		Interval:  max(time.Second, (r.To.Sub(r.From) / 1000).Truncate(time.Second), q.MinInterval),
		Variables: q.Variables,
		Dashboard: q.Dashboard,
		User:      q.User,
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(exp.Text), nil
}

func searchOf(text string) (search, error) {
	s, bad := parseSearch(text)
	if bad != nil {
		return search{}, bad.err()
	}
	return s, nil
}

func (d *Datasource) searchRows(ctx context.Context, s search, r datasource.TimeRange, limit int) ([]traceRow, error) {
	if r.From.After(time.Now()) {
		return nil, datasource.Errorf(datasource.ErrRangeInFuture, "%s", futureRange)
	}
	attributes, err := s.attributesJSON()
	if err != nil {
		return nil, unreadable()
	}
	params := url.Values{
		"query.startTimeMin": {r.From.UTC().Format(timeParamLayout)},
		"query.startTimeMax": {r.To.UTC().Format(timeParamLayout)},
		searchDepthParam:     {strconv.Itoa(min(limit, maxSearchDepth))},
	}
	setIf(params, "query.serviceName", s.service)
	setIf(params, "query.operationName", s.operation)
	if len(s.tags) > 0 {
		params.Set("query.attributes", attributes)
	}
	if s.minDuration > 0 {
		params.Set("query.durationMin", s.minDuration.String())
	}
	if s.maxDuration > 0 {
		params.Set("query.durationMax", s.maxDuration.String())
	}
	var resp summaryResponse
	if err := d.getJSON(ctx, summariesPath, params, &resp); err != nil {
		return nil, err
	}
	rows := make([]traceRow, 0, len(resp.Summaries))
	for _, t := range resp.Summaries {
		rows = append(rows, traceRow{
			traceID:  paddedID(t.TraceID),
			name:     t.RootOp,
			service:  t.RootSvc,
			start:    time.Unix(0, int64(t.Start)).UTC(),
			duration: max(0, int64(t.End-t.Start)),
			spans:    t.SpanCount,
			errors:   t.ErrorCount,
		})
	}
	slices.SortStableFunc(rows, func(a, b traceRow) int {
		return cmp.Or(b.start.Compare(a.start), strings.Compare(a.traceID, b.traceID))
	})
	return rows, nil
}

func setIf(params url.Values, key, value string) {
	if value != "" {
		params.Set(key, value)
	}
}

func paddedID(id string) string {
	id = strings.ToLower(id)
	if len(id) < traceIDHexLen {
		id = strings.Repeat("0", traceIDHexLen-len(id)) + id
	}
	return id
}

func traceFrame(rows []traceRow) datasource.Frame {
	column := func(name string, typ datasource.FieldType, value func(traceRow) any) datasource.Field {
		values := make([]any, len(rows))
		for i, r := range rows {
			values[i] = value(r)
		}
		return datasource.Field{Name: name, Type: typ, Values: values}
	}
	return datasource.Frame{Fields: []datasource.Field{
		column("TraceId", datasource.FieldString, func(r traceRow) any { return r.traceID }),
		column("SpanId", datasource.FieldString, func(r traceRow) any { return r.traceID }),
		column("SpanName", datasource.FieldString, func(r traceRow) any { return r.name }),
		column("ServiceName", datasource.FieldString, func(r traceRow) any { return r.service }),
		column("StartTime", datasource.FieldTime, func(r traceRow) any { return r.start }),
		column("Duration", datasource.FieldNumber, func(r traceRow) any { return r.duration }),
		column("Spans", datasource.FieldNumber, func(r traceRow) any { return datasource.Number(float64(r.spans)) }),
		column("Errors", datasource.FieldNumber, func(r traceRow) any { return datasource.Number(float64(r.errors)) }),
	}}
}
