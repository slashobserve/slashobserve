package triage

import (
	"fmt"
	"strings"
	"time"
)

func renderCases() []renderCase {
	var cases []renderCase
	add := func(name string, r Report, budget int) {
		cases = append(cases, renderCase{Name: name, Report: reportJSONOf(r), Budget: budget})
	}
	add("a sample report", sampleReport(), MaxBlockBytes)
	none := sampleReport()
	none.Candidates, none.Background = nil, nil
	add("nothing moved", none, MaxBlockBytes)
	cut := sampleReport()
	for i := range 40 {
		cut.Skipped = append(cut.Skipped, Skip{Step: fmt.Sprintf("step %d", i), Reason: strings.Repeat("x", 60)})
	}
	for i := range 10 {
		cut.Background = append(cut.Background, Row{Service: fmt.Sprintf("svc%d", i), Signal: SignalErrors, Rung: time.Hour, Window: 0.1, Baseline: 0.1, Unit: "ratio"})
	}
	for _, budget := range []int{1 << 20, 4096, 2048, 1500, 1200, 300} {
		add(fmt.Sprintf("cut to %d bytes", budget), cut, budget)
	}
	for _, c := range []struct {
		name    string
		skipped []Skip
	}{
		{"a skip with no signal", []Skip{{Step: "service map", Reason: "the read failed"}}},
		{"errors skipped", []Skip{{Step: "metric rates over 1h", Reason: "the read failed", Signals: []string{SignalErrors}}}},
		{"skipped signals deduplicated and sorted", []Skip{{Step: "metric rates over 1h", Signals: []string{SignalLatency}}, {Step: "metric rates over 6h", Signals: []string{SignalLatency, SignalErrors}}}},
	} {
		r := sampleReport()
		r.Candidates, r.Background, r.Skipped = nil, nil, c.skipped
		add(c.name, r, MaxBlockBytes)
	}
	gap := sampleReport()
	gap.Skipped = []Skip{{Step: "metric rates over 1h", Source: "prom", Reason: "the read failed", Signals: []string{SignalErrors}}}
	gap.Skipped = append(gap.Skipped, cut.Skipped[1:]...)
	gap.Background = cut.Background
	for _, budget := range []int{3000, 2000, 1500} {
		add(fmt.Sprintf("a skipped error read kept through a cut to %d bytes", budget), gap, budget)
	}
	odd := sampleReport()
	odd.Question = &Window{From: t0.Add(-2 * time.Hour), To: t0.Add(-30 * time.Minute)}
	odd.Duration = 1250 * time.Millisecond
	odd.Services = append(odd.Services, ServiceSource{Source: "tempo", Type: "tempo", Signal: "traces"}, ServiceSource{Source: "loki", Type: "loki", Signal: "logs", Field: "service_name", Services: strings.Fields("a b c d e f g h i j k l m n")})
	odd.Rates = []Row{
		{Service: "cart", Signal: SignalRequests, Rung: time.Hour, Window: 1234.5, Baseline: 0.000123, Unit: "/s", Source: "prom"},
		{Service: "cart", Signal: SignalErrors, Rung: time.Hour, Window: 0.0001, Baseline: 0, Unit: "ratio", Source: "prom"},
		{Service: "cart", Signal: SignalLatency, Rung: time.Hour, Window: 12.345, Baseline: 1e7, Unit: "ms", Source: "prom"},
		{Service: "ad", Signal: SignalRequests, Rung: time.Hour, Window: 2, Baseline: 2, Unit: "spans/s", Source: "tempo"},
	}
	odd.Background = append(odd.Background, Row{Service: "quote", Signal: SignalErrors, Rung: 90 * time.Second, Window: 3, Baseline: 2.5, Unit: "error lines/s", Source: "loki"})
	c := &odd.Candidates[0]
	c.Origins = append(c.Origins, Origin{Source: "tempo", Service: "payment", Operation: "  Charge\n card  " + strings.Repeat("é", 70), Traces: 3, Example: "00f067aa0ba902b7"})
	c.Events = append(c.Events, Event{Kind: EventAnnotation, At: t0.Add(-25 * time.Minute), Text: "[deploy] payment v2"}, Event{Kind: EventDeployLine, Source: "loki", Service: "payment", At: t0.Add(-21 * time.Minute)})
	c.Patterns = append(c.Patterns, Pattern{Source: "loki", Service: "payment", Text: "retry \"<_>\" \t now", Window: 40, Baseline: 12})
	c.Rows = append(c.Rows, Row{Service: "payment", Signal: SignalLatency, Rung: 6 * time.Hour, Window: 2500, Baseline: 90, Unit: "ms", Source: "prom", Basis: "span metrics", Moved: true})
	odd.Candidates = append(odd.Candidates, Candidate{ID: "c2", Service: "ad", Rows: []Row{{Service: "ad", Signal: SignalErrors, Rung: 24 * time.Hour, Window: 0.5, Baseline: 0.01, Unit: "ratio", Source: "loki", Basis: "log levels", Moved: true}}})
	add("a question window, odd durations and long text", odd, MaxBlockBytes)
	for _, ms := range []int64{0, 49, 50, 1249, 61250, 3723400} {
		r := none
		r.Duration = time.Duration(ms) * time.Millisecond
		add(fmt.Sprintf("a sweep of %d ms", ms), r, MaxBlockBytes)
	}
	return cases
}
