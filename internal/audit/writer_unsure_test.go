package audit

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func TestAChangeEventStoredByATimedOutWriteIsNotDroppedWhenQueuedAgain(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: 20 * time.Millisecond}, 0)
	row := s.views(t, 1)[0]
	if err := s.recorder.insertNow(t.Context(), row); err != nil {
		t.Fatal(err)
	}
	s.recorder.writer.handOff(row)
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

func (s storeRig) staleView(t *testing.T) dbsqlc.InsertAuditEventParams {
	t.Helper()
	row := s.views(t, 1)[0]
	row.OccurredAt = pgtype.Timestamptz{Time: time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC), Valid: true}
	return row
}

func TestHandedOffEventsAreWrittenWhenOnlyOneOfThemIsAlreadyStored(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: 20 * time.Millisecond}, 0)
	rows := s.views(t, 3)
	if err := s.recorder.insertNow(t.Context(), rows[0]); err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		s.recorder.writer.handOff(row)
	}
	stop := s.run(t)
	s.waitForEvents(t, 3)
	stop()
	if got := s.metrics.droppedFor(DroppedWriteFailed); got != 0 {
		t.Fatalf("dropped %s = %d, want 0", DroppedWriteFailed, got)
	}
	if n := s.count(t, "action = 'dashboard.view'"); n != 3 {
		t.Fatalf("stored views = %d, want 3", n)
	}
}

func TestOneRefusedHandedOffEventCostsOnlyItself(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: 20 * time.Millisecond}, 0)
	good := s.views(t, 2)
	s.recorder.writer.handOff(good[0])
	s.recorder.writer.handOff(s.staleView(t))
	s.recorder.writer.handOff(good[1])
	stop := s.run(t)
	s.waitForEvents(t, 2)
	stop()
	if got := s.metrics.droppedFor(DroppedWriteFailed); got != 1 {
		t.Fatalf("dropped %s = %d, want only the refused event", DroppedWriteFailed, got)
	}
	if n := s.count(t, "action = 'audit.gap' AND (data->>'lost')::int = 1"); n != 1 {
		t.Fatalf("gap events with lost = 1: %d, want 1", n)
	}
}

func TestARefusedHeldBatchDoesNotTakeHandedOffEventsWithIt(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: 20 * time.Millisecond}, 0)
	s.recorder.writer.held = [][]dbsqlc.InsertAuditEventParams{{s.staleView(t)}}
	s.recorder.writer.handOff(s.views(t, 1)[0])
	stop := s.run(t)
	s.waitForEvents(t, 1)
	stop()
	if got := s.metrics.droppedFor(DroppedWriteFailed); got != 1 {
		t.Fatalf("dropped %s = %d, want only the refused batch", DroppedWriteFailed, got)
	}
	if n := s.count(t, "action = 'audit.gap' AND (data->>'lost')::int = 1"); n != 1 {
		t.Fatalf("gap events with lost = 1: %d, want 1", n)
	}
}

func TestShutdownCountsHeldAndHandedOffEventsStillUnwritten(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: time.Hour, timeout: 100 * time.Millisecond, settle: 50 * time.Millisecond}, 0)
	release := s.stallWrites(t)
	defer release()
	stop := s.run(t)
	s.enqueueViews(t, 3)
	if err := s.recorder.RecordNow(t.Context(), Event{Action: "migration.apply", Target: Target{Kind: "migration", ID: "80"}}); err != nil {
		t.Fatal(err)
	}
	stop()
	if got := s.metrics.droppedFor(DroppedWriteFailed); got != 4 {
		t.Fatalf("dropped %s = %d, want the 3 views and the handed off event", DroppedWriteFailed, got)
	}
	if got := s.recorder.writer.lost.Load(); got != 4 {
		t.Fatalf("lost = %d, want 4 for the gap row", got)
	}
}

func TestADirectWriteAfterTheWriterStoppedIsNotHandedOff(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: time.Hour, timeout: 100 * time.Millisecond, settle: 50 * time.Millisecond}, 0)
	release := s.stallWrites(t)
	defer release()
	s.run(t)()
	err := s.recorder.RecordNow(t.Context(), Event{Action: "migration.apply", Target: Target{Kind: "migration", ID: "80"}})
	if err == nil {
		t.Fatal("RecordNow after the writer stopped reported the event as kept")
	}
}

func TestAHandOffRefusedForALackOfRoomIsNotCountedAsABufferDrop(t *testing.T) {
	s := newStoreRig(t, writerLimits{buffer: 1}, 0)
	rows := s.views(t, 2)
	if !s.recorder.writer.handOff(rows[0]) {
		t.Fatal("the first hand off was refused")
	}
	if s.recorder.writer.handOff(rows[1]) {
		t.Fatal("a hand off was accepted past the limit")
	}
	if got := s.metrics.droppedFor(DroppedBufferFull); got != 0 {
		t.Fatalf("dropped %s = %d, want 0", DroppedBufferFull, got)
	}
}
