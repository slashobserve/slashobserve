package elasticsearch

import (
	"context"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const tailLag = 5 * time.Second

var (
	_ datasource.LineSource = (*Datasource)(nil)
	_ datasource.LagSource  = (*Datasource)(nil)
)

func (d *Datasource) TailLag() time.Duration { return tailLag }

func (d *Datasource) Lines(ctx context.Context, req datasource.LinesRequest) ([]datasource.Frame, error) {
	if err := req.Range.Validate(); err != nil {
		return nil, err
	}
	if req.Limit < 1 {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "limit must be at least 1")
	}
	order := "desc"
	if req.Direction == datasource.Forward {
		order = "asc"
	}
	lim := d.opts.Limits.Tighten(req.Query.Limits)
	ctx, cancel := bound(ctx, lim.Timeout)
	defer cancel()
	prep, err := d.prepare(ctx, req.Query, req.Range, instantInterval(req.Range, req.Query.MinInterval), lim)
	if err != nil {
		return nil, err
	}
	if prep.user.aggs() != nil {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "lines need a KQL, Lucene or filter-only query body")
	}
	size := min(req.Limit, maxLogLines)
	body := buildBody(prep.user, d.opts.TimeField, req.Range, prep.adhoc, plan{kind: searchLines, size: size, paged: true, timeout: lim.Timeout, totalHits: false})
	body["sort"] = lineSort(d.opts.TimeField, order)
	return d.lineFrames(ctx, body, prep.mapping, size)
}

func lineSort(timeField, order string) []any {
	return []any{
		map[string]any{timeField: map[string]any{"order": order, "numeric_type": "date_nanos", "unmapped_type": "date"}},
		map[string]any{"_doc": order},
	}
}

func (d *Datasource) lineFrames(ctx context.Context, body map[string]any, m *Mapping, size int) ([]datasource.Frame, error) {
	raw, err := d.search(ctx, body)
	if err != nil {
		return nil, err
	}
	resp, err := parseSearch(raw)
	if err != nil {
		return nil, err
	}
	frame, err := d.hitsResult(resp, m)
	if err != nil {
		return nil, err
	}
	frame.Meta = &datasource.FrameMeta{Truncated: len(resp.Hits.Hits) == size, Partial: resp.partial()}
	return []datasource.Frame{frame}, nil
}
