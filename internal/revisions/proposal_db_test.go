package revisions_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func proposeFor(t *testing.T, d *dbtest.Database, dash, run pgtype.UUID) {
	t.Helper()
	d.Exec(t, "INSERT INTO proposals (dashboard_id, title, author_kind, author_id, author_name, run_id) VALUES ($1, 'proposed', 'user', $2, 'Ada', $3)", dash, mustUUID(t, userID), run)
}

func assertSucceededWithProposal(t *testing.T, svc *revisions.Service, id pgtype.UUID) {
	t.Helper()
	stored, err := svc.GetByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != string(revisions.StateSucceeded) || stored.Status != string(revisions.StatusSucceeded) || stored.ResultKind != string(revisions.ResultProposal) ||
		stored.ResultVersion.Valid || stored.FailureCode.Valid || stored.Error.Valid || stored.LeaseUntil.Valid || !stored.FinishedAt.Valid {
		t.Fatalf("run = state %s, status %s, kind %s, version %v, failure %v, error %v, lease %v, finished %v; want succeeded with a proposal and no version",
			stored.State, stored.Status, stored.ResultKind, stored.ResultVersion, stored.FailureCode, stored.Error, stored.LeaseUntil, stored.FinishedAt.Valid)
	}
}

func TestARunThatProposedSucceedsWithAProposalAndLeavesItsCommentThreadOpen(t *testing.T) {
	var d *dbtest.Database
	var svc *revisions.Service
	svc, d = newService(t, reviserFunc(func(_ context.Context, req revisions.Request) (revisions.Result, error) {
		proposeFor(t, d, mustUUID(t, req.DashboardID), mustUUID(t, req.ID))
		return revisions.Result{ProposalID: "0b0b0b0b-0b0b-4b0b-8b0b-0b0b0b0b0b0b"}, nil
	}))
	dash := seedDashboard(t, d, 1)
	root := newThread(t, d, dash, "p1")
	row, err := svc.SubmitForThread(context.Background(), dash, root.ID, nil, "go", user)
	if err != nil {
		t.Fatal(err)
	}

	startWorker(t, svc, d, revisions.WorkerOptions{})
	waitForStatus(t, svc, dash, row.ID.String(), revisions.StatusSucceeded)

	assertSucceededWithProposal(t, svc, row.ID)
	thread, err := d.Queries().GetThread(context.Background(), root.ID)
	if err != nil {
		t.Fatal(err)
	}
	if thread.Status != "open" || thread.ResolvedByVersion.Valid {
		t.Fatalf("thread = %+v, want it still open", thread)
	}
}

func TestACanceledRunThatHadProposedSettlesAsSucceededWithAProposal(t *testing.T) {
	cases := map[string]func(t *testing.T, svc *revisions.Service, d *dbtest.Database, id pgtype.UUID){
		"canceled with a live lease, then swept": func(t *testing.T, svc *revisions.Service, d *dbtest.Database, id pgtype.UUID) {
			if _, err := svc.Cancel(context.Background(), id, user); err != nil {
				t.Fatal(err)
			}
			expireLease(t, d, id)
			claimOne(t, svc)
		},
		"canceled after its lease expired": func(t *testing.T, svc *revisions.Service, d *dbtest.Database, id pgtype.UUID) {
			expireLease(t, d, id)
			if _, err := svc.Cancel(context.Background(), id, user); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, settle := range cases {
		t.Run(name, func(t *testing.T) {
			svc, d := newService(t, succeedWith(2))
			dash := seedDashboard(t, d, 1)
			rev := submit(t, svc, dash, "live")
			id := mustUUID(t, rev.ID)
			claimOne(t, svc)
			proposeFor(t, d, dash, id)

			settle(t, svc, d, id)

			assertSucceededWithProposal(t, svc, id)
		})
	}
}

func TestAnExhaustedRunThatHadProposedSucceedsWithAProposal(t *testing.T) {
	svc, d := newService(t, succeedWith(9))
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, "proposed by attempt 2, then the process died")
	id := mustUUID(t, rev.ID)
	proposeFor(t, d, dash, id)
	d.Exec(t, "UPDATE agent_requests SET state = 'running', status = 'running', attempts = 2, started_at = clock_timestamp(), lease_until = clock_timestamp() - interval '1 second' WHERE id = $1", id)

	startWorker(t, svc, d, revisions.WorkerOptions{})
	waitForStatus(t, svc, dash, rev.ID, revisions.StatusSucceeded)

	assertSucceededWithProposal(t, svc, id)
}
