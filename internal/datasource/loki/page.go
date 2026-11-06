package loki

import (
	"cmp"
	"context"
	"slices"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const bodyField = "Body"

var _ datasource.PageSource = (*Datasource)(nil)

func (d *Datasource) PagesByOffset() bool { return false }

type lineRow struct {
	frame, index int
	ns           int64
	fp           string
}

func (d *Datasource) QueryPage(ctx context.Context, req datasource.PageRequest) ([]datasource.Frame, datasource.PageInfo, error) {
	page, limit, rng := max(req.Page, 1), req.PageSize+1, req.Range
	seen := map[string]bool{}
	if c := req.Cursor; c != nil {
		page, limit, rng.To = c.Page, limit+len(c.Seen), time.Unix(0, c.EndNs)
		for _, fp := range c.Seen {
			seen[fp] = true
		}
	}
	frames, fetched, err := d.lines(ctx, datasource.LinesRequest{Query: req.Query, Range: rng, Direction: datasource.Backward, Limit: limit})
	if err != nil {
		return nil, datasource.PageInfo{}, err
	}
	rows := slices.DeleteFunc(rowsOf(frames), func(r lineRow) bool { return seen[r.fp] })
	hasNext := len(rows) > req.PageSize || fetched == limit
	rows = rows[:min(len(rows), req.PageSize)]
	info := datasource.PageInfo{Page: page, PageSize: req.PageSize, HasNext: hasNext}
	info.Total, info.TotalRelation = datasource.PageTotal(page, req.PageSize, len(rows), hasNext)
	if hasNext && len(rows) > 0 {
		oldest := rows[len(rows)-1].ns
		var boundary []string
		for _, r := range rows {
			if r.ns == oldest {
				boundary = append(boundary, r.fp)
			}
		}
		if len(boundary) == req.PageSize {
			return nil, datasource.PageInfo{}, datasource.Errorf(datasource.ErrLimitExceeded, "%s", datasource.PageTimestampShared)
		}
		info.Next = &datasource.PageCursor{Page: page + 1, EndNs: oldest + 1, Seen: boundary}
	}
	return keepRows(frames, rows), info, nil
}

func rowsOf(frames []datasource.Frame) []lineRow {
	var rows []lineRow
	for i, f := range frames {
		bodies := fieldValues(f, bodyField)
		for j := range f.Rows() {
			ns, _ := datasource.TimeNsOf(f, j)
			line, _ := bodies[j].(string)
			rows = append(rows, lineRow{frame: i, index: j, ns: ns, fp: datasource.RowFingerprint(f.Labels, line)})
		}
	}
	slices.SortFunc(rows, func(a, b lineRow) int {
		return cmp.Or(cmp.Compare(b.ns, a.ns), cmp.Compare(a.fp, b.fp))
	})
	return rows
}

func fieldValues(f datasource.Frame, name string) []any {
	for _, field := range f.Fields {
		if field.Name == name {
			return field.Values
		}
	}
	return make([]any, f.Rows())
}

func keepRows(frames []datasource.Frame, kept []lineRow) []datasource.Frame {
	indexes := make([][]int, len(frames))
	for _, r := range kept {
		indexes[r.frame] = append(indexes[r.frame], r.index)
	}
	out := []datasource.Frame{}
	for i, f := range frames {
		if len(indexes[i]) == 0 {
			continue
		}
		slices.Sort(indexes[i])
		fields := make([]datasource.Field, len(f.Fields))
		for k, field := range f.Fields {
			values := make([]any, 0, len(indexes[i]))
			for _, j := range indexes[i] {
				values = append(values, field.Values[j])
			}
			fields[k] = datasource.Field{Name: field.Name, Type: field.Type, Values: values}
		}
		out = append(out, datasource.Frame{Name: f.Name, Labels: f.Labels, Fields: fields})
	}
	return out
}
