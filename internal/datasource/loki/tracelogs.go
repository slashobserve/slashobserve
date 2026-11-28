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
	if err != nil || datasource.FrameRows(frames) > 0 {
		return frames, err
	}
	return d.traceLines(ctx, req, lineFilterTraceQuery(selector, req))
}

func (d *Datasource) traceSelector(ctx context.Context, req datasource.TraceLogsRequest) (string, bool, error) {
	ctx, cancel := datasource.Bound(ctx, discoverTimeout)
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
	text := selector + " | trace_id=" + strconv.Quote(req.TraceID)
	if req.SpanID != "" {
		text += " | span_id=" + strconv.Quote(req.SpanID)
	}
	return text
}

func lineFilterTraceQuery(selector string, req datasource.TraceLogsRequest) string {
	text := selector + " |= " + strconv.Quote(req.TraceID)
	if req.SpanID != "" {
		text += " |= " + strconv.Quote(req.SpanID)
	}
	return text
}

func (d *Datasource) traceLines(ctx context.Context, req datasource.TraceLogsRequest, text string) ([]datasource.Frame, error) {
	lim := d.opts.Limits.Tighten(datasource.Limits{MaxRows: req.Limit})
	return d.queryText(ctx, text, lim, req.Range, datasource.InstantInterval(req.Range, 0), true)
}
