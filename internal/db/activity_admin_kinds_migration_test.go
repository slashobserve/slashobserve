package db_test

import (
	"context"
	"testing"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

const activityAdminKindsVersion = 59

const insertAdminActivity = `INSERT INTO activity (type, target_kind, target_id, actor_kind, actor_id, actor_name, data)
VALUES ('webhook.deleted', 'webhook', gen_random_uuid(), 'user', gen_random_uuid(), 'Root', '{}'::jsonb)`

func TestActivityAdminKindsRollBackAndReapply(t *testing.T) {
	ctx := context.Background()
	d := dbtest.NewUnmigrated(t)
	if err := d.MigrateUpTo(ctx, activityAdminKindsVersion-1); err != nil {
		t.Fatal(err)
	}
	if _, err := d.SQLDB(t).Exec(insertAdminActivity); err == nil {
		t.Fatal("an admin target kind was accepted before the migration")
	}
	if err := d.MigrateUpTo(ctx, activityAdminKindsVersion); err != nil {
		t.Fatal(err)
	}
	if _, err := d.SQLDB(t).Exec(insertAdminActivity); err != nil {
		t.Fatalf("an admin target kind is refused after the migration: %v", err)
	}
	if err := d.MigrateDownTo(ctx, activityAdminKindsVersion-1); err != nil {
		t.Fatalf("down: %v", err)
	}
	if got := foScalar(t, d.SQLDB(t), `SELECT count(*) FROM activity`); got != "0" {
		t.Errorf("%s admin rows survived the rollback", got)
	}
	if err := d.MigrateUpTo(ctx, activityAdminKindsVersion); err != nil {
		t.Fatalf("up again: %v", err)
	}
}
