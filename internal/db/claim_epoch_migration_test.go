package db_test

import (
	"context"
	"testing"
)

const claimEpochVersion = 101

const claimEpochColumnsSQL = `SELECT count(*)::text FROM information_schema.columns
WHERE column_name = 'claim_epoch' AND table_name IN ('agent_requests', 'run_tokens')`

func TestClaimEpochMigrationStartsLiveRunsAtTheirAttempt(t *testing.T) {
	ctx := context.Background()
	d := newMigratedTo(t, claimEpochVersion-1)
	sqlDB := d.SQLDB(t)
	ada := convUser(t, sqlDB, "Ada")
	run := func(set string) string {
		id := foScalar(t, sqlDB, `INSERT INTO agent_requests (prompt, requested_by_kind, requested_by_id, requested_by_name, intent)
VALUES ('p', 'user', $1::uuid, 'Ada', 'ask') RETURNING id::text`, ada)
		if set != "" {
			d.Exec(t, `UPDATE agent_requests SET `+set+` WHERE id = $1::uuid`, id)
		}
		return id
	}
	running := run(`state = 'running', status = 'running', attempts = 2, started_at = clock_timestamp(), lease_until = clock_timestamp() + interval '1 hour'`)
	d.Exec(t, `INSERT INTO run_tokens (token_hash, agent_request_id, attempt, expires_at) VALUES (sha256('a'::bytea), $1::uuid, 2, now() + interval '1 hour')`, running)
	rows := []struct {
		name, id, want string
	}{
		{"a run nobody claimed", run(""), "0"},
		{"a run held on its second attempt", running, "2"},
		{"a run queued again with one attempt spent", run(`attempts = 1`), "1"},
		{"a run that failed after two attempts", run(`state = 'failed', status = 'failed', attempts = 2, started_at = now(), finished_at = now(), error = 'stopped', failure_code = 'no_progress'`), "0"},
	}

	if err := d.MigrateUpTo(ctx, claimEpochVersion); err != nil {
		t.Fatalf("up: %v", err)
	}
	for _, r := range rows {
		if got := foScalar(t, sqlDB, `SELECT claim_epoch::text FROM agent_requests WHERE id = $1::uuid`, r.id); got != r.want {
			t.Errorf("%s: claim_epoch = %s, want %s", r.name, got, r.want)
		}
	}
	if got := foScalar(t, sqlDB, `SELECT (claim_epoch IS NULL)::text FROM run_tokens WHERE agent_request_id = $1::uuid`, running); got != "true" {
		t.Errorf("a token minted before the migration: claim_epoch is null = %s, want true so the token keeps working", got)
	}
	if got := foScalar(t, sqlDB, `SELECT count(*)::text FROM run_tokens t JOIN agent_requests r ON r.id = t.agent_request_id
WHERE t.attempt = r.attempts AND (t.claim_epoch IS NULL OR t.claim_epoch = r.claim_epoch)`); got != "1" {
		t.Errorf("tokens that still match their run = %s, want 1", got)
	}

	if err := d.MigrateDownTo(ctx, claimEpochVersion-1); err != nil {
		t.Fatalf("down: %v", err)
	}
	if got := foScalar(t, sqlDB, claimEpochColumnsSQL); got != "0" {
		t.Errorf("claim_epoch columns left after down = %s, want none", got)
	}
	if err := d.MigrateUpTo(ctx, claimEpochVersion); err != nil {
		t.Fatalf("up again: %v", err)
	}
	if got := foScalar(t, sqlDB, claimEpochColumnsSQL); got != "2" {
		t.Errorf("claim_epoch columns after migrating up again = %s, want 2", got)
	}
}
