package agentsview

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/auth"
)

func (r *rig) recordedRunLists(t *testing.T) []string {
	t.Helper()
	rows, err := r.db.SQLDB(t).QueryContext(t.Context(), `SELECT concat_ws(' ', actor_id::text, outcome, coalesce(nullif(target_kind, ''), '-'), coalesce(nullif(target_id, ''), '-'), data->>'scope')
		FROM audit_events WHERE action = 'run.list'`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var got []string
	for rows.Next() {
		var row string
		if err := rows.Scan(&row); err != nil {
			t.Fatal(err)
		}
		got = append(got, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	slices.Sort(got)
	return got
}

func TestEveryListOfAnAgentsRunsIsRecordedOncePerReaderAgentAndScopeInTheWindow(t *testing.T) {
	r := newRig(t, Observe{})
	account := r.serviceAccount(t, "nightly", true)
	r.seed(t, seededRun{kind: "builtin", by: bob, prompt: "why is payroll slow"})
	recorder := audit.New(audit.Config{DB: r.db.DB, Logger: slog.New(slog.DiscardHandler)})
	audit.Install(recorder)
	t.Cleanup(func() { audit.Install(nil) })
	recorded, err := recorder.Middleware()
	if err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		recorder.Run(ctx)
	}()
	handler := NewHandler(r.db.DB, Observe{}, slog.New(slog.DiscardHandler))
	for _, read := range []struct {
		who     auth.Principal
		queries []string
	}{
		{root, []string{"kind=builtin", "kind=builtin", "kind=service_account&id=" + account}},
		{ada, []string{"kind=builtin"}},
	} {
		gin.SetMode(gin.TestMode)
		engine := gin.New()
		who := read.who
		handler.Register(auth.NewRoutes(engine.Group("/api/v1", recorded, func(c *gin.Context) {
			ctx := auth.WithPrincipal(c.Request.Context(), who)
			audit.Update(ctx, func(s *audit.Scope) { s.Actor = auth.AuditActor(who) })
			c.Request = c.Request.WithContext(ctx)
		})))
		for _, query := range read.queries {
			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/agents/runs?"+query, nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("%s listed %s with %d: %s", who.Name, query, rec.Code, rec.Body)
			}
		}
	}
	stop()
	<-done

	want := []string{
		root.ID + " success - - workspace",
		root.ID + " success service_account " + account + " workspace",
		ada.ID + " success - - you",
	}
	slices.Sort(want)
	if got := r.recordedRunLists(t); !slices.Equal(got, want) {
		t.Fatalf("recorded run lists =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
