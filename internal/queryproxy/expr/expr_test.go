package expr

import (
	"errors"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

var origin = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func at(seconds int) time.Time { return origin.Add(time.Duration(seconds) * time.Second) }

func timed(labels map[string]string, points ...[2]float64) datasource.Frame {
	times := make([]any, len(points))
	values := make([]any, len(points))
	for i, p := range points {
		times[i] = at(int(p[0]))
		if math.IsNaN(p[1]) {
			values[i] = nil
		} else {
			values[i] = p[1]
		}
	}
	return datasource.Frame{Labels: labels, Fields: []datasource.Field{
		{Name: "Time", Type: datasource.FieldTime, Values: times},
		{Name: "Value", Type: datasource.FieldNumber, Values: values},
	}}
}

func number(labels map[string]string, v float64) datasource.Frame {
	return datasource.Frame{Labels: labels, Fields: []datasource.Field{
		{Name: "Value", Type: datasource.FieldNumber, Values: []any{v}},
	}}
}

var null = math.NaN()

func valuesOf(t *testing.T, f datasource.Frame) []float64 {
	t.Helper()
	field := f.Fields[len(f.Fields)-1]
	if field.Type != datasource.FieldNumber {
		t.Fatalf("last field is %v, want a number field", field.Type)
	}
	out := make([]float64, len(field.Values))
	for i, v := range field.Values {
		if v == nil {
			out[i] = null
			continue
		}
		out[i] = v.(float64)
	}
	return out
}

func sameValues(got, want []float64) bool {
	return slices.EqualFunc(got, want, func(a, b float64) bool {
		if math.IsNaN(a) || math.IsNaN(b) {
			return math.IsNaN(a) == math.IsNaN(b)
		}
		return math.Abs(a-b) <= 1e-9
	})
}

var window = datasource.TimeRange{From: at(0), To: at(60)}

func eval(t *testing.T, e spec.Expression, inputs map[string][]datasource.Frame) []datasource.Frame {
	t.Helper()
	out, err := Eval(e, inputs, window, datasource.Limits{})
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}
	return out
}

func TestMath(t *testing.T) {
	a := []datasource.Frame{timed(map[string]string{"pod": "x"}, [2]float64{0, 10}, [2]float64{30, 20}, [2]float64{60, null})}
	cases := []struct {
		name string
		math string
		want []float64
	}{
		{"scalar times series", "$A * 2", []float64{20, 40, null}},
		{"series minus scalar", "$A - 1", []float64{9, 19, null}},
		{"scalar divided by series", "100 / $A", []float64{10, 5, null}},
		{"modulo", "$A % 7", []float64{3, 6, null}},
		{"power is right associative", "2 ** 3 ** 2", []float64{512}},
		{"power binds tighter than unary minus", "-2 ** 2", []float64{-4}},
		{"precedence", "1 + 2 * 3", []float64{7}},
		{"parentheses", "(1 + 2) * 3", []float64{9}},
		{"braced ref", "${A} + 1", []float64{11, 21, null}},
		{"comparison gives one or zero", "$A > 15", []float64{0, 1, null}},
		{"equality", "$A == 10", []float64{1, 0, null}},
		{"inequality", "$A != 10", []float64{0, 1, null}},
		{"less or equal", "$A <= 10", []float64{1, 0, null}},
		{"and", "($A > 5) && ($A < 15)", []float64{1, 0, null}},
		{"or", "($A < 5) || ($A > 15)", []float64{0, 1, null}},
		{"not", "!($A > 15)", []float64{1, 0, null}},
		{"abs", "abs(0 - $A)", []float64{10, 20, null}},
		{"ceil", "ceil(2.1)", []float64{3}},
		{"floor", "floor(2.9)", []float64{2}},
		{"round", "round(2.5)", []float64{3}},
		{"log", "log(exp(2))", []float64{2}},
		{"sqrt", "sqrt(16)", []float64{4}},
		{"is_null on a null point", "is_null($A)", []float64{0, 0, 1}},
		{"is_nan on a null point", "is_nan($A)", []float64{0, 0, 1}},
		{"is_number", "is_number($A)", []float64{1, 1, 0}},
		{"is_inf", "is_inf(1 / 0)", []float64{1}},
		{"division by zero is infinite", "is_inf($A / 0)", []float64{1, 1, 0}},
		{"exponent literal", "1e2 + 0.5", []float64{100.5}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := eval(t, spec.Expression{Type: spec.ExpressionMath, Math: tc.math}, map[string][]datasource.Frame{"A": a})
			if len(out) != 1 {
				t.Fatalf("got %d frames, want 1", len(out))
			}
			if got := valuesOf(t, out[0]); !sameValues(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMathOutputShape(t *testing.T) {
	a := []datasource.Frame{timed(map[string]string{"pod": "x"}, [2]float64{0, 1}, [2]float64{30, 2})}
	out := eval(t, spec.Expression{Type: spec.ExpressionMath, Math: "$A + 1"}, map[string][]datasource.Frame{"A": a})
	if len(out[0].Fields) != 2 || out[0].Fields[0].Type != datasource.FieldTime || out[0].Fields[1].Name != "Value" {
		t.Fatalf("fields = %+v, want Time and Value", out[0].Fields)
	}
	if out[0].Labels["pod"] != "x" {
		t.Errorf("labels = %v, want pod=x kept", out[0].Labels)
	}
	if got := out[0].Fields[0].Values[1]; got != at(30) {
		t.Errorf("second time = %v, want %v", got, at(30))
	}

	single := eval(t, spec.Expression{Type: spec.ExpressionMath, Math: "$B * 2"}, map[string][]datasource.Frame{"B": {number(nil, 4)}})
	if len(single[0].Fields) != 1 || single[0].Fields[0].Type != datasource.FieldNumber {
		t.Errorf("a number input gives %+v, want one number field", single[0].Fields)
	}
}

func TestMathJoinsSeries(t *testing.T) {
	podX := map[string]string{"pod": "x"}
	podY := map[string]string{"pod": "y"}
	both := map[string]string{"pod": "x", "ns": "shop"}
	t.Run("equal labels pair on timestamps", func(t *testing.T) {
		inputs := map[string][]datasource.Frame{
			"A": {timed(podX, [2]float64{0, 10}, [2]float64{30, 20}, [2]float64{60, 30}), timed(podY, [2]float64{0, 1}, [2]float64{30, 2})},
			"B": {timed(podX, [2]float64{0, 2}, [2]float64{60, 3}), timed(podY, [2]float64{30, 4})},
		}
		out := eval(t, spec.Expression{Type: spec.ExpressionMath, Math: "$A * $B"}, inputs)
		if len(out) != 2 {
			t.Fatalf("got %d series, want 2", len(out))
		}
		byPod := map[string][]float64{}
		for _, f := range out {
			byPod[f.Labels["pod"]] = valuesOf(t, f)
		}
		if !sameValues(byPod["x"], []float64{20, 90}) {
			t.Errorf("x = %v, want [20 90] (only timestamps both sides have)", byPod["x"])
		}
		if !sameValues(byPod["y"], []float64{8}) {
			t.Errorf("y = %v, want [8]", byPod["y"])
		}
	})
	t.Run("a subset label set pairs with the larger one and the larger labels win", func(t *testing.T) {
		inputs := map[string][]datasource.Frame{
			"A": {timed(both, [2]float64{0, 8})},
			"B": {timed(podX, [2]float64{0, 2})},
			"C": {timed(nil, [2]float64{0, 4})},
		}
		out := eval(t, spec.Expression{Type: spec.ExpressionMath, Math: "$A / $B / $C"}, inputs)
		if len(out) != 1 || out[0].Labels["ns"] != "shop" || !sameValues(valuesOf(t, out[0]), []float64{1}) {
			t.Fatalf("got %+v, want one series with ns=shop and value 1", out)
		}
	})
	t.Run("unmatched series are dropped", func(t *testing.T) {
		inputs := map[string][]datasource.Frame{
			"A": {timed(podX, [2]float64{0, 1}), timed(podY, [2]float64{0, 2})},
			"B": {timed(podX, [2]float64{0, 10}), timed(map[string]string{"pod": "z"}, [2]float64{0, 5})},
		}
		out := eval(t, spec.Expression{Type: spec.ExpressionMath, Math: "$A + $B"}, inputs)
		if len(out) != 1 || out[0].Labels["pod"] != "x" {
			t.Fatalf("got %+v, want only pod x", out)
		}
	})
	t.Run("a number broadcasts over every series", func(t *testing.T) {
		inputs := map[string][]datasource.Frame{
			"A": {timed(podX, [2]float64{0, 1}, [2]float64{30, 2}), timed(podY, [2]float64{0, 3})},
			"B": {number(nil, 10)},
		}
		out := eval(t, spec.Expression{Type: spec.ExpressionMath, Math: "$A * $B"}, inputs)
		if len(out) != 2 {
			t.Fatalf("got %d series, want 2", len(out))
		}
		if !sameValues(valuesOf(t, out[0]), []float64{10, 20}) || !sameValues(valuesOf(t, out[1]), []float64{30}) {
			t.Errorf("values = %v and %v", valuesOf(t, out[0]), valuesOf(t, out[1]))
		}
	})
	t.Run("two single series pair whatever their labels", func(t *testing.T) {
		inputs := map[string][]datasource.Frame{
			"A": {timed(podX, [2]float64{0, 6})},
			"B": {timed(podY, [2]float64{0, 3})},
		}
		out := eval(t, spec.Expression{Type: spec.ExpressionMath, Math: "$A / $B"}, inputs)
		if len(out) != 1 || !sameValues(valuesOf(t, out[0]), []float64{2}) {
			t.Fatalf("got %+v, want one series [2]", out)
		}
	})
	t.Run("a null point stays null", func(t *testing.T) {
		inputs := map[string][]datasource.Frame{
			"A": {timed(podX, [2]float64{0, null}, [2]float64{30, 2})},
			"B": {timed(podX, [2]float64{0, 1}, [2]float64{30, 1})},
		}
		out := eval(t, spec.Expression{Type: spec.ExpressionMath, Math: "$A + $B"}, inputs)
		if !sameValues(valuesOf(t, out[0]), []float64{null, 3}) {
			t.Errorf("got %v", valuesOf(t, out[0]))
		}
	})
}

func TestMathErrors(t *testing.T) {
	a := map[string][]datasource.Frame{"A": {timed(nil, [2]float64{0, 1})}}
	cases := []struct {
		name string
		math string
		in   map[string][]datasource.Frame
	}{
		{"unknown function", "sin($A)", a},
		{"function without parentheses", "abs $A", a},
		{"unbalanced parenthesis", "($A + 1", a},
		{"trailing operator", "$A +", a},
		{"stray character", "$A @ 1", a},
		{"lonely dollar", "$ + 1", a},
		{"bad number", "1.2.3", a},
		{"missing input", "$B + 1", a},
		{"two arguments", "abs(1, 2)", a},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Eval(spec.Expression{Type: spec.ExpressionMath, Math: tc.math}, tc.in, window, datasource.Limits{})
			if !errors.Is(err, ErrExpression) {
				t.Fatalf("err = %v, want ErrExpression", err)
			}
		})
	}
}

func TestMathLimits(t *testing.T) {
	var frames []datasource.Frame
	for i := 0; i < 5; i++ {
		frames = append(frames, timed(map[string]string{"n": string(rune('a' + i))}, [2]float64{0, 1}))
	}
	inputs := map[string][]datasource.Frame{"A": frames, "B": {number(nil, 1)}}
	_, err := Eval(spec.Expression{Type: spec.ExpressionMath, Math: "$A + $B"}, inputs, window, datasource.Limits{MaxSeries: 3})
	if !errors.Is(err, datasource.ErrLimitExceeded) {
		t.Fatalf("err = %v, want ErrLimitExceeded", err)
	}
	_, err = Eval(spec.Expression{Type: spec.ExpressionMath, Math: "$A + $B"}, inputs, window, datasource.Limits{MaxPoints: 2})
	if !errors.Is(err, datasource.ErrLimitExceeded) {
		t.Fatalf("points: err = %v, want ErrLimitExceeded", err)
	}
}

func TestInputShape(t *testing.T) {
	table := datasource.Frame{Fields: []datasource.Field{{Name: "v", Type: datasource.FieldNumber, Values: []any{1.0, 2.0}}}}
	badTime := datasource.Frame{Fields: []datasource.Field{
		{Name: "Time", Type: datasource.FieldTime, Values: []any{1.0}},
		{Name: "Value", Type: datasource.FieldNumber, Values: []any{1.0}},
	}}
	badNumber := datasource.Frame{Fields: []datasource.Field{{Name: "v", Type: datasource.FieldNumber, Values: []any{"x"}}}}
	for name, frame := range map[string]datasource.Frame{"table": table, "bad time": badTime, "bad number": badNumber} {
		t.Run(name, func(t *testing.T) {
			_, err := Eval(spec.Expression{Type: spec.ExpressionReduce, Input: "A", Reducer: "sum"}, map[string][]datasource.Frame{"A": {frame}}, window, datasource.Limits{})
			if !errors.Is(err, ErrExpression) {
				t.Fatalf("err = %v, want ErrExpression", err)
			}
		})
	}
	_, err := Eval(spec.Expression{Type: spec.ExpressionReduce, Input: "A", Reducer: "sum"}, nil, window, datasource.Limits{})
	if !errors.Is(err, ErrExpression) {
		t.Errorf("missing input: err = %v, want ErrExpression", err)
	}
	_, err = Eval(spec.Expression{Type: "sql"}, nil, window, datasource.Limits{})
	if !errors.Is(err, ErrExpression) {
		t.Errorf("unknown type: err = %v, want ErrExpression", err)
	}
}

func TestReduce(t *testing.T) {
	frame := timed(map[string]string{"pod": "x"}, [2]float64{0, 4}, [2]float64{10, 1}, [2]float64{20, 3}, [2]float64{30, 2})
	replace := 0.0
	cases := []struct {
		reducer string
		want    float64
	}{
		{"mean", 2.5}, {"min", 1}, {"max", 4}, {"sum", 10}, {"count", 4}, {"last", 2}, {"median", 2.5},
	}
	for _, tc := range cases {
		t.Run(tc.reducer, func(t *testing.T) {
			out := eval(t, spec.Expression{Type: spec.ExpressionReduce, Input: "A", Reducer: tc.reducer}, map[string][]datasource.Frame{"A": {frame}})
			if len(out) != 1 || len(out[0].Fields) != 1 {
				t.Fatalf("got %+v, want one frame with one field", out)
			}
			if got := valuesOf(t, out[0]); !sameValues(got, []float64{tc.want}) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
			if out[0].Labels["pod"] != "x" {
				t.Errorf("labels = %v, want pod=x", out[0].Labels)
			}
		})
	}
	t.Run("odd median", func(t *testing.T) {
		f := timed(nil, [2]float64{0, 5}, [2]float64{10, 1}, [2]float64{20, 3})
		out := eval(t, spec.Expression{Type: spec.ExpressionReduce, Input: "A", Reducer: "median"}, map[string][]datasource.Frame{"A": {f}})
		if got := valuesOf(t, out[0]); !sameValues(got, []float64{3}) {
			t.Errorf("got %v", got)
		}
	})

	withNull := timed(nil, [2]float64{0, 4}, [2]float64{10, null}, [2]float64{20, 2})
	modes := []struct {
		name string
		e    spec.Expression
		want float64
	}{
		{"default is strict", spec.Expression{Reducer: "sum"}, null},
		{"strict", spec.Expression{Reducer: "sum", Mode: "strict"}, null},
		{"strict count", spec.Expression{Reducer: "count", Mode: "strict"}, null},
		{"drop sum", spec.Expression{Reducer: "sum", Mode: "dropNonNumbers"}, 6},
		{"drop count", spec.Expression{Reducer: "count", Mode: "dropNonNumbers"}, 2},
		{"drop mean", spec.Expression{Reducer: "mean", Mode: "dropNonNumbers"}, 3},
		{"replace sum", spec.Expression{Reducer: "sum", Mode: "replaceNonNumbers", ReplaceWith: &replace}, 6},
		{"replace count", spec.Expression{Reducer: "count", Mode: "replaceNonNumbers", ReplaceWith: &replace}, 3},
		{"replace min", spec.Expression{Reducer: "min", Mode: "replaceNonNumbers", ReplaceWith: &replace}, 0},
	}
	for _, tc := range modes {
		t.Run(tc.name, func(t *testing.T) {
			tc.e.Type, tc.e.Input = spec.ExpressionReduce, "A"
			out := eval(t, tc.e, map[string][]datasource.Frame{"A": {withNull}})
			if got := valuesOf(t, out[0]); !sameValues(got, []float64{tc.want}) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}

	t.Run("empty series", func(t *testing.T) {
		empty := timed(nil)
		for reducer, want := range map[string]float64{"sum": 0, "count": 0, "mean": null, "max": null, "last": null, "median": null} {
			out := eval(t, spec.Expression{Type: spec.ExpressionReduce, Input: "A", Reducer: reducer}, map[string][]datasource.Frame{"A": {empty}})
			if got := valuesOf(t, out[0]); !sameValues(got, []float64{want}) {
				t.Errorf("%s: got %v, want %v", reducer, got, want)
			}
		}
	})
	t.Run("replace without a value fails", func(t *testing.T) {
		_, err := Eval(spec.Expression{Type: spec.ExpressionReduce, Input: "A", Reducer: "sum", Mode: "replaceNonNumbers"}, map[string][]datasource.Frame{"A": {withNull}}, window, datasource.Limits{})
		if !errors.Is(err, ErrExpression) {
			t.Errorf("err = %v, want ErrExpression", err)
		}
	})
	t.Run("one frame per input series", func(t *testing.T) {
		in := map[string][]datasource.Frame{"A": {timed(map[string]string{"pod": "x"}, [2]float64{0, 1}), timed(map[string]string{"pod": "y"}, [2]float64{0, 2})}}
		out := eval(t, spec.Expression{Type: spec.ExpressionReduce, Input: "A", Reducer: "max"}, in)
		if len(out) != 2 || out[1].Labels["pod"] != "y" {
			t.Errorf("got %+v", out)
		}
	})
	t.Run("a number input reduces to itself", func(t *testing.T) {
		out := eval(t, spec.Expression{Type: spec.ExpressionReduce, Input: "A", Reducer: "sum"}, map[string][]datasource.Frame{"A": {number(nil, 7)}})
		if got := valuesOf(t, out[0]); !sameValues(got, []float64{7}) {
			t.Errorf("got %v", got)
		}
	})
}

func TestResample(t *testing.T) {
	frame := timed(map[string]string{"pod": "x"},
		[2]float64{0, 1}, [2]float64{10, 3},
		[2]float64{40, 8}, [2]float64{50, 4},
	)
	cases := []struct {
		name       string
		down, up   string
		wantValues []float64
	}{
		{"mean pad", "mean", "pad", []float64{2, 2, 6, 6}},
		{"min backfill", "min", "backfill", []float64{1, 4, 4, null}},
		{"max fillNA", "max", "fillNA", []float64{3, null, 8, null}},
		{"sum pad", "sum", "pad", []float64{4, 4, 12, 12}},
		{"last fillNA", "last", "fillNA", []float64{3, null, 4, null}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := eval(t, spec.Expression{Type: spec.ExpressionResample, Input: "A", Window: "20s", Downsampler: tc.down, Upsampler: tc.up}, map[string][]datasource.Frame{"A": {frame}})
			if len(out) != 1 {
				t.Fatalf("got %d frames", len(out))
			}
			times := out[0].Fields[0].Values
			if len(times) != 4 || times[0] != at(0) || times[3] != at(60) {
				t.Fatalf("times = %v, want 0s, 20s, 40s, 60s", times)
			}
			if got := valuesOf(t, out[0]); !sameValues(got, tc.wantValues) {
				t.Errorf("got %v, want %v", got, tc.wantValues)
			}
		})
	}
	t.Run("upsampling into more points", func(t *testing.T) {
		coarse := timed(nil, [2]float64{0, 10}, [2]float64{60, 20})
		out := eval(t, spec.Expression{Type: spec.ExpressionResample, Input: "A", Window: "30s", Downsampler: "mean", Upsampler: "pad"}, map[string][]datasource.Frame{"A": {coarse}})
		if got := valuesOf(t, out[0]); !sameValues(got, []float64{10, 10, 20}) {
			t.Errorf("got %v", got)
		}
	})
	t.Run("points outside the range are ignored", func(t *testing.T) {
		wide := timed(nil, [2]float64{-30, 100}, [2]float64{0, 1}, [2]float64{90, 100})
		out := eval(t, spec.Expression{Type: spec.ExpressionResample, Input: "A", Window: "1m", Downsampler: "sum", Upsampler: "fillNA"}, map[string][]datasource.Frame{"A": {wide}})
		if got := valuesOf(t, out[0]); !sameValues(got, []float64{1, null}) {
			t.Errorf("got %v", got)
		}
	})
	t.Run("null points do not count", func(t *testing.T) {
		gappy := timed(nil, [2]float64{0, null}, [2]float64{10, 5})
		out := eval(t, spec.Expression{Type: spec.ExpressionResample, Input: "A", Window: "30s", Downsampler: "mean", Upsampler: "fillNA"}, map[string][]datasource.Frame{"A": {gappy}})
		if got := valuesOf(t, out[0]); !sameValues(got, []float64{5, null, null}) {
			t.Errorf("got %v", got)
		}
	})
	t.Run("a number cannot be resampled", func(t *testing.T) {
		_, err := Eval(spec.Expression{Type: spec.ExpressionResample, Input: "A", Window: "20s", Downsampler: "mean", Upsampler: "pad"}, map[string][]datasource.Frame{"A": {number(nil, 1)}}, window, datasource.Limits{})
		if !errors.Is(err, ErrExpression) {
			t.Errorf("err = %v, want ErrExpression", err)
		}
	})
	t.Run("too many buckets hits the points limit", func(t *testing.T) {
		_, err := Eval(spec.Expression{Type: spec.ExpressionResample, Input: "A", Window: "1ms", Downsampler: "mean", Upsampler: "pad"}, map[string][]datasource.Frame{"A": {frame}}, window, datasource.Limits{MaxPoints: 1000})
		if !errors.Is(err, datasource.ErrLimitExceeded) {
			t.Errorf("err = %v, want ErrLimitExceeded", err)
		}
	})
	t.Run("an unusable range fails", func(t *testing.T) {
		_, err := Eval(spec.Expression{Type: spec.ExpressionResample, Input: "A", Window: "20s", Downsampler: "mean", Upsampler: "pad"}, map[string][]datasource.Frame{"A": {frame}}, datasource.TimeRange{}, datasource.Limits{})
		if !errors.Is(err, ErrExpression) {
			t.Errorf("err = %v, want ErrExpression", err)
		}
	})
}

func TestThreshold(t *testing.T) {
	frame := timed(map[string]string{"pod": "x"}, [2]float64{0, 1}, [2]float64{10, 2}, [2]float64{20, 3}, [2]float64{30, 4}, [2]float64{40, null})
	cases := []struct {
		evaluator string
		params    []float64
		want      []float64
	}{
		{"gt", []float64{2}, []float64{0, 0, 1, 1, null}},
		{"lt", []float64{2}, []float64{1, 0, 0, 0, null}},
		{"withinRange", []float64{2, 4}, []float64{0, 0, 1, 0, null}},
		{"outsideRange", []float64{2, 4}, []float64{1, 0, 0, 0, null}},
		{"withinRangeIncluded", []float64{2, 4}, []float64{0, 1, 1, 1, null}},
		{"outsideRangeIncluded", []float64{2, 4}, []float64{1, 1, 0, 1, null}},
		{"withinRange", []float64{4, 2}, []float64{0, 0, 1, 0, null}},
	}
	for _, tc := range cases {
		t.Run(tc.evaluator, func(t *testing.T) {
			out := eval(t, spec.Expression{Type: spec.ExpressionThreshold, Input: "A", Evaluator: tc.evaluator, Params: tc.params}, map[string][]datasource.Frame{"A": {frame}})
			if got := valuesOf(t, out[0]); !sameValues(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
			if len(out[0].Fields) != 2 || out[0].Labels["pod"] != "x" {
				t.Errorf("shape = %+v labels %v, want time and value with pod=x", out[0].Fields, out[0].Labels)
			}
		})
	}
	t.Run("wrong parameter count fails", func(t *testing.T) {
		_, err := Eval(spec.Expression{Type: spec.ExpressionThreshold, Input: "A", Evaluator: "gt", Params: []float64{1, 2}}, map[string][]datasource.Frame{"A": {frame}}, window, datasource.Limits{})
		if !errors.Is(err, ErrExpression) {
			t.Errorf("err = %v, want ErrExpression", err)
		}
	})
}
