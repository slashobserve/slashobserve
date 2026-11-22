package observations_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/audit/audittest"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/observations"
)

func TestAnArrangementThatChangesNothingIsRecordedWithTheNoChangeReason(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	a := r.addPanel(t, editor, o)
	b := r.addText(t, editor, o, "b")
	mw, flush := audittest.Middleware(t, r.db)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	observations.NewHandler(r.service, slog.New(slog.DiscardHandler)).Register(auth.NewRoutes(engine.Group("/api/v1", mw, func(c *gin.Context) {
		auth.Authenticated(c.Request.Context(), editor, audit.AuthSession, "")
		c.Request = c.Request.WithContext(datasource.WithAccess(auth.WithPrincipal(c.Request.Context(), editor), datasource.Access{By: editor}))
	})))
	put := func(ids ...string) {
		t.Helper()
		blocks := make([]string, len(ids))
		for i, id := range ids {
			blocks[i] = `{"id":"` + id + `"}`
		}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/v1/observations/"+o.ID+"/layout", strings.NewReader(`{"blocks":[`+strings.Join(blocks, ",")+`]}`))
		req.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d: %s", rec.Code, rec.Body)
		}
	}
	put(b.ID, a.ID)
	put(b.ID, a.ID)
	flush()

	got := r.db.AuditEvents(t, "observation.arrange")
	if len(got) != 2 {
		t.Fatalf("events = %+v, want two", got)
	}
	if got[0].Reason != "" || got[0].Data["changed"] == nil {
		t.Fatalf("the real change = %+v, want no reason and a changed list", got[0])
	}
	if got[1].Outcome != "success" || got[1].Reason != "no_change" || got[1].TargetID != o.ID {
		t.Fatalf("the repeat = %+v, want a success with reason no_change on %s", got[1], o.ID)
	}
}
