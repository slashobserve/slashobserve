package runsession

import (
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/brief"
	"github.com/slashobserve/slashobserve/internal/agent/guard"
	"github.com/slashobserve/slashobserve/internal/agent/route"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/agent/triage"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

var twoLeads = []triage.Lead{
	{ID: "c1", Service: "payment", Text: "payment: errors 25% from 0.1% (1h); onset 2026-11-20T09:40:00Z", Next: "Read payment errors before and after the onset"},
	{ID: "c2", Service: "ad", Text: "ad: latency 2.5 s from 90 ms (1h); onset 2026-11-20T09:50:00Z", Next: "Read ad latency before and after the onset"},
}

func TestTheLedgerIsSeededWithOneOpenHypothesisPerCandidate(t *testing.T) {
	own := []revisions.Hypothesis{{ID: "h1", Text: "mine", Status: revisions.HypothesisOpen}}
	cases := []struct {
		name   string
		loop   bool
		task   route.Task
		triage *Triage
		ledger []revisions.Hypothesis
		want   []string
	}{
		{"an incident with candidates", true, route.Incident, &Triage{Leads: twoLeads}, nil, []string{"c1", "c2"}},
		{"a run with no type", true, route.Full, &Triage{Leads: twoLeads}, nil, []string{"c1", "c2"}},
		{"the loop off", false, route.Incident, &Triage{Leads: twoLeads}, nil, nil},
		{"a measure question", true, route.Measure, &Triage{Leads: twoLeads}, nil, nil},
		{"no candidates", true, route.Incident, &Triage{}, nil, nil},
		{"no sweep", true, route.Incident, nil, nil, nil},
		{"a ledger already kept", true, route.Incident, &Triage{Leads: twoLeads}, own, []string{"h1"}},
	}
	for _, c := range cases {
		s := State{Guard: guard.New(tools.ModeInvestigate), Loop: c.loop, Task: c.task, Triage: c.triage, Ledger: c.ledger}
		s.seedLedger()
		var ids []string
		for _, h := range s.Ledger {
			ids = append(ids, h.ID)
		}
		if strings.Join(ids, ",") != strings.Join(c.want, ",") {
			t.Errorf("%s: ledger = %v, want %v", c.name, ids, c.want)
		}
	}
	s := State{Guard: guard.New(tools.ModeInvestigate), Loop: true, Task: route.Incident, Triage: &Triage{Leads: twoLeads}}
	s.seedLedger()
	if h := s.Ledger[1]; h.Status != revisions.HypothesisOpen || h.Text != twoLeads[1].Text || h.Next != twoLeads[1].Next || len(h.Evidence) != 0 {
		t.Errorf("seeded hypothesis = %+v", h)
	}
}

func TestTheFirstBriefShowsTheSeededLedger(t *testing.T) {
	s := newScript(t, true)
	s.sess.state.Task = route.Incident
	s.sess.state.Triage = &Triage{Block: "<triage>\nc1 payment\n</triage>", Leads: twoLeads}
	s.sess.state.seedLedger()
	got := s.brief(brief.Fresh)
	want := "<hypotheses>\nYour ledger so far; send the whole ledger with note_hypotheses when it changes, and do not repeat a read that ruled one out.\n" +
		"- c1 [open] " + twoLeads[0].Text + "\n  next: " + twoLeads[0].Next + "\n" +
		"- c2 [open] " + twoLeads[1].Text + "\n  next: " + twoLeads[1].Next + "\n</hypotheses>"
	if !strings.Contains(got.Message, want) {
		t.Fatalf("the first brief lacks the seeded ledger:\n%s", got.Message)
	}
}
