package triage

import (
	"math"
	"slices"
	"time"
)

type Point struct {
	At    time.Time
	Value float64
}

type Rule struct {
	Factor  float64
	Floor   float64
	Sustain int
}

var (
	ErrorRule   = Rule{Factor: 2, Floor: 0.01, Sustain: 3}
	LatencyRule = Rule{Factor: 1.5, Floor: 0, Sustain: 3}
	LogRule     = Rule{Factor: 2, Floor: 0, Sustain: 3}
)

func (r Rule) threshold(baseline float64) float64 {
	return math.Max(r.Factor*baseline, baseline+r.Floor)
}

func Onset(points []Point, windowStart time.Time, r Rule) (time.Time, bool) {
	baseline, window := split(points, windowStart)
	if len(window) == 0 {
		return time.Time{}, false
	}
	limit := r.threshold(median(values(baseline)))
	sustain := max(r.Sustain, 1)
	run := 0
	for i, p := range window {
		if !(p.Value > limit) {
			run = 0
			continue
		}
		run++
		if run == sustain {
			return window[i-run+1].At, true
		}
	}
	if run >= min(sustain, minTrailingRun) {
		return window[len(window)-run].At, true
	}
	return time.Time{}, false
}

const minTrailingRun = 2

func split(points []Point, windowStart time.Time) (baseline, window []Point) {
	for _, p := range points {
		if math.IsNaN(p.Value) || math.IsInf(p.Value, 0) {
			continue
		}
		if p.At.Before(windowStart) {
			baseline = append(baseline, p)
		} else {
			window = append(window, p)
		}
	}
	return baseline, window
}

func values(points []Point) []float64 {
	out := make([]float64, len(points))
	for i, p := range points {
		out[i] = p.Value
	}
	return out
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := slices.Clone(v)
	slices.Sort(s)
	mid := len(s) / 2
	if len(s)%2 == 1 {
		return s[mid]
	}
	return (s[mid-1] + s[mid]) / 2
}
