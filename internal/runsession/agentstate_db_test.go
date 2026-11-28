package runsession

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
)

func compact(raw json.RawMessage) string {
	var b bytes.Buffer
	if json.Compact(&b, raw) != nil {
		return string(raw)
	}
	return b.String()
}

func TestTheAgentStateIsKeptWithTheSessionAndOnlyItsCarryReachesTheNextAttempt(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	state := &tools.AgentState{Session: json.RawMessage(`{"turn":3}`), Carry: json.RawMessage(`{"leads":["c1"]}`)}
	if err := r.svc.NoteAgent(context.Background(), req, tools.AgentReport{State: state}); err != nil {
		t.Fatal(err)
	}
	got, err := r.svc.Brief(context.Background(), req, "")
	if err != nil {
		t.Fatal(err)
	}
	if got.AgentState == nil || compact(got.AgentState.Session) != `{"turn":3}` || compact(got.AgentState.Carry) != `{"leads":["c1"]}` {
		t.Fatalf("agent state = %+v", got.AgentState)
	}

	id := mustUUID(t, req.ID)
	r.d.Exec(t, `UPDATE agent_requests SET attempts = attempts + 1, claim_epoch = claim_epoch + 1 WHERE id = $1`, id)
	req.Attempt++
	req.ClaimEpoch++
	next, err := r.svc.Brief(context.Background(), req, "")
	if err != nil {
		t.Fatal(err)
	}
	if next.AgentState == nil || next.AgentState.Session != nil || compact(next.AgentState.Carry) != `{"leads":["c1"]}` {
		t.Fatalf("next attempt's agent state = %+v", next.AgentState)
	}

	cleared := &tools.AgentState{Session: json.RawMessage(`null`)}
	if err := r.svc.NoteAgent(context.Background(), req, tools.AgentReport{State: cleared}); err != nil {
		t.Fatal(err)
	}
	if again, err := r.svc.Brief(context.Background(), req, ""); err != nil || again.AgentState != nil {
		t.Fatalf("agent state after a null report = %+v, %v", again.AgentState, err)
	}
}

func TestReportedStepsAreRecordedCleanedLikeTheServersOwn(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	steps := []tools.ReportedStep{
		{Phase: tools.PhaseQuery, Tool: "triage", Status: "ok", Message: "  Swept 4 sources\x00 "},
		{Phase: tools.PhaseSave, Status: "error", Message: "Sent the finding back"},
		{Phase: tools.PhaseQuery, Status: "ok", Message: "   "},
	}
	if err := r.svc.NoteAgent(context.Background(), req, tools.AgentReport{Steps: steps}); err != nil {
		t.Fatal(err)
	}
	got := r.steps.all()
	if len(got) != 2 {
		t.Fatalf("steps = %+v", got)
	}
	if got[0].Tool != "triage" || got[0].Message != "Swept 4 sources" || got[0].Phase != tools.PhaseQuery || got[0].Status != "ok" {
		t.Fatalf("first step = %+v", got[0])
	}
	if got[1].Tool != stopTool || got[1].Status != "error" {
		t.Fatalf("second step = %+v", got[1])
	}
}

func TestNoteAgentOfAnEndedRunAnswersRunEnded(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	if _, err := r.svc.Finish(context.Background(), req, tools.RunFinish{Outcome: outcomeFailed, Message: "x"}); err != nil {
		t.Fatal(err)
	}
	err := r.svc.NoteAgent(context.Background(), req, tools.AgentReport{State: &tools.AgentState{Carry: json.RawMessage(`1`)}})
	if !errors.Is(err, tools.ErrRunEnded) {
		t.Fatalf("err = %v, want tools.ErrRunEnded", err)
	}
}
