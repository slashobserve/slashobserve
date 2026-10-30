package expr

import (
	"math"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

var functions = map[string]func(float64) float64{
	"abs":       math.Abs,
	"ceil":      math.Ceil,
	"floor":     math.Floor,
	"round":     math.Round,
	"log":       math.Log,
	"exp":       math.Exp,
	"sqrt":      math.Sqrt,
	"is_null":   func(v float64) float64 { return flag(isNull(v)) },
	"is_nan":    func(v float64) float64 { return flag(isNull(v)) },
	"is_number": func(v float64) float64 { return flag(isNumber(v)) },
	"is_inf":    func(v float64) float64 { return flag(math.IsInf(v, 0)) },
}

func comparison(test func(a, b float64) bool) func(a, b float64) float64 {
	return func(a, b float64) float64 {
		if isNull(a) || isNull(b) {
			return math.NaN()
		}
		return flag(test(a, b))
	}
}

var binaryOps = map[string]func(a, b float64) float64{
	"+":  func(a, b float64) float64 { return a + b },
	"-":  func(a, b float64) float64 { return a - b },
	"*":  func(a, b float64) float64 { return a * b },
	"/":  func(a, b float64) float64 { return a / b },
	"%":  math.Mod,
	"**": math.Pow,
	"==": comparison(func(a, b float64) bool { return a == b }),
	"!=": comparison(func(a, b float64) bool { return a != b }),
	"<":  comparison(func(a, b float64) bool { return a < b }),
	"<=": comparison(func(a, b float64) bool { return a <= b }),
	">":  comparison(func(a, b float64) bool { return a > b }),
	">=": comparison(func(a, b float64) bool { return a >= b }),
	"&&": comparison(func(a, b float64) bool { return a != 0 && b != 0 }),
	"||": comparison(func(a, b float64) bool { return a != 0 || b != 0 }),
}

type mathEval struct {
	refs   map[string][]series
	limits datasource.Limits
	points int
}

func evalMath(text string, inputs map[string][]datasource.Frame, limits datasource.Limits) ([]series, error) {
	tree, err := parseMath(text)
	if err != nil {
		return nil, invalid("math: %v", err)
	}
	m := &mathEval{refs: map[string][]series{}, limits: limits}
	for _, name := range spec.MathRefs(text) {
		frames, ok := inputs[name]
		if !ok {
			return nil, invalid("input %q is not available", name)
		}
		s, err := fromFrames(name, frames)
		if err != nil {
			return nil, err
		}
		m.refs[name] = s
	}
	return m.eval(tree)
}

func (m *mathEval) eval(n node) ([]series, error) {
	switch n := n.(type) {
	case numberNode:
		return []series{{values: []float64{n.value}}}, nil
	case refNode:
		return m.refs[n.name], nil
	case unaryNode:
		x, err := m.eval(n.x)
		if err != nil {
			return nil, err
		}
		return mapValues(x, unaryOp(n.op)), nil
	case callNode:
		x, err := m.eval(n.arg)
		if err != nil {
			return nil, err
		}
		return mapValues(x, functions[n.name]), nil
	case binaryNode:
		l, err := m.eval(n.l)
		if err != nil {
			return nil, err
		}
		r, err := m.eval(n.r)
		if err != nil {
			return nil, err
		}
		return m.join(l, r, binaryOps[n.op])
	}
	return nil, invalid("math: unsupported expression")
}

func unaryOp(op string) func(float64) float64 {
	if op == "!" {
		return func(v float64) float64 {
			if isNull(v) {
				return v
			}
			return flag(v == 0)
		}
	}
	return func(v float64) float64 { return -v }
}

func (m *mathEval) join(l, r []series, f func(a, b float64) float64) ([]series, error) {
	pairByPosition := len(l) == 1 && len(r) == 1
	var out []series
	for _, a := range l {
		for _, b := range r {
			if !pairByPosition && !labelsRelated(a.labels, b.labels) {
				continue
			}
			s := combine(a, b, f)
			out = append(out, s)
			m.points += len(s.values)
			if len(out) > m.limits.MaxSeries {
				return nil, datasource.Errorf(datasource.ErrLimitExceeded, "series limit exceeded: expression produced more than %d series", m.limits.MaxSeries)
			}
			if m.points > m.limits.MaxPoints {
				return nil, datasource.Errorf(datasource.ErrLimitExceeded, "points limit exceeded: expression produced more than %d points", m.limits.MaxPoints)
			}
		}
	}
	return out, nil
}

func labelsRelated(a, b map[string]string) bool {
	return isSubset(a, b) || isSubset(b, a)
}

func isSubset(small, big map[string]string) bool {
	for k, v := range small {
		if got, ok := big[k]; !ok || got != v {
			return false
		}
	}
	return true
}

func combine(a, b series, f func(a, b float64) float64) series {
	labels := a.labels
	if len(b.labels) > len(a.labels) {
		labels = b.labels
	}
	out := series{labels: labels}
	switch {
	case a.timed && b.timed:
		byTime := make(map[int64]float64, len(b.times))
		for i, t := range b.times {
			byTime[t.UnixNano()] = b.values[i]
		}
		out.timed = true
		for i, t := range a.times {
			if v, ok := byTime[t.UnixNano()]; ok {
				out.times = append(out.times, t)
				out.values = append(out.values, f(a.values[i], v))
			}
		}
	case a.timed:
		out.timed, out.times = true, a.times
		out.values = make([]float64, len(a.values))
		for i, v := range a.values {
			out.values[i] = f(v, b.values[0])
		}
	case b.timed:
		out.timed, out.times = true, b.times
		out.values = make([]float64, len(b.values))
		for i, v := range b.values {
			out.values[i] = f(a.values[0], v)
		}
	default:
		out.values = []float64{f(a.values[0], b.values[0])}
	}
	return out
}
