package triage

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPickMetrics(t *testing.T) {
	for _, tc := range []struct {
		name            string
		calls, duration []string
		label           string
		wantOK          bool
		basis, requests string
		bucket          string
		scale           float64
	}{
		{"span metrics first", []string{"calls_total", "traces_span_metrics_calls_total"}, []string{"traces_span_metrics_duration_seconds_bucket", "http_server_request_duration_seconds_bucket"}, "service_name", true, basisSpanMetrics, "traces_span_metrics_calls_total{" + spanKinds + "}", "traces_span_metrics_duration_seconds_bucket", 1000},
		{"HTTP server histogram otherwise", nil, []string{"http_server_request_duration_seconds_bucket", "http_server_request_duration_seconds_count"}, "job", true, basisHTTPMetrics, "http_server_request_duration_seconds_count", "http_server_request_duration_seconds_bucket", 1000},
		{"span calls without a histogram", []string{"calls_total"}, nil, "service", true, basisSpanMetrics, "calls_total{" + spanKinds + "}", "", 1},
		{"nothing usable", []string{"rpc_calls"}, []string{"gc_duration_seconds"}, "service_name", false, "", "", "", 0},
		{"a label that is not a label name", []string{"calls_total"}, nil, "service.name", false, "", "", "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := pickMetrics(tc.calls, tc.duration, tc.label)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v", ok)
			}
			if !ok {
				return
			}
			if got.basis != tc.basis || got.requests != tc.requests || got.bucket != tc.bucket || got.scale != tc.scale {
				t.Fatalf("plan = %+v", got)
			}
		})
	}
}

func TestMetricQueries(t *testing.T) {
	plan, _ := pickMetrics(nil, []string{"http_server_request_duration_seconds_bucket"}, "service_name")
	if got, want := plan.errorQuery(2*time.Minute), `sum by (service_name) (rate(http_server_request_duration_seconds_count{http_response_status_code=~"5.."}[120s])) or sum by (service_name) (rate(http_server_request_duration_seconds_count{http_status_code=~"5.."}[120s]))`; got != want {
		t.Errorf("errors = %s", got)
	}
	if got, want := plan.latencyQuery(6*time.Minute), `histogram_quantile(0.95, sum by (service_name, le) (rate(http_server_request_duration_seconds_bucket[360s])))`; got != want {
		t.Errorf("latency = %s", got)
	}
	if got := rateWindow(30 * time.Second); got != 2*time.Minute {
		t.Errorf("rate window = %s", got)
	}
}

func TestChangedPatterns(t *testing.T) {
	got := changedPatterns([]PatternTrend{
		{Text: "new error", Level: "error", Trend: []int64{0, 0, 0, 2, 3}},
		{Text: "steady", Level: "info", Trend: []int64{5, 5, 5, 5, 5, 5}},
		{Text: "grew", Level: "warn", Trend: []int64{1, 1, 10, 10}},
		{Text: "too few to be new", Trend: []int64{0, 0, 1, 1}},
	}, "loki", "payment")
	var texts []string
	for _, p := range got {
		texts = append(texts, p.Text)
	}
	if want := []string{"new error", "grew"}; !slices.Equal(texts, want) {
		t.Fatalf("patterns = %v, want %v", texts, want)
	}
	if !got[0].New() || got[1].New() {
		t.Errorf("new flags = %v %v", got[0].New(), got[1].New())
	}
}

func pts(start time.Time, step time.Duration, values ...float64) []Point {
	out := make([]Point, len(values))
	for i, v := range values {
		out[i] = Point{At: start.Add(time.Duration(i) * step), Value: v}
	}
	return out
}

func TestBurstStarts(t *testing.T) {
	sparse := append(pts(t0, time.Minute, 3, 2), pts(t0.Add(10*time.Minute), time.Minute, 4, 1, 1)...)
	got := burstStarts([]Series{{Labels: map[string]string{"service_name": "cart"}, Points: sparse}}, "service_name", "loki")
	if len(got) != 2 || !got[0].At.Equal(t0) || !got[1].At.Equal(t0.Add(10*time.Minute)) || got[1].Service != "cart" {
		t.Fatalf("bursts = %+v", got)
	}
}

func TestFlagFirstSeen(t *testing.T) {
	from := t0
	got := flagFirstSeen([]Series{
		{Labels: map[string]string{"feature_flag_key": "paymentFailure", "feature_flag_result_variant": "50%"}, Points: pts(t0, 10*time.Minute, 0, 0, 0, 4, 5)},
		{Labels: map[string]string{"feature_flag_key": "paymentFailure", "feature_flag_variant": "off"}, Points: pts(t0, 10*time.Minute, 9, 9, 9, 5, 4)},
		{Labels: map[string]string{"other": "x"}, Points: pts(t0, time.Minute, 1)},
	}, "loki", from)
	if len(got) != 2 {
		t.Fatalf("events = %+v", got)
	}
	if !strings.Contains(got[0].Text, "variant off already evaluated") || !got[1].At.Equal(t0.Add(30*time.Minute)) || !strings.Contains(got[1].Text, "paymentFailure variant 50% first seen") {
		t.Fatalf("events = %+v", got)
	}
}

func TestErrorLinesFillsMissingStepsWithZero(t *testing.T) {
	label := map[string]string{"service_name": "cart", levelLabel: "info"}
	errs := map[string]string{"service_name": "cart", levelLabel: "ERROR"}
	got := errorLines([]Series{
		{Labels: label, Points: pts(t0, time.Minute, 5, 5, 5)},
		{Labels: errs, Points: []Point{{At: t0.Add(2 * time.Minute), Value: 7}}},
	}, "service_name")
	want := pts(t0, time.Minute, 0, 0, 7)
	if !slices.Equal(got["cart"], want) {
		t.Fatalf("error lines = %v, want %v", got["cart"], want)
	}
}

type scripted struct {
	mu    sync.Mutex
	asked []string
	stats func(in TraceStatsInput) TraceStats
}

func (s *scripted) note(what string) {
	s.mu.Lock()
	s.asked = append(s.asked, what)
	s.mu.Unlock()
}

func (s *scripted) Services(_ context.Context, uid, signal string) (string, []string, error) {
	s.note("services " + uid)
	switch uid {
	case "tempo":
		return "resource.service.name", []string{"checkout", "frontend", "payment"}, nil
	case "loki":
		return "service_name", []string{"checkout", "frontend", "payment"}, nil
	}
	return "", nil, nil
}

func (s *scripted) MetricNames(context.Context, string, string) ([]string, error) { return nil, nil }

func (s *scripted) Range(_ context.Context, uid, query string, rng Window, points int) ([]Series, error) {
	s.note("range " + uid + " " + query)
	step := rng.To.Sub(rng.From) / time.Duration(points)
	switch {
	case strings.Contains(query, "feature_flag_key"):
		onset := end.Add(-22 * time.Minute)
		var p []Point
		for at := rng.From; at.Before(rng.To); at = at.Add(step) {
			if !at.Before(onset) {
				p = append(p, Point{At: at, Value: 3})
			}
		}
		return []Series{{Labels: map[string]string{"feature_flag_key": "paymentFailure", "feature_flag_result_variant": "on"}, Points: p}}, nil
	case strings.Contains(query, "deploy"):
		return nil, nil
	}
	return nil, nil
}

func (s *scripted) TraceStats(_ context.Context, uid string, in TraceStatsInput) (TraceStats, error) {
	s.note("stats " + uid + " " + in.Query)
	return s.stats(in), nil
}

func (s *scripted) ErrorOrigins(_ context.Context, uid string, _ Window) ([]Origin, error) {
	s.note("origins " + uid)
	return []Origin{{Source: uid, Service: "payment", Operation: "Charge", Traces: 30, Example: "abc", RelayedBy: []string{"checkout", "frontend"}}}, nil
}

func (s *scripted) Patterns(_ context.Context, uid, query string, _ Window) ([]PatternTrend, error) {
	s.note("patterns " + query)
	return []PatternTrend{{Text: "charge failed <_>", Level: "error", Trend: []int64{0, 0, 0, 0, 5, 9}}}, nil
}

func (s *scripted) Annotations(context.Context, Window) ([]Event, error) { return nil, nil }

func TestSweepFallsBackToTraceRatesAndFindsTheFlagBeforeTheOnset(t *testing.T) {
	faulty := map[string]bool{"payment": true, "checkout": true, "frontend": true}
	reader := &scripted{stats: func(in TraceStatsInput) TraceStats {
		if strings.Contains(in.Query, "feature_flag") {
			return TraceStats{}
		}
		if in.GroupBy == traceServiceKey {
			now := in.Range.To.Equal(end)
			var groups []TraceGroup
			for _, svc := range []string{"checkout", "frontend", "payment"} {
				g := TraceGroup{Value: svc, Spans: 1000}
				if now && faulty[svc] && in.Range.To.Sub(in.Range.From) == time.Hour {
					g.Errors = 300
				}
				groups = append(groups, g)
			}
			return TraceStats{Groups: groups}
		}
		var buckets []TraceBucket
		step := in.Range.To.Sub(in.Range.From) / time.Duration(in.Buckets)
		for at := in.Range.From; at.Before(in.Range.To); at = at.Add(step) {
			b := TraceBucket{Start: at, Spans: 100}
			onset := end.Add(-20 * time.Minute)
			if strings.Contains(in.Query, "frontend") || strings.Contains(in.Query, "checkout") {
				onset = end.Add(-16 * time.Minute)
			}
			if !at.Before(onset) {
				b.Errors = 40
			}
			buckets = append(buckets, b)
		}
		return TraceStats{Buckets: buckets}
	}}
	out := Sweeper{Reader: reader, Limits: DefaultLimits}.Sweep(context.Background(), Input{Sources: []Source{{"loki", "loki"}, {"tempo", "tempo"}}, End: end})
	for _, want := range []string{
		"span counts, errors and p95 duration per service on tempo from trace spans",
		"c1 payment: errors 30% from 0% (1h, tempo trace spans); onset 2026-11-20T11:40:00Z; knock-on, relaying its errors: checkout, frontend",
		"origin (tempo): payment Charge, 30 failing traces, example abc, relayed by checkout, frontend",
		"change (loki): flag at 2026-11-20T11:40:00Z",
		"paymentFailure variant on first seen",
		`log pattern (loki, error) new, 14 lines: "charge failed <_>"`,
	} {
		if !strings.Contains(out.Block, want) {
			t.Errorf("block lacks %q:\n%s", want, out.Block)
		}
	}
	if strings.Contains(out.Block, "c2 ") {
		t.Errorf("knock-on services became candidates:\n%s", out.Block)
	}
	if !slices.Contains(reader.asked, `patterns {service_name="payment"}`) {
		t.Errorf("patterns were not read for the candidate: %v", reader.asked)
	}
}
