package auditfolder

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/audit/trail"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/auditsource"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const viewedDashboard = "55555555-5555-4555-8555-555555555555"

func recordRequests(t *testing.T, d *dbtest.Database, requests func(engine *gin.Engine)) {
	t.Helper()
	recorder := audit.New(audit.Config{DB: d.DB, Server: "test", Version: "test"})
	mw, err := recorder.Middleware()
	if err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	engine.GET("/api/v1/dashboards/:id", mw, func(c *gin.Context) { c.Status(http.StatusOK) })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		recorder.Run(ctx)
	}()
	requests(engine)
	cancel()
	<-done
}

func starterIssues(t *testing.T, d *dbtest.Database, title string) (*spec.Dashboard, []spec.Issue) {
	t.Helper()
	logger := slog.New(slog.DiscardHandler)
	inst, err := auditsource.Instance(trail.NewReader(d.DB, logger), logger)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := datasource.NewRegistry(logger)
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.AddBuiltIn(inst); err != nil {
		t.Fatal(err)
	}
	starters, err := Starters()
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range starters {
		if st.Title != title {
			continue
		}
		dash, err := spec.Parse(st.Spec)
		if err != nil {
			t.Fatal(err)
		}
		ctx := datasource.WithAccess(t.Context(), datasource.Access{System: true})
		issues, err := queryproxy.NewRunner(registry, logger).CheckSpec(ctx, dash, time.Now(), datasource.QueryUser{})
		if err != nil {
			t.Fatal(err)
		}
		return dash, issues
	}
	t.Fatalf("no starter dashboard %q", title)
	return nil, nil
}

func issuesOfPanel(d *spec.Dashboard, issues []spec.Issue, id string) []spec.Issue {
	var out []spec.Issue
	for i, p := range d.Panels {
		if p.ID != id {
			continue
		}
		prefix := fmt.Sprintf("/panels/%d", i)
		for _, issue := range issues {
			if issue.Path == prefix || strings.HasPrefix(issue.Path, prefix+"/") {
				out = append(out, issue)
			}
		}
	}
	return out
}

func TestMostViewedDashboardsCountsTheViewsTheTrailRecords(t *testing.T) {
	d := dbtest.New(t)
	recordRequests(t, d, func(engine *gin.Engine) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboards/"+viewedDashboard, nil)
		engine.ServeHTTP(httptest.NewRecorder(), req)
	})
	dash, issues := starterIssues(t, d, "Data access")
	if got := issuesOfPanel(dash, issues, "top_dashboards"); len(got) != 0 {
		t.Fatalf("Most viewed dashboards after a view: %+v, want rows", got)
	}
}

func TestAnAgentAndMCPPanelWithNoEventsYetIsNotAHealthProblem(t *testing.T) {
	d := dbtest.New(t)
	dash, issues := starterIssues(t, d, "Agent and MCP activity")
	for _, id := range []string{"top_agents", "top_clients", "agent_events"} {
		if got := issuesOfPanel(dash, issues, id); len(got) != 0 {
			t.Errorf("%s on an empty trail: %+v, want none", id, got)
		}
	}
}
