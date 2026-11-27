package triage

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func sampleReport() Report {
	onset := t0.Add(-20 * time.Minute)
	return Report{
		End:      t0,
		Services: []ServiceSource{{Source: "prom", Type: "prometheus", Signal: "metrics", Field: "service_name", Services: []string{"cart", "checkout", "payment"}}},
		Signals:  []string{"requests, errors and p95 latency on prom from span metrics traces_span_metrics_calls_total"},
		Candidates: []Candidate{{
			ID: "c1", Service: "payment", Onset: &onset,
			Rows:     []Row{{Service: "payment", Signal: SignalErrors, Rung: time.Hour, Window: 0.25, Baseline: 0.001, Unit: "ratio", Source: "prom", Basis: "span metrics", Moved: true, Onset: &onset}},
			KnockOn:  []string{"checkout"},
			Origins:  []Origin{{Source: "tempo", Service: "payment", Operation: "Charge", Message: "invalid token", Traces: 41, Example: "4bf92f3577b34da6a3ce929d0e0e4736", RelayedBy: []string{"checkout"}}},
			Events:   []Event{{Kind: EventFlag, Source: "loki", At: onset.Add(-time.Minute), Text: "paymentFailure variant 50% first seen"}},
			Patterns: []Pattern{{Source: "loki", Service: "payment", Level: "error", Text: "charge failed <_>", Window: 120}},
		}},
		Background: []Row{{Service: "cart", Signal: SignalErrors, Rung: time.Hour, Window: 0.02, Baseline: 0.019, Unit: "ratio", Source: "prom"}},
		Skipped:    []Skip{{Step: "trace rates", Source: "jaeger", Reason: "prom span metrics cover them"}},
		Calls:      17,
		Duration:   3200 * time.Millisecond,
	}
}

func TestRenderStatesTheWindowsCandidatesAndThatItIsNotEvidence(t *testing.T) {
	got := Render(sampleReport(), MaxBlockBytes)
	for _, want := range []string{
		"<triage>\n",
		"a starting point, not evidence",
		"every figure your answer states needs its own read",
		"Windows: 1h, 6h, 24h, each ending 2026-11-20T10:00:00Z",
		"Services on prom (prometheus metrics, in service_name): cart, checkout, payment",
		"c1 payment: errors 25% from 0.1% (1h, prom span metrics); onset 2026-11-20T09:40:00Z; knock-on, relaying its errors: checkout",
		`origin (tempo): payment Charge "invalid token", 41 failing traces, example 4bf92f3577b34da6a3ce929d0e0e4736`,
		"change (loki): flag at 2026-11-20T09:39:00Z: paymentFailure variant 50% first seen",
		`log pattern (loki, error) new, 120 lines: "charge failed <_>"`,
		"Background, not a candidate: cart errors 2%, 1.9% before it",
		"Skipped: trace rates on jaeger: prom span metrics cover them.",
		"Sweep: 17 datasource calls in 3.2s.",
		"\n</triage>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("block lacks %q:\n%s", want, got)
		}
	}
}

func TestRenderWithNothingMovedSaysSo(t *testing.T) {
	r := sampleReport()
	r.Candidates, r.Background = nil, nil
	if got := Render(r, MaxBlockBytes); !strings.Contains(got, "Candidates: none.") {
		t.Fatalf("block lacks the no-candidate line:\n%s", got)
	}
}

func TestRenderCutsLowestPriorityFirstWithinTheBudget(t *testing.T) {
	r := sampleReport()
	for i := range 40 {
		r.Skipped = append(r.Skipped, Skip{Step: fmt.Sprintf("step %d", i), Reason: strings.Repeat("x", 60)})
	}
	for i := range 10 {
		r.Background = append(r.Background, Row{Service: fmt.Sprintf("svc%d", i), Signal: SignalErrors, Rung: time.Hour, Window: 0.1, Baseline: 0.1, Unit: "ratio"})
	}
	full := Render(r, 1<<20)
	for _, budget := range []int{4096, 2048, 1500, 1200} {
		got := Render(r, budget)
		if len(got) > budget {
			t.Fatalf("budget %d: block is %d bytes", budget, len(got))
		}
		if !strings.HasPrefix(got, "<triage>\n") || !strings.HasSuffix(got, "\n</triage>") || !strings.Contains(got, "Sweep: 17") {
			t.Fatalf("budget %d: kept lines are missing:\n%s", budget, got)
		}
		if !strings.Contains(got, "Cut for size:") {
			t.Fatalf("budget %d: no cut note", budget)
		}
		if got != Render(r, budget) {
			t.Fatalf("budget %d: render is not deterministic", budget)
		}
	}
	got := Render(r, 4096)
	if strings.Contains(got, "Skipped: step 39") || !strings.Contains(got, "c1 payment") {
		t.Fatalf("skipped steps must go before candidates:\n%s", got)
	}
	if strings.Contains(Render(r, 1500), "Skipped:") && strings.Contains(Render(r, 1500), "Background") {
		t.Fatal("skipped lines stay while background lines are cut first")
	}
	if !strings.Contains(full, "Skipped: step 39") || strings.Contains(full, "Cut for size") {
		t.Fatal("an unlimited budget cuts nothing")
	}
}

func TestClipTextKeepsRunesWhole(t *testing.T) {
	got := clipText(strings.Repeat("é", 100))
	if !strings.HasSuffix(got, "...") || len(got) > maxTextBytes+3 {
		t.Fatalf("clip = %q", got)
	}
	for _, r := range strings.TrimSuffix(got, "...") {
		if r != 'é' {
			t.Fatalf("clip split a rune: %q", got)
		}
	}
}
