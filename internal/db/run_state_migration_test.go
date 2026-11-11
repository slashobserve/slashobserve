package db_test

import (
	"context"
	"testing"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

const (
	runStateVersion   = 35
	runStateDashboard = "00000000-0000-4000-8000-0000000000a1"
)

func TestRunStateBackfill(t *testing.T) {
	ctx := context.Background()
	d := dbtest.NewUnmigrated(t)
	if err := d.MigrateUpTo(ctx, runStateVersion-1); err != nil {
		t.Fatal(err)
	}
	seedDashboard(t, d, runStateDashboard, 3)
	sqlDB := d.SQLDB(t)

	cases := []struct {
		name       string
		insert     string
		wantState  string
		wantResult string
	}{
		{
			"queued", `INSERT INTO agent_requests (prompt, requested_by_kind, requested_by_id, requested_by_name, source, status)
VALUES ('queued', 'user', gen_random_uuid(), 'seed', 'observe', 'queued')`,
			"queued", "none",
		},
		{
			"running with a live lease", `INSERT INTO agent_requests (prompt, requested_by_kind, requested_by_id, requested_by_name, source, status, attempts, lease_until)
VALUES ('running', 'user', gen_random_uuid(), 'seed', 'observe', 'running', 1, now() + interval '1 minute')`,
			"running", "none",
		},
		{
			"running with a cancel requested", `INSERT INTO agent_requests (prompt, requested_by_kind, requested_by_id, requested_by_name, source, status, attempts, lease_until,
  cancel_requested_at, canceled_by_kind, canceled_by_id, canceled_by_name)
VALUES ('canceling', 'user', gen_random_uuid(), 'seed', 'observe', 'running', 1, now() + interval '1 minute', now(), 'user', gen_random_uuid(), 'seed')`,
			"canceling", "none",
		},
		{
			"succeeded with a version", `INSERT INTO agent_requests (prompt, requested_by_kind, requested_by_id, requested_by_name, source, status, attempts,
  dashboard_id, base_version, result_version, finished_at)
VALUES ('succeeded', 'user', gen_random_uuid(), 'seed', 'prompt', 'succeeded', 1, '` + runStateDashboard + `', 2, 3, now())`,
			"succeeded", "version",
		},
		{
			"failed after a lost lease", `INSERT INTO agent_requests (prompt, requested_by_kind, requested_by_id, requested_by_name, source, status, attempts, failure_code, error, finished_at)
VALUES ('lost', 'user', gen_random_uuid(), 'seed', 'observe', 'failed', 3, 'lease_expired', 'agent run did not finish', now())`,
			"failed", "none",
		},
		{
			"failed as canceled", `INSERT INTO agent_requests (prompt, requested_by_kind, requested_by_id, requested_by_name, source, status, attempts, failure_code, error, finished_at,
  cancel_requested_at, canceled_by_kind, canceled_by_id, canceled_by_name)
VALUES ('canceled', 'user', gen_random_uuid(), 'seed', 'observe', 'failed', 1, 'canceled', 'canceled', now(), now(), 'user', gen_random_uuid(), 'seed')`,
			"canceled", "none",
		},
		{
			"failed with a kept build", `INSERT INTO agent_requests (prompt, requested_by_kind, requested_by_id, requested_by_name, source, status, attempts, failure_code, error, finished_at,
  dashboard_id, base_version, result_version, build_saved_at)
VALUES ('kept', 'user', gen_random_uuid(), 'seed', 'prompt', 'failed', 1, 'budget_exhausted', 'stopped', now(), '` + runStateDashboard + `', 2, 3, now())`,
			"failed", "partial_version",
		},
		{
			"suggest that answered", `INSERT INTO agent_requests (prompt, requested_by_kind, requested_by_id, requested_by_name, source, status, attempts,
  dashboard_id, base_version, panel_id, suggestion, finished_at)
VALUES ('suggest', 'user', gen_random_uuid(), 'seed', 'suggest', 'succeeded', 1, '` + runStateDashboard + `', 3, 'p1', '{}'::jsonb, now())`,
			"succeeded", "suggestion",
		},
		{
			"investigation with a finding", `INSERT INTO agent_requests (prompt, requested_by_kind, requested_by_id, requested_by_name, source, status, attempts, finished_at)
VALUES ('investigate', 'user', gen_random_uuid(), 'seed', 'investigate', 'succeeded', 1, now())`,
			"succeeded", "observation",
		},
	}
	ids := make([]string, len(cases))
	for i, c := range cases {
		if err := sqlDB.QueryRow(c.insert + ` RETURNING id`).Scan(&ids[i]); err != nil {
			t.Fatalf("%s: insert: %v", c.name, err)
		}
	}
	if _, err := sqlDB.Exec(`INSERT INTO findings (agent_request_id, title, markdown, time_from, time_to, created_by_kind, created_by_id, created_by_name)
VALUES ($1, 't', 'm', now() - interval '1 hour', now(), 'user', gen_random_uuid(), 'seed')`, ids[len(ids)-1]); err != nil {
		t.Fatal(err)
	}

	if err := d.MigrateUpTo(ctx, runStateVersion); err != nil {
		t.Fatal(err)
	}
	for i, c := range cases {
		var state, result string
		if err := sqlDB.QueryRow(`SELECT state, result_kind FROM agent_requests WHERE id = $1`, ids[i]).Scan(&state, &result); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if state != c.wantState || result != c.wantResult {
			t.Errorf("%s: state %q result_kind %q, want %q and %q", c.name, state, result, c.wantState, c.wantResult)
		}
	}
}

func TestRunStateConstraints(t *testing.T) {
	ctx := context.Background()
	d := dbtest.NewUnmigrated(t)
	if err := d.MigrateUpTo(ctx, runStateVersion); err != nil {
		t.Fatal(err)
	}
	sqlDB := d.SQLDB(t)
	cases := []struct {
		name, state, status string
		failureCode         any
		want                string
	}{
		{"queued with a matching status", "queued", "queued", nil, ""},
		{"running state with a queued status", "running", "queued", nil, "agent_requests_state_status_check"},
		{"failed state carrying the canceled code", "failed", "failed", "canceled", "agent_requests_state_status_check"},
		{"canceled state without the canceled code", "canceled", "failed", "lease_expired", "agent_requests_state_status_check"},
		{"canceling without a cancel request", "canceling", "running", nil, "agent_requests_canceling_check"},
		{"an unknown state", "paused", "queued", nil, "agent_requests_state_check"},
	}
	for _, c := range cases {
		_, err := sqlDB.Exec(`INSERT INTO agent_requests (prompt, requested_by_kind, requested_by_id, requested_by_name, source, state, status, failure_code)
VALUES ('p', 'user', gen_random_uuid(), 'seed', 'observe', $1, $2, $3)`, c.state, c.status, c.failureCode)
		if got := constraintOf(err); got != c.want || (c.want == "" && err != nil) {
			t.Errorf("%s: error = %v, want violation %q", c.name, err, c.want)
		}
	}
}

func TestRunStateDown(t *testing.T) {
	ctx := context.Background()
	d := dbtest.NewUnmigrated(t)
	if err := d.MigrateUpTo(ctx, runStateVersion); err != nil {
		t.Fatal(err)
	}
	if got := runStateColumns(t, d); got != 2 {
		t.Fatalf("columns after up = %d, want 2", got)
	}
	if err := d.MigrateDownTo(ctx, runStateVersion-1); err != nil {
		t.Fatalf("down: %v", err)
	}
	if got := runStateColumns(t, d); got != 0 {
		t.Fatalf("columns after down = %d, want 0", got)
	}
	if err := d.MigrateUpTo(ctx, runStateVersion); err != nil {
		t.Fatalf("up again: %v", err)
	}
	if got := runStateColumns(t, d); got != 2 {
		t.Fatalf("columns after up again = %d, want 2", got)
	}
}

func runStateColumns(t *testing.T, d *dbtest.Database) int {
	t.Helper()
	var n int
	if err := d.SQLDB(t).QueryRow(`SELECT count(*) FROM information_schema.columns
WHERE table_name = 'agent_requests' AND column_name IN ('state', 'result_kind')`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
