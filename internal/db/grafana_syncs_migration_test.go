package db_test

import (
	"context"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

const (
	grafanaSyncsVersion = 36
	syncA               = "00000000-0000-4000-8000-0000000000a1"
	syncB               = "00000000-0000-4000-8000-0000000000b2"
	syncCanceled        = "00000000-0000-4000-8000-0000000000c3"
	syncRunning         = "00000000-0000-4000-8000-0000000000d4"
	syncTimeLayout      = `YYYY-MM-DD"T"HH24:MI:SS.US"Z"`
)

func insertGrafanaConnection(t *testing.T, d *dbtest.Database, name, lastSync string) string {
	t.Helper()
	var id string
	err := d.SQLDB(t).QueryRow(`INSERT INTO grafana_connections (name, url, token_ciphertext, last_sync)
VALUES ($1, 'http://grafana.example', '\x01'::bytea, NULLIF($2, '')::jsonb) RETURNING id`, name, lastSync).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestGrafanaSyncsBackfill(t *testing.T) {
	ctx := context.Background()
	d := dbtest.NewUnmigrated(t)
	if err := d.MigrateUpTo(ctx, grafanaSyncsVersion-1); err != nil {
		t.Fatal(err)
	}
	connA := insertGrafanaConnection(t, d, "a", `{"id":"`+syncA+`","status":"finished","total":3,"done":3,"failed":1,"skipped":1,"started_at":"2026-11-01T10:00:00.123456789Z","finished_at":"2026-11-01T10:05:00Z"}`)
	connB := insertGrafanaConnection(t, d, "b", `{"id":"`+syncB+`","status":"failed","total":0,"done":0,"failed":0,"skipped":0,"error":"Grafana refused the token (status 401).","started_at":"2026-11-02T10:00:00Z","finished_at":"2026-11-02T10:00:01Z"}`)
	insertGrafanaConnection(t, d, "c", "")
	insertGrafanaConnection(t, d, "d", `{"id":"not-a-uuid","status":"finished","started_at":"2026-11-03T10:00:00Z"}`)
	if err := d.MigrateUpTo(ctx, grafanaSyncsVersion); err != nil {
		t.Fatal(err)
	}
	sqlDB := d.SQLDB(t)

	var rows int
	if err := sqlDB.QueryRow(`SELECT count(*) FROM grafana_syncs`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 2 {
		t.Fatalf("backfilled %d syncs, want 2", rows)
	}

	var (
		connection, state                      string
		total, done, failed, skipped           int
		errorIsNull, requestIsNull, leaseIsNil bool
		startedAt                              string
	)
	err := sqlDB.QueryRow(`SELECT connection_id, state, total, done, failed, skipped, error IS NULL, request IS NULL, lease_id IS NULL,
  to_char(started_at AT TIME ZONE 'UTC', '`+syncTimeLayout+`')
FROM grafana_syncs WHERE id = $1`, syncA).Scan(&connection, &state, &total, &done, &failed, &skipped, &errorIsNull, &requestIsNull, &leaseIsNil, &startedAt)
	if err != nil {
		t.Fatal(err)
	}
	if connection != connA || state != "finished" || total != 3 || done != 3 || failed != 1 || skipped != 1 {
		t.Fatalf("sync A = connection %s state %s counts %d %d %d %d", connection, state, total, done, failed, skipped)
	}
	if !errorIsNull || !requestIsNull || !leaseIsNil {
		t.Fatalf("sync A kept error, request or lease: error null %v, request null %v, lease null %v", errorIsNull, requestIsNull, leaseIsNil)
	}
	if startedAt != "2026-11-01T10:00:00.123457Z" {
		t.Fatalf("sync A started_at = %s", startedAt)
	}

	var message string
	err = sqlDB.QueryRow(`SELECT connection_id, state, error FROM grafana_syncs WHERE id = $1`, syncB).Scan(&connection, &state, &message)
	if err != nil {
		t.Fatal(err)
	}
	if connection != connB || state != "failed" || message != "Grafana refused the token (status 401)." {
		t.Fatalf("sync B = connection %s state %s error %q", connection, state, message)
	}
}

func TestGrafanaSyncsConstraints(t *testing.T) {
	ctx := context.Background()
	d := dbtest.NewUnmigrated(t)
	if err := d.MigrateUpTo(ctx, grafanaSyncsVersion); err != nil {
		t.Fatal(err)
	}
	sqlDB := d.SQLDB(t)

	const running = `INSERT INTO grafana_syncs (connection_id, state, request, started_by_kind, started_by_id, lease_id, leased_by, lease_until)
VALUES ($1, 'running', '{}', 'user', gen_random_uuid(), gen_random_uuid(), 's', now())`
	const finished = `INSERT INTO grafana_syncs (connection_id, state, finished_at) VALUES ($1, 'finished', now())`

	cases := []struct {
		name       string
		setup      string
		insert     string
		constraint string
	}{
		{"a second running sync for one connection", running, running, "grafana_syncs_running_idx"},
		{"a running sync without its request", "", `INSERT INTO grafana_syncs (connection_id, state) VALUES ($1, 'running')`, "grafana_syncs_resumable_check"},
		{"a finished sync without finished_at", "", `INSERT INTO grafana_syncs (connection_id, state) VALUES ($1, 'finished')`, "grafana_syncs_finished_check"},
		{"a finished sync holding a lease", "", `INSERT INTO grafana_syncs (connection_id, state, finished_at, lease_id, leased_by, lease_until)
VALUES ($1, 'finished', now(), gen_random_uuid(), 's', now())`, "grafana_syncs_lease_state_check"},
		{"a finished sync with an error", "", `INSERT INTO grafana_syncs (connection_id, state, finished_at, error) VALUES ($1, 'finished', now(), 'boom')`, "grafana_syncs_error_check"},
		{"three lease losses", "", `INSERT INTO grafana_syncs (connection_id, state, request, started_by_kind, started_by_id, lease_losses)
VALUES ($1, 'running', '{}', 'user', gen_random_uuid(), 3)`, "grafana_syncs_lease_losses_check"},
		{"a failed item without an error", finished, `INSERT INTO grafana_sync_items (sync_id, uid, title, outcome)
SELECT id, 'u', 't', 'failed' FROM grafana_syncs WHERE connection_id = $1`, "grafana_sync_items_error_check"},
		{"a second item with the same uid", `WITH s AS (INSERT INTO grafana_syncs (connection_id, state, finished_at) VALUES ($1, 'finished', now()) RETURNING id)
INSERT INTO grafana_sync_items (sync_id, uid, title, outcome) SELECT id, 'u', 't', 'imported' FROM s`, `INSERT INTO grafana_sync_items (sync_id, uid, title, outcome)
SELECT id, 'u', 't2', 'updated' FROM grafana_syncs WHERE connection_id = $1`, "grafana_sync_items_uid_key"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			connection := insertGrafanaConnection(t, d, c.name, "")
			if c.setup != "" {
				if _, err := sqlDB.Exec(c.setup, connection); err != nil {
					t.Fatalf("setup: %v", err)
				}
			}
			_, err := sqlDB.Exec(c.insert, connection)
			if err == nil {
				t.Fatal("the row was accepted")
			}
			if !strings.Contains(err.Error(), c.constraint) {
				t.Fatalf("error %q does not name %s", err, c.constraint)
			}
		})
	}

	t.Run("a finished sync beside the running one", func(t *testing.T) {
		connection := insertGrafanaConnection(t, d, "history", "")
		if _, err := sqlDB.Exec(running, connection); err != nil {
			t.Fatal(err)
		}
		if _, err := sqlDB.Exec(finished, connection); err != nil {
			t.Fatalf("a finished sync was refused: %v", err)
		}
	})
}

func TestGrafanaSyncsDown(t *testing.T) {
	ctx := context.Background()
	d := dbtest.NewUnmigrated(t)
	if err := d.MigrateUpTo(ctx, grafanaSyncsVersion-1); err != nil {
		t.Fatal(err)
	}
	connA := insertGrafanaConnection(t, d, "a", `{"id":"`+syncA+`","status":"finished","total":3,"done":3,"failed":1,"skipped":1,"started_at":"2026-11-01T10:00:00Z","finished_at":"2026-11-01T10:05:00Z"}`)
	connC := insertGrafanaConnection(t, d, "c", "")
	if err := d.MigrateUpTo(ctx, grafanaSyncsVersion); err != nil {
		t.Fatal(err)
	}
	sqlDB := d.SQLDB(t)
	if _, err := sqlDB.Exec(`INSERT INTO grafana_syncs (id, connection_id, state, total, done, started_at, finished_at)
VALUES ($1, $2, 'canceled', 5, 2, '2026-11-05T10:00:00Z', '2026-11-05T10:01:00Z')`, syncCanceled, connA); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec(`INSERT INTO grafana_syncs (id, connection_id, state, request, started_by_kind, started_by_id)
VALUES ($1, $2, 'running', '{}', 'user', gen_random_uuid())`, syncRunning, connC); err != nil {
		t.Fatal(err)
	}

	if err := d.MigrateDownTo(ctx, grafanaSyncsVersion-1); err != nil {
		t.Fatalf("down: %v", err)
	}
	var gone bool
	if err := sqlDB.QueryRow(`SELECT to_regclass('grafana_syncs') IS NULL AND to_regclass('grafana_sync_items') IS NULL`).Scan(&gone); err != nil {
		t.Fatal(err)
	}
	if !gone {
		t.Fatal("the sync tables still exist after the down migration")
	}
	var status string
	var hasError bool
	if err := sqlDB.QueryRow(`SELECT last_sync->>'status', last_sync ? 'error' FROM grafana_connections WHERE id = $1`, connA).Scan(&status, &hasError); err != nil {
		t.Fatal(err)
	}
	if status != "canceled" || hasError {
		t.Fatalf("connection A last_sync status %q, has error %v", status, hasError)
	}
	var lastSyncIsNull bool
	if err := sqlDB.QueryRow(`SELECT last_sync IS NULL FROM grafana_connections WHERE id = $1`, connC).Scan(&lastSyncIsNull); err != nil {
		t.Fatal(err)
	}
	if !lastSyncIsNull {
		t.Fatal("connection C gained a last_sync from a running sync")
	}

	if err := d.MigrateUpTo(ctx, grafanaSyncsVersion); err != nil {
		t.Fatalf("up again: %v", err)
	}
	var restored string
	if err := sqlDB.QueryRow(`SELECT state FROM grafana_syncs WHERE connection_id = $1`, connA).Scan(&restored); err != nil {
		t.Fatal(err)
	}
	if restored != "canceled" {
		t.Fatalf("backfill after the round trip = %q, want canceled", restored)
	}
}
