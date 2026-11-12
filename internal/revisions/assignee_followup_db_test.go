package revisions_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func connectedGrant(t *testing.T, d *dbtest.Database) string {
	t.Helper()
	d.Exec(t, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, name) VALUES ($1::uuid, 'issuer', $1, 'ada@example.com', 'Ada') ON CONFLICT (id) DO NOTHING`, userID)
	var id string
	err := d.SQLDB(t).QueryRowContext(context.Background(), `WITH c AS (INSERT INTO oauth_clients (name, redirect_uris) VALUES ('Codex', ARRAY['http://127.0.0.1:1/cb']) RETURNING id)
		INSERT INTO oauth_grants (client_id, user_id, resource, expires_at) SELECT id, $1::uuid, 'http://localhost:8080/mcp', now() + interval '1 day' FROM c RETURNING id::text`, userID).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func runTaker(t *testing.T, d *dbtest.Database, takesRuns bool) string {
	t.Helper()
	var id string
	if err := d.SQLDB(t).QueryRowContext(context.Background(), `INSERT INTO service_accounts (name, takes_runs) VALUES ('ci-bot', $1) RETURNING id::text`, takesRuns).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

type assigneeCase struct {
	name     string
	kind     string
	setup    func(t *testing.T, d *dbtest.Database) string
	change   func(t *testing.T, d *dbtest.Database, id string)
	by       auth.Principal
	wantKeep bool
}

func assigneeCases() []assigneeCase {
	revoke := func(t *testing.T, d *dbtest.Database, id string) {
		d.Exec(t, `UPDATE oauth_grants SET revoked_at = now(), revoked_reason = 'person' WHERE id = $1::uuid`, id)
	}
	expire := func(t *testing.T, d *dbtest.Database, id string) {
		d.Exec(t, `UPDATE oauth_grants SET expires_at = now() - interval '1 minute' WHERE id = $1::uuid`, id)
	}
	stopTakingRuns := func(t *testing.T, d *dbtest.Database, id string) {
		d.Exec(t, `UPDATE service_accounts SET takes_runs = false WHERE id = $1::uuid`, id)
	}
	account := func(t *testing.T, d *dbtest.Database) string { return runTaker(t, d, true) }
	return []assigneeCase{
		{"a live connected agent", "grant", connectedGrant, nil, user, true},
		{"a live service account", "service_account", account, nil, user, true},
		{"a revoked grant", "grant", connectedGrant, revoke, user, false},
		{"an expired grant", "grant", connectedGrant, expire, user, false},
		{"a grant of another person", "grant", connectedGrant, nil, root, false},
		{"a service account that stopped taking runs", "service_account", account, stopTakingRuns, user, false},
	}
}

func TestRetryKeepsTheAssigneeAndRefusesOneThatCannotTakeRuns(t *testing.T) {
	for _, tt := range assigneeCases() {
		t.Run(tt.name, func(t *testing.T) {
			svc, d := newService(t, succeedWith(2))
			dash := seedDashboard(t, d, 1)
			assignee := tt.setup(t, d)
			first := submit(t, svc, dash, "look again")
			assignTo(t, d, first.ID, tt.kind, assignee)
			if _, err := svc.Cancel(context.Background(), mustUUID(t, first.ID), user); err != nil {
				t.Fatal(err)
			}
			if tt.change != nil {
				tt.change(t, d, assignee)
			}

			next, err := svc.Retry(context.Background(), mustUUID(t, first.ID), tt.by)
			var runs int
			if countErr := d.SQLDB(t).QueryRow("SELECT count(*) FROM agent_requests").Scan(&runs); countErr != nil {
				t.Fatal(countErr)
			}
			if !tt.wantKeep {
				if !errors.Is(err, revisions.ErrAssigneeUnavailable) || runs != 1 {
					t.Fatalf("retry = %v with %d runs, want the assignee refusal and no new run", err, runs)
				}
				return
			}
			if err != nil {
				t.Fatalf("retry: %v", err)
			}
			if next.AssignedKind != tt.kind || next.AssignedID.String() != assignee {
				t.Fatalf("retry assigned to %s %s, want %s %s", next.AssignedKind, next.AssignedID.String(), tt.kind, assignee)
			}
		})
	}
}

func TestRetryOfAnOutsideAgentsRunDoesNotNeedTheBuiltInAgent(t *testing.T) {
	d := dbtest.New(t)
	svc := newTestService(d.DB, nil, quietLogger())
	dash := seedDashboard(t, d, 1)
	grant := connectedGrant(t, d)
	for _, source := range []string{"prompt", "investigate"} {
		first := insertCanceledRun(t, d, dash, source, "grant", grant)
		next, err := svc.Retry(context.Background(), first, user)
		if err != nil || next.AssignedKind != "grant" || next.AssignedID.String() != grant || next.Source != source {
			t.Fatalf("retry of a %s run = %+v, %v, want a %s run for the grant", source, next, err, source)
		}
	}
	builtin := insertCanceledRun(t, d, dash, "prompt", "builtin", "")
	if _, err := svc.Retry(context.Background(), builtin, user); !errors.Is(err, revisions.ErrUnavailable) {
		t.Fatalf("retry of a built-in run = %v, want ErrUnavailable", err)
	}
}

func insertCanceledRun(t *testing.T, d *dbtest.Database, dash pgtype.UUID, source, kind, assignee string) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	row := d.SQLDB(t).QueryRowContext(context.Background(), `INSERT INTO agent_requests (dashboard_id, source, prompt, base_version, requested_by_kind, requested_by_id, requested_by_name, assigned_kind, assigned_id, state, status, failure_code, finished_at)
		VALUES ($1, $5::text, 'look again', CASE WHEN $5::text = 'investigate' THEN NULL ELSE 1 END, 'user', $2::uuid, 'Ada', $3, NULLIF($4, '')::uuid, 'canceled', 'failed', 'canceled', now()) RETURNING id`, dash, userID, kind, assignee, source)
	if err := row.Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestContinueKeepsTheAssigneeAndRefusesOneThatCannotTakeRuns(t *testing.T) {
	for _, tt := range assigneeCases() {
		if tt.by.ID != userID {
			continue
		}
		t.Run(tt.name, func(t *testing.T) {
			svc, d := newService(t, succeedWith(2))
			ctx := context.Background()
			dash := seedDashboard(t, d, 1)
			assignee := tt.setup(t, d)
			rev := submit(t, svc, dash, "add a panel")
			id := mustUUID(t, rev.ID)
			assignTo(t, d, rev.ID, tt.kind, assignee)
			claimant := revisions.Claimant{Kind: revisions.AssigneeKind(tt.kind), ID: assignee, Person: userID}
			claimed, found := claimAs(t, svc, claimant)
			if !found {
				t.Fatal("the assignee did not claim its run")
			}
			if _, err := d.Queries().UpsertAgentRequestBuild(ctx, dbsqlc.UpsertAgentRequestBuildParams{
				AgentRequestID: id, DashboardID: dash, BaseVersion: pgtype.Int4{Int32: 1, Valid: true}, Spec: []byte(`{}`), Panels: 1, Attempt: claimed.Row.Attempts,
			}); err != nil {
				t.Fatal(err)
			}
			d.Exec(t, "UPDATE agent_requests SET state = 'failed', status = 'failed', failure_code = 'no_progress', error = 'stopped', lease_until = NULL, finished_at = now() WHERE id = $1", id)
			if tt.change != nil {
				tt.change(t, d, assignee)
			}

			next, err := svc.Continue(ctx, id, user, "")
			if !tt.wantKeep {
				var runs int
				if countErr := d.SQLDB(t).QueryRow("SELECT count(*) FROM agent_requests").Scan(&runs); countErr != nil {
					t.Fatal(countErr)
				}
				if !errors.Is(err, revisions.ErrAssigneeUnavailable) || runs != 1 {
					t.Fatalf("continue = %v with %d runs, want the assignee refusal and no new run", err, runs)
				}
				if _, err := d.Queries().GetAgentRequestBuild(ctx, id); err != nil {
					t.Fatalf("build after refused continue: %v, want it kept on the stopped request", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("continue: %v", err)
			}
			if next.AssignedKind != tt.kind || next.AssignedID.String() != assignee {
				t.Fatalf("continue assigned to %s %s, want %s %s", next.AssignedKind, next.AssignedID.String(), tt.kind, assignee)
			}
		})
	}
}
