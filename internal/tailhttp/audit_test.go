package tailhttp

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

func TestAnOpenTailIsRecordedAsAStreamedQueryRun(t *testing.T) {
	d := dbtest.New(t)
	recorder := audit.New(audit.Config{DB: d.DB})
	mw, err := recorder.Middleware()
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	logger := slog.New(slog.DiscardHandler)
	registry, err := datasource.NewRegistry(logger, datasource.Instance{UID: "logs", Datasource: &fakeLines{typ: "loki", responses: []response{lines(1)}}})
	if err != nil {
		t.Fatal(err)
	}
	sessions := &fakeSessions{}
	sessions.valid.Store(true)
	h := NewHandler(registry, queryproxy.NewRunner(registry, logger), sessions, logger)
	h.pollInterval = testPoll
	viewer := auth.Principal{Kind: auth.KindUser, ID: "11111111-1111-4111-8111-111111111111", Name: "Ada", Role: auth.RoleViewer}
	finished := make(chan struct{})
	engine := gin.New()
	api := engine.Group("/api/v1", func(c *gin.Context) {
		c.Next()
		close(finished)
	}, mw, func(c *gin.Context) {
		ctx := auth.WithPrincipal(c.Request.Context(), viewer)
		audit.Update(ctx, func(s *audit.Scope) { s.Actor = auth.AuditActor(viewer) })
		c.Request = c.Request.WithContext(datasource.WithAccess(ctx, datasource.Access{By: viewer}))
	})
	h.Register(auth.NewRoutes(api))
	server := httptest.NewServer(engine)
	t.Cleanup(server.Close)

	runCtx, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		recorder.Run(runCtx)
	}()
	resp, err := http.Get(server.URL + "/api/v1/datasources/logs/tail?query=%7Bapp%3D%22shop%22%7D")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	select {
	case <-finished:
	case <-time.After(waitTimeout):
		t.Fatal("the tail did not end after the client left")
	}
	stop()
	<-done

	var action, outcome, targetID, stream, query, rows, from string
	var to *string
	err = d.SQLDB(t).QueryRowContext(t.Context(), `SELECT action, outcome, target_id, data->>'stream', data->>'query', data->>'row_count', data->>'from', data->>'to' FROM audit_events`).Scan(&action, &outcome, &targetID, &stream, &query, &rows, &from, &to)
	if err != nil {
		t.Fatal(err)
	}
	if action != "query.run" || outcome != "success" || targetID != "logs" || stream != "true" || query != `{app="shop"}` || rows != "1" || from == "" || to != nil {
		t.Fatalf("event = %s %s on %s stream %s query %s rows %s from %s to %v, want a successful streamed query.run with its text, first rows and start", action, outcome, targetID, stream, query, rows, from, to)
	}
}
