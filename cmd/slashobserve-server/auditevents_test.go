package main

import (
	"testing"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

type systemEvent struct {
	action, outcome, actorKind, initiator, origin, targetKind string
}

func TestStartingAndStoppingTheServerRecordsBothAndEveryMigrationItApplied(t *testing.T) {
	database := dbtest.NewUnmigrated(t)
	addr, exited := startServer(t, database.URL(), nil)
	serveThenTerminate(t, addr, exited)

	rows, err := database.SQLDB(t).QueryContext(t.Context(), `SELECT action, outcome, actor_kind, initiator, origin, coalesce(target_kind, '')
FROM audit_events WHERE action IN ('server.start', 'server.stop') ORDER BY occurred_at`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var got []systemEvent
	for rows.Next() {
		var e systemEvent
		if err := rows.Scan(&e.action, &e.outcome, &e.actorKind, &e.initiator, &e.origin, &e.targetKind); err != nil {
			t.Fatal(err)
		}
		got = append(got, e)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	want := []systemEvent{
		{"server.start", "success", "system", "system", "job", "server"},
		{"server.stop", "success", "system", "system", "job", "server"},
	}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("events = %+v, want %+v", got, want)
	}

	var applied int
	err = database.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM audit_events WHERE action = 'migration.apply' AND target_kind = 'migration' AND outcome = 'success'`).Scan(&applied)
	if err != nil {
		t.Fatal(err)
	}
	var migrations int
	if err := database.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM goose_db_version WHERE version_id > 0`).Scan(&migrations); err != nil {
		t.Fatal(err)
	}
	if applied != migrations || migrations == 0 {
		t.Fatalf("migration.apply events = %d, want one for each of the %d applied migrations", applied, migrations)
	}
}
