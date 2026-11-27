package triage

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestMetricRows(t *testing.T) {
	windowStart := t0.Add(5 * time.Minute)
	req := pts(t0, time.Minute, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10)
	flat := pts(t0, time.Minute, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	rising := pts(t0, time.Minute, 0, 0, 0, 0, 0, 5, 5, 5, 5, 5)
	cases := []struct {
		name       string
		errs       map[string][]Point
		errsRead   bool
		wantSignal []string
		wantMoved  bool
	}{
		{"error read failed", nil, false, []string{SignalRequests}, false},
		{"error read ok and empty is a real zero", nil, true, []string{SignalRequests, SignalErrors}, false},
		{"error read ok with zero errors", map[string][]Point{"cart": flat}, true, []string{SignalRequests, SignalErrors}, false},
		{"error read ok with a rise", map[string][]Point{"cart": rising}, true, []string{SignalRequests, SignalErrors}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rows := metricRows(Row{Rung: time.Hour}, windowStart, map[string][]Point{"cart": req}, c.errs, nil, 1, c.errsRead)
			var got []string
			moved := false
			for _, r := range rows {
				got = append(got, r.Signal)
				moved = moved || r.Moved
			}
			if strings.Join(got, ",") != strings.Join(c.wantSignal, ",") {
				t.Fatalf("signals = %v, want %v", got, c.wantSignal)
			}
			if moved != c.wantMoved {
				t.Fatalf("moved = %v, want %v", moved, c.wantMoved)
			}
		})
	}
}

type errorReadFails struct {
	slowReader
	errs atomic.Int64
}

func (r *errorReadFails) Range(_ context.Context, _, query string, rng Window, points int) ([]Series, error) {
	if strings.Contains(query, "STATUS_CODE_ERROR") {
		r.errs.Add(1)
		return nil, errors.New("upstream timed out")
	}
	step := rng.To.Sub(rng.From) / time.Duration(points)
	var p []Point
	for at := rng.From; at.Before(rng.To); at = at.Add(step) {
		p = append(p, Point{At: at, Value: 10})
	}
	return []Series{{Labels: map[string]string{"service_name": "cart"}, Points: p}}, nil
}

func TestSweepDoesNotReportAFailedErrorReadAsAnAllClear(t *testing.T) {
	reader := &errorReadFails{}
	out := Sweeper{Reader: reader, Limits: DefaultLimits}.Sweep(context.Background(), Input{Sources: []Source{{"prom", "prometheus"}}, End: end})
	if reader.errs.Load() == 0 {
		t.Fatal("the error query was never read")
	}
	for _, want := range []string{
		"not an all-clear",
		"the reads of errors were skipped or failed",
		"Skipped: metric rates over 1h on prom: the read failed: upstream timed out.",
	} {
		if !strings.Contains(out.Block, want) {
			t.Errorf("block lacks %q:\n%s", want, out.Block)
		}
	}
	if strings.Contains(out.Block, "errors 0%") {
		t.Errorf("a failed error read was rendered as 0%%:\n%s", out.Block)
	}
	if strings.Contains(out.Block, "No service's errors or latency moved") {
		t.Errorf("the block still states an all-clear:\n%s", out.Block)
	}
}

func TestSweepKeepsToOneConcurrencyLimitAcrossNestedParallelReads(t *testing.T) {
	for _, limit := range []int{1, 2, 4} {
		reader := &gauge{slowReader: slowReader{delay: 2 * time.Millisecond}}
		lim := Limits{Calls: 200, Time: 10 * time.Second, Concurrency: limit}
		done := make(chan struct{})
		go func() {
			Sweeper{Reader: reader, Limits: lim}.Sweep(context.Background(), Input{Sources: manySources(), End: end})
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			t.Fatalf("limit %d: the sweep deadlocked", limit)
		}
		if peak := reader.peak.Load(); peak > int64(limit) || peak == 0 {
			t.Errorf("limit %d: peak reads in flight = %d", limit, peak)
		}
	}
}

type gauge struct {
	slowReader
	now, peak atomic.Int64
}

func (g *gauge) hold(ctx context.Context) error {
	n := g.now.Add(1)
	defer g.now.Add(-1)
	for {
		p := g.peak.Load()
		if n <= p || g.peak.CompareAndSwap(p, n) {
			break
		}
	}
	return g.wait(ctx)
}

func (g *gauge) Services(ctx context.Context, _, _ string) (string, []string, error) {
	return "service_name", []string{"a", "b"}, g.hold(ctx)
}

func (g *gauge) MetricNames(ctx context.Context, _, _ string) ([]string, error) {
	return []string{"traces_span_metrics_calls_total", "traces_span_metrics_duration_seconds_bucket"}, g.hold(ctx)
}

func (g *gauge) Range(ctx context.Context, _, _ string, _ Window, _ int) ([]Series, error) {
	return nil, g.hold(ctx)
}

func (g *gauge) TraceStats(ctx context.Context, _ string, _ TraceStatsInput) (TraceStats, error) {
	return TraceStats{Groups: []TraceGroup{{Value: "k", Spans: 1}}}, g.hold(ctx)
}

func (g *gauge) ErrorOrigins(ctx context.Context, _ string, _ Window) ([]Origin, error) {
	return nil, g.hold(ctx)
}

func (g *gauge) Patterns(ctx context.Context, _, _ string, _ Window) ([]PatternTrend, error) {
	return nil, g.hold(ctx)
}

func (g *gauge) Annotations(ctx context.Context, _ Window) ([]Event, error) {
	return nil, g.hold(ctx)
}
