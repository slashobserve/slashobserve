package datasource

import (
	"cmp"
	"context"
	"slices"
)

const (
	TraceDurationSpanBudget = 20000
	MaxDurationGroups       = 20
)

type TraceDurationRequest struct {
	Query    string
	GroupBy  string
	Range    TimeRange
	Limits   Limits
	User     QueryUser
	MaxSpans int
}

type DurationQuantiles struct {
	Value         string
	Spans, Errors int64
	P50, P95, P99 float64
}

type TraceDurations struct {
	Window          DurationQuantiles
	Groups          []DurationQuantiles
	GroupsTruncated bool
	Exact           bool
}

type TraceDurationSource interface {
	TraceDurations(ctx context.Context, req TraceDurationRequest) (TraceDurations, error)
}

type SpanDuration struct {
	Group   string
	Grouped bool
	Nanos   int64
	Error   bool
}

// ExactQuantile takes the value at position floor(q*n) of the sorted values, the position ClickHouse quantileExact reads, so both stores agree.
func ExactQuantile(sorted []int64, q float64) int64 {
	if len(sorted) == 0 {
		return 0
	}
	i := len(sorted) - 1
	if q < 1 {
		i = min(int(q*float64(len(sorted))), len(sorted)-1)
	}
	return sorted[max(i, 0)]
}

func FoldDurations(spans []SpanDuration, grouped bool, maxGroups int) (window DurationQuantiles, groups []DurationQuantiles, truncated bool) {
	all := make([]int64, 0, len(spans))
	byGroup := map[string][]int64{}
	errs := map[string]int64{}
	for _, s := range spans {
		all = append(all, s.Nanos)
		if s.Error {
			window.Errors++
		}
		if grouped && s.Grouped {
			byGroup[s.Group] = append(byGroup[s.Group], s.Nanos)
			if s.Error {
				errs[s.Group]++
			}
		}
	}
	window = quantilesOf("", all, window.Errors)
	for value, durations := range byGroup {
		groups = append(groups, quantilesOf(value, durations, errs[value]))
	}
	slices.SortFunc(groups, func(a, b DurationQuantiles) int {
		return cmp.Or(cmp.Compare(b.Spans, a.Spans), cmp.Compare(a.Value, b.Value))
	})
	if maxGroups > 0 && len(groups) > maxGroups {
		return window, groups[:maxGroups], true
	}
	return window, groups, false
}

func quantilesOf(value string, durations []int64, errors int64) DurationQuantiles {
	slices.Sort(durations)
	q := DurationQuantiles{Value: value, Spans: int64(len(durations)), Errors: errors}
	if len(durations) > 0 {
		q.P50 = nanosToMillis(ExactQuantile(durations, 0.5))
		q.P95 = nanosToMillis(ExactQuantile(durations, 0.95))
		q.P99 = nanosToMillis(ExactQuantile(durations, 0.99))
	}
	return q
}

func nanosToMillis(n int64) float64 { return float64(n) / 1e6 }
