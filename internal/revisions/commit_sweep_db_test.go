package revisions_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

var errCommitLost = errors.New("the commit lost the run")

type terminalSweep struct {
	name    string
	prepare string
	run     func(ctx context.Context, svc *revisions.Service, id pgtype.UUID) error
	// A FOR UPDATE lock also waits for the version insert's KEY SHARE lock, so it cannot run between that insert and the marker.
	locksTheRunForUpdate bool
}

func terminalSweeps() []terminalSweep {
	sweep := func(ctx context.Context, svc *revisions.Service, _ pgtype.UUID) error { return svc.Sweep(ctx) }
	return []terminalSweep{
		{
			name:    "exhausted",
			prepare: "UPDATE agent_requests SET attempts = $2, lease_until = clock_timestamp() - interval '1 second' WHERE id = $1",
			run:     sweep,
		},
		{
			name: "canceled",
			prepare: `UPDATE agent_requests SET state = 'canceling', cancel_requested_at = clock_timestamp(), canceled_by_kind = 'user',
  canceled_by_id = '` + userID + `', canceled_by_name = 'Ada', lease_until = clock_timestamp() - interval '1 second' WHERE id = $1 AND $2::int > 0`,
			run: sweep,
		},
		{
			name: "unclaimed",
			prepare: `UPDATE agent_requests SET assigned_kind = 'service_account', assigned_id = gen_random_uuid(),
  created_at = clock_timestamp() - interval '1 day', lease_until = clock_timestamp() - interval '1 day' WHERE id = $1 AND $2::int > 0`,
			run: sweep,
		},
		{
			name:    "cancel",
			prepare: "UPDATE agent_requests SET lease_until = clock_timestamp() - interval '1 second' WHERE id = $1 AND $2::int > 0",
			run: func(ctx context.Context, svc *revisions.Service, id pgtype.UUID) error {
				_, err := svc.Cancel(ctx, id, user)
				return err
			},
			locksTheRunForUpdate: true,
		},
	}
}

type committingRun struct {
	run       dbsqlc.AgentRequest
	dashboard pgtype.UUID
}

func claimedCreateRun(t *testing.T, svc *revisions.Service, d *dbtest.Database, sweep terminalSweep) committingRun {
	t.Helper()
	ctx := reading(context.Background())
	row, err := svc.SubmitObserve(ctx, user, "build checkout health")
	if err != nil {
		t.Fatal(err)
	}
	claimed, found := claimOne(t, svc)
	if !found || claimed.ID != row.ID {
		t.Fatalf("claimed %v (found %v), want %v", claimed.ID, found, row.ID)
	}
	d.Exec(t, "INSERT INTO agent_request_builds (agent_request_id, spec, summary) VALUES ($1, '{}', 's')", claimed.ID)
	d.Exec(t, sweep.prepare, claimed.ID, revisions.MaxAttempts)
	return committingRun{run: claimed, dashboard: seedDashboard(t, d, 0)}
}

func (c committingRun) writeVersion(ctx context.Context, q *dbsqlc.Queries) error {
	author, err := db.ParseUUID(userID)
	if err != nil {
		return err
	}
	if _, err := q.InsertDashboardVersion(ctx, dbsqlc.InsertDashboardVersionParams{
		DashboardID: c.dashboard, Version: 1, Spec: []byte(`{}`), Summary: "s",
		AuthorKind: "user", AuthorID: author, AuthorName: "Ada", AgentRequestID: c.run.ID,
	}); err != nil {
		return err
	}
	n, err := q.DeleteAgentRequestBuild(ctx, dbsqlc.DeleteAgentRequestBuildParams{AgentRequestID: c.run.ID, ClaimEpoch: c.run.ClaimEpoch})
	if err != nil || n == 0 {
		return errors.Join(err, errCommitLost)
	}
	return nil
}

func (c committingRun) mark(ctx context.Context, q *dbsqlc.Queries) error {
	n, err := q.MarkAgentRequestCommitted(ctx, dbsqlc.MarkAgentRequestCommittedParams{
		Kind: "version", Version: pgtype.Int4{Int32: 1, Valid: true}, DashboardID: c.dashboard, ID: c.run.ID, ClaimEpoch: c.run.ClaimEpoch,
	})
	if err == nil && n == 0 {
		err = errCommitLost
	}
	return err
}

func waitingOnALock(t *testing.T, d *dbtest.Database) bool {
	t.Helper()
	var n int
	err := d.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM pg_stat_activity
WHERE datname = current_database() AND wait_event_type = 'Lock' AND pid <> pg_backend_pid()`).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n > 0
}

func TestATerminalSweepWaitsForACommitAndCountsItsVersion(t *testing.T) {
	for _, sweep := range terminalSweeps() {
		t.Run(sweep.name, func(t *testing.T) {
			svc, d := newService(t, succeedWith(1))
			ctx := reading(context.Background())
			c := claimedCreateRun(t, svc, d, sweep)

			marked := make(chan struct{})
			release := make(chan struct{})
			committed := make(chan error, 1)
			go func() {
				committed <- d.InTx(ctx, func(q *dbsqlc.Queries) error {
					if err := c.writeVersion(ctx, q); err != nil {
						return err
					}
					if err := c.mark(ctx, q); err != nil {
						return err
					}
					close(marked)
					<-release
					return nil
				})
			}()
			select {
			case <-marked:
			case err := <-committed:
				t.Fatalf("commit ended before it marked the run: %v", err)
			}

			swept := make(chan error, 1)
			go func() { swept <- sweep.run(ctx, svc, c.run.ID) }()
			deadline := time.Now().Add(10 * time.Second)
			for !waitingOnALock(t, d) {
				select {
				case err := <-swept:
					close(release)
					t.Fatalf("the sweep finished (%v) while the commit held the run", err)
				default:
				}
				if time.Now().After(deadline) {
					close(release)
					t.Fatal("the sweep never waited on the commit's lock")
				}
				time.Sleep(10 * time.Millisecond)
			}
			close(release)
			if err := <-committed; err != nil {
				t.Fatalf("commit: %v", err)
			}
			if err := <-swept; err != nil {
				t.Fatalf("sweep: %v", err)
			}

			got, err := d.Queries().GetAgentRequestByID(ctx, c.run.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.State != "succeeded" || got.Status != "succeeded" || got.ResultKind != "version" || got.Intent != "apply" {
				t.Errorf("state %s, status %s, result_kind %s, intent %s; want a succeeded apply with its version", got.State, got.Status, got.ResultKind, got.Intent)
			}
			if !got.ResultVersion.Valid || got.ResultVersion.Int32 != 1 || got.DashboardID != c.dashboard {
				t.Errorf("result_version %v and dashboard %v, want version 1 of %v", got.ResultVersion, got.DashboardID, c.dashboard)
			}
			if got.FailureCode.Valid || got.Error.Valid {
				t.Errorf("failure %q %q, want none", got.FailureCode.String, got.Error.String)
			}
		})
	}
}

func TestACommitAfterATerminalSweepSavesNoVersion(t *testing.T) {
	for _, sweep := range terminalSweeps() {
		t.Run(sweep.name, func(t *testing.T) {
			svc, d := newService(t, succeedWith(1))
			ctx := reading(context.Background())
			c := claimedCreateRun(t, svc, d, sweep)

			if sweep.locksTheRunForUpdate {
				if err := sweep.run(ctx, svc, c.run.ID); err != nil {
					t.Fatalf("sweep: %v", err)
				}
			}
			err := d.InTx(ctx, func(q *dbsqlc.Queries) error {
				if err := c.writeVersion(ctx, q); err != nil {
					return err
				}
				if !sweep.locksTheRunForUpdate {
					if err := sweep.run(ctx, svc, c.run.ID); err != nil {
						t.Errorf("sweep: %v", err)
					}
				}
				return c.mark(ctx, q)
			})
			if !errors.Is(err, errCommitLost) {
				t.Fatalf("commit after the sweep = %v, want the marker to find no running run", err)
			}

			got, err := d.Queries().GetAgentRequestByID(ctx, c.run.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.State == "succeeded" || got.ResultVersion.Valid || got.CommittedKind.Valid {
				t.Errorf("state %s, result_version %v, committed %v; want the run ended without a version", got.State, got.ResultVersion, got.CommittedKind)
			}
			var versions int
			if err := d.SQLDB(t).QueryRowContext(ctx, "SELECT count(*) FROM dashboard_versions WHERE agent_request_id = $1", c.run.ID.String()).Scan(&versions); err != nil {
				t.Fatal(err)
			}
			if versions != 0 {
				t.Errorf("%d versions saved under the run, want none", versions)
			}
		})
	}
}

func TestDeletingTheDashboardOfACommittedRunClearsItsMarkedVersion(t *testing.T) {
	svc, d := newService(t, succeedWith(1))
	ctx := reading(context.Background())
	c := claimedCreateRun(t, svc, d, terminalSweeps()[0])
	if err := d.InTx(ctx, func(q *dbsqlc.Queries) error {
		if err := c.writeVersion(ctx, q); err != nil {
			return err
		}
		return c.mark(ctx, q)
	}); err != nil {
		t.Fatal(err)
	}
	revised := seedDashboard(t, d, 1)
	revision := mustUUID(t, submit(t, svc, revised, "add a panel").ID)
	d.Exec(t, "UPDATE agent_requests SET committed_kind = 'version', committed_dashboard_id = dashboard_id, committed_version = 1 WHERE id = $1", revision)

	d.Exec(t, "DELETE FROM dashboards WHERE id = $1", revised)
	d.Exec(t, "DELETE FROM dashboards WHERE id = $1", c.dashboard)

	if _, err := d.Queries().GetAgentRequestByID(ctx, revision); err == nil {
		t.Error("the revision run outlived its dashboard")
	}
	got, err := d.Queries().GetAgentRequestByID(ctx, c.run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CommittedVersion.Valid || got.CommittedDashboardID.Valid || got.CommittedKind.String != "version" {
		t.Errorf("committed %v, version %v of %v; want the kind kept and the version cleared with its dashboard", got.CommittedKind, got.CommittedVersion, got.CommittedDashboardID)
	}
	if err := svc.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if got, err = d.Queries().GetAgentRequestByID(ctx, c.run.ID); err != nil {
		t.Fatal(err)
	}
	if got.State != "failed" || got.ResultVersion.Valid || got.DashboardID.Valid {
		t.Errorf("state %s, result_version %v, dashboard %v; want a failed run, because its version is gone", got.State, got.ResultVersion, got.DashboardID)
	}
}
