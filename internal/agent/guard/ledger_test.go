package guard

import (
	"strconv"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const toolNoteHypotheses = "note_hypotheses"

func TestALedgerCallIsAdmittedPastTheLimitWhileAReadIsRefused(t *testing.T) {
	s := New(tools.ModeInvestigate)
	s.InvestigationCalls = 6
	s.Window.Calls = 6
	s.BeginTurn(false)
	ledger := s.Admit(0, jsonCall(toolNoteHypotheses, `{"hypotheses":[]}`))
	read := s.Admit(1, jsonCall("list_metrics", `{}`))
	if ledger.Verdict != Dispatch || ledger.Phase != tools.PhaseLedger {
		t.Fatalf("ledger = %+v", ledger)
	}
	if read.Verdict != Refuse {
		t.Fatalf("read = %+v", read)
	}
}

func TestALedgerCallIsNeitherAReadNorAnAnsweredCall(t *testing.T) {
	s := New(tools.ModeInvestigate)
	s.BeginTurn(false)
	call := jsonCall(toolNoteHypotheses, `{"hypotheses":[]}`)
	adm := s.Admit(0, call)
	if adm.Verdict != Dispatch || s.Turn.ReadsInFlight != 0 || s.Turn.Answered != 0 || len(s.Reads) != 0 {
		t.Fatalf("admission %+v, turn %+v, reads %v", adm, s.Turn, s.Reads)
	}
	read := jsonCall("list_metrics", `{}`)
	s.Admit(1, read)
	s.EndRead(call)
	if s.Turn.ReadsInFlight != 1 {
		t.Fatalf("ending the ledger call released a read: in flight %d", s.Turn.ReadsInFlight)
	}
	if settled, err := s.Settle(call, tools.Result{}, []byte(`{}`)); err != nil || settled.Note != "" || settled.Stop != nil || settled.Replayable {
		t.Fatalf("settled %+v, err %v", settled, err)
	}
}

func TestAnyNumberOfIdenticalLedgerCallsIsAdmitted(t *testing.T) {
	s := New(tools.ModeInvestigate)
	for i := range 5 {
		s.BeginTurn(false)
		if adm := s.Admit(0, jsonCall(toolNoteHypotheses, `{"hypotheses":[]}`)); adm.Verdict != Dispatch {
			t.Fatalf("call %d = %+v", i+1, adm)
		}
	}
}

func TestTurnsOfOnlyLedgerCallsStallTheRun(t *testing.T) {
	s := New(tools.ModeInvestigate)
	var stop *Stop
	for range maxStalledTurns {
		s.BeginTurn(false)
		s.Admit(0, jsonCall(toolNoteHypotheses, `{"hypotheses":[]}`))
		_, stop = s.EndTurn()
	}
	if stop == nil || stop.Failure != revisions.FailureNoProgress {
		t.Fatalf("stop = %+v", stop)
	}
	if want := "Stopped: 2 turns in a row only updated the hypotheses"; stop.Step != want {
		t.Fatalf("step = %q, want %q", stop.Step, want)
	}
}

func TestALedgerTurnFollowedByARefusedTurnStallsWithTheRefusalReason(t *testing.T) {
	s := New(tools.ModeInvestigate)
	s.BeginTurn(false)
	s.Admit(0, jsonCall(toolNoteHypotheses, `{"hypotheses":[]}`))
	s.EndTurn()
	s.BeginTurn(false)
	_, stop := s.EndTurn()
	if stop == nil {
		t.Fatal("the run was not stopped")
	}
	if want := "Stopped: 2 turns in a row had every call refused"; stop.Step != want {
		t.Fatalf("step = %q, want %q", stop.Step, want)
	}
}

func TestALedgerCallBesideAReadDoesNotStallTheTurn(t *testing.T) {
	s := New(tools.ModeInvestigate)
	s.BeginTurn(false)
	s.Admit(0, jsonCall(toolNoteHypotheses, `{"hypotheses":[]}`))
	s.Admit(1, jsonCall("list_metrics", `{}`))
	if _, stop := s.EndTurn(); stop != nil || s.Window.Stalled != 0 {
		t.Fatalf("stop %+v, stalled %d", stop, s.Window.Stalled)
	}
}

const (
	findingWithCause   = `{"markdown":"Pool exhausted.","root_cause":{"component":"checkout","kind":"resource","summary":"The pool is full."}}`
	findingRuledOut    = `{"markdown":"Not the deploy.","ruled_out":["deploy"]}`
	findingPlain       = `{"markdown":"Nothing found."}`
	openLedger         = `{"hypotheses":[{"id":"h1","text":"pool","status":"open"}]}`
	ledgerBehindPrefix = "the finding names a cause or ruled-out items"
)

func ledgerOf(statuses ...string) tools.Result {
	var out []revisions.Hypothesis
	for i, status := range statuses {
		h := revisions.Hypothesis{ID: "h" + strconv.Itoa(i), Text: "cause", Status: status}
		if status != revisions.HypothesisOpen {
			h.Evidence = []string{"read it"}
		}
		out = append(out, h)
	}
	return tools.Result{Ledger: out}
}

func watched() State {
	s := New(tools.ModeInvestigate)
	s.WatchLedger()
	return s
}

func noteLedger(t *testing.T, s *State, res tools.Result) {
	t.Helper()
	s.BeginTurn(false)
	c := jsonCall(toolNoteHypotheses, openLedger)
	s.Admit(0, c)
	if _, err := s.Settle(c, res, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	s.EndTurn()
}

func saveTurn(s *State, input string) Admission {
	s.BeginTurn(false)
	return s.Admit(0, jsonCall(toolSaveFinding, input))
}

func TestAFindingThatNamesACauseIsSentBackOnceWhileEveryHypothesisIsOpen(t *testing.T) {
	for _, input := range []string{findingWithCause, findingRuledOut} {
		s := watched()
		noteLedger(t, &s, ledgerOf(revisions.HypothesisOpen, revisions.HypothesisOpen))
		first := saveTurn(&s, input)
		if first.Verdict != Refuse || !strings.Contains(string(first.Content), ledgerBehindPrefix) || !strings.Contains(string(first.Content), "note_hypotheses") {
			t.Fatalf("first save = %+v %s", first, first.Content)
		}
		if s.Counts.Nudges != 1 {
			t.Errorf("nudges = %d", s.Counts.Nudges)
		}
		if second := saveTurn(&s, input); second.Verdict != Dispatch {
			t.Fatalf("second save = %+v %s", second, second.Content)
		}
		if s.Counts.Nudges != 1 {
			t.Errorf("nudges after the second save = %d", s.Counts.Nudges)
		}
	}
}

func TestAFindingWithNoLedgerAtAllIsAlsoSentBackOnce(t *testing.T) {
	s := watched()
	if first := saveTurn(&s, findingWithCause); first.Verdict != Refuse {
		t.Fatalf("first save = %+v", first)
	}
	if second := saveTurn(&s, findingWithCause); second.Verdict != Dispatch {
		t.Fatalf("second save = %+v", second)
	}
}

func TestAFindingIsNotSentBackOnceTheLedgerHasMoved(t *testing.T) {
	s := watched()
	noteLedger(t, &s, ledgerOf(revisions.HypothesisOpen))
	noteLedger(t, &s, ledgerOf(revisions.HypothesisSupport))
	if adm := saveTurn(&s, findingWithCause); adm.Verdict != Dispatch {
		t.Fatalf("save = %+v %s", adm, adm.Content)
	}
	if s.Counts.Nudges != 0 {
		t.Errorf("nudges = %d", s.Counts.Nudges)
	}
}

func TestAFindingThatNamesNoCauseOrRuledOutItemIsNotSentBack(t *testing.T) {
	for _, input := range []string{findingPlain, `{"markdown":"x","root_cause":null,"ruled_out":[]}`} {
		s := watched()
		noteLedger(t, &s, ledgerOf(revisions.HypothesisOpen))
		if adm := saveTurn(&s, input); adm.Verdict != Dispatch {
			t.Fatalf("save %s = %+v %s", input, adm, adm.Content)
		}
	}
}

func TestARejectedLedgerCallDoesNotCountAsAnUpdate(t *testing.T) {
	s := watched()
	noteLedger(t, &s, tools.Result{IsError: true, Ledger: ledgerOf(revisions.HypothesisSupport).Ledger})
	if adm := saveTurn(&s, findingWithCause); adm.Verdict != Refuse {
		t.Fatalf("save = %+v", adm)
	}
}

func TestWithTheLoopOffAFindingIsNeverSentBackForTheLedger(t *testing.T) {
	s := New(tools.ModeInvestigate)
	if adm := saveTurn(&s, findingWithCause); adm.Verdict != Dispatch {
		t.Fatalf("save = %+v %s", adm, adm.Content)
	}
	if s.Ledger != nil || s.Counts.Nudges != 0 {
		t.Fatalf("ledger %+v, nudges %d", s.Ledger, s.Counts.Nudges)
	}
}

func TestTheLedgerWatchOnlyStartsInAnInvestigation(t *testing.T) {
	s := New(tools.ModeCreate)
	s.WatchLedger()
	if s.Ledger != nil {
		t.Fatal("a build run watches the ledger")
	}
}
