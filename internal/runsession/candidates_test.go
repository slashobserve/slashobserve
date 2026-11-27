package runsession

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/brief"
	"github.com/slashobserve/slashobserve/internal/agent/guard"
	"github.com/slashobserve/slashobserve/internal/agent/route"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/agent/triage"
	"github.com/slashobserve/slashobserve/internal/findings"
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

func closedAs(id, closing string, evidence ...string) revisions.Hypothesis {
	return revisions.Hypothesis{ID: id, Text: "t", Status: revisions.ClosingStatus(closing), Closing: closing, Evidence: evidence}
}

func TestTheConcludeRuleNamesWhatIsUnaccountedFor(t *testing.T) {
	cause := closedAs("c1", revisions.ClosingCause, "payment 5xx 25%")
	transient := closedAs("c2", revisions.ClosingTransient, "ad p95 2.5 s at 09:50", "ad p95 90 ms at 10:10")
	cases := []struct {
		name      string
		ledger    []revisions.Hypothesis
		reads     map[string]int
		finding   string
		want      []string
		wantsOpen []string
	}{
		{"every candidate closed and the cause named", []revisions.Hypothesis{cause, transient}, map[string]int{"c2": 2}, `{"causes":[{"component":{"name":"payment"}}]}`, nil, nil},
		{"a candidate still open", []revisions.Hypothesis{cause, {ID: "c2", Text: "t", Status: "open", Next: "read ad latency"}}, nil, `{"causes":[{"component":{"name":"payment"}}]}`, []string{"c2 (ad) is still open; next: read ad latency"}, []string{"c2"}},
		{"a candidate dropped from the ledger", []revisions.Hypothesis{cause}, nil, `{"causes":[{"component":{"name":"payment"}}]}`, []string{"c2 (ad) is missing from the ledger"}, []string{"c2"}},
		{"a candidate with no closing", []revisions.Hypothesis{cause, {ID: "c2", Text: "t", Status: "ruled_out", Evidence: []string{"e"}}}, nil, `{"causes":[{"component":{"name":"payment"}}]}`, []string{"c2 (ad) is ruled_out with no closing"}, []string{"c2"}},
		{"transient with no later read", []revisions.Hypothesis{cause, transient}, map[string]int{"c2": 1}, `{"causes":[{"component":{"name":"payment"}}]}`, []string{"c2 (ad) is closed as transient, but no read after the first one shows it gone"}, []string{"c2"}},
		{"a cause the finding leaves out", []revisions.Hypothesis{cause, transient}, map[string]int{"c2": 2}, `{"causes":[{"component":{"name":"checkout"}}]}`, []string{"c1 (payment) is closed as cause, but causes does not name payment"}, nil},
		{"a cause closed as transient", []revisions.Hypothesis{cause, transient}, map[string]int{"c2": 2}, `{"causes":[{"component":{"name":"payment"}},{"component":{"name":"AD"}}]}`, []string{"causes names ad, which c2 closed as transient"}, nil},
		{"a legacy root cause", []revisions.Hypothesis{cause, transient}, map[string]int{"c2": 2}, `{"root_cause":{"component":"payment"}}`, nil, nil},
		{"no incident over excluded candidates", []revisions.Hypothesis{closedAs("c1", revisions.ClosingBackground, "e"), closedAs("c2", revisions.ClosingUnrelated, "e")}, nil, `{"no_incident":true}`, nil, nil},
		{"no incident over a cause", []revisions.Hypothesis{cause, closedAs("c2", revisions.ClosingUnrelated, "e")}, nil, `{"no_incident":true}`, []string{"no_incident is set, but c1 (payment) is closed as cause"}, nil},
		{"no incident over an open candidate", []revisions.Hypothesis{closedAs("c1", revisions.ClosingBackground, "e")}, nil, `{"no_incident":true}`, []string{"c2 (ad) is missing"}, []string{"c2"}},
	}
	for _, c := range cases {
		problems, open := candidateProblems(twoLeads, c.ledger, c.reads, concludedOf(json.RawMessage(c.finding)))
		if len(problems) != len(c.want) {
			t.Errorf("%s: problems = %q, want %q", c.name, problems, c.want)
			continue
		}
		for i, want := range c.want {
			if !strings.HasPrefix(problems[i], want) {
				t.Errorf("%s: problem %q, want %q", c.name, problems[i], want)
			}
		}
		var ids []string
		for _, l := range open {
			ids = append(ids, l.ID)
		}
		if strings.Join(ids, ",") != strings.Join(c.wantsOpen, ",") {
			t.Errorf("%s: open = %v, want %v", c.name, ids, c.wantsOpen)
		}
	}
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
