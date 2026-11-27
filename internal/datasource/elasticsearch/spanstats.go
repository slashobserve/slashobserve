package elasticsearch

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const (
	timeAgg       = "time"
	errorsAgg     = "errors"
	quantilesAgg  = "q"
	histogramAgg  = "hist"
	histogramTops = 40

	spanQuantilesNote = "Per-bucket quantiles are the search engine's percentile estimates (t-digest), so they are approximate, and the histogram lists bucket upper bounds, not quantiles."
	spanGroupHint     = "group_by must be service (or resource.service.name), name, status, kind, statusMessage, span.<tag> for a span tag or resource.<tag> for a process tag"
	spanFilterHint    = "the trace filter is a query over the span documents in the datasource's query language, such as process.serviceName:checkout; a Query DSL body may hold only query"
)

var (
	_ datasource.TraceStatsSource    = (*SpanIndex)(nil)
	_ datasource.TraceDurationSource = (*SpanIndex)(nil)
)

var percentKeys = [3]string{"50.0", "95.0", "99.0"}

func (s *SpanIndex) spanFilter(ctx context.Context, text string) (map[string]any, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return matchAll(), nil
	}
	form, body := Classify(text, s.opts.QueryLanguage)
	m, err := s.Mapping(ctx)
	if err != nil {
		return nil, err
	}
	user, err := compileUser(form, body, m)
	if err != nil {
		return nil, err
	}
	if len(user.dsl) > 0 {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "%s", spanFilterHint)
	}
	return user.clause, nil
}

// durationRanges buckets span durations by powers of two microseconds; a range's to is exclusive, so each bucket holds (2^(j-1), 2^j] whole microseconds.
func durationRanges() []any {
	ranges := []any{map[string]any{"key": "0", "to": 2}}
	for j := 1; j <= histogramTops; j++ {
		r := map[string]any{"key": strconv.Itoa(j), "from": int64(1)<<(j-1) + 1}
		if j < histogramTops {
			r["to"] = int64(1)<<j + 1
		}
		ranges = append(ranges, r)
	}
	return ranges
}

func statsBody(filter map[string]any, r datasource.TimeRange, step time.Duration, timeout string) map[string]any {
	return map[string]any{
		"timeout":          timeout,
		"size":             0,
		"track_total_hits": false,
		"query":            filterOf(spanRange(r), filter),
		"aggs": map[string]any{
			timeAgg: map[string]any{
				"date_histogram": map[string]any{"field": JaegerTimeField, "fixed_interval": strconv.FormatInt(max(step.Milliseconds(), 1), 10) + "ms", "min_doc_count": 1},
				"aggs": map[string]any{
					errorsAgg:    map[string]any{"filter": errorSpan()},
					quantilesAgg: map[string]any{"percentiles": map[string]any{"field": spanDurationField, "percents": []float64{50, 95, 99}}},
				},
			},
			histogramAgg: map[string]any{"range": map[string]any{"field": spanDurationField, "ranges": durationRanges()}},
		},
	}
}

type statsResponse struct {
	TimedOut bool `json:"timed_out"`
	Shards   struct {
		Failed int `json:"failed"`
	} `json:"_shards"`
	Aggregations struct {
		Time struct {
			Buckets []struct {
				Key      int64 `json:"key"`
				DocCount int64 `json:"doc_count"`
				Errors   struct {
					DocCount int64 `json:"doc_count"`
				} `json:"errors"`
				Q struct {
					Values map[string]*float64 `json:"values"`
				} `json:"q"`
			} `json:"buckets"`
		} `json:"time"`
		Hist struct {
			Buckets []struct {
				Key      string `json:"key"`
				DocCount int64  `json:"doc_count"`
			} `json:"buckets"`
		} `json:"hist"`
	} `json:"aggregations"`
}

func (s *SpanIndex) TraceStats(ctx context.Context, req datasource.TraceStatsRequest) (datasource.TraceStats, error) {
	if err := req.Range.Validate(); err != nil {
		return datasource.TraceStats{}, err
	}
	if req.Step <= 0 {
		return datasource.TraceStats{}, datasource.Errorf(datasource.ErrInvalidQuery, "step must be positive")
	}
	lim := s.opts.Limits.Tighten(req.Limits)
	ctx, cancel := bound(ctx, lim.Timeout)
	defer cancel()
	filter, err := s.spanFilter(ctx, req.Query)
	if err != nil {
		return datasource.TraceStats{}, err
	}
	raw, err := s.search(ctx, statsBody(filter, req.Range, req.Step, s.timeout()))
	if err != nil {
		return datasource.TraceStats{}, err
	}
	var resp statsResponse
	if json.NewDecoder(bytes.NewReader(raw)).Decode(&resp) != nil {
		return datasource.TraceStats{}, unreadable()
	}
	return foldSpanStats(req.Range, req.Step, resp), nil
}

func foldSpanStats(r datasource.TimeRange, step time.Duration, resp statsResponse) datasource.TraceStats {
	n := int(r.To.Sub(r.From) / step)
	if r.To.Sub(r.From)%step != 0 {
		n++
	}
	stats := datasource.TraceStats{Step: step, Partial: resp.TimedOut || resp.Shards.Failed > 0, Buckets: make([]datasource.TraceBucket, max(n, 1)), Histogram: []datasource.DurationBucket{}}
	for i := range stats.Buckets {
		stats.Buckets[i].Start = r.From.Add(time.Duration(i) * step)
	}
	for _, b := range resp.Aggregations.Time.Buckets {
		i := int(time.UnixMilli(b.Key).Sub(r.From) / step)
		if i < 0 || i >= len(stats.Buckets) || b.DocCount == 0 {
			continue
		}
		tb := &stats.Buckets[i]
		tb.Spans += b.DocCount
		tb.Errors += b.Errors.DocCount
		tb.P50, tb.P95, tb.P99 = microsToMillis(b.Q.Values[percentKeys[0]]), microsToMillis(b.Q.Values[percentKeys[1]]), microsToMillis(b.Q.Values[percentKeys[2]])
	}
	for _, b := range stats.Buckets {
		stats.Spans += b.Spans
		stats.Errors += b.Errors
	}
	for _, b := range resp.Aggregations.Hist.Buckets {
		j, err := strconv.Atoi(b.Key)
		if err != nil || b.DocCount == 0 {
			continue
		}
		stats.Histogram = append(stats.Histogram, datasource.DurationBucket{LeMS: float64(int64(1)<<j) / microsPerMilli, Count: b.DocCount})
	}
	if stats.Spans > 0 {
		stats.Note = spanQuantilesNote
	}
	return stats
}

func microsToMillis(v *float64) *float64 {
	if v == nil || math.IsNaN(*v) || math.IsInf(*v, 0) {
		return nil
	}
	ms := *v / microsPerMilli
	return &ms
}

type spanGrouper func(spanDoc) string

var spanGroupFields = map[string]spanGrouper{
	"service":               func(d spanDoc) string { return d.service },
	"resource.service.name": func(d spanDoc) string { return d.service },
	"service.name":          func(d spanDoc) string { return d.service },
	"name":                  func(d spanDoc) string { return d.name },
	"operationName":         func(d spanDoc) string { return d.name },
	"status":                func(d spanDoc) string { return d.status },
	"kind":                  func(d spanDoc) string { return d.kind },
	"statusMessage":         func(d spanDoc) string { return d.statusMessage },
}

func spanGroup(groupBy string) (spanGrouper, bool) {
	groupBy = strings.TrimSpace(groupBy)
	if g, ok := spanGroupFields[groupBy]; ok {
		return g, true
	}
	scope, key, ok := strings.Cut(groupBy, ".")
	if !ok || key == "" || len(key) > maxGroupKeyBytes {
		return nil, false
	}
	switch scope {
	case "span", "":
		return func(d spanDoc) string { return scalarOrEmpty(d.attrs[key]) }, true
	case "resource":
		return func(d spanDoc) string { return scalarOrEmpty(d.resource[key]) }, true
	}
	return nil, false
}

const maxGroupKeyBytes = 128

func scalarOrEmpty(v any) string {
	text, _ := scalarText(v)
	return text
}

var durationSource = []string{spanDurationField, spanIDField, spanOperationField, spanServiceField, "tags", "tag", "process"}

func (s *SpanIndex) TraceDurations(ctx context.Context, req datasource.TraceDurationRequest) (datasource.TraceDurations, error) {
	if err := req.Range.Validate(); err != nil {
		return datasource.TraceDurations{}, err
	}
	var group spanGrouper
	if strings.TrimSpace(req.GroupBy) != "" {
		var ok bool
		if group, ok = spanGroup(req.GroupBy); !ok {
			return datasource.TraceDurations{}, datasource.Errorf(datasource.ErrInvalidQuery, "%s", spanGroupHint)
		}
	}
	lim := s.opts.Limits.Tighten(req.Limits)
	ctx, cancel := bound(ctx, lim.Timeout)
	defer cancel()
	filter, err := s.spanFilter(ctx, req.Query)
	if err != nil {
		return datasource.TraceDurations{}, err
	}
	scan, err := s.scan(ctx, filterOf(spanRange(req.Range), filter), "desc", cmp.Or(max(req.MaxSpans, 0), datasource.TraceDurationSpanBudget), durationSource)
	if err != nil {
		return datasource.TraceDurations{}, err
	}
	spans := make([]datasource.SpanDuration, 0, len(scan.spans))
	for _, raw := range scan.spans {
		doc, err := decodeSpan(raw)
		if err != nil {
			return datasource.TraceDurations{}, err
		}
		sd := datasource.SpanDuration{Nanos: doc.durationNanos, Error: datasource.IsErrorStatus(doc.status)}
		if group != nil {
			sd.Group = group(doc)
			sd.Grouped = sd.Group != ""
		}
		spans = append(spans, sd)
	}
	out := datasource.TraceDurations{Exact: scan.complete()}
	out.Window, out.Groups, out.GroupsTruncated = datasource.FoldDurations(spans, group != nil, datasource.MaxDurationGroups)
	return out, nil
}

func (s spanScan) complete() bool { return int64(len(s.spans)) >= s.total && !s.partial }
