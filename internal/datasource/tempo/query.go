package tempo

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/macros"
	"github.com/slashobserve/slashobserve/internal/datasource/otlphttp"
)

const (
	traceIDHexLen = 32
	emptyQuery    = `a Tempo query needs a trace id or a TraceQL query such as {resource.service.name="checkout"}`
)

func quotedInt64(b []byte) (int64, error) {
	return strconv.ParseInt(strings.Trim(string(b), `"`), 10, 64)
}

type searchMetrics struct {
	CompletedJobs  int64       `json:"completedJobs"`
	TotalJobs      int64       `json:"totalJobs"`
	InspectedBytes json.Number `json:"inspectedBytes"`
	cut            bool
}

func (m searchMetrics) partial() bool {
	return m.cut || m.TotalJobs > 0 && m.CompletedJobs < m.TotalJobs
}

func (m searchMetrics) plus(o searchMetrics) searchMetrics {
	return searchMetrics{
		CompletedJobs:  m.CompletedJobs + o.CompletedJobs,
		TotalJobs:      m.TotalJobs + o.TotalJobs,
		InspectedBytes: json.Number(strconv.FormatInt(m.inspectedBytes()+o.inspectedBytes(), 10)),
		cut:            m.cut || o.cut,
	}
}

func (m searchMetrics) inspectedBytes() int64 {
	n, err := m.InspectedBytes.Int64()
	if err != nil {
		return 0
	}
	return n
}

type searchResponse struct {
	Traces []struct {
		TraceID         string             `json:"traceID"`
		RootServiceName string             `json:"rootServiceName"`
		RootTraceName   string             `json:"rootTraceName"`
		Start           otlphttp.UnixNanos `json:"startTimeUnixNano"`
		DurationMs      int64              `json:"durationMs"`
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
	ctx, cancel := datasource.Bound(ctx, lim.Timeout)
	defer cancel()
	text, err := expandQuery(q, r)
	if err != nil {
		return nil, err
	}
	q.Expanded(text)
	if id, ok := otlphttp.TraceIDOf(text); ok {
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

func expandQuery(q datasource.Query, r datasource.TimeRange) (string, error) {
	exp, err := macros.Expand(macros.TraceQL, q.Text, macros.Env{
		Range:     r,
		Interval:  datasource.InstantInterval(r, q.MinInterval),
		Variables: q.Variables,
		Dashboard: q.Dashboard,
		User:      q.User,
	})
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(exp.Text)
	if text == "" {
		return "", datasource.Errorf(datasource.ErrInvalidQuery, "%s", emptyQuery)
	}
	return text, nil
}

func (d *Datasource) search(ctx context.Context, text string, r datasource.TimeRange, lim datasource.Limits) ([]datasource.Frame, error) {
	rows, metrics, err := d.searchRows(ctx, text, r, lim.MaxRows)
	if err != nil {
		return nil, err
	}
	if len(rows) > lim.MaxRows {
		return nil, datasource.Errorf(datasource.ErrLimitExceeded, "row limit exceeded: more than %d traces", lim.MaxRows)
	}
	if len(rows) == 0 && !metrics.partial() {
		return []datasource.Frame{}, nil
	}
	if len(rows) == 0 {
		return []datasource.Frame{{Fields: []datasource.Field{}, Meta: searchMeta(metrics, false)}}, nil
	}
	var meta *datasource.FrameMeta
	if truncated, partial := len(rows) == lim.MaxRows, metrics.partial(); truncated || partial || metrics.TotalJobs > 0 {
		meta = searchMeta(metrics, truncated)
	}
	return []datasource.Frame{traceFrame(rows, meta)}, nil
}

const SearchDurationHint = "Duration is rounded to whole milliseconds by Tempo search; open the trace for the exact duration"

func searchMeta(metrics searchMetrics, truncated bool) *datasource.FrameMeta {
	return &datasource.FrameMeta{
		Truncated:      truncated,
		Partial:        metrics.partial(),
		JobsDone:       metrics.CompletedJobs,
		JobsTotal:      metrics.TotalJobs,
		InspectedBytes: metrics.inspectedBytes(),
	}
}

var (
	queryHints    = regexp.MustCompile(`(?i)\bwith\s*\(`)
	quotedStrings = regexp.MustCompile("\"(?:[^\"\\\\]|\\\\.)*\"|`[^`]*`")
)

func newestFirst(text string) string {
	if queryHints.MatchString(quotedStrings.ReplaceAllString(text, `""`)) {
		return text
	}
	return text + " with (most_recent=true)"
}

func (d *Datasource) searchRows(ctx context.Context, text string, r datasource.TimeRange, limit int) ([]otlphttp.Row, searchMetrics, error) {
	if err := rangeStartsInFuture(r); err != nil {
		return nil, searchMetrics{}, err
	}
	var (
		rows    []otlphttp.Row
		metrics searchMetrics
		seen    = map[string]bool{}
		pacer   datasource.Pacer
	)
	for _, w := range datasource.NewestFirst(r, firstSearchWindow) {
		win, ok := pacer.Start(ctx, w.To.Sub(w.From))
		if !ok {
			metrics.cut = true
			break
		}
		got, m, err := d.searchWindow(win.Ctx, text, w, limit-len(rows))
		if ranOut := win.End(err); err != nil {
			if !ranOut {
				return nil, searchMetrics{}, err
			}
			metrics.cut = true
			break
		}
		metrics = metrics.plus(m)
		for _, row := range got {
			if !seen[row.TraceID] {
				seen[row.TraceID] = true
				rows = append(rows, row)
			}
		}
		if len(rows) >= limit {
			break
		}
	}
	slices.SortStableFunc(rows, func(a, b otlphttp.Row) int {
		return cmp.Or(b.Start.Compare(a.Start), strings.Compare(a.TraceID, b.TraceID))
	})
	return rows, metrics, nil
}

const firstSearchWindow = time.Hour

func (d *Datasource) searchWindow(ctx context.Context, text string, r datasource.TimeRange, limit int) ([]otlphttp.Row, searchMetrics, error) {
	sent := newestFirst(text)
	params := url.Values{
		"q":     {sent},
		"start": {strconv.FormatInt(r.From.Unix(), 10)},
		"end":   {strconv.FormatInt(ceilSeconds(r.To), 10)},
		"limit": {strconv.Itoa(limit)},
	}
	var resp searchResponse
	if err := d.getJSON(ctx, "/api/search", params, &resp); err != nil {
		return nil, searchMetrics{}, traceQLProblem(err, text, sent)
	}
	rows := make([]otlphttp.Row, 0, len(resp.Traces))
	for _, t := range resp.Traces {
		id := strings.ToLower(t.TraceID)
		if len(id) < traceIDHexLen {
			id = strings.Repeat("0", traceIDHexLen-len(id)) + id
		}
		row := otlphttp.Row{
			TraceID:  id,
			SpanID:   id,
			Name:     t.RootTraceName,
			Service:  t.RootServiceName,
			Start:    time.Unix(0, int64(t.Start)).UTC(),
			Duration: t.DurationMs * int64(time.Millisecond),
		}
		if t.ServiceStats != nil {
			var spans, errors int64
			for _, s := range t.ServiceStats {
				spans += s.SpanCount
				errors += s.ErrorCount
			}
			row.Spans, row.Errors = &spans, &errors
		}
		rows = append(rows, row)
	}
	return rows, resp.Metrics, nil
}

func ceilSeconds(t time.Time) int64 {
	if t.Equal(t.Truncate(time.Second)) {
		return t.Unix()
	}
	return t.Unix() + 1
}

func traceFrame(rows []otlphttp.Row, meta *datasource.FrameMeta) datasource.Frame {
	full := otlphttp.Frame(rows)
	keep := []string{"TraceId", "SpanId", "SpanName", "ServiceName", "StartTime", "Duration"}
	if meta == nil {
		meta = &datasource.FrameMeta{}
	}
	meta.Hint = SearchDurationHint
	f := datasource.Frame{Meta: meta}
	for _, field := range full.Fields {
		if slices.Contains(keep, field.Name) {
			f.Fields = append(f.Fields, field)
		}
	}
	if slices.ContainsFunc(rows, func(r otlphttp.Row) bool { return r.Spans == nil }) {
		return f
	}
	spans, errors := make([]any, len(rows)), make([]any, len(rows))
	for i, r := range rows {
		spans[i], errors[i] = datasource.Number(float64(*r.Spans)), datasource.Number(float64(*r.Errors))
	}
	f.Fields = append(f.Fields,
		datasource.Field{Name: "Spans", Type: datasource.FieldNumber, Values: spans},
		datasource.Field{Name: "Errors", Type: datasource.FieldNumber, Values: errors})
	return f
}
