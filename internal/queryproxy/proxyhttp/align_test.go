package proxyhttp

import (
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

func at(offset time.Duration) time.Time {
	return time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC).Add(offset)
}

func TestAlignInstantFloorsToFiveSecondsAndKeepsDuration(t *testing.T) {
	rng := datasource.TimeRange{From: at(3 * time.Second), To: at(30*time.Minute + 7500*time.Millisecond)}
	al := align(rng, queryproxy.KindInstant, 1000)

	if want := at(30*time.Minute + 5*time.Second); !al.rng.To.Equal(want) {
		t.Fatalf("to = %v, want %v", al.rng.To, want)
	}
	if got, want := al.rng.To.Sub(al.rng.From), rng.To.Sub(rng.From); got != want {
		t.Fatalf("duration = %v, want %v", got, want)
	}
	if al.step != 0 || al.maxDataPoints != 0 {
		t.Fatalf("instant step %v and points %d, want zero", al.step, al.maxDataPoints)
	}
}

func TestAlignRangeFloorsToMultiplesOfTheStep(t *testing.T) {
	rng := datasource.TimeRange{From: at(-6*time.Hour + 1234*time.Millisecond), To: at(777 * time.Millisecond)}
	al := align(rng, queryproxy.KindRange, 1000)

	if al.step != 22*time.Second {
		t.Fatalf("step = %v, want 22s (21.6s rounded up to whole seconds)", al.step)
	}
	for name, ts := range map[string]time.Time{"from": al.rng.From, "to": al.rng.To} {
		if ts.UnixMilli()%al.step.Milliseconds() != 0 {
			t.Errorf("%s %v is not a multiple of the step", name, ts)
		}
	}
	if al.rng.From.After(rng.From) || al.rng.To.After(rng.To) {
		t.Fatalf("aligned range %v moved forward from %v", al.rng, rng)
	}
	if want := int(al.rng.To.Sub(al.rng.From) / al.step); al.maxDataPoints != want {
		t.Fatalf("points = %d, want %d", al.maxDataPoints, want)
	}
}

func TestAlignRangeCollapsedToOneStepWidensToNextStep(t *testing.T) {
	rng := datasource.TimeRange{From: at(200 * time.Millisecond), To: at(900 * time.Millisecond)}
	al := align(rng, queryproxy.KindRange, 1000)

	if al.step != time.Second || !al.rng.From.Equal(at(0)) || !al.rng.To.Equal(at(time.Second)) {
		t.Fatalf("got step %v range %v, want one 1s step from the floored start", al.step, al.rng)
	}
	if al.maxDataPoints != 1 {
		t.Fatalf("points = %d, want 1", al.maxDataPoints)
	}
}

func TestAlignShortRangeExtendsBackToMinimumPoints(t *testing.T) {
	rng := datasource.TimeRange{From: at(100 * time.Second), To: at(115 * time.Second)}
	al := align(rng, queryproxy.KindRange, 10)

	if al.step != 2*time.Second {
		t.Fatalf("step = %v, want 2s", al.step)
	}
	if !al.rng.To.Equal(at(114*time.Second)) || !al.rng.From.Equal(at(94*time.Second)) {
		t.Fatalf("range = %v, want 94s to 114s", al.rng)
	}
	if al.maxDataPoints != 10 {
		t.Fatalf("points = %d, want 10", al.maxDataPoints)
	}
}

func TestAlignNeverExceedsTheMaximumPoints(t *testing.T) {
	rng := datasource.TimeRange{From: at(-400 * 24 * time.Hour), To: at(333 * time.Millisecond)}
	al := align(rng, queryproxy.KindRange, maxMaxDataPoints)
	if al.maxDataPoints > maxMaxDataPoints {
		t.Fatalf("points = %d, want at most %d", al.maxDataPoints, maxMaxDataPoints)
	}
}

func TestAlignStepMatchesRunnerStepOnTheAlignedRange(t *testing.T) {
	offset := time.Duration(0)
	spans := []time.Duration{time.Second, 7 * time.Second, 15 * time.Second, time.Minute, 15 * time.Minute, 6 * time.Hour, 26 * time.Hour, 400 * 24 * time.Hour}
	for _, span := range spans {
		for _, points := range []int{0, 10, 11, 60, 1000, 2000, 11000} {
			for range 20 {
				offset = (offset + 7_919_137*time.Microsecond) % (48 * time.Hour)
				to := at(offset)
				rng := datasource.TimeRange{From: to.Add(-span), To: to}
				maxPoints := points
				if maxPoints == 0 {
					maxPoints = defaultMaxDataPoints
				}
				al := align(rng, queryproxy.KindRange, maxPoints)

				if got := queryproxy.StepFor(al.rng, al.maxDataPoints); got != al.step {
					t.Fatalf("span %v points %d: runner step %v, handler step %v for %v", span, maxPoints, got, al.step, al.rng)
				}
				if al.maxDataPoints < 1 || al.maxDataPoints > maxMaxDataPoints {
					t.Fatalf("span %v points %d: aligned points %d out of bounds", span, maxPoints, al.maxDataPoints)
				}
				if !al.rng.From.Before(al.rng.To) {
					t.Fatalf("span %v points %d: empty aligned range %v", span, maxPoints, al.rng)
				}
			}
		}
	}
}
