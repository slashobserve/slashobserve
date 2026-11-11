package db_test

import (
	"context"
	"testing"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

const agentClaimsVersion = 37

const agentClaimsDashboard = "00000000-0000-0000-0000-0000000000c1"

const insertAssignedRequestSQL = `INSERT INTO agent_requests (prompt, requested_by_kind, requested_by_id, requested_by_name,
  source, status, state, dashboard_id, base_version, panel_id, assigned_kind, assigned_id)
VALUES ('p', 'user', gen_random_uuid(), 'seed', $1, 'queued', 'queued', nullif($2, '')::uuid, nullif($3, 0), nullif($4, ''),
  coalesce(nullif($5, ''), 'builtin'), nullif($6, '')::uuid)
RETURNING id`

type assignedRequest struct {
	source, dashboardID, panelID, kind, assignedID string
	baseVersion                                     int
}

type columnRef struct{ table, column string }

var agentClaimsColumns = []columnRef{
	{"agent_requests", "assigned_kind"},
	{"agent_requests", "assigned_id"},
	{"agent_requests", "claimed_by"},
	{"oauth_grants", "mode"},
	{"service_accounts", "takes_runs"},
}

var agentClaimsTables = []string{"run_tokens", "agent_workers"}

func TestAgentClaimsDown(t *testing.T) {
	ctx := context.Background()
	d := dbtest.NewUnmigrated(t)
	if err := d.MigrateUpTo(ctx, agentClaimsVersion); err != nil {
		t.Fatal(err)
	}
	if missing := missingAgentClaimsObjects(t, d); len(missing) > 0 {
		t.Fatalf("objects missing after the up migration: %v", missing)
	}

	if err := d.MigrateDownTo(ctx, agentClaimsVersion-1); err != nil {
		t.Fatalf("down: %v", err)
	}
	if missing := missingAgentClaimsObjects(t, d); len(missing) != len(agentClaimsColumns)+len(agentClaimsTables) {
		t.Fatalf("objects still exist after the down migration, missing only %v", missing)
	}

	if err := d.MigrateUpTo(ctx, agentClaimsVersion); err != nil {
		t.Fatalf("up again: %v", err)
	}
	if missing := missingAgentClaimsObjects(t, d); len(missing) > 0 {
		t.Fatalf("objects missing after migrating up again: %v", missing)
	}
}

func TestAgentClaimsConstraints(t *testing.T) {
	ctx := context.Background()
	d := dbtest.NewUnmigrated(t)
	if err := d.MigrateUpTo(ctx, agentClaimsVersion); err != nil {
		t.Fatal(err)
	}
	seedDashboard(t, d, agentClaimsDashboard, 1)
	sqlDB := d.SQLDB(t)
	insert := func(r assignedRequest) (string, error) {
		var id string
		err := sqlDB.QueryRow(insertAssignedRequestSQL, r.source, r.dashboardID, r.baseVersion, r.panelID, r.kind, r.assignedID).Scan(&id)
		return id, err
	}
	const assignee = "00000000-0000-0000-0000-0000000000a1"

	id, err := insert(assignedRequest{source: "observe"})
	if err != nil {
		t.Fatal(err)
	}
	var kind string
	var assignedNull, claimedNull bool
	if err := sqlDB.QueryRow(`SELECT assigned_kind, assigned_id IS NULL, claimed_by IS NULL FROM agent_requests WHERE id = $1`, id).Scan(&kind, &assignedNull, &claimedNull); err != nil {
		t.Fatal(err)
	}
	if kind != "builtin" || !assignedNull || !claimedNull {
		t.Fatalf("an unassigned request reads back kind %q, assigned_id null %v, claimed_by null %v", kind, assignedNull, claimedNull)
	}

	assignments := []struct {
		name string
		row  assignedRequest
		want string
	}{
		{"a grant with an id", assignedRequest{source: "observe", kind: "grant", assignedID: assignee}, ""},
		{"a service account with an id", assignedRequest{source: "observe", kind: "service_account", assignedID: assignee}, ""},
		{"a grant with no id", assignedRequest{source: "observe", kind: "grant"}, "agent_requests_assigned_check"},
		{"the built-in agent with an id", assignedRequest{source: "observe", kind: "builtin", assignedID: assignee}, "agent_requests_assigned_check"},
		{"an unknown assignee kind", assignedRequest{source: "observe", kind: "other", assignedID: assignee}, "agent_requests_assigned_kind_check"},
		{"a suggestion for a grant", assignedRequest{source: "suggest", dashboardID: agentClaimsDashboard, baseVersion: 1, panelID: "p1", kind: "grant", assignedID: assignee}, "agent_requests_assigned_source_check"},
		{"a suggestion for the built-in agent", assignedRequest{source: "suggest", dashboardID: agentClaimsDashboard, baseVersion: 1, panelID: "p1"}, ""},
	}
	for _, c := range assignments {
		_, err := insert(c.row)
		if got := constraintOf(err); got != c.want || (c.want == "" && err != nil) {
			t.Errorf("%s: error = %v, want violation %q", c.name, err, c.want)
		}
	}

	if _, err := sqlDB.Exec(`UPDATE agent_requests SET claimed_by = '' WHERE id = $1`, id); constraintOf(err) != "agent_requests_claimed_by_check" {
		t.Errorf("an empty claimed_by: error = %v, want a violation of agent_requests_claimed_by_check", err)
	}

	if _, err := sqlDB.Exec(`INSERT INTO run_tokens (token_hash, agent_request_id, attempt, expires_at) VALUES (decode(repeat('aa', 31), 'hex'), $1, 1, now())`, id); constraintOf(err) != "run_tokens_token_hash_check" {
		t.Errorf("a 31-byte token hash: error = %v, want a violation of run_tokens_token_hash_check", err)
	}
	if _, err := sqlDB.Exec(`INSERT INTO run_tokens (token_hash, agent_request_id, attempt, expires_at) VALUES (sha256('a'::bytea), $1, 1, now())`, id); err != nil {
		t.Fatalf("a 32-byte token hash was refused: %v", err)
	}
	if _, err := sqlDB.Exec(`DELETE FROM agent_requests WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	var tokens int
	if err := sqlDB.QueryRow(`SELECT count(*) FROM run_tokens`).Scan(&tokens); err != nil {
		t.Fatal(err)
	}
	if tokens != 0 {
		t.Fatalf("%d run tokens survive the deletion of their request", tokens)
	}

	if _, err := sqlDB.Exec(`INSERT INTO agent_workers (id, principal_kind) VALUES ('grant:x', 'grant')`); constraintOf(err) != "agent_workers_principal_check" {
		t.Errorf("a grant worker with no principal id: error = %v, want a violation of agent_workers_principal_check", err)
	}
	if _, err := sqlDB.Exec(`INSERT INTO agent_workers (id, principal_kind) VALUES ('builtin:s1', 'builtin')`); err != nil {
		t.Errorf("a built-in worker was refused: %v", err)
	}

	var userID, grantID, mode string
	if err := sqlDB.QueryRow(`INSERT INTO users (oidc_issuer, oidc_subject, email) VALUES ('i', 's', 'a@example.com') RETURNING id`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec(`INSERT INTO oauth_clients (id, redirect_uris) VALUES ('c', ARRAY['http://localhost/cb'])`); err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.QueryRow(`INSERT INTO oauth_grants (client_id, user_id, resource, expires_at) VALUES ('c', $1, 'r', now() + interval '1 day') RETURNING id, mode`, userID).Scan(&grantID, &mode); err != nil {
		t.Fatal(err)
	}
	if mode != "direct" {
		t.Errorf("a new grant reads back mode %q, want direct", mode)
	}
	if _, err := sqlDB.Exec(`UPDATE oauth_grants SET mode = 'other' WHERE id = $1`, grantID); constraintOf(err) != "oauth_grants_mode_check" {
		t.Errorf("mode other: error = %v, want a violation of oauth_grants_mode_check", err)
	}

	var takesRuns bool
	if err := sqlDB.QueryRow(`INSERT INTO service_accounts (name, description, created_by, role) VALUES ('bot', '', $1, 'viewer') RETURNING takes_runs`, userID).Scan(&takesRuns); err != nil {
		t.Fatal(err)
	}
	if takesRuns {
		t.Error("a new service account takes runs by default")
	}
}

func missingAgentClaimsObjects(t *testing.T, d *dbtest.Database) []string {
	t.Helper()
	var missing []string
	for _, c := range agentClaimsColumns {
		var exists bool
		if err := d.SQLDB(t).QueryRow(`SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = $1 AND column_name = $2)`, c.table, c.column).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			missing = append(missing, c.table+"."+c.column)
		}
	}
	for _, table := range agentClaimsTables {
		var exists bool
		if err := d.SQLDB(t).QueryRow(`SELECT to_regclass($1) IS NOT NULL`, table).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			missing = append(missing, table)
		}
	}
	return missing
}
