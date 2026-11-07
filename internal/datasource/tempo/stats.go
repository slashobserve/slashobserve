package tempo

import (
	"cmp"
	"context"
	"encoding/json"
	"math"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const (
	metricsPath      = "/api/metrics/query_range"
	statsNeedsFilter = `trace stats need a TraceQL spanset filter such as { resource.service.name = "checkout" }`
	quantileLabel    = "p"
	histogramLabel   = "__bucket"
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

func (d *Datasource) TraceStats(ctx context.Context, req datasource.TraceStatsRequest) (datasource.TraceStats, error) {
	q := strings.TrimSpace(req.Query)
	if !strings.HasPrefix(q, "{") || !strings.HasSuffix(q, "}") {
		return datasource.TraceStats{}, datasource.Errorf(datasource.ErrInvalidQuery, "%s", statsNeedsFilter)
	}
	lim := d.opts.Limits.Tighten(req.Limits)
	ctx, cancel := bound(ctx, lim.Timeout)
	defer cancel()
	queries := []string{
		q + " | count_over_time()",
		errorsQuery(q) + " | count_over_time()",
		q + " | quantile_over_time(duration, 0.5, 0.95, 0.99)",
		q + " | histogram_over_time(duration)",
	}
	responses := make([]metricsResponse, len(queries))
	group, groupCtx := errgroup.WithContext(ctx)
	for i, text := range queries {
		group.Go(func() error {
			params := url.Values{
				"q":     {text},
				"start": {strconv.FormatInt(req.Range.From.UnixNano(), 10)},
				"end":   {strconv.FormatInt(req.Range.To.UnixNano(), 10)},
				"step":  {strconv.FormatInt(max(int64(req.Step/time.Second), 1), 10) + "s"},
			}
			return d.getJSON(groupCtx, metricsPath, params, &responses[i])
		})
	}
	if err := group.Wait(); err != nil {
		return datasource.TraceStats{}, err
	}
	return foldStats(req.Range, req.Step, responses[0], responses[1], responses[2], responses[3]), nil
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
				*into(b) += int64(math.Round(sampleValue(sample.Value)))
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

func foldStats(r datasource.TimeRange, step time.Duration, spans, errors, quantiles, histogram metricsResponse) datasource.TraceStats {
	grid := newBucketGrid(r, step)
	grid.addCounts(spans, func(b *datasource.TraceBucket) *int64 { return &b.Spans })
	grid.addCounts(errors, func(b *datasource.TraceBucket) *int64 { return &b.Errors })
	grid.addQuantiles(quantiles)
	stats := datasource.TraceStats{Step: step, Buckets: grid.buckets, Histogram: histogramOf(histogram)}
	for _, b := range grid.buckets {
		stats.Spans += b.Spans
		stats.Errors += b.Errors
	}
	for _, resp := range []metricsResponse{spans, errors, quantiles, histogram} {
		stats.Partial = stats.Partial || resp.Metrics.partial()
	}
	return stats
}

func histogramOf(resp metricsResponse) []datasource.DurationBucket {
	counts := map[float64]int64{}
	for _, s := range resp.Series {
		le, ok := labelFloat(s.Labels, histogramLabel)
		if !ok {
			continue
		}
		for _, sample := range s.Samples {
			counts[millis(le)] += int64(math.Round(sampleValue(sample.Value)))
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
