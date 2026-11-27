package datasource

import (
	"slices"
	"testing"
)

func TestExactQuantileReadsTheClickHousePosition(t *testing.T) {
	hundred := make([]int64, 100)
	for i := range hundred {
		hundred[i] = int64(i + 1)
	}
	for _, tc := range []struct {
		name   string
		values []int64
		q      float64
		want   int64
	}{
		{"empty", nil, 0.5, 0},
		{"one value", []int64{7}, 0.99, 7},
		{"median of an even count takes the upper middle", []int64{1, 2, 3, 4}, 0.5, 3},
		{"p99 of 100", hundred, 0.99, 100},
		{"p95 of 100", hundred, 0.95, 96},
		{"p50 of 100", hundred, 0.5, 51},
		{"q of 1 is the maximum", hundred, 1, 100},
		{"q of 0 is the minimum", hundred, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExactQuantile(tc.values, tc.q); got != tc.want {
				t.Fatalf("ExactQuantile(%v) = %d, want %d", tc.q, got, tc.want)
			}
		})
	}
}

func TestFoldDurationsGroupsCountsAndRanks(t *testing.T) {
	const ms = 1_000_000
	spans := []SpanDuration{
		{Group: "cart", Grouped: true, Nanos: 30 * ms},
		{Group: "checkout", Grouped: true, Nanos: 10 * ms, Error: true},
		{Group: "checkout", Grouped: true, Nanos: 20 * ms},
		{Group: "checkout", Grouped: true, Nanos: 40 * ms},
		{Nanos: 50 * ms},
		{Group: "ad", Grouped: true, Nanos: 5 * ms},
	}
	window, groups, truncated := FoldDurations(spans, true, 2)
	if window.Spans != 6 || window.Errors != 1 || window.P50 != 30 || window.P99 != 50 {
		t.Fatalf("window = %+v", window)
	}
	if !truncated {
		t.Fatal("three groups over a cap of two are not truncated")
	}
	names := []string{}
	for _, g := range groups {
		names = append(names, g.Value)
	}
	if !slices.Equal(names, []string{"checkout", "ad"}) {
		t.Fatalf("groups = %v, want checkout then the alphabetical tie", names)
	}
	if c := groups[0]; c.Spans != 3 || c.Errors != 1 || c.P50 != 20 || c.P95 != 40 || c.P99 != 40 {
		t.Fatalf("checkout = %+v", c)
	}
	if _, groups, _ := FoldDurations(spans, false, 2); groups != nil {
		t.Fatalf("ungrouped fold returned groups %+v", groups)
	}
}
