package loadgen

import (
	"math"
	"testing"
	"time"
)

func at(h, m, s, ns int) time.Time {
	return time.Date(2026, 11, 6, h, m, s, ns, time.UTC)
}

func TestDailyCurveReferenceValues(t *testing.T) {
	cases := []struct {
		at   time.Time
		want float64
	}{
		{at(0, 0, 0, 0), 0.625906},
		{at(4, 0, 0, 0), 0.4},
		{at(8, 59, 41, 7_000_000), 0.999006},
		{at(9, 0, 0, 0), 1.0},
		{at(14, 0, 0, 0), 1.6},
		{at(20, 0, 0, 0), 1.133513},
	}
	for _, c := range cases {
		if got := CurveDaily.Factor(c.at); math.Abs(got-c.want) > 1e-6 {
			t.Errorf("Factor(%s) = %.7f, want %.6f", c.at.Format("15:04:05.000"), got, c.want)
		}
	}
}

func TestDailyCurveTroughAndPeak(t *testing.T) {
	start := at(0, 0, 0, 0)
	minAt, maxAt := start, start
	minF, maxF := math.Inf(1), math.Inf(-1)
	for s := range 86400 {
		ts := start.Add(time.Duration(s) * time.Second)
		f := CurveDaily.Factor(ts)
		if f < minF {
			minF, minAt = f, ts
		}
		if f > maxF {
			maxF, maxAt = f, ts
		}
	}
	if !minAt.Equal(at(4, 0, 0, 0)) || math.Abs(minF-0.4) > 1e-9 {
		t.Errorf("minimum %.10f at %s, want 0.4 at 04:00:00", minF, minAt.Format(time.TimeOnly))
	}
	if !maxAt.Equal(at(14, 0, 0, 0)) || math.Abs(maxF-1.6) > 1e-9 {
		t.Errorf("maximum %.10f at %s, want 1.6 at 14:00:00", maxF, maxAt.Format(time.TimeOnly))
	}
}

func TestDailyCurveContinuousAtMidnight(t *testing.T) {
	for _, boundary := range []time.Time{at(0, 0, 0, 0).Add(24 * time.Hour), at(4, 0, 0, 0), at(14, 0, 0, 0)} {
		before := CurveDaily.Factor(boundary.Add(-time.Millisecond))
		after := CurveDaily.Factor(boundary)
		if math.Abs(before-after) >= 1e-6 {
			t.Errorf("step at %s: %.9f before, %.9f after", boundary.Format(time.TimeOnly), before, after)
		}
	}
}

func TestDailyCurveMeanIsOne(t *testing.T) {
	start := at(0, 0, 0, 0)
	var sum float64
	for s := range 86400 {
		sum += CurveDaily.Factor(start.Add(time.Duration(s) * time.Second))
	}
	if mean := sum / 86400; math.Abs(mean-1) > 1e-4 {
		t.Errorf("mean factor %.6f, want 1.0", mean)
	}
}

func TestFlatCurveIsOne(t *testing.T) {
	for _, ts := range []time.Time{at(4, 0, 0, 0), at(14, 0, 0, 0)} {
		if got := CurveFlat.Factor(ts); got != 1 {
			t.Errorf("flat Factor(%s) = %v, want 1", ts.Format(time.TimeOnly), got)
		}
	}
}

func TestDailyCurveUsesLocation(t *testing.T) {
	ist := time.FixedZone("ist", 5*3600+1800)
	local := time.Date(2026, 11, 6, 14, 0, 0, 0, ist)
	if utc := local.UTC(); utc.Hour() != 8 || utc.Minute() != 30 {
		t.Fatalf("14:00 ist is %s UTC, want 08:30", utc.Format(time.TimeOnly))
	}
	if got := CurveDaily.Factor(local); math.Abs(got-1.6) > 1e-9 {
		t.Errorf("Factor(14:00 ist) = %.9f, want 1.6", got)
	}
}
