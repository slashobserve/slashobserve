package audit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestARequestStillRunningWhenTheWriterStopsIsRecorded(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: time.Hour}, 0)
	started, release := make(chan struct{}), make(chan struct{})
	engine := s.engine(t, func(c *gin.Context) {
		close(started)
		<-release
		c.Status(http.StatusOK)
	})
	stop := s.run(t)
	served := make(chan string, 1)
	go func() {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboards/"+dashboardID, nil)
		req.RemoteAddr = "192.0.2.7:4100"
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
		served <- rec.Header().Get(RequestIDHeader)
	}()
	<-started
	stopped := make(chan struct{})
	go func() {
		stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(200 * time.Millisecond):
	}
	close(release)
	request, err := uuid.Parse(<-served)
	if err != nil {
		t.Fatal(err)
	}
	<-stopped
	if got := s.eventsOf(t, request); len(got) != 1 || got[0].action != "dashboard.view" {
		t.Fatalf("events = %+v, want the stream's dashboard.view", got)
	}
}

func TestTheFinalFlushIsBounded(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: time.Hour, timeout: 200 * time.Millisecond, settle: 200 * time.Millisecond}, 0)
	s.enqueueViews(t, 3)
	locker := s.db.SQLDB(t)
	tx, err := locker.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(t.Context(), "LOCK TABLE audit_events IN ACCESS EXCLUSIVE MODE"); err != nil {
		t.Fatal(err)
	}
	s.recorder.inflight.Add(1)
	defer s.recorder.inflight.Add(-1)
	stop := s.run(t)
	start := time.Now()
	stop()
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("the writer took %s to stop behind a locked table", took)
	}
	if got := s.metrics.droppedFor(DroppedWriteFailed); got != 3 {
		t.Fatalf("dropped %s = %d, want the 3 queued events", DroppedWriteFailed, got)
	}
}
