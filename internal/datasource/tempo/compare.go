package tempo

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"

	"golang.org/x/sync/errgroup"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const compareConcurrency = 4

var _ datasource.TraceAttributeSource = (*Datasource)(nil)

func (d *Datasource) TraceAttributes(ctx context.Context, req datasource.TraceAttributesRequest) ([]datasource.TraceAttributeSet, error) {
	found := make([]*datasource.TraceAttributeSet, len(req.IDs))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(compareConcurrency)
	for i, id := range req.IDs {
		if groupCtx.Err() != nil {
			break
		}
		group.Go(func() error {
			callCtx, cancel := bound(groupCtx, d.opts.Limits.Timeout)
			defer cancel()
			rows, _, err := d.fetchSpans(callCtx, id, req.Range)
			switch {
			case errors.Is(err, datasource.ErrNotFound):
				return nil
			case err != nil:
				return err
			}
			if req.MaxSpans > 0 && len(rows) > req.MaxSpans {
				rows = rows[:req.MaxSpans]
			}
			found[i] = &datasource.TraceAttributeSet{ID: id, Attributes: pairsOf(rows)}
			return nil
		})
	}
	err := group.Wait()
	sets := make([]datasource.TraceAttributeSet, 0, len(found))
	for _, set := range found {
		if set != nil {
			sets = append(sets, *set)
		}
	}
	if err == nil && ctx.Err() != nil {
		err = datasource.ContextError(ctx, ctx.Err(), 0)
	}
	if err != nil && (ctx.Err() == nil || len(sets) == 0) {
		return nil, err
	}
	return sets, nil
}

func pairsOf(rows []spanRow) []datasource.TraceAttribute {
	seen := map[datasource.TraceAttribute]struct{}{}
	add := func(key, value string) { seen[datasource.TraceAttribute{Key: key, Value: value}] = struct{}{} }
	for _, r := range rows {
		add("name", r.name)
		add("kind", r.kind)
		add("status", r.status)
		for key, value := range r.res {
			if text, ok := scalarText(value); ok {
				add("resource."+key, text)
			}
		}
		for key, value := range r.attrs {
			if text, ok := scalarText(value); ok {
				add("span."+key, text)
			}
		}
	}
	pairs := make([]datasource.TraceAttribute, 0, len(seen))
	for pair := range seen {
		pairs = append(pairs, pair)
	}
	slices.SortFunc(pairs, func(a, b datasource.TraceAttribute) int {
		return cmp.Or(cmp.Compare(a.Key, b.Key), cmp.Compare(a.Value, b.Value))
	})
	return pairs
}

func scalarText(value any) (string, bool) {
	switch v := value.(type) {
	case string:
		return v, true
	case bool:
		return strconv.FormatBool(v), true
	case json.Number:
		return v.String(), true
	}
	return "", false
}
