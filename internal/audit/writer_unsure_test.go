package audit

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestAChangeEventStoredByATimedOutWriteIsNotDroppedWhenQueuedAgain(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: 20 * time.Millisecond}, 0)
	row := s.views(t, 1)[0]
	if err := s.recorder.insertNow(t.Context(), row); err != nil {
		t.Fatal(err)
	}
	s.recorder.writer.enqueueUnsure(row)
	stop := s.run(t)
	s.waitForBatch(t)
	stop()
	if got := s.metrics.droppedFor(DroppedWriteFailed); got != 0 {
		t.Fatalf("dropped %s = %d, want 0", DroppedWriteFailed, got)
	}
	if n := s.count(t, "action = 'dashboard.view'"); n != 1 {
		t.Fatalf("stored views = %d, want 1", n)
	}
	if n := s.count(t, "action = 'audit.gap'"); n != 0 {
		t.Fatalf("gap events = %d, want 0", n)
	}
}

func TestAChangeEventWhoseWriteTheDatabaseRefusesStillFails(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: time.Hour}, 0)
	row := s.views(t, 1)[0]
	row.OccurredAt = pgtype.Timestamptz{Time: time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC), Valid: true}
	if err := s.recorder.insertNow(t.Context(), row); err == nil {
		t.Fatal("a refused write was reported as kept")
	}
}
