package audit

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

type lockProbe struct {
	s storeRig
}

func (p lockProbe) connect(t *testing.T) *pgx.Conn {
	t.Helper()
	conn, err := pgx.Connect(t.Context(), p.s.db.URL())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	return conn
}

func (p lockProbe) pid(t *testing.T, conn *pgx.Conn) int32 {
	t.Helper()
	var pid int32
	if err := conn.QueryRow(t.Context(), "SELECT pg_backend_pid()").Scan(&pid); err != nil {
		t.Fatal(err)
	}
	return pid
}

func (p lockProbe) locksOf(t *testing.T, observer *pgx.Conn, pid int32) string {
	t.Helper()
	rows, err := observer.Query(t.Context(), `SELECT coalesce(c.relname, l.locktype) || ' ' || l.mode || CASE WHEN l.granted THEN '' ELSE ' (waiting)' END
FROM pg_catalog.pg_locks l LEFT JOIN pg_catalog.pg_class c ON c.oid = l.relation
WHERE l.pid = $1 AND (c.relname LIKE 'audit_events%' OR NOT l.granted)
ORDER BY 1`, pid)
	if err != nil {
		t.Fatal(err)
	}
	locks, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(locks, ", ")
}

func (p lockProbe) insertWithin(t *testing.T, conn *pgx.Conn, limit time.Duration) (time.Duration, error) {
	t.Helper()
	row, err := p.s.recorder.row(t.Context(), Event{Action: "dashboard.view"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), limit)
	defer cancel()
	start := time.Now()
	err = dbsqlc.New(conn).InsertAuditEvent(ctx, row)
	return time.Since(start), err
}

func (p lockProbe) openWriter(t *testing.T) pgx.Tx {
	t.Helper()
	tx, err := p.connect(t).Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	row, err := p.s.recorder.row(t.Context(), Event{Action: "dashboard.view"})
	if err != nil {
		t.Fatal(err)
	}
	if err := dbsqlc.New(tx).InsertAuditEvent(t.Context(), row); err != nil {
		t.Fatal(err)
	}
	return tx
}

func TestAddingAPartitionDoesNotStopWriters(t *testing.T) {
	p := lockProbe{s: newStoreRig(t, writerLimits{}, 0)}
	writer := p.openWriter(t)
	observer := p.connect(t)
	adder := p.connect(t)
	adderPID := p.pid(t, adder)
	day := utcDay(time.Now()).AddDate(0, 0, DaysAhead+5)

	tx, err := adder.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	q := dbsqlc.New(tx)
	if err := q.SetAuditLockTimeout(t.Context(), attachLockTimeout); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, err := q.AddAuditPartition(t.Context(), pgtype.Date{Time: day, Valid: true}); err != nil {
		t.Fatalf("CREATE TABLE (LIKE) and ATTACH PARTITION while a writer's transaction is open: %v", err)
	}
	t.Logf("CREATE TABLE (LIKE) + ATTACH PARTITION took %s while another transaction held an uncommitted insert", time.Since(start).Round(time.Millisecond))
	t.Logf("locks held by the adding transaction before commit: %s", p.locksOf(t, observer, adderPID))

	took, err := p.insertWithin(t, observer, time.Second)
	if err != nil {
		t.Fatalf("an insert while the attach is uncommitted: %v", err)
	}
	t.Logf("an insert into audit_events while the attach was uncommitted took %s", took.Round(time.Millisecond))
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := writer.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestDetachingConcurrentlyDoesNotStopWriters(t *testing.T) {
	p := lockProbe{s: newStoreRig(t, writerLimits{}, 0)}
	day := utcDay(time.Now()).AddDate(0, 0, -9)
	if err := p.s.recorder.addPartition(t.Context(), day); err != nil {
		t.Fatal(err)
	}
	writer := p.openWriter(t)
	observer := p.connect(t)
	detacher := p.connect(t)
	detacherPID := p.pid(t, detacher)

	done := make(chan error, 1)
	go func() {
		_, err := detacher.Exec(context.Background(), "ALTER TABLE audit_events DETACH PARTITION "+partitionName(day)+" CONCURRENTLY")
		done <- err
	}()

	var waiting string
	deadline := time.Now().Add(10 * time.Second)
	for waiting == "" && time.Now().Before(deadline) {
		select {
		case err := <-done:
			t.Logf("DETACH PARTITION CONCURRENTLY finished without waiting for the open writer: %v", err)
			return
		case <-time.After(20 * time.Millisecond):
		}
		if err := observer.QueryRow(t.Context(), `SELECT coalesce(wait_event_type || ':' || wait_event, '') FROM pg_catalog.pg_stat_activity WHERE pid = $1`, detacherPID).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
	}
	if waiting == "" {
		t.Fatal("DETACH PARTITION CONCURRENTLY never reported a wait")
	}
	t.Logf("DETACH PARTITION CONCURRENTLY waits on %s while a writer's transaction is open", waiting)
	t.Logf("locks held by the detaching session while it waits: %s", p.locksOf(t, observer, detacherPID))

	took, err := p.insertWithin(t, observer, time.Second)
	if err != nil {
		t.Fatalf("an insert while the detach waits: %v", err)
	}
	t.Logf("an insert into audit_events while the detach waited took %s", took.Round(time.Millisecond))

	if err := writer.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("detach after the writer committed: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the detach did not finish after the writer committed")
	}
	attached := p.s.partitions(t)
	for _, name := range attached {
		if name == partitionName(day) {
			t.Fatalf("%s is still attached", name)
		}
	}
	t.Logf("the detach finished once the writer committed")
}

func TestAnAttachThatCannotGetItsLockGivesUpWithinTheTimeout(t *testing.T) {
	p := lockProbe{s: newStoreRig(t, writerLimits{}, 0)}
	blocker, err := p.connect(t).Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = blocker.Rollback(context.Background()) }()
	if _, err := blocker.Exec(t.Context(), "LOCK TABLE audit_events IN ACCESS EXCLUSIVE MODE"); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	err = p.s.recorder.addPartition(t.Context(), utcDay(time.Now()).AddDate(0, 0, DaysAhead+7))
	took := time.Since(start)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "55P03" {
		t.Fatalf("an attach behind an exclusive lock: %v, want a lock timeout", err)
	}
	if took > 5*time.Second {
		t.Fatalf("the attach waited %s, want about %s", took, attachLockTimeout)
	}
	t.Logf("an attach behind an ACCESS EXCLUSIVE lock gave up after %s: %v", took.Round(time.Millisecond), err)
}
