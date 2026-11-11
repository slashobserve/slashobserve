package revisions_test

import (
	"context"
	"errors"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/lifecycle"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const otherUserID = "22222222-2222-4222-8222-222222222222"

var otherUser = auth.Principal{Kind: auth.KindUser, ID: otherUserID, Name: "Grace", Email: "grace@example.com", Role: auth.RoleEditor}

func TestCancelOfARunWithALiveLeaseMovesItToCanceling(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, "live")
	claimOne(t, svc)

	got, err := svc.Cancel(context.Background(), mustUUID(t, rev.ID), user)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != string(revisions.StateCanceling) || got.Status != string(revisions.StatusRunning) || !got.CancelRequestedAt.Valid {
		t.Fatalf("canceled live run = state %s status %s, want canceling and running", got.State, got.Status)
	}
	assertTypes(t, runRecords(t, d, got.ID), "run.queued", "run.started", "run.canceling")
}

func TestARepeatCancelOfACancelingRunChangesNothing(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, "live")
	claimOne(t, svc)
	id := mustUUID(t, rev.ID)
	if _, err := svc.Cancel(context.Background(), id, user); err != nil {
		t.Fatal(err)
	}

	got, err := svc.Cancel(context.Background(), id, user)
	if err != nil {
		t.Fatalf("repeat cancel: %v", err)
	}
	if got.State != string(revisions.StateCanceling) {
		t.Fatalf("state %s, want canceling", got.State)
	}
	assertTypes(t, runRecords(t, d, id), "run.queued", "run.started", "run.canceling")
}

func TestACancelingRunIsNotClaimedAndTheClaimSweepSettlesItOnceItsLeaseExpired(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, "live")
	id := mustUUID(t, rev.ID)
	claimOne(t, svc)
	if _, err := svc.Cancel(context.Background(), id, user); err != nil {
		t.Fatal(err)
	}
	if _, found := claimOne(t, svc); found {
		t.Fatal("a canceling run with a live lease was claimed")
	}

	expireLease(t, d, id)
	if _, found := claimOne(t, svc); found {
		t.Fatal("a canceling run was claimed after its lease expired")
	}
	stored, err := svc.GetByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != string(revisions.StateCanceled) || stored.Status != string(revisions.StatusFailed) || stored.FailureCode.String != string(revisions.FailureCanceled) || stored.LeaseUntil.Valid || !stored.FinishedAt.Valid {
		t.Fatalf("swept run = %+v, want canceled, failed with code canceled, no lease and finished", stored)
	}
	assertTypes(t, runRecords(t, d, id), "run.queued", "run.started", "run.canceling", "run.canceled")
}

func TestTheClaimSweepSettlesACancelingRunWithASavedVersionAsSucceeded(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, "live")
	id := mustUUID(t, rev.ID)
	claimOne(t, svc)
	if _, err := svc.Cancel(context.Background(), id, user); err != nil {
		t.Fatal(err)
	}
	saveVersionUnder(t, d, dash, id, 2)
	expireLease(t, d, id)

	claimOne(t, svc)
	stored, err := svc.GetByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != string(revisions.StateSucceeded) || stored.ResultVersion.Int32 != 2 {
		t.Fatalf("swept run = state %s version %d, want succeeded with version 2", stored.State, stored.ResultVersion.Int32)
	}
}

func TestARunningRunWhoseLeaseExpiredIsClaimedAgainWithoutPassingThroughQueued(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, "crashed")
	id := mustUUID(t, rev.ID)
	claimOne(t, svc)
	expireLease(t, d, id)

	again, found := claimOne(t, svc)
	if !found || again.ID != id {
		t.Fatalf("claim = %+v, found %v; want the same run again", again, found)
	}
	if again.State != string(revisions.StateRunning) || again.Attempts != 2 {
		t.Fatalf("reclaimed run = state %s attempts %d, want running and 2", again.State, again.Attempts)
	}
	assertTypes(t, runRecords(t, d, id), "run.queued", "run.started", "run.started")
}

func TestReleasingACancelingRunSettlesItCanceledInsteadOfRequeueingIt(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, "live")
	id := mustUUID(t, rev.ID)
	claimed, _ := claimOne(t, svc)
	if _, err := svc.Cancel(context.Background(), id, user); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.ReleaseRow(context.Background(), claimed, false); err != nil {
		t.Fatalf("release: %v", err)
	}
	stored, err := svc.GetByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != string(revisions.StateCanceled) || stored.FailureCode.String != string(revisions.FailureCanceled) {
		t.Fatalf("released canceling run = state %s code %q, want canceled", stored.State, stored.FailureCode.String)
	}
	if _, found := claimOne(t, svc); found {
		t.Fatal("a released canceling run went back to the queue")
	}
}

func TestRetryByAnotherViewerStartsANewRunOwnedByTheCaller(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, "add a panel")
	id := mustUUID(t, rev.ID)
	if _, err := svc.Cancel(context.Background(), id, user); err != nil {
		t.Fatal(err)
	}

	next, err := svc.Retry(context.Background(), id, otherUser)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if next.ID == id || next.RequestedByID.String() != otherUserID || next.Prompt != "add a panel" || next.State != string(revisions.StateQueued) || next.DashboardID != dash {
		t.Fatalf("retry = %+v, want a new queued run for the caller with the same prompt on the same dashboard", next)
	}
	original, err := svc.GetByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if original.State != string(revisions.StateCanceled) {
		t.Fatalf("the original is %s, want it untouched", original.State)
	}
}

func TestRetryOfAnObserveRunStartsANewObserveRun(t *testing.T) {
	svc, _ := newService(t, succeedWith(2))
	row, err := svc.SubmitObserve(context.Background(), user, "checkout dashboard")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Cancel(context.Background(), row.ID, user); err != nil {
		t.Fatal(err)
	}

	next, err := svc.Retry(context.Background(), row.ID, user)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if next.ID == row.ID || next.Source != string(revisions.SourceObserve) || next.Prompt != "checkout dashboard" || next.DashboardID.Valid {
		t.Fatalf("retry = %+v, want a new observe run with the same prompt", next)
	}
}

func TestRetryOfAnUnfinishedRunIsRefusedWithTheStateAndWhatIsAllowed(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, "still waiting")

	_, err := svc.Retry(context.Background(), mustUUID(t, rev.ID), user)
	var refusal *lifecycle.Refusal
	if !errors.As(err, &refusal) || !errors.Is(err, lifecycle.ErrInvalidTransition) {
		t.Fatalf("err = %v, want a refusal", err)
	}
	if refusal.State != "queued" || refusal.Action != "retry" || len(refusal.Allowed) == 0 {
		t.Fatalf("refusal = %+v, want queued, retry and the allowed actions", refusal)
	}
}

func TestKeepingABuildRecordsRunKept(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, "stopped")
	id := mustUUID(t, rev.ID)
	if _, err := svc.Cancel(context.Background(), id, user); err != nil {
		t.Fatal(err)
	}

	var kept bool
	err := d.InTx(context.Background(), func(q *dbsqlc.Queries) error {
		var err error
		kept, err = revisions.KeepBuild(context.Background(), q, id, dash, 2)
		return err
	})
	if err != nil || !kept {
		t.Fatalf("keep = %v, %v", kept, err)
	}
	records := runRecords(t, d, id)
	assertTypes(t, records, "run.queued", "run.canceled", "run.kept")
	if got := records[2]; got.Data["result_version"] != float64(2) {
		t.Errorf("run.kept = %+v, want result_version 2", got)
	}
}

func TestKeepingTheBuildOfARunningRunChangesNothing(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, "running")
	id := mustUUID(t, rev.ID)
	claimOne(t, svc)

	var kept bool
	err := d.InTx(context.Background(), func(q *dbsqlc.Queries) error {
		var err error
		kept, err = revisions.KeepBuild(context.Background(), q, id, dash, 2)
		return err
	})
	if err != nil || kept {
		t.Fatalf("keep = %v, %v; want it refused without an error", kept, err)
	}
	assertTypes(t, runRecords(t, d, id), "run.queued", "run.started")
}
