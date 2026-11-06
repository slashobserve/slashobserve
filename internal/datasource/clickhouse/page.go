package clickhouse

import (
	"context"

	ch "github.com/ClickHouse/clickhouse-go/v2"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

var _ datasource.PageSource = (*Datasource)(nil)

func (d *Datasource) PagesByOffset() bool { return true }

func (d *Datasource) QueryPage(ctx context.Context, req datasource.PageRequest) ([]datasource.Frame, datasource.PageInfo, error) {
	page := max(req.Page, 1)
	offset := (page - 1) * req.PageSize
	var (
		frames []datasource.Frame
		info   datasource.PageInfo
	)
	err := d.readLines(ctx, req.Query, req.Range, func(ctx context.Context, inner string, lim datasource.Limits, extra ch.Settings) error {
		lim.MaxRows = min(req.PageSize, lim.MaxRows)
		rs, err := d.fetchWith(ctx, linesQuery(inner, datasource.Backward), lim, linesParameters(req.Range, lim.MaxRows, offset), extra)
		if err != nil {
			return err
		}
		lim.MaxRows = 1
		counted, err := d.fetchWith(ctx, countQuery(inner), lim, rangeParameters(req.Range), extra)
		if err != nil {
			return err
		}
		total, ok := countOf(counted)
		if !ok {
			return datasource.Errorf(datasource.ErrUnavailable, "the count query returned no number")
		}
		frames = []datasource.Frame{rs.tableFrame()}
		info = pageInfo(page, req.PageSize, offset, len(rs.rows), total)
		return nil
	})
	return frames, info, err
}

func pageInfo(page, pageSize, offset, rows, total int) datasource.PageInfo {
	info := datasource.PageInfo{Page: page, PageSize: pageSize, Total: int64(total), TotalRelation: datasource.TotalExact, HasNext: offset+rows < total}
	if info.HasNext {
		info.Next = &datasource.PageCursor{Page: page + 1}
	}
	return info
}
