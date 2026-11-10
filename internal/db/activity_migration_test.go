package db_test

import (
	"context"
	"testing"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

const activityVersion = 27

func TestActivityDown(t *testing.T) {
	ctx := context.Background()
	d := dbtest.NewUnmigrated(t)
	if err := d.MigrateUpTo(ctx, activityVersion); err != nil {
		t.Fatal(err)
	}
	if !activityTableExists(t, d) {
		t.Fatal("activity table missing after the up migration")
	}

	if err := d.MigrateDownTo(ctx, activityVersion-1); err != nil {
		t.Fatalf("down: %v", err)
	}
	if activityTableExists(t, d) {
		t.Fatal("activity table still exists after the down migration")
	}

	if err := d.MigrateUpTo(ctx, activityVersion); err != nil {
		t.Fatalf("up again: %v", err)
	}
	if !activityTableExists(t, d) {
		t.Fatal("activity table missing after migrating up again")
	}
}

func activityTableExists(t *testing.T, d *dbtest.Database) bool {
	t.Helper()
	var exists bool
	if err := d.SQLDB(t).QueryRow(`SELECT to_regclass('activity') IS NOT NULL`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	return exists
}
