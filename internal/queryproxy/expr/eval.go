package expr

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

var ErrExpression = errors.New("expression failed")

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrExpression, fmt.Sprintf(format, args...))
}

type series struct {
	name   string
	labels map[string]string
	timed  bool
	times  []time.Time
	values []float64
}

func (s series) points() int { return len(s.values) }

func Eval(e spec.Expression, inputs map[string][]datasource.Frame, rng datasource.TimeRange, limits datasource.Limits) ([]datasource.Frame, error) {
	limits = limits.WithDefaults(datasource.DefaultLimits)
	var (
		out []series
		err error
	)
	switch e.Type {
	case spec.ExpressionMath:
		out, err = evalMath(e.Math, inputs, limits)
	case spec.ExpressionReduce:
		out, err = withInput(e, inputs, func(in []series) ([]series, error) { return reduce(e, in) })
	case spec.ExpressionResample:
		out, err = withInput(e, inputs, func(in []series) ([]series, error) { return resample(e, in, rng, limits) })
	case spec.ExpressionThreshold:
		out, err = withInput(e, inputs, func(in []series) ([]series, error) { return threshold(e, in) })
	default:
		return nil, invalid("unknown expression type %q", e.Type)
	}
	if err != nil {
		return nil, err
	}
	if err := checkLimits(out, limits); err != nil {
		return nil, err
	}
	return toFrames(out), nil
}

func withInput(e spec.Expression, inputs map[string][]datasource.Frame, run func([]series) ([]series, error)) ([]series, error) {
	frames, ok := inputs[e.Input]
	if !ok {
		return nil, invalid("input %q is not available", e.Input)
	}
	in, err := fromFrames(e.Input, frames)
	if err != nil {
		return nil, err
	}
	return run(in)
}

func checkLimits(out []series, limits datasource.Limits) error {
	if len(out) > limits.MaxSeries {
		return datasource.Errorf(datasource.ErrLimitExceeded, "series limit exceeded: expression produced more than %d series", limits.MaxSeries)
	}
	points := 0
	for _, s := range out {
		points += s.points()
	}
	if points > limits.MaxPoints {
		return datasource.Errorf(datasource.ErrLimitExceeded, "points limit exceeded: expression produced more than %d points", limits.MaxPoints)
	}
	return nil
}

func fromFrames(ref string, frames []datasource.Frame) ([]series, error) {
	var out []series
	for _, f := range frames {
		timeAt := slices.IndexFunc(f.Fields, func(field datasource.Field) bool { return field.Type == datasource.FieldTime })
		for _, field := range f.Fields {
			if field.Type != datasource.FieldNumber {
				continue
			}
			s := series{name: f.Name, labels: f.Labels, values: make([]float64, len(field.Values))}
			for i, v := range field.Values {
				n, err := numberOf(v)
				if err != nil {
					return nil, invalid("input %q: %v", ref, err)
				}
				s.values[i] = n
			}
			if timeAt >= 0 {
				times, err := timesOf(f.Fields[timeAt], len(s.values))
				if err != nil {
					return nil, invalid("input %q: %v", ref, err)
				}
				s.timed, s.times = true, times
			} else if len(s.values) != 1 {
				return nil, invalid("input %q is a table, not a time series or a number", ref)
			}
			out = append(out, s)
		}
	}
	return out, nil
}

func numberOf(v any) (float64, error) {
	switch n := v.(type) {
	case nil:
		return math.NaN(), nil
	case float64:
		return n, nil
	}
	return 0, fmt.Errorf("a number field holds a %T value", v)
}

func timesOf(field datasource.Field, rows int) ([]time.Time, error) {
	if len(field.Values) != rows {
		return nil, fmt.Errorf("time field %q has %d values for %d numbers", field.Name, len(field.Values), rows)
	}
	times := make([]time.Time, rows)
	for i, v := range field.Values {
		t, ok := v.(time.Time)
		if !ok {
			return nil, fmt.Errorf("time field %q holds a %T value", field.Name, v)
		}
		times[i] = t
	}
	return times, nil
}

func toFrames(in []series) []datasource.Frame {
	out := make([]datasource.Frame, len(in))
	for i, s := range in {
		values := make([]any, len(s.values))
		for j, v := range s.values {
			values[j] = datasource.Number(v)
		}
		fields := make([]datasource.Field, 0, 2)
		if s.timed {
			times := make([]any, len(s.times))
			for j, t := range s.times {
				times[j] = t
			}
			fields = append(fields, datasource.Field{Name: "Time", Type: datasource.FieldTime, Values: times})
		}
		fields = append(fields, datasource.Field{Name: "Value", Type: datasource.FieldNumber, Values: values})
		out[i] = datasource.Frame{Name: s.name, Labels: s.labels, Fields: fields}
	}
	return out
}

func isNull(v float64) bool { return math.IsNaN(v) }

func isNumber(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func flag(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func mapValues(in []series, f func(float64) float64) []series {
	out := make([]series, len(in))
	for i, s := range in {
		s.values = slices.Clone(s.values)
		for j, v := range s.values {
			s.values[j] = f(v)
		}
		out[i] = s
	}
	return out
}
