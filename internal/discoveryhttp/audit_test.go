package discoveryhttp

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

func TestSamplingRowsRecordsAQueryRunOnTheDatasource(t *testing.T) {
	d := dbtest.New(t)
	recorder := audit.New(audit.Config{DB: d.DB})
	mw, err := recorder.Middleware()
	if err != nil {
		t.Fatal(err)
	}
	viewer := auth.Principal{Kind: auth.KindUser, ID: "11111111-1111-4111-8111-111111111111", Name: "Ada", Role: auth.RoleViewer}
	engine := gin.New()
	group := engine.Group("/api/v1", mw, func(c *gin.Context) {
		ctx := auth.WithPrincipal(c.Request.Context(), viewer)
		audit.Update(ctx, func(s *audit.Scope) { s.Actor = auth.AuditActor(viewer) })
		c.Request = c.Request.WithContext(ctx)
	})
	NewHandler(&fakeReader{out: map[string]any{"rows": []any{}}}, slog.New(slog.DiscardHandler)).Register(auth.NewRoutes(group))

	ctx, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		recorder.Run(ctx)
	}()
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/datasources/ch/sample?table=otel_logs", nil))
	stop()
	<-done
	if rec.Code != http.StatusOK {
		t.Fatalf("sample = %d %s", rec.Code, rec.Body)
	}

	var action, outcome, actorID, targetKind, targetID, route, data string
	var status int
	err = d.SQLDB(t).QueryRowContext(t.Context(), `SELECT action, outcome, actor_id::text, target_kind, target_id, route, status, data::text FROM audit_events`).
		Scan(&action, &outcome, &actorID, &targetKind, &targetID, &route, &status, &data)
	if err != nil {
		t.Fatal(err)
	}
	if action != "query.run" || outcome != "success" || actorID != viewer.ID || targetKind != "datasource" || targetID != "ch" || route != "/api/v1/datasources/{uid}/sample" || status != http.StatusOK || data != "{}" {
		t.Fatalf("event = %s %s by %s on %s %s via %s (%d) data %s", action, outcome, actorID, targetKind, targetID, route, status, data)
	}
}
