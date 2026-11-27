package triage

import (
	"slices"
	"testing"
	"time"
)

func at(minutes int) *time.Time {
	t := t0.Add(time.Duration(minutes) * time.Minute)
	return &t
}

func moved(service, signal string, onset *time.Time) Row {
	return Row{Service: service, Signal: signal, Rung: time.Hour, Window: 0.2, Baseline: 0.01, Unit: "ratio", Source: "prom", Moved: true, Onset: onset}
}

func ids(cs []Candidate) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.ID + ":" + c.Service
	}
	return out
}

func TestCluster(t *testing.T) {
	for _, tc := range []struct {
		name       string
		findings   Findings
		want       []string
		knockOn    map[string][]string
		background []string
	}{
		{
			name: "nothing moved gives no candidate",
			findings: Findings{Rows: []Row{
				{Service: "cart", Signal: SignalErrors, Rung: time.Hour, Unit: "ratio", Source: "prom"},
				{Service: "cart", Signal: SignalLatency, Rung: time.Hour, Window: 20, Baseline: 19, Unit: "ms", Source: "prom"},
			}},
		},
		{
			name: "two independent faults with different onsets give two candidates in onset order",
			findings: Findings{Rows: []Row{
				moved("payment", SignalErrors, at(40)),
				moved("ad", SignalLatency, at(10)),
			}},
			want: []string{"c1:ad", "c2:payment"},
		},
		{
			name: "a downstream service that relays an upstream fault and starts later is its knock-on",
			findings: Findings{
				Rows:    []Row{moved("frontend", SignalErrors, at(12)), moved("payment", SignalErrors, at(10))},
				Origins: []Origin{{Source: "tempo", Service: "payment", Operation: "Charge", Traces: 40, RelayedBy: []string{"checkout", "frontend"}}},
			},
			want:    []string{"c1:payment"},
			knockOn: map[string][]string{"c1": {"frontend"}},
		},
		{
			name: "a service whose errors start before the one it relays stays its own candidate",
			findings: Findings{
				Rows:    []Row{moved("frontend", SignalErrors, at(5)), moved("payment", SignalErrors, at(10))},
				Origins: []Origin{{Service: "payment", RelayedBy: []string{"frontend"}}},
			},
			want: []string{"c1:frontend", "c2:payment"},
		},
		{
			name: "a knock-on of a knock-on joins the root candidate",
			findings: Findings{
				Rows: []Row{moved("frontend", SignalErrors, at(14)), moved("checkout", SignalErrors, at(12)), moved("payment", SignalErrors, at(10))},
				Origins: []Origin{
					{Service: "payment", RelayedBy: []string{"checkout"}},
					{Service: "checkout", RelayedBy: []string{"frontend"}},
				},
			},
			want:    []string{"c1:payment"},
			knockOn: map[string][]string{"c1": {"checkout", "frontend"}},
		},
		{
			name: "errors at a similar level before the window are background, not a candidate",
			findings: Findings{Rows: []Row{
				{Service: "cart", Signal: SignalErrors, Rung: 6 * time.Hour, Window: 0.02, Baseline: 0.018, Unit: "ratio", Source: "prom"},
				{Service: "cart", Signal: SignalErrors, Rung: time.Hour, Window: 0.021, Baseline: 0.02, Unit: "ratio", Source: "prom"},
				moved("payment", SignalErrors, at(10)),
			}},
			want:       []string{"c1:payment"},
			background: []string{"cart"},
		},
		{
			name: "a moved request rate alone is not a candidate",
			findings: Findings{Rows: []Row{
				{Service: "cart", Signal: SignalRequests, Rung: time.Hour, Window: 10, Baseline: 2, Unit: "/s", Moved: true, Onset: at(3)},
			}},
		},
		{
			name: "a move without an onset comes after the timed ones",
			findings: Findings{Rows: []Row{
				moved("ad", SignalErrors, nil),
				moved("payment", SignalErrors, at(50)),
			}},
			want: []string{"c1:payment", "c2:ad"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, background := Cluster(tc.findings)
			if g := ids(got); !slices.Equal(g, tc.want) {
				t.Fatalf("candidates = %v, want %v", g, tc.want)
			}
			for _, c := range got {
				if want := tc.knockOn[c.ID]; !slices.Equal(c.KnockOn, want) {
					t.Errorf("%s knock-on = %v, want %v", c.ID, c.KnockOn, want)
				}
			}
			var bg []string
			for _, r := range background {
				bg = append(bg, r.Service)
			}
			if !slices.Equal(bg, tc.background) {
				t.Errorf("background = %v, want %v", bg, tc.background)
			}
		})
	}
}

func TestClusterKeepsTheFinestRungAndTheEarliestOnset(t *testing.T) {
	coarse := moved("payment", SignalErrors, at(5))
	coarse.Rung = 6 * time.Hour
	fine := moved("payment", SignalErrors, at(20))
	slow := moved("payment", SignalLatency, at(15))
	got, _ := Cluster(Findings{Rows: []Row{coarse, fine, slow}})
	if len(got) != 1 || len(got[0].Rows) != 2 {
		t.Fatalf("candidates = %+v", got)
	}
	if got[0].Rows[0].Rung != time.Hour {
		t.Errorf("errors row rung = %s, want the 1h row", got[0].Rows[0].Rung)
	}
	if !got[0].Onset.Equal(*at(15)) {
		t.Errorf("onset = %s, want the earliest of the kept rows", got[0].Onset)
	}
}

func TestClusterAttachesChangeEventsBeforeTheOnset(t *testing.T) {
	events := []Event{
		{Kind: EventFlag, At: t0.Add(-5 * time.Minute), Text: "paymentFailure=50%"},
		{Kind: EventDeployLine, Service: "payment", At: t0.Add(-20 * time.Minute)},
		{Kind: EventDeployLine, Service: "cart", At: t0.Add(-2 * time.Minute)},
		{Kind: EventAnnotation, At: t0.Add(-2 * time.Hour), Text: "too early"},
		{Kind: EventAnnotation, At: t0.Add(20 * time.Minute), Text: "after"},
	}
	got, _ := Cluster(Findings{Rows: []Row{moved("payment", SignalErrors, at(0))}, Events: events})
	var kinds []string
	for _, e := range got[0].Events {
		kinds = append(kinds, e.Kind+"@"+e.Service)
	}
	if want := []string{"flag@", "deploy lines@payment"}; !slices.Equal(kinds, want) {
		t.Fatalf("events = %v, want %v", kinds, want)
	}
}
