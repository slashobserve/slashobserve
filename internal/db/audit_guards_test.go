package db_test

import (
	"testing"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

func TestTruncatingTheAuditTrailOrAPartitionIsRefused(t *testing.T) {
	d := dbtest.New(t)
	sqlDB := d.SQLDB(t)
	var today string
	if err := sqlDB.QueryRowContext(t.Context(), `SELECT 'audit_events_' || to_char(now() AT TIME ZONE 'UTC', 'YYYYMMDD')`).Scan(&today); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"audit_events", today} {
		if _, err := sqlDB.ExecContext(t.Context(), "TRUNCATE "+table); err == nil {
			t.Errorf("TRUNCATE %s succeeded, want it refused", table)
		}
	}
	var added bool
	if err := sqlDB.QueryRowContext(t.Context(), `SELECT audit_events_add_partition('2099-12-29')`).Scan(&added); err != nil || !added {
		t.Fatalf("add a partition: %v %v", added, err)
	}
	if _, err := sqlDB.ExecContext(t.Context(), "TRUNCATE audit_events_20991229"); err == nil {
		t.Error("TRUNCATE of a partition the job added succeeded, want it refused")
	}
}

func TestAddingAPartitionRefusesATableItDidNotCreate(t *testing.T) {
	d := dbtest.New(t)
	sqlDB := d.SQLDB(t)
	d.Exec(t, `CREATE TABLE audit_events_20991231 (LIKE audit_events INCLUDING DEFAULTS INCLUDING CONSTRAINTS INCLUDING GENERATED)`)
	d.Exec(t, `INSERT INTO audit_events_20991231 (id, occurred_at, category, action, outcome, actor_kind, initiator, origin, server_id, server_version)
VALUES (gen_random_uuid(), '2099-12-31 12:00:00+00', 'system', 'audit.gap', 'success', 'system', 'system', 'job', 'forged', 'forged')`)
	if _, err := sqlDB.ExecContext(t.Context(), `SELECT audit_events_add_partition('2099-12-31')`); err == nil {
		t.Fatal("a table someone else created was attached as a partition")
	}
	var forged int
	if err := sqlDB.QueryRowContext(t.Context(), `SELECT count(*) FROM audit_events WHERE server_id = 'forged'`).Scan(&forged); err != nil {
		t.Fatal(err)
	}
	if forged != 0 {
		t.Fatal("the forged rows became part of the audit trail")
	}
}
