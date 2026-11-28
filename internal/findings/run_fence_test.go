package findings_test

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/findings"
)

func (r rig) holdRun(t *testing.T, run pgtype.UUID, epoch int) {
	t.Helper()
	r.db.Exec(t, `UPDATE agent_requests SET state = 'running', status = 'running', attempts = 1, claim_epoch = $2,
  lease_until = clock_timestamp() + interval '1 minute' WHERE id = $1`, run, epoch)
}

func TestAnAnswerSavedByTheRunsHolderMarksTheRunCommitted(t *testing.T) {
	r := newRig(t)
	run := r.request(t, editor, r.open)
	r.holdRun(t, run, 1)

	r.save(t, editor, findings.SaveInput{AgentRequestID: run, DashboardID: r.open, ClaimEpoch: 1})
	if n := r.count(t, "SELECT count(*) FROM agent_requests WHERE id = $1 AND committed_kind = 'finding'", run.String()); n != 1 {
		t.Fatal("the run that saved an answer is not marked committed, so a sweep waiting on it would miss the answer")
	}
}

func TestAnAnswerOfARunItsHolderLostIsNotSaved(t *testing.T) {
	r := newRig(t)
	run := r.request(t, editor, r.open)
	r.holdRun(t, run, 2)

	err := r.saveErr(t, editor, findings.SaveInput{AgentRequestID: run, DashboardID: r.open, ClaimEpoch: 1})
	if !errors.Is(err, findings.ErrRunLost) {
		t.Fatalf("save by an earlier claim = %v, want ErrRunLost", err)
	}
	if n := r.count(t, "SELECT count(*) FROM findings WHERE agent_request_id = $1", run.String()); n != 0 {
		t.Fatalf("%d answers saved for a claim that no longer holds the run, want none", n)
	}
}
