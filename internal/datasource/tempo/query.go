package tempo

import (
	"cmp"
	"context"
	"encoding/json"
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
	traceIDHexLen = 32
	emptyQuery    = `a Tempo query needs a trace id or a TraceQL query such as {resource.service.name="checkout"}`
)

type keyValue struct {
	Key   string   `json:"key"`
	Value anyValue `json:"value"`
}

type anyValue struct {
	StringValue *string         `json:"stringValue"`
	BoolValue   *bool           `json:"boolValue"`
	IntValue    json.RawMessage `json:"intValue"`
	DoubleValue json.RawMessage `json:"doubleValue"`
	BytesValue  *string         `json:"bytesValue"`
	ArrayValue  *struct {
		Values []anyValue `json:"values"`
	} `json:"arrayValue"`
	KvlistValue *struct {
		Values []keyValue `json:"values"`
	} `json:"kvlistValue"`
}

type unixNanos int64

func (n *unixNanos) UnmarshalJSON(b []byte) error {
	v, err := quotedInt64(b)
	if err != nil {
		return err
	}
	*n = unixNanos(v)
	return nil
}

func quotedInt64(b []byte) (int64, error) {
	return strconv.ParseInt(strings.Trim(string(b), `"`), 10, 64)
}

type searchMetrics struct {
	CompletedJobs int64 `json:"completedJobs"`
	TotalJobs     int64 `json:"totalJobs"`
}

func (m searchMetrics) partial() bool {
	return m.TotalJobs > 0 && m.CompletedJobs < m.TotalJobs
}

type searchResponse struct {
	Traces []struct {
		TraceID         string    `json:"traceID"`
		RootServiceName string    `json:"rootServiceName"`
		RootTraceName   string    `json:"rootTraceName"`
		Start           unixNanos `json:"startTimeUnixNano"`
		DurationMs      int64     `json:"durationMs"`
		ServiceStats    map[string]struct {
			SpanCount  int64 `json:"spanCount"`
			ErrorCount int64 `json:"errorCount"`
		} `json:"serviceStats"`
	} `json:"traces"`
	Metrics searchMetrics `json:"metrics"`
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
	exp, err := macros.Expand(macros.TraceQL, q.Text, macros.Env{
		Range:     r,
		Interval:  max(time.Second, (r.To.Sub(r.From) / 1000).Truncate(time.Second), q.MinInterval),
		Variables: q.Variables,
		Dashboard: q.Dashboard,
		User:      q.User,
	})
	if err != nil {
		return nil, err
	}
	text := strings.TrimSpace(exp.Text)
	if text == "" {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "%s", emptyQuery)
	}
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
	return d.search(ctx, text, r, lim)
}

func traceIDOf(text string) (string, bool) {
	text = strings.TrimSpace(text)
	if len(text) >= 2 && (text[0] == '"' || text[0] == '`') && text[len(text)-1] == text[0] {
		text = text[1 : len(text)-1]
	}
	if !datasource.IsTraceID(text) {
		return "", false
	}
	return strings.ToLower(text), true
}

func (d *Datasource) search(ctx context.Context, text string, r datasource.TimeRange, lim datasource.Limits) ([]datasource.Frame, error) {
	params := url.Values{
		"q":     {text},
		"start": {strconv.FormatInt(r.From.Unix(), 10)},
		"end":   {strconv.FormatInt(ceilSeconds(r.To), 10)},
		"limit": {strconv.Itoa(lim.MaxRows)},
	}
	var resp searchResponse
	if err := d.getJSON(ctx, "/api/search", params, &resp); err != nil {
		return nil, err
	}
	if len(resp.Traces) > lim.MaxRows {
		return nil, datasource.Errorf(datasource.ErrLimitExceeded, "row limit exceeded: more than %d traces", lim.MaxRows)
	}
	rows := make([]spanRow, 0, len(resp.Traces))
	for _, t := range resp.Traces {
		id := strings.ToLower(t.TraceID)
		if len(id) < traceIDHexLen {
			id = strings.Repeat("0", traceIDHexLen-len(id)) + id
		}
		row := spanRow{
			traceID:  id,
			spanID:   id,
			name:     t.RootTraceName,
			service:  t.RootServiceName,
			start:    time.Unix(0, int64(t.Start)).UTC(),
			duration: t.DurationMs * int64(time.Millisecond),
		}
		if t.ServiceStats != nil {
			var spans, errors int64
			for _, s := range t.ServiceStats {
				spans += s.SpanCount
				errors += s.ErrorCount
			}
			row.spans, row.errors = &spans, &errors
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return []datasource.Frame{}, nil
	}
	slices.SortStableFunc(rows, func(a, b spanRow) int {
		return cmp.Or(b.start.Compare(a.start), strings.Compare(a.traceID, b.traceID))
	})
	f := traceFrame(rows)
	if truncated, partial := len(rows) == lim.MaxRows, resp.Metrics.partial(); truncated || partial {
		f.Meta = &datasource.FrameMeta{Truncated: truncated, Partial: partial}
	}
	return []datasource.Frame{f}, nil
}

func ceilSeconds(t time.Time) int64 {
	if t.Equal(t.Truncate(time.Second)) {
		return t.Unix()
	}
	return t.Unix() + 1
}

func traceFrame(rows []spanRow) datasource.Frame {
	full := spanFrame(rows)
	keep := []string{"TraceId", "SpanId", "SpanName", "ServiceName", "StartTime", "Duration"}
	f := datasource.Frame{}
	for _, field := range full.Fields {
		if slices.Contains(keep, field.Name) {
			f.Fields = append(f.Fields, field)
		}
	}
	if slices.ContainsFunc(rows, func(r spanRow) bool { return r.spans == nil }) {
		return f
	}
	spans, errors := make([]any, len(rows)), make([]any, len(rows))
	for i, r := range rows {
		spans[i], errors[i] = datasource.Number(float64(*r.spans)), datasource.Number(float64(*r.errors))
	}
	f.Fields = append(f.Fields,
		datasource.Field{Name: "Spans", Type: datasource.FieldNumber, Values: spans},
		datasource.Field{Name: "Errors", Type: datasource.FieldNumber, Values: errors})
	return f
}
