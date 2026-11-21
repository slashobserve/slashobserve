package observations_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/audit/audittest"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/observations"
)

func TestOpeningAnObservationRecordsWhenItLastChanged(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	mw, flush := audittest.Middleware(t, r.db)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	observations.NewHandler(r.service, slog.New(slog.DiscardHandler)).Register(auth.NewRoutes(engine.Group("/api/v1", mw, func(c *gin.Context) {
		c.Request = c.Request.WithContext(datasource.WithAccess(auth.WithPrincipal(c.Request.Context(), editor), datasource.Access{By: editor}))
	})))
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/observations/"+o.ID, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	flush()
	got := r.db.AuditEvents(t, "observation.view")
	if len(got) != 1 || got[0].Outcome != "success" || got[0].TargetID != o.ID {
		t.Fatalf("events = %+v, want one successful view of %s", got, o.ID)
	}
	if want := o.UpdatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"); got[0].Data["updated_at"] != want {
		t.Fatalf("data = %v, want updated_at %s", got[0].Data, want)
	}
}
