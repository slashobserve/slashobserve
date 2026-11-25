package tempo

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const (
	metricsPath           = "/api/metrics/query_range"
	groupByNeedsAttribute = `group_by must be one TraceQL attribute such as resource.service.name, span.http.route or .http.method, or one of name, status, kind and statusMessage`
	statsNeedsFilter      = `trace stats need a TraceQL spanset filter such as { resource.service.name = "checkout" }`
	quantileLabel         = "p"
	quantileQuery         = "quantile_over_time(duration, 0.5, 0.95, 0.99)"
	maxGroups             = 20
	maxGroupByBytes       = 128
	histogramLabel        = "__bucket"
	maxCount              = 1 << 53
)

type metricsResponse struct {
	Series  []metricsSeries `json:"series"`
	Metrics searchMetrics   `json:"metrics"`
}

type metricsSeries struct {
	Labels  []metricsLabel  `json:"labels"`
	Samples []metricsSample `json:"samples"`
}

type metricsLabel struct {
	Key   string   `json:"key"`
	Value anyValue `json:"value"`
}

type metricsSample struct {
	Timestamp unixMillis      `json:"timestampMs"`
	Value     json.RawMessage `json:"value"`
}

type unixMillis int64

func (n *unixMillis) UnmarshalJSON(b []byte) error {
	v, err := quotedInt64(b)
	if err != nil {
		return err
	}
	*n = unixMillis(v)
	return nil
}

type statsResponses struct {
	spans, errors, quantiles, histogram, window metricsResponse
	groupSpans, groupErrors, groupQuantiles     metricsResponse
}

type statsJob struct {
	query string
	step  time.Duration
	into  *metricsResponse
}

func (d *Datasource) TraceStats(ctx context.Context, req datasource.TraceStatsRequest) (datasource.TraceStats, error) {
	q := strings.TrimSpace(req.Query)
	if !strings.HasPrefix(q, "{") || !strings.HasSuffix(q, "}") {
		return datasource.TraceStats{}, datasource.Errorf(datasource.ErrInvalidQuery, "%s", statsNeedsFilter)
	}
	groupBy := strings.TrimSpace(req.GroupBy)
	if groupBy != "" && !validGroupBy(groupBy) {
		return datasource.TraceStats{}, datasource.Errorf(datasource.ErrInvalidQuery, "%s", groupByNeedsAttribute)
	}
	lim := d.opts.Limits.Tighten(req.Limits)
	ctx, cancel := bound(ctx, lim.Timeout)
	defer cancel()
	whole := req.Range.To.Sub(req.Range.From)
	var r statsResponses
	jobs := []statsJob{
		{q + " | count_over_time()", req.Step, &r.spans},
		{errorsQuery(q) + " | count_over_time()", req.Step, &r.errors},
		{q + " | " + quantileQuery, req.Step, &r.quantiles},
		{q + " | histogram_over_time(duration)", req.Step, &r.histogram},
		{q + " | " + quantileQuery, whole, &r.window},
	}
	if groupBy != "" {
		by := " by (" + groupBy + ")"
		jobs = append(jobs,
			statsJob{q + " | count_over_time()" + by, whole, &r.groupSpans},
			statsJob{errorsQuery(q) + " | count_over_time()" + by, whole, &r.groupErrors},
			statsJob{q + " | " + quantileQuery + by, whole, &r.groupQuantiles},
		)
	}
	group, groupCtx := errgroup.WithContext(ctx)
	for _, job := range jobs {
		group.Go(func() error {
			params := url.Values{
				"q":     {job.query},
				"start": {strconv.FormatInt(req.Range.From.UnixNano(), 10)},
				"end":   {strconv.FormatInt(req.Range.To.UnixNano(), 10)},
				"step":  {stepParam(job.step)},
			}
			return d.getJSON(groupCtx, metricsPath, params, job.into)
		})
	}
	if err := group.Wait(); err != nil {
		return datasource.TraceStats{}, err
	}
	return foldStats(req.Range, req.Step, groupBy, r), nil
}

func stepParam(step time.Duration) string {
	return strconv.FormatInt(max(int64(math.Ceil(step.Seconds())), 1), 10) + "s"
}

var (
	scopedAttribute = regexp.MustCompile(`^(\.|(resource|span|event|link|instrumentation)\.)[A-Za-z_][A-Za-z0-9_-]*(\.[A-Za-z0-9_-]+)*$`)
	groupIntrinsics = []string{"name", "status", "kind", "statusMessage"}
)

func validGroupBy(attribute string) bool {
	return len(attribute) <= maxGroupByBytes && (scopedAttribute.MatchString(attribute) || slices.Contains(groupIntrinsics, attribute))
}

func errorsQuery(q string) string {
	inner := strings.TrimSpace(q[1 : len(q)-1])
	if inner == "" {
		return "{ status = error }"
	}
	return strings.TrimSpace(q[:len(q)-1]) + " && status = error }"
}

type bucketGrid struct {
	from    time.Time
	step    time.Duration
	buckets []datasource.TraceBucket
}

func newBucketGrid(r datasource.TimeRange, step time.Duration) *bucketGrid {
	g := &bucketGrid{from: r.From, step: step, buckets: make([]datasource.TraceBucket, bucketCount(r, step))}
	for i := range g.buckets {
		g.buckets[i].Start = r.From.Add(time.Duration(i) * step)
	}
	return g
}

func (g *bucketGrid) at(ts unixMillis) *datasource.TraceBucket {
	i := int(time.UnixMilli(int64(ts)).Sub(g.from) / g.step)
	if i < 0 || i >= len(g.buckets) {
		return nil
	}
	return &g.buckets[i]
}

func (g *bucketGrid) addCounts(resp metricsResponse, into func(*datasource.TraceBucket) *int64) {
	for _, s := range resp.Series {
		for _, sample := range s.Samples {
			if b := g.at(sample.Timestamp); b != nil {
				*into(b) += sampleCount(sample.Value)
			}
		}
	}
}

func (g *bucketGrid) addQuantiles(resp metricsResponse) {
	for _, s := range resp.Series {
		p, ok := labelFloat(s.Labels, quantileLabel)
		if !ok {
			continue
		}
		for _, sample := range s.Samples {
			b := g.at(sample.Timestamp)
			v := sampleValue(sample.Value)
			if b == nil || b.Spans == 0 || math.IsNaN(v) || math.IsInf(v, 0) {
				continue
			}
			if slot := quantileSlot(b, p); slot != nil {
				ms := millis(v)
				*slot = &ms
			}
		}
	}
}

func quantileSlot(b *datasource.TraceBucket, p float64) **float64 {
	switch p {
	case 0.5:
		return &b.P50
	case 0.95:
		return &b.P95
	case 0.99:
		return &b.P99
	}
	return nil
}

func foldStats(r datasource.TimeRange, step time.Duration, groupBy string, resp statsResponses) datasource.TraceStats {
	grid := newBucketGrid(r, step)
	grid.addCounts(resp.spans, func(b *datasource.TraceBucket) *int64 { return &b.Spans })
	grid.addCounts(resp.errors, func(b *datasource.TraceBucket) *int64 { return &b.Errors })
	grid.addQuantiles(resp.quantiles)
	stats := datasource.TraceStats{Step: step, Buckets: grid.buckets, Histogram: histogramOf(resp.histogram), Window: &datasource.TraceQuantiles{}}
	for _, b := range grid.buckets {
		stats.Spans += b.Spans
		stats.Errors += b.Errors
	}
	for _, s := range resp.window.Series {
		if p, ok := labelFloat(s.Labels, quantileLabel); ok {
			assignQuantile(stats.Window, p, lastFinite(s.Samples))
		}
	}
	if groupBy != "" {
		stats.GroupBy = groupBy
		stats.Groups, stats.GroupsTruncated = foldGroups(resp.groupSpans, resp.groupErrors, resp.groupQuantiles)
	}
	for _, m := range []metricsResponse{resp.spans, resp.errors, resp.quantiles, resp.histogram, resp.window, resp.groupSpans, resp.groupErrors, resp.groupQuantiles} {
		stats.Partial = stats.Partial || m.Metrics.partial()
	}
	return stats
}

func foldGroups(spans, errors, quantiles metricsResponse) ([]datasource.TraceGroup, bool) {
	byValue := map[string]*datasource.TraceGroup{}
	at := func(s metricsSeries) *datasource.TraceGroup {
		value, ok := groupValue(s.Labels)
		if !ok {
			return nil
		}
		if byValue[value] == nil {
			byValue[value] = &datasource.TraceGroup{Value: value}
		}
		return byValue[value]
	}
	for _, s := range spans.Series {
		if g := at(s); g != nil {
			g.Spans += sumCounts(s.Samples)
		}
	}
	for _, s := range errors.Series {
		if g := at(s); g != nil {
			g.Errors += sumCounts(s.Samples)
		}
	}
	for _, s := range quantiles.Series {
		if p, ok := labelFloat(s.Labels, quantileLabel); ok {
			if g := at(s); g != nil {
				assignQuantile(&g.TraceQuantiles, p, lastFinite(s.Samples))
			}
		}
	}
	groups := make([]datasource.TraceGroup, 0, len(byValue))
	for _, g := range byValue {
		groups = append(groups, *g)
	}
	slices.SortFunc(groups, func(a, b datasource.TraceGroup) int {
		return cmp.Or(cmp.Compare(b.Spans, a.Spans), cmp.Compare(a.Value, b.Value))
	})
	if len(groups) > maxGroups {
		return groups[:maxGroups], true
	}
	return groups, false
}

func groupValue(labels []metricsLabel) (string, bool) {
	for _, l := range labels {
		if l.Key == quantileLabel {
			continue
		}
		switch v := l.Value.plain().(type) {
		case string:
			return v, v != ""
		case nil:
			return "", false
		default:
			return fmt.Sprint(v), true
		}
	}
	return "", false
}

func sumCounts(samples []metricsSample) int64 {
	var total int64
	for _, s := range samples {
		if v := sampleValue(s.Value); v >= 0 && v < maxCount {
			total += int64(math.Round(v))
		}
	}
	return total
}

func lastFinite(samples []metricsSample) *float64 {
	for i := len(samples) - 1; i >= 0; i-- {
		if v := sampleValue(samples[i].Value); !math.IsNaN(v) && !math.IsInf(v, 0) {
			ms := millis(v)
			return &ms
		}
	}
	return nil
}

func assignQuantile(q *datasource.TraceQuantiles, p float64, value *float64) {
	switch p {
	case 0.5:
		q.P50 = value
	case 0.95:
		q.P95 = value
	case 0.99:
		q.P99 = value
	}
}

func histogramOf(resp metricsResponse) []datasource.DurationBucket {
	counts := map[float64]int64{}
	for _, s := range resp.Series {
		le, ok := labelFloat(s.Labels, histogramLabel)
		if !ok {
			continue
		}
		for _, sample := range s.Samples {
			counts[millis(le)] += sampleCount(sample.Value)
		}
	}
	out := []datasource.DurationBucket{}
	for le, n := range counts {
		if n > 0 {
			out = append(out, datasource.DurationBucket{LeMS: le, Count: n})
		}
	}
	slices.SortFunc(out, func(a, b datasource.DurationBucket) int { return cmp.Compare(a.LeMS, b.LeMS) })
	return out
}

// Tempo answers durations in seconds; rounding the product to the nanosecond keeps float noise off the wire.
func millis(seconds float64) float64 {
	return math.Round(seconds*1e9) / 1e6
}

func bucketCount(r datasource.TimeRange, step time.Duration) int {
	span := r.To.Sub(r.From)
	n := int(span / step)
	if span%step != 0 {
		n++
	}
	return max(n, 1)
}

func sampleValue(raw json.RawMessage) float64 {
	v, err := strconv.ParseFloat(strings.Trim(string(raw), `"`), 64)
	if err != nil {
		return math.NaN()
	}
	return v
}

func sampleCount(raw json.RawMessage) int64 {
	v := math.Round(sampleValue(raw))
	if math.IsNaN(v) || v < 0 || v >= maxCount {
		return 0
	}
	return int64(v)
}

func labelFloat(labels []metricsLabel, key string) (float64, bool) {
	for _, l := range labels {
		if l.Key != key {
			continue
		}
		switch v := l.Value.plain().(type) {
		case json.Number:
			f, err := v.Float64()
			return f, err == nil
		case string:
			f, err := strconv.ParseFloat(v, 64)
			return f, err == nil
		}
	}
	return 0, false
}
