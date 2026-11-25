package guard

import (
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
