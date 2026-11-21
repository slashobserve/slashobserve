package revisions_test

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/origin"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

type noBuilds struct{}

func (noBuilds) ViewBuild(context.Context, auth.Principal, pgtype.UUID) (revisions.BuildView, error) {
	return revisions.BuildView{}, revisions.ErrBuildNotFound
}

func (noBuilds) SaveStoppedBuild(context.Context, auth.Principal, pgtype.UUID) (revisions.SavedBuild, error) {
	return revisions.SavedBuild{}, revisions.ErrBuildNotFound
}

func recordingAudit(t *testing.T, d *dbtest.Database) (*audit.Recorder, func()) {
	t.Helper()
	recorder := audit.New(audit.Config{DB: d.DB, Logger: quietLogger()})
	audit.Install(recorder)
	t.Cleanup(func() { audit.Install(nil) })
	ctx, stop := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		recorder.Run(ctx)
	}()
	return recorder, func() {
		stop()
		<-done
	}
}

func mcpContext(t *testing.T, who auth.Principal) context.Context {
	t.Helper()
	return audit.Open(origin.With(auth.WithPrincipal(t.Context(), who), origin.Origin{Kind: origin.MCP}), audit.Scope{Actor: auth.AuditActor(who), Initiator: audit.InitiatorPerson, Origin: audit.Origin{Kind: "mcp"}})
}

func TestAReadableRunWithNoBuildIsNotARefusedRead(t *testing.T) {
	d := dbtest.New(t)
	svc := newTestService(d.DB, succeedWith(2), quietLogger(), revisions.WithBuilds(noBuilds{}))
	run := startTurn(t, svc, user, revisions.StartRequest{Prompt: text("which logs exist")}).String()
	_, flush := recordingAudit(t, d)
	registry := tools.New(tools.Deps{Runs: revisions.NewRunDesk(svc, revisions.ObserveOptions{}), Logger: quietLogger()})
	eve := auth.Principal{Kind: auth.KindUser, ID: strangerID, Name: "Eve", Role: auth.RoleEditor}

	for _, who := range []auth.Principal{bea, eve} {
		res, err := registry.Call(mcpContext(t, who), "get_run_build", []byte(`{"id":"`+run+`"}`))
		if err != nil || !res.IsError {
			t.Fatalf("%s: get_run_build = %+v, %v; want a tool error", who.Name, res, err)
		}
	}
	flush()

	want := []string{
		"run.view " + adminID + " success " + run + " " + run + " " + userID,
		"run.view " + strangerID + " denied " + run + " - -",
	}
	slices.Sort(want)
	if got := recordedReads(t, d); !slices.Equal(got, want) {
		t.Fatalf("recorded reads = %q, want %q", got, want)
	}
}

type queueEvent struct {
	outcome, reason, status string
}

func queueEvents(t *testing.T, d *dbtest.Database, mcp bool) []queueEvent {
	t.Helper()
	rows, err := d.SQLDB(t).QueryContext(t.Context(), `SELECT outcome, coalesce(reason, '-'), coalesce(status::text, '-') FROM audit_events
		WHERE action = 'run.queue' AND outcome <> 'success' AND (origin = 'mcp') = $1 ORDER BY id`, mcp)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var got []queueEvent
	for rows.Next() {
		var e queueEvent
		if err := rows.Scan(&e.outcome, &e.reason, &e.status); err != nil {
			t.Fatal(err)
		}
		got = append(got, e)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestARefusedStartOverMCPLeavesTheSameRunQueueEventAsOverHTTP(t *testing.T) {
	d := dbtest.New(t)
	svc := newTestService(d.DB, succeedWith(2), quietLogger())
	dash := seedDashboard(t, d, 1).String()
	chat := startTurn(t, svc, user, revisions.StartRequest{Prompt: text("which logs exist")}).String()
	recorder, flush := recordingAudit(t, d)
	recorded, err := recorder.Middleware()
	if err != nil {
		t.Fatal(err)
	}
	registry := tools.New(tools.Deps{Runs: revisions.NewRunDesk(svc, revisions.ObserveOptions{}), Logger: quietLogger()})
	eve := auth.Principal{Kind: auth.KindUser, ID: strangerID, Name: "Eve", Role: auth.RoleEditor}
	const noSuch = "00000000-0000-4000-8000-000000000009"

	refuse := func(name string, who auth.Principal, body string, want int) {
		t.Helper()
		gin.SetMode(gin.TestMode)
		engine := gin.New()
		revisions.NewObserveHandler(svc, revisions.ObserveOptions{}).Register(auth.NewRoutes(engine.Group("/api/v1", recorded, func(c *gin.Context) {
			ctx := auth.WithPrincipal(c.Request.Context(), who)
			audit.Update(ctx, func(s *audit.Scope) { s.Actor = auth.AuditActor(who) })
			c.Request = c.Request.WithContext(ctx)
		})))
		if rec := call(engine, http.MethodPost, "/api/v1/observe", body); rec.Code != want {
			t.Fatalf("%s: HTTP answered %d, want %d: %s", name, rec.Code, want, rec.Body)
		}
		res, err := registry.Call(mcpContext(t, who), "start_run", []byte(body))
		if err != nil || !res.IsError {
			t.Fatalf("%s: start_run = %+v, %v; want a tool error", name, res, err)
		}
	}

	refuse("an Admin in someone's chat", bea, `{"prompt":"x","chat_id":"`+chat+`"}`, http.StatusForbidden)
	refuse("a chat that is not theirs", eve, `{"prompt":"x","chat_id":"`+chat+`"}`, http.StatusNotFound)
	refuse("a dashboard that never was", user, `{"prompt":"x","dashboard_id":"`+noSuch+`"}`, http.StatusNotFound)
	refuse("a Viewer revising a dashboard", vic, `{"prompt":"x","dashboard_id":"`+dash+`"}`, http.StatusForbidden)
	refuse("a Home chat on a board", user, `{"prompt":"x","chat_id":"`+chat+`","dashboard_id":"`+dash+`"}`, http.StatusBadRequest)
	startTurn(t, svc, user, revisions.StartRequest{Prompt: text("two")})
	startTurn(t, svc, user, revisions.StartRequest{Prompt: text("three")})
	refuse("one new-dashboard run too many", user, `{"prompt":"x"}`, http.StatusTooManyRequests)
	flush()

	overHTTP, overMCP := queueEvents(t, d, false), queueEvents(t, d, true)
	if len(overHTTP) != 6 || !slices.Equal(overHTTP, overMCP) {
		t.Fatalf("run.queue events over HTTP = %+v, over MCP = %+v; want the same six", overHTTP, overMCP)
	}
}
