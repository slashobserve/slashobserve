package datasource_test

import (
	"context"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func TestNewestFirstDoublesUpToADayAndCoversTheRange(t *testing.T) {
	to := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	week := datasource.TimeRange{From: to.Add(-7 * 24 * time.Hour), To: to}
	windows := datasource.NewestFirst(week, time.Hour)
	var spans []time.Duration
	for i, w := range windows {
		spans = append(spans, w.To.Sub(w.From))
		if i > 0 && !w.To.Equal(windows[i-1].From) {
			t.Fatalf("window %d ends at %v, want the start of the newer one %v", i, w.To, windows[i-1].From)
		}
		if w.To.Sub(w.From) > datasource.WidestWindow {
			t.Fatalf("window %d spans %v, want at most a day", i, w.To.Sub(w.From))
		}
	}
	if !windows[0].To.Equal(to) || !windows[len(windows)-1].From.Equal(week.From) {
		t.Fatalf("windows %v do not cover the week", windows)
	}
	h := time.Hour
	want := []time.Duration{h, 2 * h, 4 * h, 8 * h, 16 * h, 24 * h, 24 * h, 24 * h, 24 * h, 24 * h, 17 * h}
	if len(spans) != len(want) {
		t.Fatalf("spans = %v, want %v", spans, want)
	}
	for i := range want {
		if spans[i] != want[i] {
			t.Fatalf("spans = %v, want %v", spans, want)
		}
	}
}

func TestAPacerRefusesAWindowItProjectsPastTheDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var p datasource.Pacer
	first, ok := p.Start(ctx, time.Hour)
	if !ok {
		t.Fatal("the first window was refused")
	}
	time.Sleep(600 * time.Millisecond)
	first.End(nil)
	if _, ok := p.Start(ctx, 4*time.Hour); ok {
		t.Fatal("a window projected at 2.4 s was started with under 1.4 s left before the reserve")
	}
	if w, ok := p.Start(ctx, time.Hour); !ok {
		t.Fatal("a window projected at 0.6 s was refused")
	} else if deadline, _ := w.Ctx.Deadline(); time.Until(deadline) > 1500*time.Millisecond {
		t.Fatalf("the window may run %v, want it to end a second before the limit", time.Until(deadline))
	}
}

func TestAWindowCutByItsDeadlineRanOutButACallerCancelDidNot(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1100*time.Millisecond)
	defer cancel()
	var p datasource.Pacer
	w, _ := p.Start(ctx, time.Hour)
	<-w.Ctx.Done()
	if !w.End(w.Ctx.Err()) {
		t.Fatal("a window cut by its deadline did not report running out")
	}
	parent, stop := context.WithCancel(context.Background())
	w, _ = p.Start(parent, time.Hour)
	stop()
	if w.End(context.Canceled) {
		t.Fatal("a caller cancel was reported as running out")
	}
}
