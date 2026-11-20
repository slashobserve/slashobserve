package audit

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/logtest"
)

func TestARequestWhoseCommitCannotBeCheckedIsRecordedAsUnknown(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: time.Hour}, 0)
	logger, logs := logtest.New()
	s.recorder = restrictedRecorder(t, s, logger, "INSERT ON audit_events", "USAGE ON SEQUENCE audit_events_seq")
	engine := s.engine(t, func(c *gin.Context) {
		err := s.db.InTx(c.Request.Context(), func(q *dbsqlc.Queries) error {
			if err := s.recorder.Record(c.Request.Context(), q, Event{Action: "dashboard.update", Target: Target{Kind: "dashboard", ID: c.Param("id")}}); err != nil {
				return err
			}
			return errors.New("lost the race")
		})
		if err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": "the dashboard changed", "code": "version_conflict"})
		}
	})
	_, request := serve(t, engine, http.MethodPost, "/api/v1/dashboards/"+dashboardID+"/versions")
	want := requestEvent{action: "dashboard.update", outcome: "unknown", reason: "commit_check_failed", targetKind: "dashboard", targetID: dashboardID, route: "/api/v1/dashboards/{id}/versions", method: "POST", status: 409}
	if got := s.eventsOf(t, request); len(got) != 1 || got[0] != want {
		t.Fatalf("events = %+v, want %+v", got, want)
	}
	logs.AssertSafeFailure(t, logs.Find(t, "audit commit check failed"), "audit_check_failed")
}
