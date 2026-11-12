package db_test

import (
	"context"
	"testing"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

const inboxVersion = 43

func inboxObjects(t *testing.T, d *dbtest.Database) string {
	t.Helper()
	return foScalar(t, d.SQLDB(t), `SELECT concat_ws(',',
  coalesce(to_regclass('inbox_cursors')::text, '-'),
  coalesce(to_regclass('activity_inbox_idx')::text, '-'),
  coalesce(to_regprocedure('inbox_reason(activity, uuid, uuid[])')::text, '-'))`)
}

func TestInboxCursorsFollowTheirUser(t *testing.T) {
	ctx := context.Background()
	d := dbtest.NewUnmigrated(t)
	if err := d.MigrateUpTo(ctx, inboxVersion); err != nil {
		t.Fatal(err)
	}
	sqlDB := d.SQLDB(t)
	user := foScalar(t, sqlDB, `INSERT INTO users (oidc_issuer, oidc_subject, email, name) VALUES ('https://idp', gen_random_uuid()::text, 'ada@example.com', 'Ada') RETURNING id::text`)
	d.Exec(t, `INSERT INTO inbox_cursors (user_id, read_xid, read_seq) VALUES ($1::uuid, pg_current_xact_id(), 7)`, user)
	if got := foScalar(t, sqlDB, `SELECT count(*)::text FROM inbox_cursors`); got != "1" {
		t.Fatalf("%s cursors, want 1", got)
	}
	d.Exec(t, `DELETE FROM users WHERE id = $1::uuid`, user)
	if got := foScalar(t, sqlDB, `SELECT count(*)::text FROM inbox_cursors`); got != "0" {
		t.Errorf("%s cursors after the user was deleted, want 0", got)
	}
}

func TestInboxRollsBackAndReapplies(t *testing.T) {
	ctx := context.Background()
	d := dbtest.NewUnmigrated(t)
	if err := d.MigrateUpTo(ctx, inboxVersion); err != nil {
		t.Fatal(err)
	}
	if got := inboxObjects(t, d); got != "inbox_cursors,activity_inbox_idx,inbox_reason(activity,uuid,uuid[])" {
		t.Fatalf("after up the objects are %q", got)
	}
	if err := d.MigrateDownTo(ctx, inboxVersion-1); err != nil {
		t.Fatalf("down: %v", err)
	}
	if got := inboxObjects(t, d); got != "-,-,-" {
		t.Errorf("after down the objects are %q, want none", got)
	}
	if err := d.MigrateUpTo(ctx, inboxVersion); err != nil {
		t.Fatalf("up again: %v", err)
	}
	if got := inboxObjects(t, d); got != "inbox_cursors,activity_inbox_idx,inbox_reason(activity,uuid,uuid[])" {
		t.Errorf("after up again the objects are %q", got)
	}
}
