package elasticsearch

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

var _ datasource.PageSource = (*Datasource)(nil)

func (d *Datasource) PagesByOffset() bool { return true }

func (d *Datasource) QueryPage(ctx context.Context, req datasource.PageRequest) ([]datasource.Frame, datasource.PageInfo, error) {
	if req.PageSize <= 0 {
		return nil, datasource.PageInfo{}, datasource.Errorf(datasource.ErrInvalidQuery, "page size must be positive")
	}
	page := max(req.Page, 1)
	lim := d.opts.Limits.Tighten(req.Query.Limits)
	ctx, cancel := bound(ctx, lim.Timeout)
	defer cancel()
	prep, err := d.prepare(ctx, req.Query, req.Range, instantInterval(req.Range, req.Query.MinInterval), lim)
	if err != nil {
		return nil, datasource.PageInfo{}, err
	}
	if prep.user.aggs() != nil {
		return nil, datasource.PageInfo{}, datasource.Errorf(datasource.ErrInvalidQuery, "paging needs a KQL, Lucene or filter-only query body")
	}
	window := prep.mapping.Window
	if page > window/req.PageSize {
		return nil, datasource.PageInfo{}, pagePastWindow(window)
	}
	p := plan{kind: searchLines, size: req.PageSize, from: (page - 1) * req.PageSize, paged: true, timeout: lim.Timeout, totalHits: true}
	raw, err := d.search(ctx, buildBody(prep.user, d.opts.TimeField, req.Range, prep.adhoc, p))
	if err != nil {
		return nil, datasource.PageInfo{}, resultWindowError(err, window)
	}
	resp, err := parseSearch(raw)
	if err != nil {
		return nil, datasource.PageInfo{}, err
	}
	frame, err := d.hitsResult(resp, prep.mapping)
	if err != nil {
		return nil, datasource.PageInfo{}, err
	}
	frame.Meta = &datasource.FrameMeta{Partial: resp.partial()}
	return []datasource.Frame{frame}, pageInfo(resp, page, req.PageSize, window), nil
}

func pageInfo(resp searchResponse, page, pageSize, window int) datasource.PageInfo {
	total := resp.Hits.Total.Value
	info := datasource.PageInfo{
		Page:          page,
		PageSize:      pageSize,
		Total:         total,
		TotalRelation: datasource.TotalAtLeast,
		HasNext:       int64(page)*int64(pageSize) < min(total, int64(window)),
		Window:        window,
	}
	if resp.Hits.Total.Relation == "eq" {
		info.TotalRelation = datasource.TotalExact
	}
	return info
}

func pagePastWindow(window int) error {
	return datasource.Errorf(datasource.ErrInvalidQuery, "pages past %s results are not available on this index; narrow the time range", groupThousands(window))
}

func groupThousands(n int) string {
	digits := strconv.Itoa(n)
	var b strings.Builder
	for i, c := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return b.String()
}

func resultWindowError(err error, window int) error {
	var upstream *datasource.UpstreamError
	if errors.As(err, &upstream) && strings.Contains(upstream.Message, "Result window is too large") {
		return pagePastWindow(window)
	}
	return err
}
