package threads

import (
	"errors"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func (r *rig) sharedDashboardAnswer(t *testing.T) (pgtype.UUID, pgtype.UUID, Shared) {
	t.Helper()
	dash := r.createDashboard(t, "rps", "errors")
	id := dash.String()
	run := r.privateAnswer(t, revisions.StartRequest{DashboardID: &id}, "prom", "errors")
	shared, err := r.service.ShareRun(reading(t.Context()), testUser, ShareInput{RunID: run})
	if err != nil {
		t.Fatal(err)
	}
	return dash, run, shared
}

func TestTheAskerWithdrawsASharedAnswerBackToOnlyThem(t *testing.T) {
	r := newShareRig(t)
	_, run, shared := r.sharedDashboardAnswer(t)
	r.reply(t, testUser, shared.Thread.ID, "and the latency too")

	if _, err := r.findingOfRun(t, otherEditor, run); err != nil {
		t.Fatalf("a second viewer reads the shared answer before the withdraw: %v", err)
	}
	if err := r.service.Withdraw(reading(t.Context()), otherEditor, mustParse(t, shared.Thread.ID)); !errors.Is(err, ErrNothingShared) {
		t.Fatalf("a second viewer withdrew the asker's answer: %v", err)
	}

	if err := r.service.Withdraw(reading(t.Context()), testUser, mustParse(t, shared.Thread.ID)); err != nil {
		t.Fatal(err)
	}
	if n := r.messageCount(t); n != 0 {
		t.Fatalf("messages after the withdraw = %d, want 0", n)
	}
	stored, err := r.db.Queries().GetAgentRequestByID(reading(t.Context()), run)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ThreadMessageID.Valid || stored.ThreadRepliedAt.Valid || !revisions.PrivateAsk(stored) {
		t.Fatalf("the withdrawn run is %+v", stored)
	}
	if f, err := r.findingOfRun(t, testUser, run); err != nil || f.ObservationID != "" {
		t.Fatalf("the asker reads the answer after the withdraw: %+v, %v", f, err)
	}
	if _, err := r.findingOfRun(t, otherEditor, run); !errors.Is(err, findings.ErrNotFound) {
		t.Fatalf("a second viewer reads the answer after the withdraw: %v", err)
	}
	if _, err := r.service.ReadMessage(reading(t.Context()), otherEditor, mustParse(t, shared.Message.ID)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a second viewer reads the withdrawn thread: %v", err)
	}
	events := r.db.AuditEvents(t, "run.withdraw")
	if len(events) != 1 || events[0].TargetKind != "run" || events[0].TargetID != run.String() || events[0].ActorID != testUserID {
		t.Fatalf("run.withdraw events = %+v, want one for the run by its requester", events)
	}
	if err := r.service.Withdraw(reading(t.Context()), testUser, mustParse(t, shared.Thread.ID)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a second withdraw: %v", err)
	}

	again, err := r.service.ShareRun(reading(t.Context()), testUser, ShareInput{RunID: run})
	if err != nil || again.Replayed || again.Thread.ID == shared.Thread.ID {
		t.Fatalf("share after the withdraw = %+v, %v", again, err)
	}
	if replies := r.runReplies(t, again.Thread.ID); len(replies) != 1 {
		t.Fatalf("replies after the share again = %+v, want one", replies)
	}
}

func TestAWithdrawIsRefusedOnceAnotherPersonReplied(t *testing.T) {
	r := newShareRig(t)
	_, run, shared := r.sharedDashboardAnswer(t)
	r.reply(t, otherEditor, shared.Thread.ID, "seen it here too")

	if err := r.service.Withdraw(reading(t.Context()), testUser, mustParse(t, shared.Thread.ID)); !errors.Is(err, ErrOthersWroteHere) {
		t.Fatalf("withdraw after another person replied: %v", err)
	}
	stored, err := r.db.Queries().GetAgentRequestByID(reading(t.Context()), run)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ThreadMessageID.String() != shared.Message.ID {
		t.Fatalf("the refused withdraw changed the run: %+v", stored)
	}
	if n := r.messageCount(t); n != 3 {
		t.Fatalf("messages after a refused withdraw = %d, want 3", n)
	}
	if events := r.db.AuditEvents(t, "run.withdraw"); len(events) != 0 {
		t.Fatalf("run.withdraw events after a refusal = %+v", events)
	}
	r.call(t, testUser, http.MethodDelete, "/threads/"+shared.Thread.ID+"/share", nil).expect(t, http.StatusConflict, CodeOthersWroteHere)
}

func TestSomeoneWhoCannotViewTheThreadCannotWithdrawIt(t *testing.T) {
	r := newShareRig(t)
	dash, _, shared := r.sharedDashboardAnswer(t)
	foldertest.Move(t, r.db, dash, foldertest.Restricted(t, r.db, "Payroll", payrollGroup))

	if err := r.service.Withdraw(reading(t.Context()), viewer, mustParse(t, shared.Thread.ID)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("withdraw by someone who cannot view: %v", err)
	}
	r.call(t, viewer, http.MethodDelete, "/threads/"+shared.Thread.ID+"/share", nil).expect(t, http.StatusNotFound, "not_found")
	if n := r.messageCount(t); n != 2 {
		t.Fatalf("messages after the refusal = %d, want 2", n)
	}
}

func TestWithdrawingAnObservationAnswerKeepsTheBlocksItWrote(t *testing.T) {
	r := newShareRig(t)
	observation := r.filedObservationIn(t, testUser, pgtype.UUID{})
	id := observation.String()
	run := r.privateAnswer(t, revisions.StartRequest{ObservationID: &id}, "prom", "")
	shared, err := r.service.ShareRun(reading(t.Context()), testUser, ShareInput{RunID: run})
	if err != nil {
		t.Fatal(err)
	}
	blocks := r.blockCount(t, observation)

	if got := r.callStatus(t, testUser, http.MethodDelete, "/threads/"+shared.Thread.ID+"/share"); got != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", got)
	}
	if after := r.blockCount(t, observation); after != blocks {
		t.Fatalf("blocks %d -> %d, want the written blocks kept", blocks, after)
	}
	if n := r.messageCount(t); n != 0 {
		t.Fatalf("messages after the withdraw = %d, want 0", n)
	}
	events := r.db.AuditEvents(t, "run.withdraw")
	if len(events) != 1 || events[0].TargetID != run.String() {
		t.Fatalf("run.withdraw events = %+v", events)
	}
}
