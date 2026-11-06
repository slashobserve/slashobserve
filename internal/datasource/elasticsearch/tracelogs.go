package elasticsearch

import (
	"context"
	"strconv"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

var _ datasource.TraceLogSource = (*Datasource)(nil)

func (d *Datasource) TraceLogs(ctx context.Context, req datasource.TraceLogsRequest) ([]datasource.Frame, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	ctx, cancel := bound(ctx, d.opts.Limits.Timeout)
	defer cancel()
	m, err := d.Mapping(ctx)
	if err != nil {
		return nil, err
	}
	roles, err := m.roles(d.opts.Fields)
	if err != nil {
		return nil, err
	}
	if roles.TraceID == "" {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "no trace id field in the mapping; set fields.traceId")
	}
	filter := []any{timeRangeClause(d.opts.TimeField, req.Range), exactTerm(m, roles.TraceID, req.TraceID)}
	if req.SpanID != "" {
		if roles.SpanID == "" {
			return nil, datasource.Errorf(datasource.ErrInvalidQuery, "no span id field in the mapping; set fields.spanId")
		}
		filter = append(filter, exactTerm(m, roles.SpanID, req.SpanID))
	}
	if req.Service != "" && roles.Service != "" {
		filter = append(filter, exactTerm(m, roles.Service, req.Service))
	}
	size := min(req.Limit, maxLogLines)
	body := map[string]any{
		"timeout":          strconv.FormatInt(d.opts.Limits.Timeout.Milliseconds(), 10) + "ms",
		"query":            map[string]any{"bool": map[string]any{"filter": filter}},
		"size":             size,
		"track_total_hits": false,
		"sort":             lineSort(d.opts.TimeField, "desc"),
	}
	return d.lineFrames(ctx, body, m, size)
}

func exactTerm(m *Mapping, field, value string) map[string]any {
	if f := m.Fields[field]; f.Kind == KindText && f.Keyword != "" {
		field = f.Keyword
	}
	return map[string]any{"term": map[string]any{field: value}}
}
