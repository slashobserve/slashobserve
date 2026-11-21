package audit

import (
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db"
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
	s.recorder.writer.held = []*statement{{rows: []dbsqlc.InsertAuditEventParams{s.staleView(t)}}}
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
	s := newStoreRig(t, writerLimits{batch: 2, every: time.Hour, timeout: 100 * time.Millisecond, settle: 50 * time.Millisecond}, 0)
	release := s.stallWrites(t)
	defer release()
	stop := s.run(t)
	s.enqueueViews(t, 5)
	if err := s.recorder.RecordNow(t.Context(), Event{Action: "migration.apply", Target: Target{Kind: "migration", ID: "80"}}); err != nil {
		t.Fatal(err)
	}
	stop()
	if got := s.metrics.droppedFor(DroppedWriteFailed); got != 6 {
		t.Fatalf("dropped %s = %d, want the 5 views and the handed off event", DroppedWriteFailed, got)
	}
	if got := s.recorder.writer.lost.Load(); got != 6 {
		t.Fatalf("lost = %d, want 6 for the gap row", got)
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

func TestABatchThatKeepsTimingOutIsKeptUntilTheDatabaseRecovers(t *testing.T) {
	s := newStoreRig(t, writerLimits{batch: 2, every: 10 * time.Millisecond, timeout: 50 * time.Millisecond}, 0)
	release := s.stallWrites(t)
	stop := s.run(t)
	defer stop()
	s.enqueueViews(t, 2)
	for range 6 {
		s.waitForBatch(t)
	}
	release()
	s.waitForEvents(t, 2)
	if got := s.metrics.droppedFor(DroppedWriteFailed); got != 0 {
		t.Fatalf("dropped %s = %d, want 0", DroppedWriteFailed, got)
	}
}

func TestFoldedEventsAreNotAddedToAFullBatchWhileOneIsHeld(t *testing.T) {
	s := newStoreRig(t, writerLimits{batch: 2, buffer: 2, every: 10 * time.Millisecond, timeout: 50 * time.Millisecond, fold: time.Millisecond}, 0)
	release := s.stallWrites(t)
	stop := s.run(t)
	defer stop()
	s.enqueueViews(t, 2)
	s.waitForBatch(t)
	s.enqueueViews(t, 2)
	s.waitForQueueToEmpty(t)
	for range 2 {
		row, err := s.recorder.row(t.Context(), Event{Action: "auth.login_failed", Reason: "invalid_token"})
		if err != nil {
			t.Fatal(err)
		}
		s.recorder.writer.enqueue(row)
	}
	for range 5 {
		s.waitForBatch(t)
	}
	if got := s.recorder.writer.folds.openCount(); got != 1 {
		t.Fatalf("open folds = %d, want the due fold left alone while a batch is held", got)
	}
	release()
	s.waitForEvents(t, 4)
	deadline := time.Now().Add(5 * time.Second)
	for s.count(t, "action = 'auth.login_failed'") < 1 {
		if time.Now().After(deadline) {
			t.Fatal("the folded events were never written")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestAHeldBatchTheServerCancelsIsSplitInHalf(t *testing.T) {
	s := newStoreRig(t, writerLimits{timeout: 5 * time.Second}, 0)
	s.db.Exec(t, `DO $$ BEGIN EXECUTE format('ALTER DATABASE %I SET statement_timeout = ''100ms''', current_database()); END $$`)
	limited, err := db.Open(t.Context(), db.Config{URL: s.db.URL()}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(limited.Close)
	s.recorder.db = limited
	release := s.stallWrites(t)
	defer release()
	s.recorder.writer.held = []*statement{{rows: s.views(t, 4)}}
	if s.recorder.writer.retryHeld(t.Context()) {
		t.Fatal("a canceled statement was reported as written")
	}
	if got := len(s.recorder.writer.held); got != 2 {
		t.Fatalf("held statements = %d, want 2 halves", got)
	}
	if a, b := len(s.recorder.writer.held[0].rows), len(s.recorder.writer.held[1].rows); a != 2 || b != 2 {
		t.Fatalf("halves hold %d and %d rows, want 2 and 2", a, b)
	}
}

func TestHeldHandedOffRowsCountAgainstTheHandOffLimit(t *testing.T) {
	s := newStoreRig(t, writerLimits{buffer: 3, every: 10 * time.Millisecond, timeout: 50 * time.Millisecond}, 0)
	release := s.stallWrites(t)
	defer release()
	stop := s.run(t)
	defer stop()
	for _, row := range s.views(t, 3) {
		if !s.recorder.writer.handOff(row) {
			t.Fatal("a hand off under the limit was refused")
		}
	}
	for range 3 {
		s.waitForBatch(t)
	}
	if s.recorder.writer.handOff(s.views(t, 1)[0]) {
		t.Fatal("a hand off was accepted while the limit's worth of rows is still held")
	}
}

func TestAHandOffIsAcceptedAgainOnceItsRowWasWritten(t *testing.T) {
	s := newStoreRig(t, writerLimits{buffer: 1, every: 10 * time.Millisecond}, 0)
	stop := s.run(t)
	defer stop()
	if !s.recorder.writer.handOff(s.views(t, 1)[0]) {
		t.Fatal("the first hand off was refused")
	}
	s.waitForEvents(t, 1)
	deadline := time.Now().Add(5 * time.Second)
	for !s.recorder.writer.handOff(s.views(t, 1)[0]) {
		if time.Now().After(deadline) {
			t.Fatal("a hand off was still refused after the earlier row was written")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestReplayingHeldRowsLeavesRoomForTheNextBatch(t *testing.T) {
	s := newStoreRig(t, writerLimits{batch: 2, every: time.Hour}, 0)
	w := s.recorder.writer
	for _, row := range s.views(t, 5) {
		w.handOff(row)
	}
	next := s.views(t, 1)
	w.flush(t.Context(), next)
	if got := s.count(t, "action = 'dashboard.view'"); got != 3 {
		t.Fatalf("stored views after one flush = %d, want 2 replayed rows and the new batch", got)
	}
	if got := len(w.held); got != 3 {
		t.Fatalf("held statements = %d, want 3 left for the next flush", got)
	}
}

func TestABatchThatKeepsTimingOutOnTheClientDeadlineIsSplitInHalf(t *testing.T) {
	s := newStoreRig(t, writerLimits{timeout: 50 * time.Millisecond}, 0)
	release := s.stallWrites(t)
	defer release()
	w := s.recorder.writer
	w.held = []*statement{{rows: s.views(t, 4)}}
	for attempt := 1; attempt < splitAfterTimeouts; attempt++ {
		if w.retryHeld(t.Context()) {
			t.Fatal("a stalled write was reported as written")
		}
		if got := len(w.held); got != 1 {
			t.Fatalf("held statements after %d timeouts = %d, want the batch kept whole", attempt, got)
		}
	}
	if w.retryHeld(t.Context()) {
		t.Fatal("a stalled write was reported as written")
	}
	if got := len(w.held); got != 2 {
		t.Fatalf("held statements after %d timeouts = %d, want 2 halves", splitAfterTimeouts, got)
	}
}

func TestADroppedBatchIsCountedFromItsOwnRows(t *testing.T) {
	s := newStoreRig(t, writerLimits{}, 0)
	w := s.recorder.writer
	views := s.views(t, 3)
	w.abandon(views)
	if got := s.metrics.droppedFor(DroppedWriteFailed); got != 3 {
		t.Fatalf("dropped %s = %d, want 3", DroppedWriteFailed, got)
	}
	if got := w.lost.Load(); got != 3 {
		t.Fatalf("lost = %d, want 3 when the rows hold no gap row", got)
	}
	gap, err := s.recorder.row(t.Context(), Event{Action: ActionGap, Data: map[string]any{"lost": 4}})
	if err != nil {
		t.Fatal(err)
	}
	w.abandon(append(s.views(t, 2), gap))
	if got := s.metrics.droppedFor(DroppedWriteFailed); got != 5 {
		t.Fatalf("dropped %s = %d, want the gap's events left out of the count", DroppedWriteFailed, got)
	}
	if got := w.lost.Load(); got != 9 {
		t.Fatalf("lost = %d, want the gap's 4 put back with the 2 events", got)
	}
}

func TestShutdownWritesEveryHeldStatementWhileTheDatabaseIsHealthy(t *testing.T) {
	s := newStoreRig(t, writerLimits{batch: 2, every: time.Hour}, 0)
	stop := s.run(t)
	for _, row := range s.views(t, 6) {
		s.recorder.writer.handOff(row)
	}
	stop()
	if n := s.count(t, "action = 'dashboard.view'"); n != 6 {
		t.Fatalf("stored views after shutdown = %d, want all 6 held rows", n)
	}
	if got := s.metrics.droppedFor(DroppedWriteFailed); got != 0 {
		t.Fatalf("dropped %s = %d, want 0", DroppedWriteFailed, got)
	}
}
