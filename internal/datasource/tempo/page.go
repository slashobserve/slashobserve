package tempo

import (
	"context"
	"slices"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const pagingNeedsSearch = "paging needs a TraceQL search"

var _ datasource.PageSource = (*Datasource)(nil)

func (d *Datasource) PagesByOffset() bool { return false }

func (d *Datasource) QueryPage(ctx context.Context, req datasource.PageRequest) ([]datasource.Frame, datasource.PageInfo, error) {
	if err := req.Range.Validate(); err != nil {
		return nil, datasource.PageInfo{}, err
	}
	lim := d.opts.Limits.Tighten(req.Query.Limits)
	ctx, cancel := bound(ctx, lim.Timeout)
	defer cancel()
	text, err := expandQuery(req.Query, req.Range)
	if err != nil {
		return nil, datasource.PageInfo{}, err
	}
	if _, ok := traceIDOf(text); ok {
		return nil, datasource.PageInfo{}, datasource.Errorf(datasource.ErrInvalidQuery, "%s", pagingNeedsSearch)
	}
	page, limit, rng := max(req.Page, 1), req.PageSize+1, req.Range
	shown := func(spanRow) bool { return false }
	if c := req.Cursor; c != nil {
		page, limit, rng.To = c.Page, limit+len(c.Seen), time.Unix(0, c.EndNs)
		shown = func(r spanRow) bool { return r.start.UnixNano() >= c.EndNs || slices.Contains(c.Seen, r.traceID) }
	}
	rows, metrics, err := d.searchRows(ctx, text, rng, limit)
	if err != nil {
		return nil, datasource.PageInfo{}, err
	}
	fetched := len(rows)
	rows = slices.DeleteFunc(rows, shown)
	hasNext := len(rows) > req.PageSize || fetched >= limit
	rows = rows[:min(len(rows), req.PageSize)]
	info := datasource.PageInfo{Page: page, PageSize: req.PageSize, HasNext: hasNext, TotalRelation: datasource.TotalUnknown}
	if !metrics.partial() {
		info.Total, info.TotalRelation = datasource.PageTotal(page, req.PageSize, len(rows), hasNext)
	}
	if hasNext && len(rows) > 0 {
		oldest := rows[len(rows)-1].start
		var boundary []string
		for _, r := range rows {
			if r.start.Truncate(time.Second).Equal(oldest.Truncate(time.Second)) {
				boundary = append(boundary, r.traceID)
			}
		}
		if len(boundary) == req.PageSize {
			return nil, datasource.PageInfo{}, datasource.Errorf(datasource.ErrLimitExceeded, "%s", datasource.PageTimestampShared)
		}
		info.Next = &datasource.PageCursor{Page: page + 1, EndNs: oldest.UnixNano() + 1, Seen: boundary}
	}
	if len(rows) == 0 {
		return []datasource.Frame{}, info, nil
	}
	f := traceFrame(rows)
	if metrics.partial() || metrics.TotalJobs > 0 {
		f.Meta = searchMeta(metrics, false)
	}
	return []datasource.Frame{f}, info, nil
}
