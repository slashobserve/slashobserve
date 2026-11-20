package db_test

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

const (
	historyYears       = 10
	futureTarget       = "99999999-9999-4999-8999-999999999999"
	maxMigrationLocks  = 2000
	maxHistoryMonthly  = historyYears*12 + 1
	auditMigrationFile = "migrations/00074_audit_events.sql"
)

func withYearsOfActivity(t *testing.T) *dbtest.Database {
	t.Helper()
	d := newMigratedTo(t, auditEventsVersion-1)
	d.Exec(t, `INSERT INTO activity (type, target_kind, target_id, actor_kind, actor_id, actor_name, origin, data, created_at)
SELECT 'dashboard.created', 'dashboard', gen_random_uuid(), 'user', gen_random_uuid(), 'Ada', 'web', '{"title": "Checkout"}', at
FROM generate_series(now() - make_interval(years => $1), now(), interval '1 day') AS at`, historyYears)
	d.Exec(t, `INSERT INTO activity (type, target_kind, target_id, actor_kind, actor_id, actor_name, origin, data, created_at)
VALUES ('dashboard.created', 'dashboard', $1, 'user', gen_random_uuid(), 'Ada', 'web', '{}', now() + interval '30 days')`, futureTarget)
	return d
}

func TestTheAuditBackfillPutsYearsOfHistoryInMonthlyPartitions(t *testing.T) {
	ctx := context.Background()
	d := withYearsOfActivity(t)
	if err := d.MigrateUpTo(ctx, auditEventsVersion); err != nil {
		t.Fatalf("up: %v", err)
	}
	var monthly, daily, wantDaily, backfilled, future, historic int
	err := d.SQLDB(t).QueryRowContext(ctx, `SELECT
  (SELECT count(*) FROM pg_catalog.pg_inherits i JOIN pg_catalog.pg_class c ON c.oid = i.inhrelid WHERE i.inhparent = 'audit_events'::regclass AND c.relname ~ '^audit_events_[0-9]{6}$'),
  (SELECT count(*) FROM pg_catalog.pg_inherits i JOIN pg_catalog.pg_class c ON c.oid = i.inhrelid WHERE i.inhparent = 'audit_events'::regclass AND c.relname ~ '^audit_events_[0-9]{8}$'),
  ((now() AT TIME ZONE 'UTC')::date + 14) - ((now() - interval '1 hour') AT TIME ZONE 'UTC')::date + 1,
  (SELECT count(*) FROM audit_events WHERE server_id = 'backfill'),
  (SELECT count(*) FROM audit_events WHERE target_id = $1),
  (SELECT count(*) FROM activity WHERE target_id <> $1::uuid)`, futureTarget).Scan(&monthly, &daily, &wantDaily, &backfilled, &future, &historic)
	if err != nil {
		t.Fatal(err)
	}
	if monthly == 0 || monthly > maxHistoryMonthly || daily != wantDaily || wantDaily < audit.DaysAhead+1 {
		t.Errorf("partitions: %d monthly, %d daily; want at most %d monthly and %d daily", monthly, daily, maxHistoryMonthly, wantDaily)
	}
	if backfilled != historic || future != 0 {
		t.Errorf("backfilled %d of %d historic rows and %d future-dated rows, want all historic and no future one", backfilled, historic, future)
	}
}

func TestTheAuditMigrationHoldsFewLocksOverYearsOfHistory(t *testing.T) {
	ctx := context.Background()
	d := withYearsOfActivity(t)
	up := auditMigrationUp(t)
	config, err := pgx.ParseConfig(d.URL())
	if err != nil {
		t.Fatal(err)
	}
	var notices []string
	config.OnNotice = func(_ *pgconn.PgConn, n *pgconn.Notice) { notices = append(notices, n.Message) }
	conn, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(ctx) }()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	start := time.Now()
	if _, err := conn.PgConn().Exec(ctx, up).ReadAll(); err != nil {
		t.Fatalf("migration body: %v", err)
	}
	took := time.Since(start)
	var locks int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM pg_catalog.pg_locks WHERE pid = pg_backend_pid()`).Scan(&locks); err != nil {
		t.Fatal(err)
	}
	t.Logf("migration over %d years of daily activity: %d locks held at commit, %s", historyYears, locks, took.Round(time.Millisecond))
	if locks > maxMigrationLocks {
		t.Fatalf("the migration holds %d locks, want at most %d", locks, maxMigrationLocks)
	}
	if !slices.ContainsFunc(notices, func(m string) bool { return strings.HasPrefix(m, "1 activity rows dated after ") }) {
		t.Errorf("notices = %q, want one counting the future-dated activity row", notices)
	}
}

func auditMigrationUp(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(auditMigrationFile)
	if err != nil {
		t.Fatal(err)
	}
	up, _, ok := strings.Cut(string(raw), "-- +goose Down")
	if !ok {
		t.Fatalf("%s has no Down section", auditMigrationFile)
	}
	return up
}
