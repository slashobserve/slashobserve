package runsession

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/brief"
	"github.com/slashobserve/slashobserve/internal/agent/route"
	"github.com/slashobserve/slashobserve/internal/agent/triage"
	"github.com/slashobserve/slashobserve/internal/findings"
)

var twoLeads = []triage.Lead{
	{ID: "c1", Service: "payment", Text: "payment: errors 25% from 0.1% (1h); onset 2026-11-20T09:40:00Z", Next: "Read payment errors before and after the onset"},
	{ID: "c2", Service: "ad", Text: "ad: latency 2.5 s from 90 ms (1h); onset 2026-11-20T09:50:00Z", Next: "Read ad latency before and after the onset"},
}

func incidentScript(t *testing.T, calls int) *script {
	t.Helper()
	s := newScript(t, true)
	s.sess.state.Task = route.Incident
	s.sess.state.Guard.InvestigationCalls = calls
	s.sess.state.Triage = &Triage{Leads: twoLeads}
	s.sess.state.seedLedger()
	return s
}

const (
	paymentFinding  = `{"title":"Payment failures","markdown":"Payment fails.","causes":[{"component":{"kind":"service","name":"payment"},"kind":"dependency","mechanism":"m"}]}`
	causeOnlyLedger = `{"hypotheses":[{"id":"c1","text":"payment errors rose","status":"supported","closing":"cause","evidence":["payment 5xx 25% from 0.1%"]},{"id":"c2","text":"ad latency rose","status":"open","next_check":"read ad latency again"}]}`
	bothLedger      = `{"hypotheses":[{"id":"c1","text":"payment errors rose","status":"supported","closing":"cause","evidence":["payment 5xx 25% from 0.1%"]},{"id":"c2","text":"ad latency rose","status":"ruled_out","closing":"transient","evidence":["ad p95 2.5 s at 09:50","ad p95 90 ms at 10:10"]}]}`
)

func candidateItems(a admitted) []string {
	var out []string
	for _, u := range a.grounding.unverified {
		if u.Kind == findings.UnverifiedCandidate {
			out = append(out, u.Text)
		}
	}
	return out
}

func TestAnIncidentRunConcludesOnlyWhenEveryCandidateIsClosed(t *testing.T) {
	s := incidentScript(t, 30)
	s.queryLatency(&Turn{ID: "t1", Calls: 1})
	s.note(causeOnlyLedger, &Turn{ID: "t2", Calls: 1})

	first := s.admitSave(paymentFinding, &Turn{ID: "t3", Calls: 1})
	if first.answer == nil || !first.answer.IsError {
		t.Fatal("a finding over an open candidate was admitted")
	}
	for _, want := range []string{"c2 (ad) is still open; next: read ad latency again", "transient or unrelated"} {
		if !strings.Contains(string(first.answer.Content), want) {
			t.Errorf("the send-back lacks %q: %s", want, first.answer.Content)
		}
	}

	s.queryLatency(&Turn{ID: "t4", Calls: 1})
	s.note(bothLedger, &Turn{ID: "t5", Calls: 1})
	second := s.admitSave(paymentFinding, &Turn{ID: "t6", Calls: 1})
	if second.answer != nil {
		t.Fatalf("the closed finding was sent back: %s", second.answer.Content)
	}
	if items := candidateItems(second); len(items) != 0 {
		t.Errorf("closed candidates were recorded as open: %v", items)
	}
	if got := concludedOf(json.RawMessage(paymentFinding)); len(got.names) != 1 || got.names[0] != "payment" {
		t.Errorf("stated causes = %v", got.names)
	}
}

func TestASecondSaveIsAcceptedWithTheOpenCandidatesRecorded(t *testing.T) {
	s := incidentScript(t, 30)
	s.queryLatency(&Turn{ID: "t1", Calls: 1})
	s.note(causeOnlyLedger, &Turn{ID: "t2", Calls: 1})
	if first := s.admitSave(paymentFinding, &Turn{ID: "t3", Calls: 1}); first.answer == nil {
		t.Fatal("the first save was admitted")
	}
	second := s.admitSave(paymentFinding, &Turn{ID: "t4", Calls: 1})
	if second.answer != nil {
		t.Fatalf("the second save was sent back: %s", second.answer.Content)
	}
	if items := candidateItems(second); strings.Join(items, ",") != "c2 ad" {
		t.Errorf("recorded = %v", items)
	}
}

func TestAtTheCallLimitAFindingWithAnOpenCandidateIsAcceptedAndRecordsIt(t *testing.T) {
	s := incidentScript(t, 3)
	s.queryLatency(&Turn{ID: "t1", Calls: 1})
	s.note(causeOnlyLedger, &Turn{ID: "t2", Calls: 1})
	got := s.admitSave(paymentFinding, &Turn{ID: "t3", Calls: 1})
	if got.answer != nil {
		t.Fatalf("a save at the call limit was sent back: %s", got.answer.Content)
	}
	if items := candidateItems(got); strings.Join(items, ",") != "c2 ad" {
		t.Errorf("recorded = %v", items)
	}
}

func TestOnlyAnIncidentRunWithCandidatesHasTheConcludeRule(t *testing.T) {
	for _, task := range []route.Task{route.Measure, route.Traces} {
		s := newScript(t, true)
		s.sess.state.Task = task
		s.sess.state.Triage = &Triage{Leads: twoLeads}
		s.queryLatency(&Turn{ID: "t1", Calls: 1})
		s.note(causeOnlyLedger, &Turn{ID: "t2", Calls: 1})
		if got := s.admitSave(paymentFinding, &Turn{ID: "t3", Calls: 1}); got.answer != nil || len(candidateItems(got)) != 0 {
			t.Errorf("%s: the conclude rule applied", task.Name())
		}
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
