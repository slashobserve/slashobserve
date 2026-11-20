package audit

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(dbtest.Main(m))
}

type storeRig struct {
	db       *dbtest.Database
	recorder *Recorder
	metrics  *countingMetrics
}

func newStoreRig(t *testing.T, limits writerLimits, retention time.Duration) storeRig {
	t.Helper()
	d := dbtest.New(t)
	m := newCountingMetrics()
	r := New(Config{DB: d.DB, Server: "test", Version: "test", Metrics: m, Retention: retention})
	r.writer = newWriter(r, limits)
	return storeRig{db: d, recorder: r, metrics: m}
}

func (s storeRig) run(t *testing.T) (stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.recorder.Run(ctx)
	}()
	return func() {
		cancel()
		<-done
	}
}

func (s storeRig) count(t *testing.T, where string, args ...any) int {
	t.Helper()
	var n int
	if err := s.db.SQLDB(t).QueryRowContext(t.Context(), "SELECT count(*) FROM audit_events WHERE "+where, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (s storeRig) waitForBatch(t *testing.T) {
	t.Helper()
	select {
	case <-s.metrics.batches:
	case <-time.After(5 * time.Second):
		t.Fatal("no batch was written")
	}
}

func (s storeRig) enqueueViews(t *testing.T, n int) {
	t.Helper()
	for range n {
		row, err := s.recorder.row(t.Context(), Event{Action: "dashboard.view", Target: Target{Kind: "dashboard", ID: "d1"}})
		if err != nil {
			t.Fatal(err)
		}
		s.recorder.writer.enqueue(row)
	}
}

func TestRecordCommitsAndRollsBackWithItsTransaction(t *testing.T) {
	s := newStoreRig(t, writerLimits{}, 0)
	errRollback := errors.New("roll back")
	err := s.db.InTx(t.Context(), func(q *dbsqlc.Queries) error {
		if err := s.recorder.Record(t.Context(), q, Event{Action: "folder.create", Target: Target{Kind: "folder", ID: "f1"}}); err != nil {
			return err
		}
		return errRollback
	})
	if !errors.Is(err, errRollback) {
		t.Fatal(err)
	}
	if n := s.count(t, "true"); n != 0 {
		t.Fatalf("events after rollback = %d, want 0", n)
	}
	err = s.db.InTx(t.Context(), func(q *dbsqlc.Queries) error {
		return s.recorder.Record(t.Context(), q, Event{Action: "folder.create", Target: Target{Kind: "folder", ID: "f1"}})
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := s.count(t, "action = 'folder.create' AND target_id = 'f1' AND actor_kind = 'system' AND origin = 'job'"); n != 1 {
		t.Fatalf("committed events = %d, want 1", n)
	}
}

func TestRecordNowWritesAChangeAtOnceAndQueuesAnAccess(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: time.Hour}, 0)
	if err := s.recorder.RecordNow(t.Context(), Event{Action: "job.run", Target: Target{Kind: "job", ID: "dashboard_health"}}); err != nil {
		t.Fatal(err)
	}
	if n := s.count(t, "action = 'job.run'"); n != 1 {
		t.Fatalf("change events = %d, want 1", n)
	}
	if err := s.recorder.RecordNow(t.Context(), Event{Action: "dashboard.view"}); err != nil {
		t.Fatal(err)
	}
	if n := s.count(t, "action = 'dashboard.view'"); n != 0 {
		t.Fatalf("access events before a flush = %d, want 0", n)
	}
	s.run(t)()
	if n := s.count(t, "action = 'dashboard.view'"); n != 1 {
		t.Fatalf("access events after shutdown = %d, want 1", n)
	}
}

func TestEventsRefuseUpdateAndDelete(t *testing.T) {
	s := newStoreRig(t, writerLimits{}, 0)
	if err := s.recorder.RecordNow(t.Context(), Event{Action: "job.run"}); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{"UPDATE audit_events SET outcome = 'failed'", "DELETE FROM audit_events"} {
		_, err := s.db.SQLDB(t).ExecContext(t.Context(), statement)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
			t.Errorf("%s: err = %v, want insufficient_privilege", statement, err)
		}
	}
	if n := s.count(t, "outcome = 'success'"); n != 1 {
		t.Fatalf("events after the refused statements = %d, want the one unchanged", n)
	}
}

func TestTheWriterFlushesAFullBatchAtOnce(t *testing.T) {
	s := newStoreRig(t, writerLimits{batch: 3, every: time.Hour}, 0)
	defer s.run(t)()
	s.enqueueViews(t, 3)
	s.waitForBatch(t)
	if n := s.count(t, "action = 'dashboard.view'"); n != 3 {
		t.Fatalf("events after a full batch = %d, want 3", n)
	}
}

func TestTheWriterFlushesAPartBatchOnItsTimer(t *testing.T) {
	s := newStoreRig(t, writerLimits{batch: 500, every: 20 * time.Millisecond}, 0)
	defer s.run(t)()
	s.enqueueViews(t, 2)
	deadline := time.After(5 * time.Second)
	for s.count(t, "action = 'dashboard.view'") < 2 {
		select {
		case <-s.metrics.batches:
		case <-deadline:
			t.Fatal("the timer never flushed the part batch")
		}
	}
}

func TestTheWriterFlushesOnShutdown(t *testing.T) {
	s := newStoreRig(t, writerLimits{batch: 500, every: time.Hour}, 0)
	stop := s.run(t)
	s.enqueueViews(t, 4)
	stop()
	if n := s.count(t, "action = 'dashboard.view'"); n != 4 {
		t.Fatalf("events after shutdown = %d, want 4", n)
	}
}

func TestAFullBufferDropsAndRecordsOneGap(t *testing.T) {
	s := newStoreRig(t, writerLimits{batch: 500, every: time.Hour, buffer: 2, wait: time.Millisecond}, 0)
	s.enqueueViews(t, 5)
	if got := s.metrics.droppedFor(DroppedBufferFull); got != 3 {
		t.Fatalf("dropped = %d, want 3", got)
	}
	s.run(t)()
	if n := s.count(t, "action = 'dashboard.view'"); n != 2 {
		t.Fatalf("kept events = %d, want 2", n)
	}
	if n := s.count(t, "action = 'audit.gap' AND (data->>'lost')::int = 3 AND actor_kind = 'system'"); n != 1 {
		t.Fatalf("gap events with lost = 3: %d, want 1", n)
	}
}
