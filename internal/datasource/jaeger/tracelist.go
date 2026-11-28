package jaeger

import (
	"context"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const listNeedsService = "Jaeger searches one service at a time, so name the service"

var _ datasource.TraceListSource = (*Datasource)(nil)

func (d *Datasource) ListTraces(ctx context.Context, req datasource.TraceListRequest) (datasource.TraceList, error) {
	if err := req.Range.Validate(); err != nil {
		return datasource.TraceList{}, err
	}
	if req.Filter.Service == "" {
		return datasource.TraceList{}, datasource.Errorf(datasource.ErrInvalidQuery, "%s", listNeedsService)
	}
	lim := d.opts.Limits.Tighten(req.Limits)
	ctx, cancel := datasource.Bound(ctx, lim.Timeout)
	defer cancel()
	s := search{service: req.Filter.Service, operation: req.Filter.Operation, tags: map[string]string{}}
	if req.Errors {
		s.tags["error"] = "true"
	}
	limit := searchDepth(max(req.Limit, 1))
	if req.Limit <= 0 {
		limit = datasource.TraceListBudget
	}
	rows, err := d.searchRows(ctx, s, req.Range, limit)
	if err != nil {
		return datasource.TraceList{}, err
	}
	out := datasource.TraceList{Traces: make([]datasource.TraceSummary, 0, len(rows)), Complete: len(rows) < limit}
	for _, r := range rows {
		if req.Filter.Route != "" && r.name != req.Filter.Route || time.Duration(r.duration) < req.MinDuration {
			continue
		}
		out.Traces = append(out.Traces, datasource.TraceSummary{ID: r.traceID, RootService: r.service, Route: r.name, Start: r.start, Duration: time.Duration(r.duration)})
	}
	return out, nil
}
