package db_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

const (
	runContextVersion   = 42
	runContextDashboard = "00000000-0000-4000-8000-0000000000a1"
)

type runContextFixture struct {
	sqlDB                           *sql.DB
	thread, message, run, grant, sa string
	user                            string
}

func runContextSeed(t *testing.T, d *dbtest.Database) runContextFixture {
	t.Helper()
	sqlDB := d.SQLDB(t)
	f := runContextFixture{sqlDB: sqlDB}
	f.user = foScalar(t, sqlDB, `INSERT INTO users (oidc_issuer, oidc_subject, email, name) VALUES ('https://idp', gen_random_uuid()::text, 'ada@example.com', 'Ada') RETURNING id::text`)
	d.Exec(t, `INSERT INTO oauth_clients (id, redirect_uris) VALUES ('c', ARRAY['http://localhost/cb'])`)
	f.grant = foScalar(t, sqlDB, `INSERT INTO oauth_grants (client_id, user_id, resource, expires_at) VALUES ('c', $1::uuid, 'http://localhost/mcp', now() + interval '1 day') RETURNING id::text`, f.user)
	f.sa = foScalar(t, sqlDB, `INSERT INTO service_accounts (name) VALUES ('ci-bot') RETURNING id::text`)
	seedDashboard(t, d, runContextDashboard, 1)
	seedVersion(t, d, runContextDashboard, 1, "{}")
	f.thread = foScalar(t, sqlDB, `INSERT INTO threads (dashboard_id, dashboard_version) VALUES ($1::uuid, 1) RETURNING id::text`, runContextDashboard)
	f.message = runContextMessage(t, sqlDB, f.thread)
	f.run = foID(900)
	foSeedRequest(t, d, f.run)
	return f
}

func runContextMessage(t *testing.T, sqlDB *sql.DB, thread string) string {
	t.Helper()
	return foScalar(t, sqlDB, `INSERT INTO thread_messages (thread_id, body, author_kind, author_id, author_name)
VALUES ($1::uuid, 'hello', 'user', gen_random_uuid(), 'Ada') RETURNING id::text`, thread)
}

func TestRunContextMentionsNameExactlyOneTarget(t *testing.T) {
	ctx := context.Background()
	d := dbtest.NewUnmigrated(t)
	if err := d.MigrateUpTo(ctx, runContextVersion); err != nil {
		t.Fatal(err)
	}
	f := runContextSeed(t, d)

	_, err := f.sqlDB.Exec(`INSERT INTO thread_mentions (message_id, user_id, agent_grant_id, name) VALUES ($1::uuid, $2::uuid, $3::uuid, 'x')`, f.message, f.user, f.grant)
	if constraintOf(err) != "thread_mentions_one_check" {
		t.Errorf("a mention of a person and a grant: error = %v, want a violation of thread_mentions_one_check", err)
	}
	_, err = f.sqlDB.Exec(`INSERT INTO thread_mentions (message_id, agent_grant_id, agent_service_account_id, name) VALUES ($1::uuid, $2::uuid, $3::uuid, 'x')`, f.message, f.grant, f.sa)
	if constraintOf(err) != "thread_mentions_one_check" {
		t.Errorf("a mention of a grant and a service account: error = %v, want a violation of thread_mentions_one_check", err)
	}
	_, err = f.sqlDB.Exec(`INSERT INTO thread_mentions (message_id, name) VALUES ($1::uuid, 'x')`, f.message)
	if constraintOf(err) != "thread_mentions_one_check" {
		t.Errorf("a mention of nobody: error = %v, want a violation of thread_mentions_one_check", err)
	}
	if _, err := f.sqlDB.Exec(`INSERT INTO thread_mentions (message_id, agent_service_account_id, name) VALUES ($1::uuid, $2::uuid, 'ci-bot')`, f.message, f.sa); err != nil {
		t.Errorf("a mention of a service account was refused: %v", err)
	}
	if _, err := f.sqlDB.Exec(`INSERT INTO thread_mentions (message_id, agent_grant_id, name) VALUES ($1::uuid, $2::uuid, 'Claude Code')`, f.message, f.grant); err != nil {
		t.Errorf("a mention of a grant was refused: %v", err)
	}
	if _, err := f.sqlDB.Exec(`INSERT INTO thread_mentions (message_id, agent, name) VALUES ($1::uuid, 'observe', 'observe')`, f.message); err != nil {
		t.Errorf("a mention of the built-in agent was refused: %v", err)
	}

	_, err = f.sqlDB.Exec(`INSERT INTO thread_mentions (message_id, agent_grant_id, name) VALUES ($1::uuid, $2::uuid, 'again')`, f.message, f.grant)
	if constraintOf(err) != "thread_mentions_grant_key" {
		t.Errorf("a second mention of one grant: error = %v, want a violation of thread_mentions_grant_key", err)
	}
	_, err = f.sqlDB.Exec(`INSERT INTO thread_mentions (message_id, agent_service_account_id, name) VALUES ($1::uuid, $2::uuid, 'again')`, f.message, f.sa)
	if constraintOf(err) != "thread_mentions_service_account_key" {
		t.Errorf("a second mention of one service account: error = %v, want a violation of thread_mentions_service_account_key", err)
	}
	other := runContextMessage(t, f.sqlDB, f.thread)
	if _, err := f.sqlDB.Exec(`INSERT INTO thread_mentions (message_id, agent_grant_id, name) VALUES ($1::uuid, $2::uuid, 'Claude Code')`, other, f.grant); err != nil {
		t.Errorf("the same grant on another message was refused: %v", err)
	}
}

func TestRunContextDeletesFollowTheirOwners(t *testing.T) {
	ctx := context.Background()
	d := dbtest.NewUnmigrated(t)
	if err := d.MigrateUpTo(ctx, runContextVersion); err != nil {
		t.Fatal(err)
	}
	f := runContextSeed(t, d)

	d.Exec(t, `INSERT INTO thread_mentions (message_id, agent_grant_id, name) VALUES ($1::uuid, $2::uuid, 'Claude Code')`, f.message, f.grant)
	d.Exec(t, `INSERT INTO thread_mentions (message_id, agent_service_account_id, name) VALUES ($1::uuid, $2::uuid, 'ci-bot')`, f.message, f.sa)
	d.Exec(t, `DELETE FROM oauth_grants WHERE id = $1::uuid`, f.grant)
	if got := foScalar(t, f.sqlDB, `SELECT count(*)::text FROM thread_mentions WHERE message_id = $1::uuid AND agent_grant_id IS NOT NULL`, f.message); got != "0" {
		t.Errorf("%s grant mentions left after the grant was deleted, want 0", got)
	}
	d.Exec(t, `DELETE FROM service_accounts WHERE id = $1::uuid`, f.sa)
	if got := foScalar(t, f.sqlDB, `SELECT count(*)::text FROM thread_mentions WHERE message_id = $1::uuid`, f.message); got != "0" {
		t.Errorf("%s mentions left after the service account was deleted, want 0", got)
	}

	d.Exec(t, `UPDATE agent_requests SET thread_message_id = $1::uuid, thread_context = 'earlier messages' WHERE id = $2::uuid`, f.message, f.run)
	d.Exec(t, `DELETE FROM thread_messages WHERE id = $1::uuid`, f.message)
	if got := foScalar(t, f.sqlDB, `SELECT coalesce(thread_message_id::text, 'null') || '|' || thread_context FROM agent_requests WHERE id = $1::uuid`, f.run); got != "null|earlier messages" {
		t.Errorf("the run after its message was deleted holds %q, want %q", got, "null|earlier messages")
	}

	written := runContextMessage(t, f.sqlDB, f.thread)
	d.Exec(t, `UPDATE thread_messages SET run_id = $1::uuid WHERE id = $2::uuid`, f.run, written)
	d.Exec(t, `DELETE FROM agent_requests WHERE id = $1::uuid`, f.run)
	if got := foScalar(t, f.sqlDB, `SELECT coalesce(run_id::text, 'null') FROM thread_messages WHERE id = $1::uuid`, written); got != "null" {
		t.Errorf("run_id of a message after its run was deleted is %q, want null", got)
	}
}

func TestRunContextBoundsThreadContext(t *testing.T) {
	ctx := context.Background()
	d := dbtest.NewUnmigrated(t)
	if err := d.MigrateUpTo(ctx, runContextVersion); err != nil {
		t.Fatal(err)
	}
	f := runContextSeed(t, d)

	cases := []struct {
		name    string
		context string
		want    string
	}{
		{"one character", "x", ""},
		{"8000 characters", strings.Repeat("x", 8000), ""},
		{"8001 characters", strings.Repeat("x", 8001), "agent_requests_thread_context_check"},
		{"empty", "", "agent_requests_thread_context_check"},
	}
	for _, tc := range cases {
		_, err := f.sqlDB.Exec(`UPDATE agent_requests SET thread_context = $1 WHERE id = $2::uuid`, tc.context, f.run)
		if got := constraintOf(err); got != tc.want {
			t.Errorf("%s: violated %q (error %v), want %q", tc.name, got, err, tc.want)
		}
	}
	if _, err := f.sqlDB.Exec(`UPDATE agent_requests SET thread_context = NULL WHERE id = $1::uuid`, f.run); err != nil {
		t.Errorf("a null thread_context was refused: %v", err)
	}
}

func TestRunContextRollsBackAndReapplies(t *testing.T) {
	ctx := context.Background()
	d := dbtest.NewUnmigrated(t)
	if err := d.MigrateUpTo(ctx, runContextVersion); err != nil {
		t.Fatal(err)
	}
	f := runContextSeed(t, d)
	d.Exec(t, `INSERT INTO thread_mentions (message_id, user_id, name) VALUES ($1::uuid, $2::uuid, 'Ada')`, f.message, f.user)
	d.Exec(t, `INSERT INTO thread_mentions (message_id, agent_grant_id, name) VALUES ($1::uuid, $2::uuid, 'Claude Code')`, f.message, f.grant)
	d.Exec(t, `UPDATE agent_requests SET thread_message_id = $1::uuid, thread_context = 'earlier' WHERE id = $2::uuid`, f.message, f.run)
	written := runContextMessage(t, f.sqlDB, f.thread)
	d.Exec(t, `UPDATE thread_messages SET run_id = $1::uuid WHERE id = $2::uuid`, f.run, written)

	if err := d.MigrateDownTo(ctx, runContextVersion-1); err != nil {
		t.Fatalf("down: %v", err)
	}
	if got := foScalar(t, f.sqlDB, `SELECT string_agg(name, ',') FROM thread_mentions WHERE message_id = $1::uuid`, f.message); got != "Ada" {
		t.Errorf("mentions after the rollback are %q, want only the person's", got)
	}
	columns := foScalar(t, f.sqlDB, `SELECT count(*)::text FROM information_schema.columns
WHERE (table_name = 'agent_requests' AND column_name IN ('thread_message_id', 'thread_context'))
   OR (table_name = 'thread_messages' AND column_name = 'run_id')
   OR (table_name = 'thread_mentions' AND column_name IN ('agent_grant_id', 'agent_service_account_id'))`)
	if columns != "0" {
		t.Errorf("%s added columns remain after the rollback, want 0", columns)
	}
	_, err := f.sqlDB.Exec(`INSERT INTO thread_mentions (message_id, user_id, agent, name) VALUES ($1::uuid, $2::uuid, 'observe', 'x')`, written, f.user)
	if constraintOf(err) != "thread_mentions_one_check" {
		t.Errorf("after the rollback a two-target mention: error = %v, want a violation of thread_mentions_one_check", err)
	}

	if err := d.MigrateUpTo(ctx, runContextVersion); err != nil {
		t.Fatalf("up again: %v", err)
	}
	if _, err := f.sqlDB.Exec(`INSERT INTO thread_mentions (message_id, agent_grant_id, name) VALUES ($1::uuid, $2::uuid, 'Claude Code')`, f.message, f.grant); err != nil {
		t.Errorf("a grant mention after re-applying: %v", err)
	}
}
