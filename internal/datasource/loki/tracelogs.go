package loki

import (
	"context"
	"net/url"
	"slices"
	"strconv"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

var traceServiceLabels = []string{"service_name", "service", "app", "job"}

var _ datasource.TraceLogSource = (*Datasource)(nil)

func (d *Datasource) TraceLogs(ctx context.Context, req datasource.TraceLogsRequest) ([]datasource.Frame, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	selector, ok, err := d.traceSelector(ctx, req)
	if err != nil {
		return nil, err
	}
	if !ok {
		return []datasource.Frame{}, nil
	}
	frames, err := d.traceLines(ctx, req, metadataTraceQuery(selector, req))
	if err != nil || lineCount(frames) > 0 {
		return frames, err
	}
	return d.traceLines(ctx, req, lineFilterTraceQuery(selector, req))
}

func (d *Datasource) traceSelector(ctx context.Context, req datasource.TraceLogsRequest) (string, bool, error) {
	ctx, cancel := bound(ctx, discoverTimeout)
	defer cancel()
	names, err := d.getStrings(ctx, "/loki/api/v1/labels", url.Values{"start": {lokiTime(req.Range.From)}, "end": {lokiTime(req.Range.To)}})
	if err != nil {
		return "", false, err
	}
	if req.Service != "" {
		for _, label := range traceServiceLabels {
			if slices.Contains(names, label) {
				return "{" + label + "=" + strconv.Quote(req.Service) + "}", true, nil
			}
		}
	}
	label, ok := sampleLabel(names)
	if !ok {
		return "", false, nil
	}
	return "{" + label + `=~".+"}`, true, nil
}

func metadataTraceQuery(selector string, req datasource.TraceLogsRequest) string {
	text := selector + ` | trace_id="` + req.TraceID + `"`
	if req.SpanID != "" {
		text += ` | span_id="` + req.SpanID + `"`
	}
	return text
}

func lineFilterTraceQuery(selector string, req datasource.TraceLogsRequest) string {
	text := selector + ` |= "` + req.TraceID + `"`
	if req.SpanID != "" {
		text += ` |= "` + req.SpanID + `"`
	}
	return text
}

func (d *Datasource) traceLines(ctx context.Context, req datasource.TraceLogsRequest, text string) ([]datasource.Frame, error) {
	q := datasource.Query{Text: text, Limits: datasource.Limits{MaxRows: req.Limit}, User: req.User}
	return d.query(ctx, q, req.Range, instantInterval(req.Range, 0), true)
}

func lineCount(frames []datasource.Frame) int {
	n := 0
	for _, f := range frames {
		n += f.Rows()
	}
	return n
}
