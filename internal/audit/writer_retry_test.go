package audit

import (
	"database/sql"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func (s storeRig) stallWrites(t *testing.T) (release func()) {
	t.Helper()
	tx, err := s.db.SQLDB(t).BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(t.Context(), "LOCK TABLE audit_events IN ACCESS EXCLUSIVE MODE"); err != nil {
		t.Fatal(err)
	}
	return func() { releaseStall(t, tx) }
}

func releaseStall(t *testing.T, tx *sql.Tx) {
	t.Helper()
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
}

func (s storeRig) views(t *testing.T, n int) []dbsqlc.InsertAuditEventParams {
	t.Helper()
	rows := make([]dbsqlc.InsertAuditEventParams, n)
	for i := range rows {
		row, err := s.recorder.row(t.Context(), Event{Action: "dashboard.view", Target: Target{Kind: "dashboard", ID: "d1"}})
		if err != nil {
			t.Fatal(err)
		}
		rows[i] = row
	}
	return rows
}

func (s storeRig) waitForQueueToEmpty(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for len(s.recorder.writer.queue) > 0 {
		if time.Now().After(deadline) {
			t.Fatal("the writer never took the queued events")
		}
		time.Sleep(time.Millisecond)
	}
}

func (s storeRig) waitForEvents(t *testing.T, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for s.count(t, "action = 'dashboard.view'") < want {
		if time.Now().After(deadline) {
			t.Fatalf("stored views = %d, want %d", s.count(t, "action = 'dashboard.view'"), want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestABatchThatTimesOutIsWrittenOnceTheDatabaseRecovers(t *testing.T) {
	s := newStoreRig(t, writerLimits{batch: 3, every: 20 * time.Millisecond, timeout: 100 * time.Millisecond}, 0)
	release := s.stallWrites(t)
	stop := s.run(t)
	defer stop()
	s.enqueueViews(t, 3)
	s.waitForBatch(t)
	release()
	s.waitForEvents(t, 3)
	if got := s.metrics.droppedFor(DroppedWriteFailed); got != 0 {
		t.Fatalf("dropped %s = %d, want 0", DroppedWriteFailed, got)
	}
	if n := s.count(t, "action = 'audit.gap'"); n != 0 {
		t.Fatalf("gap events = %d, want 0", n)
	}
}

func TestEventsArrivingDuringAStallAreKeptUpToTheBuffer(t *testing.T) {
	s := newStoreRig(t, writerLimits{batch: 2, buffer: 2, every: 20 * time.Millisecond, timeout: 100 * time.Millisecond, wait: time.Millisecond}, 0)
	release := s.stallWrites(t)
	stop := s.run(t)
	defer stop()
	s.enqueueViews(t, 2)
	s.waitForBatch(t)
	s.enqueueViews(t, 2)
	s.waitForQueueToEmpty(t)
	s.enqueueViews(t, 4)
	if got := s.metrics.droppedFor(DroppedBufferFull); got != 2 {
		t.Fatalf("dropped %s = %d, want the 2 that found the buffer full", DroppedBufferFull, got)
	}
	release()
	s.waitForEvents(t, 6)
	if n := s.count(t, "action = 'audit.gap' AND (data->>'lost')::int = 2"); n != 1 {
		t.Fatalf("gap events with lost = 2: %d, want 1", n)
	}
}

func TestABatchStoredByAnEarlierAttemptIsNotDroppedOnItsRetry(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: 20 * time.Millisecond}, 0)
	rows := s.views(t, 3)
	if err := s.recorder.writer.write(t.Context(), rows); err != nil {
		t.Fatal(err)
	}
	s.recorder.writer.held = &heldBatch{rows: rows}
	stop := s.run(t)
	s.waitForBatch(t)
	stop()
	if s.recorder.writer.held != nil {
		t.Fatal("the batch is still held after its retry")
	}
	if got := s.metrics.droppedFor(DroppedWriteFailed); got != 0 {
		t.Fatalf("dropped %s = %d, want 0", DroppedWriteFailed, got)
	}
	if n := s.count(t, "action = 'dashboard.view'"); n != 3 {
		t.Fatalf("stored views = %d, want 3", n)
	}
	if n := s.count(t, "action = 'audit.gap'"); n != 0 {
		t.Fatalf("gap events = %d, want 0", n)
	}
}

func TestADuplicateOnTheFirstAttemptIsStillAnError(t *testing.T) {
	s := newStoreRig(t, writerLimits{batch: 3, every: time.Hour}, 0)
	rows := s.views(t, 3)
	if err := s.recorder.writer.write(t.Context(), rows); err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		s.recorder.writer.enqueue(row)
	}
	s.run(t)()
	if got := s.metrics.droppedFor(DroppedWriteFailed); got != 3 {
		t.Fatalf("dropped %s = %d, want 3", DroppedWriteFailed, got)
	}
}

func TestABatchTheDatabaseRefusesIsDroppedAndCounted(t *testing.T) {
	s := newStoreRig(t, writerLimits{batch: 1, every: time.Hour}, 0)
	row, err := s.recorder.row(t.Context(), Event{Action: "dashboard.view", Target: Target{Kind: "dashboard", ID: "d1"}})
	if err != nil {
		t.Fatal(err)
	}
	row.OccurredAt = pgtype.Timestamptz{Time: time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC), Valid: true}
	s.recorder.writer.enqueue(row)
	s.run(t)()
	if got := s.metrics.droppedFor(DroppedWriteFailed); got != 1 {
		t.Fatalf("dropped %s = %d, want 1", DroppedWriteFailed, got)
	}
}
