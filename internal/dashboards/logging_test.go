package dashboards

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/logtest"
)

func TestInternalFailureLogsOnlyCodeTypeAndIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := newRig(t, false)
	id := r.createDashboard(t)
	logger, logs := logtest.New()
	r.service.logger = logger
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), testUser))
	})
	r.service.Register(engine.Group("/api/v1"))
	r.db.Close()

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/dashboards/"+id.String(), nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d, want 500: %s", rec.Code, rec.Body.String())
	}
	record := logs.Find(t, "get dashboard")
	logs.AssertSafeFailure(t, record, "internal", "closed pool")
	if record["dashboard_id"] != id.String() || record["principal_id"] != testUserID || record["principal_kind"] != string(auth.KindUser) {
		t.Errorf("record lacks the ids: %v", record)
	}
}
