package guard

import (
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
)

func ungroundedSave(input string) Call {
	c := jsonCall(toolSaveFinding, input)
	c.Ungrounded = "send back: read it, correct it, or remove it"
	return c
}

func TestAnUngroundedFindingIsSentBackOnceEvenPastTheLimitAndNeverCountsAsARejection(t *testing.T) {
	s := New(tools.ModeInvestigate)
	s.WatchGrounding()
	s.InvestigationCalls = 6
	s.Window.Calls = 6
	s.Rejections = MaxRejectionsInRow - 1

	s.BeginTurn(true)
	first := s.Admit(0, ungroundedSave(`{"markdown":"523 ms"}`))
	if first.Verdict != Refuse || !strings.Contains(string(first.Content), "read it, correct it, or remove it") {
		t.Fatalf("first save = %+v", first)
	}
	if _, stop := s.EndTurn(); stop != nil {
		t.Fatalf("the send-back stopped the run: %+v", stop)
	}
	if s.Rejections != MaxRejectionsInRow-1 {
		t.Fatalf("rejections = %d, want them unchanged", s.Rejections)
	}

	s.BeginTurn(false)
	if read := s.Admit(0, jsonCall("list_metrics", `{"q":"recheck"}`)); read.Verdict == Refuse {
		t.Fatalf("the read after a send-back past the limit was refused: %s", read.Content)
	}
	s.EndTurn()

	s.BeginTurn(true)
	if second := s.Admit(0, ungroundedSave(`{"markdown":"523 ms"}`)); second.Verdict != Dispatch {
		t.Fatalf("the second save = %+v, want it admitted as it is", second)
	}
}

func TestWithoutTheWatchAnUngroundedFindingIsAdmitted(t *testing.T) {
	s := New(tools.ModeInvestigate)
	s.BeginTurn(true)
	if adm := s.Admit(0, ungroundedSave(`{"markdown":"523 ms"}`)); adm.Verdict != Dispatch {
		t.Fatalf("save = %+v", adm)
	}
	if s.Grounding != nil {
		t.Fatal("the grounding watch was started")
	}
}
