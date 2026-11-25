package tempo_test

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
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

func TestTraceStatsRunsTheMetricsQueries(t *testing.T) {
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
		filter + " | quantile_over_time(duration, 0.5, 0.95, 0.99)",
	}
	if got := fake.queries(); !slices.Equal(got, want) {
		t.Fatalf("queries = %q, want %q", got, want)
	}
	steps := map[string]int{}
	for _, req := range fake.requests {
		if !strings.HasPrefix(req.path, "/api/metrics/query") || req.params.Get("start") != "1793962800000000000" || req.params.Get("end") != "1793966400000000000" {
			t.Fatalf("request = %s %v", req.path, req.params)
		}
		steps[req.params.Get("step")]++
	}
	if want := map[string]int{"60s": 4, "": 1}; !maps.Equal(steps, want) {
		t.Fatalf("steps = %v, want %v", steps, want)
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
			ds, fake := serve(t, bothPaths(reply{0, `{"series":[]}`}))
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
	ds, _ := serve(t, bothPaths(reply{0, `{"series":[],"metrics":{}}`}))
	stats, err := ds.TraceStats(context.Background(), datasource.TraceStatsRequest{Query: "{}", Range: datasource.TimeRange{From: statsFrom, To: statsFrom.Add(150 * time.Second)}, Step: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Partial || stats.Spans != 0 || len(stats.Buckets) != 3 || stats.Histogram == nil || len(stats.Histogram) != 0 {
		t.Fatalf("stats = %+v, want 3 zero buckets and an empty histogram", stats)
	}
}

func TestTraceStatsKeepsUpstreamErrorKinds(t *testing.T) {
	ds, _ := serve(t, bothPaths(reply{400, "invalid TraceQL query: parse error at line 1"}))
	_, err := ds.TraceStats(context.Background(), datasource.TraceStatsRequest{Query: "{ nope", Range: statsRange, Step: time.Minute})
	if !errors.Is(err, datasource.ErrInvalidQuery) {
		t.Fatalf("err = %v, want invalid query", err)
	}
	_, err = ds.TraceStats(context.Background(), datasource.TraceStatsRequest{Query: "{ nope }", Range: statsRange, Step: time.Minute})
	if !errors.Is(err, datasource.ErrInvalidQuery) || !strings.Contains(err.Error(), "parse error") {
		t.Fatalf("err = %v, want Tempo's parse error", err)
	}
}

func TestTraceStatsCountsIgnoreSamplesThatAreNotCounts(t *testing.T) {
	filter := `{ resource.service.name = "checkout" }`
	minute := statsFrom.Add(time.Minute)
	ds, fake := serve(t, nil)
	fake.byQuery = map[string]reply{
		`{ resource.service.name = "checkout" && status = error } | count_over_time()`: {0, metricsBody()},
		filter + " | quantile_over_time(duration, 0.5, 0.95, 0.99)":                    {0, metricsBody()},
		filter + " | count_over_time()": {0, metricsBody(
			`{"labels":[],"samples":[` + sample(statsFrom, "7") + `,` + sample(minute, `"NaN"`) + `,` + sample(minute.Add(time.Minute), "1e30") + `,` + sample(minute.Add(2*time.Minute), `"-Inf"`) + `]}`)},
		filter + " | histogram_over_time(duration)": {0, metricsBody(
			`{"labels":[{"key":"__bucket","value":{"doubleValue":0.000131072}}],"samples":[` + sample(statsFrom, "3") + `,` + sample(minute, `"NaN"`) + `]}`)},
	}
	stats, err := ds.TraceStats(context.Background(), datasource.TraceStatsRequest{Query: filter, Range: statsRange, Step: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Spans != 7 {
		t.Fatalf("spans = %d, want 7", stats.Spans)
	}
	if want := []datasource.DurationBucket{{LeMS: 0.131072, Count: 3}}; !slices.Equal(stats.Histogram, want) {
		t.Fatalf("histogram = %+v, want %+v", stats.Histogram, want)
	}
}

func bothPaths(r reply) map[string]reply {
	return map[string]reply{"/api/metrics/query_range": r, "/api/metrics/query": r}
}

const quantiles = " | quantile_over_time(duration, 0.5, 0.95, 0.99)"

func quantileSeries(p, value string) string {
	return `{"labels":[{"key":"p","value":{"doubleValue":` + p + `}}],"value":` + value + `}`
}

func TestTraceStatsReturnsWholeWindowQuantiles(t *testing.T) {
	filter := `{ resource.service.name = "checkout" }`
	ds, fake := serve(t, nil)
	fake.byQuery = statsReplies(filter, `{ resource.service.name = "checkout" && status = error }`)
	fake.byStep = map[string]reply{
		filter + quantiles + "@": {0, metricsBody(quantileSeries("0.5", "0.05"), quantileSeries("0.95", "2.275"), quantileSeries("0.99", "13.6357"))},
	}
	stats, err := ds.TraceStats(context.Background(), datasource.TraceStatsRequest{Query: filter, Range: statsRange, Step: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	w := stats.Window
	if w == nil || !is(w.P50, 50) || !is(w.P95, 2275) || !is(w.P99, 13635.7) {
		t.Fatalf("window = %+v, want 50, 2275 and 13635.7 ms", w)
	}
	if stats.GroupBy != "" || len(stats.Groups) != 0 {
		t.Fatalf("groups = %q %+v, want none without group_by", stats.GroupBy, stats.Groups)
	}
	if got := fake.queries(); len(got) != 5 {
		t.Fatalf("%d queries, want 5", len(got))
	}
}

func TestTraceStatsWindowIsEmptyWhenTempoHasNoSeries(t *testing.T) {
	ds, _ := serve(t, bothPaths(reply{0, `{"series":[]}`}))
	stats, err := ds.TraceStats(context.Background(), datasource.TraceStatsRequest{Query: "{}", Range: statsRange, Step: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Window == nil || stats.Window.P50 != nil || stats.Window.P95 != nil || stats.Window.P99 != nil {
		t.Fatalf("window = %+v, want a block with null quantiles", stats.Window)
	}
}

func groupLabels(attribute, value string) string {
	return `{"key":"` + attribute + `","value":{"stringValue":"` + value + `"}}`
}

func TestTraceStatsGroupsByAnAttribute(t *testing.T) {
	filter, by := `{ span.http.route = "/pay" }`, " by (resource.service.name)"
	const attr = "resource.service.name"
	ds, fake := serve(t, nil)
	fake.byQuery = statsReplies(filter, `{ span.http.route = "/pay" && status = error }`)
	fake.byStep = map[string]reply{
		filter + " | count_over_time()" + by + "@": {0, metricsBody(
			`{"labels":[`+groupLabels(attr, "cart")+`],"value":`+"3"+`}`,
			`{"labels":[`+groupLabels(attr, "checkout")+`],"value":`+"7"+`}`,
			`{"labels":[`+groupLabels(attr, "")+`],"value":`+"99"+`}`)},
		`{ span.http.route = "/pay" && status = error } | count_over_time()` + by + "@": {0, metricsBody(
			`{"labels":[` + groupLabels(attr, "checkout") + `],"samples":[` + sample(statsFrom, "2") + `]}`)},
		filter + quantiles + by + "@": {0, metricsBody(
			`{"labels":[`+groupLabels(attr, "checkout")+`,{"key":"p","value":{"doubleValue":0.99}}],"value":`+"13.6357"+`}`,
			`{"labels":[`+groupLabels(attr, "cart")+`,{"key":"p","value":{"doubleValue":0.99}}],"value":`+`"NaN"`+`}`,
			`{"labels":[`+groupLabels(attr, "cart")+`,{"key":"p","value":{"doubleValue":0.5}}],"value":`+"0.02"+`}`)},
		filter + quantiles + "@": {0, metricsBody()},
	}
	stats, err := ds.TraceStats(context.Background(), datasource.TraceStatsRequest{Query: filter, GroupBy: " " + attr + " ", Range: statsRange, Step: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if stats.GroupBy != attr || stats.GroupsTruncated || len(stats.Groups) != 2 {
		t.Fatalf("stats = %+v, want two groups", stats)
	}
	checkout, cart := stats.Groups[0], stats.Groups[1]
	if checkout.Value != "checkout" || checkout.Spans != 7 || checkout.Errors != 2 || !is(checkout.P99, 13635.7) || checkout.P50 != nil {
		t.Fatalf("checkout = %+v", checkout)
	}
	if cart.Value != "cart" || cart.Spans != 3 || cart.Errors != 0 || !is(cart.P50, 20) || cart.P99 != nil {
		t.Fatalf("cart = %+v, want a null p99 for the NaN sample", cart)
	}
	if got := fake.queries(); len(got) != 8 {
		t.Fatalf("%d queries, want 8 with group_by", len(got))
	}
}

func TestTraceStatsCapsTheGroups(t *testing.T) {
	filter := "{}"
	var series []string
	for i := range 25 {
		series = append(series, `{"labels":[`+groupLabels("name", fmt.Sprintf("op-%02d", i))+`],"value":`+fmt.Sprint(i+1)+`}`)
	}
	ds, fake := serve(t, bothPaths(reply{0, `{"series":[]}`}))
	fake.byStep = map[string]reply{filter + " | count_over_time() by (name)@": {0, metricsBody(series...)}}
	stats, err := ds.TraceStats(context.Background(), datasource.TraceStatsRequest{Query: filter, GroupBy: "name", Range: statsRange, Step: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if !stats.GroupsTruncated || len(stats.Groups) != 20 || stats.Groups[0].Value != "op-24" || stats.Groups[19].Value != "op-05" {
		t.Fatalf("groups = %d truncated %v first %q last %q", len(stats.Groups), stats.GroupsTruncated, stats.Groups[0].Value, stats.Groups[len(stats.Groups)-1].Value)
	}
}

func TestTraceStatsRefusesAnUnsafeGroupBy(t *testing.T) {
	ds, fake := serve(t, nil)
	bad := []string{
		"resource.service.name) | rate() by (x",
		`resource.service.name"`,
		"resource.service.name, span.http.route",
		"service.name",
		"resource.",
		".",
		"span.a b",
		"{ status = error }",
		"span.x;",
		"span.x\n| count()",
		"duration",
		"span." + strings.Repeat("a", 200),
	}
	for _, attribute := range bad {
		_, err := ds.TraceStats(context.Background(), datasource.TraceStatsRequest{Query: "{}", GroupBy: attribute, Range: statsRange, Step: time.Minute})
		if !errors.Is(err, datasource.ErrInvalidQuery) || !strings.Contains(err.Error(), "group_by must be one TraceQL attribute") {
			t.Fatalf("%q: err = %v, want an invalid group_by", attribute, err)
		}
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.requests) != 0 {
		t.Fatalf("%d requests reached Tempo for refused group_by", len(fake.requests))
	}
}

func TestTraceStatsAcceptsTraceQLAttributeNames(t *testing.T) {
	ds, fake := serve(t, bothPaths(reply{0, `{"series":[]}`}))
	for _, attribute := range []string{"resource.service.name", "span.http.route", ".http.method", "span.http.status_code", "resource.k8s.pod-name", "name", "status", "kind", "statusMessage"} {
		if _, err := ds.TraceStats(context.Background(), datasource.TraceStatsRequest{Query: "{}", GroupBy: attribute, Range: statsRange, Step: time.Minute}); err != nil {
			t.Fatalf("%q: %v", attribute, err)
		}
	}
	if got := fake.queries(); !slices.Contains(got, "{} | count_over_time() by (.http.method)") {
		t.Fatalf("queries = %q", got)
	}
}

func TestTraceStatsAsksTempoForTheExactRangeOnTheInstantEndpoint(t *testing.T) {
	filter := "{}"
	from := statsFrom.Add(30 * time.Second)
	rng := datasource.TimeRange{From: from, To: from.Add(37 * time.Minute)}
	ds, fake := serve(t, bothPaths(reply{0, `{"series":[]}`}))
	fake.byStep = map[string]reply{filter + quantiles + "@": {0, `{"series":[` + quantileSeries("0.99", "1.5") + `],"metrics":{"completedJobs":1,"totalJobs":3}}`}}
	stats, err := ds.TraceStats(context.Background(), datasource.TraceStatsRequest{Query: filter, Range: rng, Step: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Window == nil || !is(stats.Window.P99, 1500) || !stats.Partial {
		t.Fatalf("stats = %+v, want the p99 read from the instant response and a partial flag", stats)
	}
	instants := 0
	for _, req := range fake.requests {
		if req.path != "/api/metrics/query" {
			continue
		}
		instants++
		if req.params.Get("start") != strconv.FormatInt(rng.From.UnixNano(), 10) || req.params.Get("end") != strconv.FormatInt(rng.To.UnixNano(), 10) || req.params.Has("step") {
			t.Fatalf("instant request = %v, want the exact range and no step", req.params)
		}
	}
	if instants != 1 {
		t.Fatalf("%d instant requests, want 1", instants)
	}
}

func TestTraceStatsRefusesARangeInTheFuture(t *testing.T) {
	ds, fake := serve(t, nil)
	future := time.Now().Add(time.Hour)
	_, err := ds.TraceStats(context.Background(), datasource.TraceStatsRequest{Query: "{}", Range: datasource.TimeRange{From: future, To: future.Add(time.Hour)}, Step: time.Minute})
	if !errors.Is(err, datasource.ErrInvalidQuery) {
		t.Fatalf("err = %v, want an invalid query", err)
	}
	if len(fake.requests) != 0 {
		t.Fatalf("%d requests reached Tempo", len(fake.requests))
	}
}
