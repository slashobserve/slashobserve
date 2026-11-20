package audit

import (
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestAPanickingHandlerIsRecordedAsFailedAndStillRecovered(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: time.Hour}, 0)
	mw, err := s.recorder.Middleware()
	if err != nil {
		t.Fatal(err)
	}
	var recovered any
	engine := gin.New()
	engine.Use(gin.CustomRecovery(func(c *gin.Context, v any) {
		recovered = v
		c.AbortWithStatus(http.StatusInternalServerError)
	}))
	engine.Group("/api/v1", mw).POST("/dashboards/:id/versions", func(*gin.Context) { panic("handler bug") })
	rec, request := serve(t, engine, http.MethodPost, "/api/v1/dashboards/"+dashboardID+"/versions")
	if rec.Code != http.StatusInternalServerError || recovered != "handler bug" {
		t.Fatalf("answer = %d, recovered %v; want the engine's recovery to see the panic", rec.Code, recovered)
	}
	s.run(t)()
	want := requestEvent{action: "dashboard.update", outcome: "failed", reason: "panic", targetKind: "dashboard", targetID: dashboardID, route: "/api/v1/dashboards/{id}/versions", method: "POST", status: 500}
	if got := s.eventsOf(t, request); len(got) != 1 || got[0] != want {
		t.Fatalf("events = %+v, want %+v", got, want)
	}
}
