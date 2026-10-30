package expr

import (
	"math"
	"slices"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func reduce(e spec.Expression, in []series) ([]series, error) {
	out := make([]series, len(in))
	for i, s := range in {
		values, strictFailed, err := applyMode(e, s.values)
		if err != nil {
			return nil, err
		}
		result := math.NaN()
		if !strictFailed {
			result = reduceValues(e.Reducer, values)
		}
		out[i] = series{name: s.name, labels: s.labels, values: []float64{result}}
	}
	return out, nil
}

func applyMode(e spec.Expression, values []float64) ([]float64, bool, error) {
	switch e.Mode {
	case "", "strict":
		if slices.ContainsFunc(values, func(v float64) bool { return !isNumber(v) }) {
			return nil, true, nil
		}
		return values, false, nil
	case "dropNonNumbers":
		kept := make([]float64, 0, len(values))
		for _, v := range values {
			if isNumber(v) {
				kept = append(kept, v)
			}
		}
		return kept, false, nil
	case spec.ModeReplaceNonNumbers:
		if e.ReplaceWith == nil {
			return nil, false, invalid("reduce: mode replaceNonNumbers needs replaceWith")
		}
		replaced := make([]float64, len(values))
		for i, v := range values {
			replaced[i] = v
			if !isNumber(v) {
				replaced[i] = *e.ReplaceWith
			}
		}
		return replaced, false, nil
	}
	return nil, false, invalid("reduce: unknown mode %q", e.Mode)
}

func reduceValues(reducer string, values []float64) float64 {
	switch reducer {
	case "count":
		return float64(len(values))
	case "sum":
		return sum(values)
	}
	if len(values) == 0 {
		return math.NaN()
	}
	switch reducer {
	case "mean":
		return sum(values) / float64(len(values))
	case "min":
		return slices.Min(values)
	case "max":
		return slices.Max(values)
	case "last":
		return values[len(values)-1]
	case "median":
		sorted := slices.Clone(values)
		slices.Sort(sorted)
		mid := len(sorted) / 2
		if len(sorted)%2 == 1 {
			return sorted[mid]
		}
		return (sorted[mid-1] + sorted[mid]) / 2
	}
	return math.NaN()
}

func sum(values []float64) float64 {
	total := 0.0
	for _, v := range values {
		total += v
	}
	return total
}
