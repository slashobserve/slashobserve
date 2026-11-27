package elasticsearch

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const (
	spanPageSize       = 5000
	compareConcurrency = 4
)

var (
	_ datasource.TraceSource          = (*SpanIndex)(nil)
	_ datasource.TraceAttributeSource = (*SpanIndex)(nil)
)

type spanHit struct {
	Source json.RawMessage `json:"_source"`
	Sort   []any           `json:"sort"`
}

type spanPage struct {
	TimedOut bool `json:"timed_out"`
	Shards   struct {
		Failed int `json:"failed"`
	} `json:"_shards"`
	Hits struct {
		Total struct {
			Value int64 `json:"value"`
		} `json:"total"`
		Hits []spanHit `json:"hits"`
	} `json:"hits"`
}

type spanScan struct {
	spans   []json.RawMessage
	total   int64
	bytes   int
	partial bool
}

func (s *SpanIndex) timeout() string {
	return strconv.FormatInt(s.opts.Limits.Timeout.Milliseconds(), 10) + "ms"
}

func spanRange(r datasource.TimeRange) map[string]any {
	return map[string]any{"range": map[string]any{JaegerTimeField: map[string]any{
		"gte":    r.From.UnixMilli(),
		"lte":    r.To.UnixMilli(),
		"format": "epoch_millis",
	}}}
}

func filterOf(clauses ...any) map[string]any {
	return map[string]any{"bool": map[string]any{"filter": clauses}}
}

func traceIDsQuery(ids []string) map[string]any {
	var forms []string
	for _, id := range ids {
		for _, form := range storedTraceIDs(id) {
			if !slices.Contains(forms, form) {
				forms = append(forms, form)
			}
		}
	}
	return map[string]any{"terms": map[string]any{spanTraceIDField: forms}}
}

func spanSort(order string) []any {
	return []any{
		map[string]any{spanStartField: map[string]any{"order": order}},
		map[string]any{spanIDField: map[string]any{"order": "asc"}},
	}
}

// scan pages through the spans a query matches with search_after, because one search returns at most max_result_window hits.
func (s *SpanIndex) scan(ctx context.Context, query map[string]any, order string, limit int, source []string) (spanScan, error) {
	var out spanScan
	var after []any
	for len(out.spans) < limit {
		body := map[string]any{
			"timeout":          s.timeout(),
			"query":            query,
			"size":             min(spanPageSize, limit-len(out.spans)),
			"sort":             spanSort(order),
			"track_total_hits": after == nil,
		}
		if source != nil {
			body["_source"] = source
		}
		if after != nil {
			body["search_after"] = after
		}
		raw, err := s.search(ctx, body)
		if err != nil {
			return spanScan{}, err
		}
		var page spanPage
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if dec.Decode(&page) != nil {
			return spanScan{}, datasource.Unreadable()
		}
		if after == nil {
			out.total = page.Hits.Total.Value
		}
		out.bytes += len(raw)
		out.partial = out.partial || page.TimedOut || page.Shards.Failed > 0
		for _, h := range page.Hits.Hits {
			out.spans = append(out.spans, h.Source)
		}
		if len(page.Hits.Hits) < body["size"].(int) || len(page.Hits.Hits) == 0 {
			break
		}
		after = page.Hits.Hits[len(page.Hits.Hits)-1].Sort
	}
	return out, nil
}

func (s *SpanIndex) Trace(ctx context.Context, id string, r datasource.TimeRange, maxSpans int) (datasource.Frame, error) {
	if !datasource.ValidTraceID(id) {
		return datasource.Frame{}, datasource.Errorf(datasource.ErrInvalidQuery, "trace id must be 16 or 32 hex characters")
	}
	ctx, cancel := datasource.Bound(ctx, s.opts.Limits.Timeout)
	defer cancel()
	docs, scan, err := s.traceSpans(ctx, id, r, maxSpans+1)
	if err != nil {
		return datasource.Frame{}, err
	}
	kept := docs[:min(len(docs), maxSpans)]
	f := spanFrame(kept)
	f.Meta = &datasource.FrameMeta{Bytes: int64(scan.bytes), Partial: scan.partial}
	if len(docs) > maxSpans {
		f.Meta.Truncated, f.Meta.RowsTotal = true, int(max(scan.total, int64(len(docs))))
	}
	return f, nil
}

func (s *SpanIndex) traceSpans(ctx context.Context, id string, r datasource.TimeRange, limit int) ([]spanDoc, spanScan, error) {
	if err := r.Validate(); err != nil {
		return nil, spanScan{}, err
	}
	scan, err := s.scan(ctx, filterOf(spanRange(r), traceIDsQuery([]string{id})), "asc", limit, nil)
	if err != nil {
		return nil, spanScan{}, err
	}
	if len(scan.spans) == 0 {
		return nil, scan, datasource.Errorf(datasource.ErrNotFound, "trace not found")
	}
	docs := make([]spanDoc, 0, len(scan.spans))
	for _, raw := range scan.spans {
		doc, err := decodeSpan(raw)
		if err != nil {
			return nil, scan, err
		}
		docs = append(docs, doc)
	}
	slices.SortStableFunc(docs, func(a, b spanDoc) int {
		return cmp.Or(a.start.Compare(b.start), strings.Compare(a.spanID, b.spanID))
	})
	return docs, scan, nil
}

func (s *SpanIndex) TraceAttributes(ctx context.Context, req datasource.TraceAttributesRequest) ([]datasource.TraceAttributeSet, error) {
	found := make([]*datasource.TraceAttributeSet, len(req.IDs))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(compareConcurrency)
	limit := cmp.Or(req.MaxSpans, datasource.CompareSpanBudget)
	for i, id := range req.IDs {
		if groupCtx.Err() != nil {
			break
		}
		group.Go(func() error {
			callCtx, cancel := datasource.Bound(groupCtx, s.opts.Limits.Timeout)
			defer cancel()
			docs, _, err := s.traceSpans(callCtx, id, req.Range, limit)
			switch {
			case errors.Is(err, datasource.ErrNotFound):
				return nil
			case err != nil:
				return err
			}
			found[i] = &datasource.TraceAttributeSet{ID: id, Attributes: pairsOf(docs)}
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

func pairsOf(docs []spanDoc) []datasource.TraceAttribute {
	seen := map[datasource.TraceAttribute]struct{}{}
	add := func(key, value string) { seen[datasource.TraceAttribute{Key: key, Value: value}] = struct{}{} }
	for _, d := range docs {
		add("name", d.name)
		add("kind", d.kind)
		add("status", d.status)
		for key, value := range d.resource {
			if text, ok := scalarText(value); ok {
				add("resource."+key, text)
			}
		}
		for key, value := range d.attrs {
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

func spanFrame(docs []spanDoc) datasource.Frame {
	columns := []struct {
		name  string
		typ   datasource.FieldType
		value func(spanDoc) any
	}{
		{"TraceId", datasource.FieldString, func(d spanDoc) any { return d.traceID }},
		{"SpanId", datasource.FieldString, func(d spanDoc) any { return d.spanID }},
		{"ParentSpanId", datasource.FieldString, func(d spanDoc) any { return d.parentID }},
		{"SpanName", datasource.FieldString, func(d spanDoc) any { return d.name }},
		{"ServiceName", datasource.FieldString, func(d spanDoc) any { return d.service }},
		{"StartTime", datasource.FieldTime, func(d spanDoc) any { return d.start }},
		{"Duration", datasource.FieldNumber, func(d spanDoc) any { return d.durationNanos }},
		{"StatusCode", datasource.FieldString, func(d spanDoc) any { return d.status }},
		{"StatusMessage", datasource.FieldString, func(d spanDoc) any { return d.statusMessage }},
		{"SpanAttributes", datasource.FieldString, func(d spanDoc) any { return jsonOrEmpty(d.attrs) }},
		{"ResourceAttributes", datasource.FieldString, func(d spanDoc) any { return jsonOrEmpty(d.resource) }},
		{"SpanKind", datasource.FieldString, func(d spanDoc) any { return d.kind }},
		{"SpanEvents", datasource.FieldString, func(d spanDoc) any { return eventsText(d.events) }},
		{"SpanLinks", datasource.FieldString, func(d spanDoc) any { return linksText(d.links) }},
	}
	f := datasource.Frame{Fields: make([]datasource.Field, len(columns))}
	for i, c := range columns {
		values := make([]any, len(docs))
		for j, d := range docs {
			values[j] = c.value(d)
		}
		f.Fields[i] = datasource.Field{Name: c.name, Type: c.typ, Values: values}
	}
	return f
}

func jsonOrEmpty(v map[string]any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func eventsText(events []datasource.SpanEvent) string {
	text, err := datasource.SpanEventsJSON(events)
	if err != nil {
		return "[]"
	}
	return text
}

func linksText(links []datasource.SpanLink) string {
	text, err := datasource.SpanLinksJSON(links)
	if err != nil {
		return "[]"
	}
	return text
}
