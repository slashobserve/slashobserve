package db_test

import (
	"context"
	"testing"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

const inboxRepairedVersion = 45

func inboxRepairedObjects(t *testing.T, d *dbtest.Database) string {
	t.Helper()
	return foScalar(t, d.SQLDB(t), `SELECT concat_ws(',',
  coalesce(to_regprocedure('inbox_reason(activity, uuid, uuid[])')::text, '-'),
  coalesce(to_regprocedure('inbox_reason(activity, uuid, uuid[], text)')::text, '-'),
  CASE WHEN pg_get_indexdef('activity_inbox_idx'::regclass) LIKE '%run.succeeded%' THEN 'succeeded' ELSE 'no-succeeded' END)`)
}

func TestInboxRepairedRollsBackAndReapplies(t *testing.T) {
	ctx := context.Background()
	d := dbtest.NewUnmigrated(t)
	if err := d.MigrateUpTo(ctx, inboxRepairedVersion); err != nil {
		t.Fatal(err)
	}
	if got := inboxRepairedObjects(t, d); got != "-,inbox_reason(activity,uuid,uuid[],text),succeeded" {
		t.Fatalf("after up the objects are %q", got)
	}
	if err := d.MigrateDownTo(ctx, inboxRepairedVersion-1); err != nil {
		t.Fatalf("down: %v", err)
	}
	if got := inboxRepairedObjects(t, d); got != "inbox_reason(activity,uuid,uuid[]),-,no-succeeded" {
		t.Errorf("after down the objects are %q, want the inbox as it was", got)
	}
	if err := d.MigrateUpTo(ctx, inboxRepairedVersion); err != nil {
		t.Fatalf("up again: %v", err)
	}
	if got := inboxRepairedObjects(t, d); got != "-,inbox_reason(activity,uuid,uuid[],text),succeeded" {
		t.Errorf("after up again the objects are %q", got)
	}
}
