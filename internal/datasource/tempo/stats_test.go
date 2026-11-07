package tempo_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

var (
	statsFrom  = time.Date(2026, 11, 6, 11, 0, 0, 0, time.UTC)
	statsRange = datasource.TimeRange{From: statsFrom, To: statsFrom.Add(time.Hour)}
)

func is(p *float64, want float64) bool {
	return p != nil && *p == want
}

func sample(at time.Time, value string) string {
	return fmt.Sprintf(`{"timestampMs":"%d","value":%s}`, at.UnixMilli(), value)
}

func metricsBody(series ...string) string {
	return `{"series":[` + strings.Join(series, ",") + `],"metrics":{"completedJobs":4,"totalJobs":4}}`
}

func statsReplies(filter, errorsFilter string) map[string]reply {
	minute := statsFrom.Add(time.Minute)
	return map[string]reply{
		filter + " | count_over_time()": {0, metricsBody(
			`{"labels":[],"samples":[` + sample(statsFrom, "5") + `,` + sample(minute, "3") + `]}`)},
		errorsFilter + " | count_over_time()": {0, metricsBody(
			`{"labels":[],"samples":[` + sample(minute, "1") + `]}`)},
		filter + " | quantile_over_time(duration, 0.5, 0.95, 0.99)": {0, metricsBody(
			`{"labels":[{"key":"p","value":{"doubleValue":0.5}}],"samples":[`+sample(statsFrom, "0.0331")+`]}`,
			`{"labels":[{"key":"p","value":{"doubleValue":0.95}}],"samples":[`+sample(statsFrom, "0.1204")+`,`+sample(statsFrom.Add(2*time.Minute), "9")+`]}`,
			`{"labels":[{"key":"p","value":{"doubleValue":0.99}}],"samples":[`+sample(statsFrom, "0.41")+`,`+sample(minute, `"NaN"`)+`]}`)},
		filter + " | histogram_over_time(duration)": {0, strings.Replace(metricsBody(
			`{"labels":[{"key":"__bucket","value":{"doubleValue":0.000262144}}],"samples":[`+sample(statsFrom, "2")+`]}`,
			`{"labels":[{"key":"__bucket","value":{"doubleValue":0.000131072}}],"samples":[`+sample(statsFrom, "5")+`,`+sample(minute, "6")+`]}`),
			`"completedJobs":4,"totalJobs":4`, `"completedJobs":1,"totalJobs":7`, 1)},
	}
}

func TestTraceStatsRunsFourMetricsQueries(t *testing.T) {
	filter, errorsFilter := `{ resource.service.name = "checkout" }`, `{ resource.service.name = "checkout" && status = error }`
	ds, fake := serve(t, nil)
	fake.byQuery = statsReplies(filter, errorsFilter)
	stats, err := ds.TraceStats(context.Background(), datasource.TraceStatsRequest{Query: " " + filter + "\n", Range: statsRange, Step: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		errorsFilter + " | count_over_time()",
		filter + " | count_over_time()",
		filter + " | histogram_over_time(duration)",
		filter + " | quantile_over_time(duration, 0.5, 0.95, 0.99)",
	}
	if got := fake.queries(); !slices.Equal(got, want) {
		t.Fatalf("queries = %q, want %q", got, want)
	}
	req := fake.last(t)
	if req.path != "/api/metrics/query_range" || req.params.Get("start") != "1793962800000000000" || req.params.Get("end") != "1793966400000000000" || req.params.Get("step") != "60s" {
		t.Fatalf("request = %s %v", req.path, req.params)
	}
	if stats.Step != time.Minute || !stats.Partial || stats.Spans != 8 || stats.Errors != 1 || len(stats.Buckets) != 60 {
		t.Fatalf("stats = %+v", stats)
	}
	first := stats.Buckets[0]
	if !first.Start.Equal(statsFrom) || first.Spans != 5 || first.Errors != 0 || !is(first.P50, 33.1) || !is(first.P95, 120.4) || !is(first.P99, 410) {
		t.Fatalf("bucket 0 = %+v", first)
	}
	second := stats.Buckets[1]
	if second.Spans != 3 || second.Errors != 1 || second.P50 != nil || second.P95 != nil || second.P99 != nil {
		t.Fatalf("bucket 1 = %+v, want 3 spans, 1 error and no quantiles", second)
	}
	if third := stats.Buckets[2]; third.Spans != 0 || third.P95 != nil {
		t.Fatalf("bucket 2 = %+v, want no spans and a null p95", third)
	}
	if last := stats.Buckets[59]; !last.Start.Equal(statsFrom.Add(59*time.Minute)) || last.Spans != 0 {
		t.Fatalf("bucket 59 = %+v", last)
	}
	histogram := []datasource.DurationBucket{{LeMS: 0.131072, Count: 11}, {LeMS: 0.262144, Count: 2}}
	if !slices.Equal(stats.Histogram, histogram) {
		t.Fatalf("histogram = %+v, want %+v", stats.Histogram, histogram)
	}
}

func TestTraceStatsRefusesANonFilter(t *testing.T) {
	ds, fake := serve(t, nil)
	for _, q := range []string{"", "count() > 2", `{ name = "x" } | count()`} {
		_, err := ds.TraceStats(context.Background(), datasource.TraceStatsRequest{Query: q, Range: statsRange, Step: time.Minute})
		if !errors.Is(err, datasource.ErrInvalidQuery) || !strings.Contains(err.Error(), "TraceQL spanset filter") {
			t.Fatalf("%q: err = %v, want an invalid query naming the filter", q, err)
		}
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.requests) != 0 {
		t.Fatalf("%d requests reached Tempo for refused queries", len(fake.requests))
	}
}

func TestTraceStatsErrorsQueryInsertsStatusError(t *testing.T) {
	cases := map[string]string{
		"{}":          "{ status = error } | count_over_time()",
		"{ }":         "{ status = error } | count_over_time()",
		`{ a = "b" }`: `{ a = "b" && status = error } | count_over_time()`,
	}
	for q, want := range cases {
		t.Run(q, func(t *testing.T) {
			ds, fake := serve(t, map[string]reply{"/api/metrics/query_range": {0, `{"series":[]}`}})
			if _, err := ds.TraceStats(context.Background(), datasource.TraceStatsRequest{Query: q, Range: statsRange, Step: time.Minute}); err != nil {
				t.Fatal(err)
			}
			if got := fake.queries(); !slices.Contains(got, want) {
				t.Fatalf("queries = %q, want one to be %q", got, want)
			}
		})
	}
}

func TestTraceStatsWithNoSeriesIsEmptyButFilled(t *testing.T) {
	ds, _ := serve(t, map[string]reply{"/api/metrics/query_range": {0, `{"series":[],"metrics":{}}`}})
	stats, err := ds.TraceStats(context.Background(), datasource.TraceStatsRequest{Query: "{}", Range: datasource.TimeRange{From: statsFrom, To: statsFrom.Add(150 * time.Second)}, Step: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Partial || stats.Spans != 0 || len(stats.Buckets) != 3 || stats.Histogram == nil || len(stats.Histogram) != 0 {
		t.Fatalf("stats = %+v, want 3 zero buckets and an empty histogram", stats)
	}
}

func TestTraceStatsKeepsUpstreamErrorKinds(t *testing.T) {
	ds, _ := serve(t, map[string]reply{"/api/metrics/query_range": {400, "invalid TraceQL query: parse error at line 1"}})
	_, err := ds.TraceStats(context.Background(), datasource.TraceStatsRequest{Query: "{ nope", Range: statsRange, Step: time.Minute})
	if !errors.Is(err, datasource.ErrInvalidQuery) {
		t.Fatalf("err = %v, want invalid query", err)
	}
	_, err = ds.TraceStats(context.Background(), datasource.TraceStatsRequest{Query: "{ nope }", Range: statsRange, Step: time.Minute})
	if !errors.Is(err, datasource.ErrInvalidQuery) || !strings.Contains(err.Error(), "parse error") {
		t.Fatalf("err = %v, want Tempo's parse error", err)
	}
}
