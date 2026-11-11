package revisions_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func expireLease(t *testing.T, d *dbtest.Database, id pgtype.UUID) {
	t.Helper()
	d.Exec(t, "UPDATE agent_requests SET lease_until = clock_timestamp() - interval '1 second' WHERE id = $1", id)
}

func saveVersionUnder(t *testing.T, d *dbtest.Database, dashboard, requestID pgtype.UUID, version int32) {
	t.Helper()
	_, err := d.Queries().InsertDashboardVersion(context.Background(), dbsqlc.InsertDashboardVersionParams{
		DashboardID: dashboard, Version: version, ParentVersion: pgtype.Int4{Int32: version - 1, Valid: true}, Spec: []byte(`{}`), Summary: "s",
		AuthorKind: "user", AuthorID: mustUUID(t, userID), AuthorName: "Ada", AgentRequestID: requestID,
	})
	if err != nil {
		t.Fatal(err)
	}
	d.Exec(t, "UPDATE dashboards SET latest_version = $2 WHERE id = $1", dashboard, version)
}

func TestCancelOfARunningRequestWithALiveLeaseOnlyFlagsIt(t *testing.T) {
	svc, _ := newService(t, succeedWith(2))
	row, err := svc.SubmitObserve(context.Background(), user, "live")
	if err != nil {
		t.Fatal(err)
	}
	claimOne(t, svc)

	got, err := svc.Cancel(context.Background(), row.ID, user)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if got.State != string(revisions.StateCanceling) || got.Status != string(revisions.StatusRunning) || !got.CancelRequestedAt.Valid || got.FailureCode.Valid || !got.LeaseUntil.Valid || got.FinishedAt.Valid {
		t.Fatalf("canceled live run = %+v, want running with only cancel_requested_at set", got)
	}
}

func TestCancelFinishesARunningRequestWhoseLeaseExpired(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	row, err := svc.SubmitObserve(context.Background(), user, "crashed")
	if err != nil {
		t.Fatal(err)
	}
	claimOne(t, svc)
	expireLease(t, d, row.ID)

	got, err := svc.Cancel(context.Background(), row.ID, user)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if got.State != string(revisions.StateCanceled) || got.Status != string(revisions.StatusFailed) || got.FailureCode.String != string(revisions.FailureCanceled) || got.Error.String != revisions.CanceledMessage || got.LeaseUntil.Valid || !got.FinishedAt.Valid || !got.CancelRequestedAt.Valid {
		t.Fatalf("canceled crashed run = %+v, want failed with code canceled, no lease and finished", got)
	}
	stored, err := svc.GetByID(context.Background(), row.ID)
	if err != nil || stored.Status != string(revisions.StatusFailed) {
		t.Fatalf("stored = %+v, %v; want the failed row persisted", stored, err)
	}
	if _, found := claimOne(t, svc); found {
		t.Fatal("a canceled request was claimed again")
	}
}

func TestCancelKeepsTheBuildOfARunningRequestWhoseLeaseExpired(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	row, err := svc.SubmitObserve(context.Background(), user, "crashed")
	if err != nil {
		t.Fatal(err)
	}
	claimed, _ := claimOne(t, svc)
	if _, err := d.Queries().UpsertAgentRequestBuild(context.Background(), dbsqlc.UpsertAgentRequestBuildParams{
		AgentRequestID: row.ID, Spec: []byte(`{}`), Attempt: claimed.Attempts,
	}); err != nil {
		t.Fatal(err)
	}
	expireLease(t, d, row.ID)

	if _, err := svc.Cancel(context.Background(), row.ID, user); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if _, err := d.Queries().GetAgentRequestBuild(context.Background(), row.ID); err != nil {
		t.Fatalf("build after cancel: %v, want it kept", err)
	}
}

func TestCancelOfAnExpiredRunWithASavedVersionSucceedsIt(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	row, err := svc.SubmitObserve(context.Background(), user, "saved before the crash")
	if err != nil {
		t.Fatal(err)
	}
	claimOne(t, svc)
	saveVersionUnder(t, d, dash, row.ID, 2)
	expireLease(t, d, row.ID)

	got, err := svc.Cancel(context.Background(), row.ID, user)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if got.Status != string(revisions.StatusSucceeded) || got.ResultVersion.Int32 != 2 || got.DashboardID != dash || got.Error.Valid || got.FailureCode.Valid || got.LeaseUntil.Valid || !got.FinishedAt.Valid {
		t.Fatalf("canceled run with a saved version = %+v, want succeeded with version 2 on the dashboard", got)
	}
}

func TestClaimStillWorksWhenBuildRetentionCleanupFails(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	d.Exec(t, `CREATE FUNCTION refuse_build_delete() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'cleanup refused'; END $$`)
	d.Exec(t, "CREATE TRIGGER refuse_build_delete BEFORE DELETE ON agent_request_builds FOR EACH STATEMENT EXECUTE FUNCTION refuse_build_delete()")
	row, err := svc.SubmitObserve(context.Background(), user, "claim anyway")
	if err != nil {
		t.Fatal(err)
	}

	claimed, found := claimOne(t, svc)
	if !found || claimed.ID != row.ID {
		t.Fatalf("claim = %+v, found %v; want the queued request despite the failing cleanup", claimed, found)
	}
}

func TestContinueRefusesWithVersionConflictWhenTheDashboardMovedOnAndEnqueuesNothing(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	ctx := context.Background()
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, "add a panel")
	id := mustUUID(t, rev.ID)
	claimed, _ := claimOne(t, svc)
	if _, err := d.Queries().UpsertAgentRequestBuild(ctx, dbsqlc.UpsertAgentRequestBuildParams{
		AgentRequestID: id, DashboardID: dash, BaseVersion: pgtype.Int4{Int32: 1, Valid: true}, Spec: []byte(`{}`), Panels: 1, Attempt: claimed.Attempts,
	}); err != nil {
		t.Fatal(err)
	}
	d.Exec(t, "UPDATE agent_requests SET state = 'failed', status = 'failed', failure_code = 'no_progress', error = 'stopped', lease_until = NULL, finished_at = now() WHERE id = $1", id)
	saveVersionUnder(t, d, dash, pgtype.UUID{}, 2)

	_, err := svc.Continue(ctx, id, user, "")
	var conflict *revisions.BuildConflictError
	if !errors.As(err, &conflict) || conflict.Latest != 2 {
		t.Fatalf("continue = %v, want BuildConflictError at version 2", err)
	}
	var requests int
	if err := d.SQLDB(t).QueryRow("SELECT count(*) FROM agent_requests").Scan(&requests); err != nil || requests != 1 {
		t.Fatalf("agent requests = %d, %v; want only the stopped one", requests, err)
	}
	if _, err := d.Queries().GetAgentRequestBuild(ctx, id); err != nil {
		t.Fatalf("build after refused continue: %v, want it kept on the stopped request", err)
	}
}
