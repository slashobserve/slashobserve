package expr

import (
	"math"
	"slices"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func threshold(e spec.Expression, in []series) ([]series, error) {
	test, err := thresholdTest(e)
	if err != nil {
		return nil, err
	}
	return mapValues(in, func(v float64) float64 {
		if isNull(v) {
			return math.NaN()
		}
		return flag(test(v))
	}), nil
}

func thresholdTest(e spec.Expression) (func(float64) bool, error) {
	params := slices.Clone(e.Params)
	slices.Sort(params)
	want := 2
	if e.Evaluator == "gt" || e.Evaluator == "lt" {
		want = 1
	}
	if len(params) != want {
		return nil, invalid("threshold: %s takes %d parameters", e.Evaluator, want)
	}
	lo, hi := params[0], params[len(params)-1]
	switch e.Evaluator {
	case "gt":
		return func(v float64) bool { return v > lo }, nil
	case "lt":
		return func(v float64) bool { return v < lo }, nil
	case "withinRange":
		return func(v float64) bool { return v > lo && v < hi }, nil
	case "outsideRange":
		return func(v float64) bool { return v < lo || v > hi }, nil
	case "withinRangeIncluded":
		return func(v float64) bool { return v >= lo && v <= hi }, nil
	case "outsideRangeIncluded":
		return func(v float64) bool { return v <= lo || v >= hi }, nil
	}
	return nil, invalid("threshold: unknown evaluator %q", e.Evaluator)
}
