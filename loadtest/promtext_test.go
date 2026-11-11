package main

import (
	"math"
	"strings"
	"testing"
)

const exposition = `# HELP http_request_duration_seconds Request time.
# TYPE http_request_duration_seconds histogram
http_request_duration_seconds_bucket{route="/a",le="0.1"} 10
http_request_duration_seconds_bucket{route="/a",le="0.5"} 30
http_request_duration_seconds_bucket{route="/a",le="+Inf"} 40
http_request_duration_seconds_bucket{route="/b",le="0.1"} 0
http_request_duration_seconds_bucket{route="/b",le="0.5"} 0
http_request_duration_seconds_bucket{route="/b",le="+Inf"} 5
slashobserve_datasource_slots_in_use{datasource="prom"} 3
slashobserve_datasource_slots_in_use{datasource="ch"} 1
process_resident_memory_bytes 1.5e+08
odd_label{path="a \"quoted\" \\ value",x="1"} 7
not a sample line
`

func TestParseExposition(t *testing.T) {
	samples := parseExposition(strings.NewReader(exposition))
	if got := sumOf(samples, "slashobserve_datasource_slots_in_use", map[string]string{"datasource": "prom"}); got != 3 {
		t.Fatalf("prom slots = %v, want 3", got)
	}
	if got := sumOf(samples, "slashobserve_datasource_slots_in_use", nil); got != 4 {
		t.Fatalf("all slots = %v, want 4", got)
	}
	if got := sumOf(samples, "process_resident_memory_bytes", nil); got != 1.5e8 {
		t.Fatalf("rss = %v, want 1.5e8", got)
	}
	if got := sumOf(samples, "odd_label", map[string]string{"path": `a "quoted" \ value`}); got != 7 {
		t.Fatalf("escaped label sum = %v, want 7", got)
	}
}

func TestBucketsAndQuantile(t *testing.T) {
	samples := parseExposition(strings.NewReader(exposition))
	a := buckets(samples, "http_request_duration_seconds", map[string]string{"route": "/a"})
	if a[math.Inf(1)] != 40 || a[0.1] != 10 {
		t.Fatalf("buckets = %v", a)
	}
	got := histogramQuantile(0.5, a)
	if math.Abs(got-0.3) > 1e-9 {
		t.Fatalf("p50 = %v, want 0.3", got)
	}
	if got := histogramQuantile(0.95, a); got != 0.5 {
		t.Fatalf("p95 in the +Inf bucket = %v, want the highest finite bound 0.5", got)
	}
}

func TestHistogramQuantileFromDeltas(t *testing.T) {
	before := map[float64]float64{0.1: 10, 0.5: 20, math.Inf(1): 20}
	after := map[float64]float64{0.1: 10, 0.5: 40, math.Inf(1): 40}
	delta := bucketDelta(before, after)
	got := histogramQuantile(0.5, delta)
	if math.Abs(got-0.3) > 1e-9 {
		t.Fatalf("delta p50 = %v, want 0.3", got)
	}
}

func TestHistogramQuantileNoObservations(t *testing.T) {
	same := map[float64]float64{0.1: 4, math.Inf(1): 4}
	if got := histogramQuantile(0.95, bucketDelta(same, same)); !math.IsNaN(got) {
		t.Fatalf("zero delta = %v, want NaN", got)
	}
	if got := histogramQuantile(0.95, nil); !math.IsNaN(got) {
		t.Fatalf("empty = %v, want NaN", got)
	}
}

func TestAddBuckets(t *testing.T) {
	sum := addBuckets(map[float64]float64{1: 2, math.Inf(1): 3}, map[float64]float64{1: 5, math.Inf(1): 5})
	if sum[1] != 7 || sum[math.Inf(1)] != 8 {
		t.Fatalf("sum = %v", sum)
	}
}

func TestGaugeMax(t *testing.T) {
	g := newGaugeMax()
	g.observe("a", 2)
	g.observe("a", 5)
	g.observe("a", 3)
	if got := g.get("a"); got != 5 {
		t.Fatalf("max = %v, want 5", got)
	}
	if got := g.get("missing"); got != 0 {
		t.Fatalf("missing = %v, want 0", got)
	}
}
