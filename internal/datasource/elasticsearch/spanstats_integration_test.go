package elasticsearch_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/elasticsearch"
)

func statsRange() datasource.TimeRange {
	from := indexedAt.Truncate(time.Minute).Add(-time.Minute)
	return datasource.TimeRange{From: from, To: from.Add(10 * time.Minute)}
}

func TestIntegrationSpanStatsCountsSpansAndErrors(t *testing.T) {
	eachFlavor(t, func(t *testing.T, flavor elasticsearch.Flavor) {
		ds := spanDS(t, flavor)
		stats, err := ds.TraceStats(context.Background(), datasource.TraceStatsRequest{Query: "process.serviceName:payment", Range: statsRange(), Step: time.Minute})
		if err != nil {
			t.Fatal(err)
		}
		if stats.Spans != checkoutTraces || stats.Errors != checkoutTraces/4 || len(stats.Buckets) != 10 || stats.Partial {
			t.Fatalf("stats = %+v", stats)
		}
		var histogram int64
		for _, b := range stats.Histogram {
			histogram += b.Count
		}
		if histogram != checkoutTraces || stats.Note == "" {
			t.Errorf("histogram = %+v, note %q", stats.Histogram, stats.Note)
		}
		var withQuantiles int
		for _, b := range stats.Buckets {
			if b.Spans > 0 && b.P95 != nil {
				withQuantiles++
			}
		}
		if withQuantiles == 0 {
			t.Errorf("no bucket has quantiles: %+v", stats.Buckets)
		}
		if _, err := ds.TraceStats(context.Background(), datasource.TraceStatsRequest{Query: `{"query":{"match_all":{}},"aggs":{}}`, Range: statsRange(), Step: time.Minute}); !errors.Is(err, datasource.ErrInvalidQuery) {
			t.Errorf("a DSL body with aggs = %v", err)
		}
	})
}

func TestIntegrationSpanDurationsAreExactWhenEverySpanIsRead(t *testing.T) {
	eachFlavor(t, func(t *testing.T, flavor elasticsearch.Flavor) {
		ds := spanDS(t, flavor)
		ctx := context.Background()
		d, err := ds.TraceDurations(ctx, datasource.TraceDurationRequest{Query: "process.serviceName:checkout or process.serviceName:payment", GroupBy: "service", Range: statsRange()})
		if err != nil {
			t.Fatal(err)
		}
		var roots []int64
		for i := range checkoutTraces {
			roots = append(roots, checkoutRootMillis(i)*int64(time.Millisecond))
		}
		slices.Sort(roots)
		ms := func(q float64) float64 { return float64(datasource.ExactQuantile(roots, q)) / 1e6 }
		if !d.Exact || d.Window.Spans != 2*checkoutTraces || d.Window.Errors != checkoutTraces/4 || len(d.Groups) != 2 {
			t.Fatalf("durations = %+v", d)
		}
		checkout := d.Groups[slices.IndexFunc(d.Groups, func(g datasource.DurationQuantiles) bool { return g.Value == "checkout" })]
		if checkout.Spans != checkoutTraces || checkout.P50 != ms(0.5) || checkout.P95 != ms(0.95) || checkout.P99 != ms(0.99) {
			t.Errorf("checkout = %+v, want p50 %v p95 %v p99 %v", checkout, ms(0.5), ms(0.95), ms(0.99))
		}
		status, err := ds.TraceDurations(ctx, datasource.TraceDurationRequest{Query: "process.serviceName:payment", GroupBy: "status", Range: statsRange()})
		if err != nil || len(status.Groups) != 2 || status.Groups[0].Value != "Unset" || status.Groups[1].Value != "Error" || status.Groups[1].Spans != checkoutTraces/4 {
			t.Errorf("by status = %+v %v", status, err)
		}
		nested, err := ds.TraceDurations(ctx, datasource.TraceDurationRequest{Query: `{"query":{"nested":{"path":"tags","query":{"bool":{"filter":[{"term":{"tags.key":"http.route"}},{"term":{"tags.value":"/checkout"}}]}}}}}`, Range: statsRange()})
		if err != nil || nested.Window.Spans != checkoutTraces {
			t.Errorf("a nested tag filter = %+v %v", nested.Window, err)
		}
		route, err := ds.TraceDurations(ctx, datasource.TraceDurationRequest{Query: "operationName:\"POST /checkout\"", GroupBy: "span.http.route", Range: statsRange()})
		if err != nil || len(route.Groups) != 1 || route.Groups[0].Value != "/checkout" {
			t.Errorf("by route = %+v %v", route, err)
		}
		sample, err := ds.TraceDurations(ctx, datasource.TraceDurationRequest{Range: statsRange(), MaxSpans: 50})
		if err != nil || sample.Exact || sample.Window.Spans != 50 {
			t.Errorf("capped = %+v %v", sample, err)
		}
		if _, err := ds.TraceDurations(ctx, datasource.TraceDurationRequest{GroupBy: "bogus", Range: statsRange()}); !errors.Is(err, datasource.ErrInvalidQuery) {
			t.Errorf("unknown group = %v", err)
		}
	})
}
