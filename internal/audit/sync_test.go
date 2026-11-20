package audit

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/logtest"
)

func restrictedRecorder(t *testing.T, s storeRig, logger *slog.Logger, grants ...string) *Recorder {
	t.Helper()
	suffix := make([]byte, 4)
	_, _ = rand.Read(suffix)
	role := "audit_restricted_" + hex.EncodeToString(suffix)
	s.db.Exec(t, "CREATE ROLE "+role+" NOLOGIN")
	s.db.Exec(t, "GRANT USAGE ON SCHEMA public TO "+role)
	for _, grant := range grants {
		s.db.Exec(t, "GRANT "+grant+" TO "+role)
	}
	u, err := url.Parse(s.db.URL())
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("options", "-c role="+role)
	u.RawQuery = strings.ReplaceAll(q.Encode(), "+", "%20")
	restricted, err := db.Open(t.Context(), db.Config{URL: u.String()}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		restricted.Close()
		s.db.Exec(t, "DROP OWNED BY "+role)
		s.db.Exec(t, "DROP ROLE "+role)
	})
	r := New(Config{DB: restricted, Server: "test", Version: "test", Metrics: s.metrics, Logger: logger})
	r.writer = newWriter(r, writerLimits{every: time.Hour})
	return r
}

func TestAFailedChangeIsWrittenBeforeTheRequestEnds(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: time.Hour}, 0)
	engine := s.engine(t, func(c *gin.Context) {
		c.JSON(http.StatusConflict, gin.H{"error": "the dashboard changed", "code": "version_conflict"})
	})
	_, request := serve(t, engine, http.MethodPost, "/api/v1/dashboards/"+dashboardID+"/versions")
	want := requestEvent{action: "dashboard.update", outcome: "failed", reason: "version_conflict", targetKind: "dashboard", targetID: dashboardID, route: "/api/v1/dashboards/{id}/versions", method: "POST", status: 409}
	if got := s.eventsOf(t, request); len(got) != 1 || got[0] != want {
		t.Fatalf("events with the batched writer stopped = %+v, want %+v", got, want)
	}
}

func TestADeniedChangeAndAReadStillGoThroughTheBatchedWriter(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: time.Hour}, 0)
	engine := s.engine(t, func(c *gin.Context) {
		if c.Request.Method == http.MethodGet {
			c.Status(http.StatusOK)
			return
		}
		c.JSON(http.StatusForbidden, gin.H{"error": "no", "code": "forbidden_role"})
	})
	_, denied := serve(t, engine, http.MethodPost, "/api/v1/dashboards/"+dashboardID+"/versions")
	_, read := serve(t, engine, http.MethodGet, "/api/v1/dashboards/"+dashboardID)
	if got := len(s.eventsOf(t, denied)) + len(s.eventsOf(t, read)); got != 0 {
		t.Fatalf("%d access events written before a flush, want 0", got)
	}
	s.run(t)()
	if len(s.eventsOf(t, denied)) != 1 || len(s.eventsOf(t, read)) != 1 {
		t.Fatal("the batched writer did not write the denied change and the read")
	}
}

func TestAFailedSynchronousWriteIsLoggedAndCounted(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: time.Hour}, 0)
	logger, logs := logtest.New()
	s.recorder = restrictedRecorder(t, s, logger)
	engine := s.engine(t, func(c *gin.Context) {
		c.JSON(http.StatusConflict, gin.H{"error": "the dashboard changed", "code": "version_conflict"})
	})
	serve(t, engine, http.MethodPost, "/api/v1/dashboards/"+dashboardID+"/versions")
	if got := s.metrics.droppedFor(DroppedRecordFailed); got != 1 {
		t.Fatalf("dropped %s = %d, want 1", DroppedRecordFailed, got)
	}
	logs.AssertSafeFailure(t, logs.Find(t, "audit request event not written"), "audit_record_failed")
}
