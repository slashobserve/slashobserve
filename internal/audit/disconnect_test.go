package audit

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func TestAClientThatLeavesWhileItsChangeCommitsIsRecordedOnceAsASuccess(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: time.Hour}, 0)
	ctx, leave := context.WithCancel(t.Context())
	engine := s.engine(t, func(c *gin.Context) {
		_ = s.db.InTx(c.Request.Context(), func(q *dbsqlc.Queries) error {
			if err := s.recorder.Record(c.Request.Context(), q, Event{Action: "dashboard.update", Target: Target{Kind: "dashboard", ID: c.Param("id"), Name: "Checkout"}}); err != nil {
				return err
			}
			leave()
			return nil
		})
		c.AbortWithStatus(clientgone.Status)
	})
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/v1/dashboards/"+dashboardID+"/versions", nil)
	req.RemoteAddr = "192.0.2.7:4100"
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	s.run(t)()
	var request string
	if err := s.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT request_id::text FROM audit_events WHERE client_ip = '192.0.2.7' ORDER BY occurred_at DESC LIMIT 1`).Scan(&request); err != nil {
		t.Fatal(err)
	}
	if n := s.count(t, `request_id = $1`, request); n != 1 {
		t.Fatalf("events for the request = %d, want only the committed success", n)
	}
	if n := s.count(t, `request_id = $1 AND outcome = 'success' AND target_name = 'Checkout'`, request); n != 1 {
		t.Fatalf("success events with the target name = %d, want 1", n)
	}
}

func TestARolledBackFailureNamesItsTargetLikeTheSuccessWould(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: time.Hour}, 0)
	engine := s.engine(t, func(c *gin.Context) {
		err := s.db.InTx(c.Request.Context(), func(q *dbsqlc.Queries) error {
			if err := s.recorder.Record(c.Request.Context(), q, Event{Action: "dashboard.update", Target: Target{Kind: "dashboard", ID: c.Param("id"), Name: "Checkout"}}); err != nil {
				return err
			}
			return errors.New("lost the race")
		})
		if err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": "the dashboard changed", "code": "version_conflict"})
		}
	})
	_, request := serve(t, engine, http.MethodPost, "/api/v1/dashboards/"+dashboardID+"/versions")
	s.run(t)()
	if n := s.count(t, `request_id = $1 AND outcome = 'failed' AND target_name = 'Checkout' AND target_id = $2`, request, dashboardID); n != 1 {
		t.Fatalf("failed events naming the target = %d, want 1", n)
	}
}
