package triage

import (
	"math"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)

func series(values ...float64) []Point {
	out := make([]Point, len(values))
	for i, v := range values {
		out[i] = Point{At: t0.Add(time.Duration(i) * time.Minute), Value: v}
	}
	return out
}

func TestOnset(t *testing.T) {
	window := t0.Add(5 * time.Minute)
	for _, tc := range []struct {
		name   string
		points []Point
		rule   Rule
		want   int
	}{
		{"sustained rise", series(1, 1, 1, 1, 1, 1, 5, 5, 5, 5), Rule{Factor: 2, Sustain: 3}, 6},
		{"first step of the run, not the step that confirms it", series(1, 1, 1, 1, 1, 4, 4, 4, 1, 1), Rule{Factor: 2, Sustain: 3}, 5},
		{"a spike is not an onset", series(1, 1, 1, 1, 1, 9, 1, 9, 1, 1), Rule{Factor: 2, Sustain: 3}, -1},
		{"a run of two that reaches the newest point counts", series(1, 1, 1, 1, 1, 1, 1, 1, 6, 6), Rule{Factor: 2, Sustain: 3}, 8},
		{"one trailing point does not", series(1, 1, 1, 1, 1, 1, 1, 1, 1, 6), Rule{Factor: 2, Sustain: 3}, -1},
		{"a rise inside the baseline is not in the window", series(1, 5, 5, 5, 5, 5, 5, 5, 5, 5), Rule{Factor: 2, Sustain: 3}, -1},
		{"zero baseline needs the floor", series(0, 0, 0, 0, 0, 0.005, 0.005, 0.005, 0.005, 0.005), ErrorRule, -1},
		{"zero baseline over the floor", series(0, 0, 0, 0, 0, 0, 0.2, 0.2, 0.2, 0.2), ErrorRule, 6},
		{"the baseline is a median, so one baseline spike does not hide a rise", series(1, 1, 50, 1, 1, 1, 3, 3, 3, 3), Rule{Factor: 2, Sustain: 3}, 6},
		{"missing values are skipped", series(1, 1, 1, 1, 1, math.NaN(), 5, 5, 5, 5), Rule{Factor: 2, Sustain: 3}, 6},
		{"no window points", series(1, 1, 1), Rule{Factor: 2, Sustain: 3}, -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := Onset(tc.points, window, tc.rule)
			if tc.want < 0 {
				if ok {
					t.Fatalf("onset at %s, want none", got)
				}
				return
			}
			if want := t0.Add(time.Duration(tc.want) * time.Minute); !ok || !got.Equal(want) {
				t.Fatalf("onset = %s %v, want %s", got, ok, want)
			}
		})
	}
}

func TestMedian(t *testing.T) {
	for _, tc := range []struct {
		in   []float64
		want float64
	}{{nil, 0}, {[]float64{3}, 3}, {[]float64{3, 1}, 2}, {[]float64{5, 1, 3}, 3}} {
		if got := median(tc.in); got != tc.want {
			t.Errorf("median(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
