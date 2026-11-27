package revisions_test

import (
	"context"
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
	_, err := d.Queries().InsertDashboardVersion(reading(context.Background()), dbsqlc.InsertDashboardVersionParams{
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
	row, err := svc.SubmitObserve(reading(context.Background()), user, "live")
	if err != nil {
		t.Fatal(err)
	}
	claimOne(t, svc)

	got, err := svc.Cancel(reading(context.Background()), row.ID, user)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if got.State != string(revisions.StateCanceling) || got.Status != string(revisions.StatusRunning) || !got.CancelRequestedAt.Valid || got.FailureCode.Valid || !got.LeaseUntil.Valid || got.FinishedAt.Valid {
		t.Fatalf("canceled live run = %+v, want running with only cancel_requested_at set", got)
	}
}

func TestCancelFinishesARunningRequestWhoseLeaseExpired(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	row, err := svc.SubmitObserve(reading(context.Background()), user, "crashed")
	if err != nil {
		t.Fatal(err)
	}
	claimOne(t, svc)
	expireLease(t, d, row.ID)

	got, err := svc.Cancel(reading(context.Background()), row.ID, user)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if got.State != string(revisions.StateCanceled) || got.Status != string(revisions.StatusFailed) || got.FailureCode.String != string(revisions.FailureCanceled) || got.Error.String != revisions.CanceledMessage || got.LeaseUntil.Valid || !got.FinishedAt.Valid || !got.CancelRequestedAt.Valid {
		t.Fatalf("canceled crashed run = %+v, want failed with code canceled, no lease and finished", got)
	}
	stored, err := svc.GetByID(reading(context.Background()), row.ID)
	if err != nil || stored.Status != string(revisions.StatusFailed) {
		t.Fatalf("stored = %+v, %v; want the failed row persisted", stored, err)
	}
	if _, found := claimOne(t, svc); found {
		t.Fatal("a canceled request was claimed again")
	}
}

func TestCancelKeepsTheBuildOfARunningRequestWhoseLeaseExpired(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	row, err := svc.SubmitObserve(reading(context.Background()), user, "crashed")
	if err != nil {
		t.Fatal(err)
	}
	claimed, _ := claimOne(t, svc)
	if _, err := d.Queries().UpsertAgentRequestBuild(reading(context.Background()), dbsqlc.UpsertAgentRequestBuildParams{
		AgentRequestID: row.ID, Spec: []byte(`{}`), ClaimEpoch: claimed.ClaimEpoch,
	}); err != nil {
		t.Fatal(err)
	}
	expireLease(t, d, row.ID)

	if _, err := svc.Cancel(reading(context.Background()), row.ID, user); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if _, err := d.Queries().GetAgentRequestBuild(reading(context.Background()), row.ID); err != nil {
		t.Fatalf("build after cancel: %v, want it kept", err)
	}
}

func TestCancelOfAnExpiredRunWithASavedVersionSucceedsIt(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	row, err := svc.SubmitObserve(reading(context.Background()), user, "saved before the crash")
	if err != nil {
		t.Fatal(err)
	}
	claimOne(t, svc)
	saveVersionUnder(t, d, dash, row.ID, 2)
	expireLease(t, d, row.ID)

	got, err := svc.Cancel(reading(context.Background()), row.ID, user)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if got.Status != string(revisions.StatusSucceeded) || got.ResultVersion.Int32 != 2 || got.DashboardID != dash || got.Error.Valid || got.FailureCode.Valid || got.LeaseUntil.Valid || !got.FinishedAt.Valid {
		t.Fatalf("canceled run with a saved version = %+v, want succeeded with version 2 on the dashboard", got)
	}
	if got.Intent != string(revisions.IntentApply) || revisions.SourceOf(got) != revisions.SourceObserve {
		t.Fatalf("canceled create run that saved a dashboard: intent %q source %q, want apply and observe", got.Intent, revisions.SourceOf(got))
	}
}

func TestAnExhaustedCreateRunThatSavedADashboardBecomesAnApply(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	row, err := svc.SubmitObserve(reading(context.Background()), user, "saved, then every attempt died")
	if err != nil {
		t.Fatal(err)
	}
	claimOne(t, svc)
	saveVersionUnder(t, d, dash, row.ID, 2)
	d.Exec(t, "UPDATE agent_requests SET attempts = 99, lease_until = clock_timestamp() - interval '1 second' WHERE id = $1", row.ID)

	claimOne(t, svc)
	got, err := svc.GetByID(reading(context.Background()), row.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != string(revisions.StateSucceeded) || got.DashboardID != dash || got.Intent != string(revisions.IntentApply) || revisions.SourceOf(got) != revisions.SourceObserve {
		t.Fatalf("exhausted create run = state %s dashboard %v intent %q source %q, want a succeeded apply on the dashboard", got.State, got.DashboardID, got.Intent, revisions.SourceOf(got))
	}
}

func TestClaimStillWorksWhenBuildRetentionCleanupFails(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	d.Exec(t, `CREATE FUNCTION refuse_build_delete() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'cleanup refused'; END $$`)
	d.Exec(t, "CREATE TRIGGER refuse_build_delete BEFORE DELETE ON agent_request_builds FOR EACH STATEMENT EXECUTE FUNCTION refuse_build_delete()")
	row, err := svc.SubmitObserve(reading(context.Background()), user, "claim anyway")
	if err != nil {
		t.Fatal(err)
	}

	claimed, found := claimOne(t, svc)
	if !found || claimed.ID != row.ID {
		t.Fatalf("claim = %+v, found %v; want the queued request despite the failing cleanup", claimed, found)
	}
}

func TestContinueStartsWhenTheDashboardMovedOnAndTakesTheStaleBuild(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	ctx := reading(context.Background())
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, "add a panel")
	id := mustUUID(t, rev.ID)
	claimed, _ := claimOne(t, svc)
	if _, err := d.Queries().UpsertAgentRequestBuild(ctx, dbsqlc.UpsertAgentRequestBuildParams{
		AgentRequestID: id, DashboardID: dash, BaseVersion: pgtype.Int4{Int32: 1, Valid: true}, Spec: []byte(`{}`), Panels: 1, ClaimEpoch: claimed.ClaimEpoch,
	}); err != nil {
		t.Fatal(err)
	}
	d.Exec(t, "UPDATE agent_requests SET state = 'failed', status = 'failed', failure_code = 'no_progress', error = 'stopped', lease_until = NULL, finished_at = now() WHERE id = $1", id)
	saveVersionUnder(t, d, dash, pgtype.UUID{}, 2)

	next, err := svc.Continue(ctx, id, user, "")
	if err != nil {
		t.Fatalf("continue of a stale build = %v, want a continuation that rebases at its start", err)
	}
	if next.ContinuesID != id || next.State != string(revisions.StateQueued) {
		t.Fatalf("continuation = continues %v state %s, want a queued continuation of %v", next.ContinuesID, next.State, id)
	}
	if _, err := d.Queries().GetAgentRequestBuild(ctx, next.ID); err != nil {
		t.Fatalf("build of the continuation: %v, want the stale build moved to it", err)
	}
}
