package tracecompare

import (
	"cmp"
	"math"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const (
	MinGap        = 0.1
	MaxValueBytes = 256
	shareScale    = 1e4
)

type Difference struct {
	Attribute, Value             string
	SelectedShare, BaselineShare float64
	SelectedCount, BaselineCount int
}

type Result struct {
	Attributes  int
	Differences []Difference
	Truncated   bool
}

func Compare(selected, baseline []datasource.TraceAttributeSet, limit int) Result {
	selectedCounts := countPairs(selected)
	baselineCounts := countPairs(baseline)
	attributes := map[string]struct{}{}
	differences := []Difference{}
	for pair := range union(selectedCounts, baselineCounts) {
		attributes[pair.Key] = struct{}{}
		d := Difference{
			Attribute:     pair.Key,
			Value:         pair.Value,
			SelectedShare: share(selectedCounts[pair], len(selected)),
			BaselineShare: share(baselineCounts[pair], len(baseline)),
			SelectedCount: selectedCounts[pair],
			BaselineCount: baselineCounts[pair],
		}
		if d.gap() >= MinGap {
			differences = append(differences, d)
		}
	}
	slices.SortFunc(differences, func(a, b Difference) int {
		return cmp.Or(cmp.Compare(b.gap(), a.gap()), cmp.Compare(a.Attribute, b.Attribute), cmp.Compare(a.Value, b.Value))
	})
	truncated := len(differences) > limit
	if truncated {
		differences = differences[:limit]
	}
	return Result{Attributes: len(attributes), Differences: differences, Truncated: truncated}
}

func (d Difference) gap() float64 {
	return round(math.Abs(d.SelectedShare - d.BaselineShare))
}

func share(count, traces int) float64 {
	if traces == 0 {
		return 0
	}
	return round(float64(count) / float64(traces))
}

func round(v float64) float64 {
	return math.Round(v*shareScale) / shareScale
}

func countPairs(sets []datasource.TraceAttributeSet) map[datasource.TraceAttribute]int {
	counts := map[datasource.TraceAttribute]int{}
	for _, set := range sets {
		seen := map[datasource.TraceAttribute]struct{}{}
		for _, pair := range set.Attributes {
			pair.Value = cut(pair.Value)
			if _, dup := seen[pair]; dup {
				continue
			}
			seen[pair] = struct{}{}
			counts[pair]++
		}
	}
	return counts
}

func union(a, b map[datasource.TraceAttribute]int) map[datasource.TraceAttribute]struct{} {
	out := make(map[datasource.TraceAttribute]struct{}, len(a)+len(b))
	for pair := range a {
		out[pair] = struct{}{}
	}
	for pair := range b {
		out[pair] = struct{}{}
	}
	return out
}

func cut(value string) string {
	if len(value) <= MaxValueBytes {
		return value
	}
	return strings.ToValidUTF8(value[:MaxValueBytes], "")
}
