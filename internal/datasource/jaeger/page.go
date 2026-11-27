package jaeger

import (
	"context"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const pagingNeedsSearch = "paging needs a Jaeger search, not a trace id"

var _ datasource.PageSource = (*Datasource)(nil)

func (d *Datasource) PagesByOffset() bool { return true }

func (d *Datasource) QueryPage(ctx context.Context, req datasource.PageRequest) ([]datasource.Frame, datasource.PageInfo, error) {
	if err := req.Range.Validate(); err != nil {
		return nil, datasource.PageInfo{}, err
	}
	if req.PageSize <= 0 {
		return nil, datasource.PageInfo{}, datasource.Errorf(datasource.ErrInvalidQuery, "page size must be positive")
	}
	lim := d.opts.Limits.Tighten(req.Query.Limits)
	ctx, cancel := datasource.Bound(ctx, lim.Timeout)
	defer cancel()
	text, err := expandQuery(req.Query, req.Range)
	if err != nil {
		return nil, datasource.PageInfo{}, err
	}
	if _, ok := traceIDOf(text); ok {
		return nil, datasource.PageInfo{}, datasource.Errorf(datasource.ErrInvalidQuery, "%s", pagingNeedsSearch)
	}
	s, err := searchOf(text)
	if err != nil {
		return nil, datasource.PageInfo{}, err
	}
	page := max(req.Page, 1)
	window := searchDepth(lim.MaxRows)
	if (page-1)*req.PageSize >= window {
		return nil, datasource.PageInfo{}, datasource.Errorf(datasource.ErrInvalidQuery, "pages past %d traces are not available; narrow the time range or the search", window)
	}
	rows, err := d.searchRows(ctx, s, req.Range, min(page*req.PageSize+1, window))
	if err != nil {
		return nil, datasource.PageInfo{}, err
	}
	from := min((page-1)*req.PageSize, len(rows))
	rows = rows[from:]
	hasNext := len(rows) > req.PageSize
	rows = rows[:min(len(rows), req.PageSize)]
	total, relation := datasource.PageTotal(page, req.PageSize, len(rows), hasNext)
	info := datasource.PageInfo{Page: page, PageSize: req.PageSize, HasNext: hasNext, Total: total, TotalRelation: relation, Window: window}
	if len(rows) == 0 {
		return []datasource.Frame{}, info, nil
	}
	return []datasource.Frame{traceFrame(rows)}, info, nil
}
