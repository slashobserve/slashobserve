package tempo

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

var _ datasource.TraceListSource = (*Datasource)(nil)

func ListQuery(req datasource.TraceListRequest) string {
	var parts []string
	add := func(condition string) { parts = append(parts, "{ "+condition+" }") }
	if req.Filter.Route != "" {
		add(`trace:rootName = "` + traceQLString(req.Filter.Route) + `"`)
	}
	if req.Filter.Service != "" {
		add(`resource.service.name = "` + traceQLString(req.Filter.Service) + `"`)
	}
	if req.Filter.Operation != "" {
		add(`name = "` + traceQLString(req.Filter.Operation) + `"`)
	}
	if req.Errors {
		add("status = error")
	}
	if req.MinDuration > 0 {
		add("trace:duration >= " + strconv.FormatInt(max(req.MinDuration.Microseconds(), 1), 10) + "us")
	}
	if len(parts) == 0 {
		return "{}"
	}
	return strings.Join(parts, " && ")
}

func (d *Datasource) ListTraces(ctx context.Context, req datasource.TraceListRequest) (datasource.TraceList, error) {
	if err := req.Range.Validate(); err != nil {
		return datasource.TraceList{}, err
	}
	lim := d.opts.Limits.Tighten(req.Limits)
	ctx, cancel := datasource.Bound(ctx, lim.Timeout)
	defer cancel()
	limit := cmpPositive(req.Limit, datasource.TraceListBudget)
	rows, metrics, err := d.searchRows(ctx, ListQuery(req), req.Range, limit)
	if err != nil {
		return datasource.TraceList{}, err
	}
	out := datasource.TraceList{Traces: make([]datasource.TraceSummary, 0, len(rows)), Complete: len(rows) < limit && !metrics.partial()}
	for _, r := range rows {
		out.Traces = append(out.Traces, datasource.TraceSummary{ID: r.TraceID, RootService: r.Service, Route: r.Name, Start: r.Start, Duration: time.Duration(r.Duration)})
	}
	return out, nil
}
