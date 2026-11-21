package dashboards

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/audit/audittest"
	"github.com/slashobserve/slashobserve/internal/auth"
)

func TestOpeningADashboardRecordsTheVersionRead(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := newRig(t, false)
	id := r.createDashboard(t)
	mw, flush := audittest.Middleware(t, r.db)
	engine := gin.New()
	api := engine.Group("/api/v1", mw, func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), withRole(testUser)))
	})
	api.GET("/dashboards/:id", r.service.getDashboard)
	r.saveConcurrent(t, id, 1, panelsSpec("second", "rps"))
	r.saveConcurrent(t, id, 2, panelsSpec("third", "rps"))
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/dashboards/"+id.String(), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	flush()
	got := r.db.AuditEvents(t, "dashboard.view")
	if len(got) != 1 || got[0].Outcome != "success" || got[0].TargetID != id.String() {
		t.Fatalf("events = %+v, want one successful view of %s", got, id)
	}
	if got[0].Data["version"] != float64(3) {
		t.Fatalf("data = %v, want version 3", got[0].Data)
	}
}
