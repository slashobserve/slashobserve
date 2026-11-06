package loki

import (
	"context"
	"net/url"
	"strconv"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const linesNeedLogQuery = `lines need a log query that starts with a stream selector, such as {service_name="checkout"}`

var _ datasource.LineSource = (*Datasource)(nil)

func (d *Datasource) Lines(ctx context.Context, req datasource.LinesRequest) ([]datasource.Frame, error) {
	if err := req.Range.Validate(); err != nil {
		return nil, err
	}
	lim := d.opts.Limits.Tighten(req.Query.Limits)
	ctx, cancel := bound(ctx, lim.Timeout)
	defer cancel()
	text, err := expand(req.Query, req.Range, instantInterval(req.Range, req.Query.MinInterval))
	if err != nil {
		return nil, err
	}
	if !spec.LogQLReturnsLines(text) {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "%s", linesNeedLogQuery)
	}
	limit := min(max(req.Limit, 1), maxLogLines)
	params := url.Values{
		"query":     {text},
		"start":     {lokiTime(req.Range.From)},
		"end":       {lokiTime(req.Range.To)},
		"limit":     {strconv.Itoa(limit)},
		"direction": {string(direction(req.Direction))},
	}
	return d.run(ctx, "/loki/api/v1/query_range", params, lim, limit)
}

func direction(d datasource.LineDirection) datasource.LineDirection {
	if d == datasource.Forward {
		return datasource.Forward
	}
	return datasource.Backward
}
