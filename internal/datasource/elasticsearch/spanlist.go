package elasticsearch

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const (
	traceAgg = "traces"
	startAgg = "start"
	longAgg  = "longest"
	// maxCandidateTraces bounds the traces one listing aggregates when several conditions must each hold in some span, under the default search.max_buckets of 65,535 with a filter bucket per condition.
	maxCandidateTraces = 8000
)

var _ datasource.TraceListSource = (*SpanIndex)(nil)

func conditionName(i int) string { return "c" + strconv.Itoa(i) }

// listConditions are the clauses a trace must each match in at least one of its spans.
func listConditions(req datasource.TraceListRequest) []any {
	var conds []any
	if req.Filter.Service != "" {
		conds = append(conds, termQuery(spanServiceField, req.Filter.Service))
	}
	if req.Filter.Operation != "" {
		conds = append(conds, termQuery(spanOperationField, req.Filter.Operation))
	}
	if req.Filter.Route != "" {
		conds = append(conds, filterOf(termQuery(spanOperationField, req.Filter.Route), rootSpan()))
	}
	if req.Errors {
		conds = append(conds, errorSpan())
	}
	if req.MinDuration > 0 {
		micros := max((req.MinDuration+time.Microsecond-1)/time.Microsecond, 1)
		conds = append(conds, map[string]any{"range": map[string]any{spanDurationField: map[string]any{"gte": int64(micros)}}})
	}
	return conds
}

func listBody(req datasource.TraceListRequest, timeout string) map[string]any {
	limit := cmp.Or(max(req.Limit, 0), datasource.TraceListBudget)
	conds := listConditions(req)
	filter := []any{spanRange(req.Range)}
	size := limit + 1
	switch len(conds) {
	case 0:
	case 1:
		filter = append(filter, conds[0])
	default:
		filter = append(filter, anyOf(conds...))
		size = max(size, maxCandidateTraces)
	}
	sub := map[string]any{
		startAgg: map[string]any{"min": map[string]any{"field": spanStartField}},
		longAgg:  map[string]any{"max": map[string]any{"field": spanDurationField}},
	}
	if len(conds) > 1 {
		for i, c := range conds {
			sub[conditionName(i)] = map[string]any{"filter": c}
		}
	}
	return map[string]any{
		"timeout":          timeout,
		"size":             0,
		"track_total_hits": false,
		"query":            map[string]any{"bool": map[string]any{"filter": filter}},
		"aggs": map[string]any{traceAgg: map[string]any{
			"terms": map[string]any{"field": spanTraceIDField, "size": size, "order": map[string]any{startAgg: "desc"}},
			"aggs":  sub,
		}},
	}
}

type traceBucket struct {
	Key    string                     `json:"key"`
	Values map[string]json.RawMessage `json:"-"`
}

func (b *traceBucket) UnmarshalJSON(raw []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	b.Values = fields
	return json.Unmarshal(fields["key"], &b.Key)
}

func (b traceBucket) metric(name string) float64 {
	var v struct {
		Value *float64 `json:"value"`
	}
	if json.Unmarshal(b.Values[name], &v) != nil || v.Value == nil {
		return 0
	}
	return *v.Value
}

func (b traceBucket) matched(name string) bool {
	var v struct {
		DocCount int64 `json:"doc_count"`
	}
	return json.Unmarshal(b.Values[name], &v) == nil && v.DocCount > 0
}

type candidate struct {
	stored  string
	start   time.Time
	longest time.Duration
}

func (s *SpanIndex) ListTraces(ctx context.Context, req datasource.TraceListRequest) (datasource.TraceList, error) {
	if err := req.Range.Validate(); err != nil {
		return datasource.TraceList{}, err
	}
	lim := s.opts.Limits.Tighten(req.Limits)
	ctx, cancel := bound(ctx, lim.Timeout)
	defer cancel()
	limit := cmp.Or(max(req.Limit, 0), datasource.TraceListBudget)
	candidates, complete, err := s.candidates(ctx, req, limit)
	if err != nil {
		return datasource.TraceList{}, err
	}
	roots, rootsPartial, err := s.roots(ctx, candidates, req.Range)
	if err != nil {
		return datasource.TraceList{}, err
	}
	out := datasource.TraceList{Traces: make([]datasource.TraceSummary, 0, len(candidates)), Complete: complete && !rootsPartial}
	for _, c := range candidates {
		summary := datasource.TraceSummary{ID: normalizeID(c.stored, traceIDLength), Start: c.start, Duration: c.longest}
		if root, ok := roots[c.stored]; ok {
			summary.RootService, summary.Route, summary.Start, summary.Duration = root.service, root.name, root.start, time.Duration(root.durationNanos)
		}
		out.Traces = append(out.Traces, summary)
	}
	slices.SortStableFunc(out.Traces, func(a, b datasource.TraceSummary) int {
		return cmp.Or(b.Start.Compare(a.Start), strings.Compare(a.ID, b.ID))
	})
	return out, nil
}

func (s *SpanIndex) candidates(ctx context.Context, req datasource.TraceListRequest, limit int) ([]candidate, bool, error) {
	body := listBody(req, s.timeout())
	raw, err := s.search(ctx, body)
	if err != nil {
		return nil, false, err
	}
	var resp struct {
		TimedOut bool `json:"timed_out"`
		Shards   struct {
			Failed int `json:"failed"`
		} `json:"_shards"`
		Aggregations map[string]struct {
			Buckets          []traceBucket `json:"buckets"`
			SumOtherDocCount int64         `json:"sum_other_doc_count"`
		} `json:"aggregations"`
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	if dec.Decode(&resp) != nil {
		return nil, false, unreadable()
	}
	agg := resp.Aggregations[traceAgg]
	conds := len(listConditions(req))
	var out []candidate
	for _, b := range agg.Buckets {
		if !matchesEvery(b, conds) {
			continue
		}
		out = append(out, candidate{stored: b.Key, start: time.UnixMicro(int64(b.metric(startAgg))).UTC(), longest: time.Duration(b.metric(longAgg)) * time.Microsecond})
	}
	complete := agg.SumOtherDocCount == 0 && !resp.TimedOut && resp.Shards.Failed == 0 && len(out) < limit
	return out[:min(len(out), limit)], complete, nil
}

func matchesEvery(b traceBucket, conds int) bool {
	if conds < 2 {
		return true
	}
	for i := range conds {
		if !b.matched(conditionName(i)) {
			return false
		}
	}
	return true
}

// roots reads the root span of each listed trace, which names its route and gives its duration.
func (s *SpanIndex) roots(ctx context.Context, candidates []candidate, r datasource.TimeRange) (map[string]spanDoc, bool, error) {
	out := map[string]spanDoc{}
	if len(candidates) == 0 {
		return out, false, nil
	}
	stored := make([]string, len(candidates))
	for i, c := range candidates {
		stored[i] = c.stored
	}
	query := filterOf(spanRange(r), map[string]any{"terms": map[string]any{spanTraceIDField: stored}}, rootSpan())
	scan, err := s.scan(ctx, query, "asc", 2*len(stored), []string{spanTraceIDField, spanIDField, spanOperationField, spanServiceField, spanStartField, spanDurationField})
	if err != nil {
		return nil, false, err
	}
	for _, raw := range scan.spans {
		var key struct {
			TraceID string `json:"traceID"`
		}
		if json.Unmarshal(raw, &key) != nil {
			return nil, false, unreadable()
		}
		if _, seen := out[key.TraceID]; seen {
			continue
		}
		doc, err := decodeSpan(raw)
		if err != nil {
			return nil, false, err
		}
		out[key.TraceID] = doc
	}
	return out, scan.partial, nil
}

func termQuery(field string, value any) map[string]any {
	return map[string]any{"term": map[string]any{field: value}}
}

func nestedTag(key, value string) map[string]any {
	return map[string]any{"nested": map[string]any{
		"path":            "tags",
		"ignore_unmapped": true,
		"query":           filterOf(termQuery("tags.key", key), termQuery("tags.value", value)),
	}}
}

func anyOf(clauses ...any) map[string]any {
	return map[string]any{"bool": map[string]any{"should": clauses, "minimum_should_match": 1}}
}

func flatTag(key string) string { return "tag." + strings.ReplaceAll(key, ".", tagDot) }

// errorSpan matches a failing span in either tag form: the error tag Jaeger sets from an OTLP error status, or otel.status_code ERROR.
func errorSpan() map[string]any {
	return anyOf(
		termQuery(flatTag(tagError), "true"),
		termQuery(flatTag(tagStatusCode), statusCodeErr),
		nestedTag(tagError, "true"),
		nestedTag(tagStatusCode, statusCodeErr),
	)
}

func rootSpan() map[string]any {
	return map[string]any{"bool": map[string]any{"must_not": []any{map[string]any{"nested": map[string]any{
		"path":            "references",
		"ignore_unmapped": true,
		"query":           termQuery("references.refType", refChildOf),
	}}}}}
}
