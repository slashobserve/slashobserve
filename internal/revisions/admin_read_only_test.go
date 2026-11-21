package revisions_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func answerActionsRouterAs(t *testing.T, svc *revisions.Service, who auth.Principal) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	opts := revisions.ObserveOptions{
		ObservationURL: func(id string) string { return "/o/" + id },
		SaveAnswer: func(_ context.Context, by auth.Principal, id pgtype.UUID) (revisions.SavedAnswer, error) {
			t.Errorf("%s reached the answer save of run %s", by.Name, id)
			return revisions.SavedAnswer{}, revisions.ErrRequestNotFound
		},
		ShareAnswer: func(_ context.Context, by auth.Principal, id, _ pgtype.UUID) (revisions.SharedAnswer, error) {
			t.Errorf("%s reached the answer share of run %s", by.Name, id)
			return revisions.SharedAnswer{}, revisions.ErrRequestNotFound
		},
	}
	revisions.NewObserveHandler(svc, opts).Register(auth.NewRoutes(r.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(datasource.WithAccess(auth.WithPrincipal(c.Request.Context(), who), datasource.Access{By: who}))
	})))
	return r
}

func runOfFinding(t *testing.T, d *dbtest.Database, finding string) dbRun {
	t.Helper()
	var run dbRun
	if err := d.SQLDB(t).QueryRowContext(t.Context(), "SELECT agent_request_id::text, chat_id::text FROM findings JOIN agent_requests ON agent_requests.id = findings.agent_request_id WHERE findings.id = $1::uuid", finding).Scan(&run.id, &run.chat); err != nil {
		t.Fatal(err)
	}
	return run
}

type dbRun struct{ id, chat string }

func TestAnAdminReadsButCannotActOnSomeoneElsesPrivateAsk(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	finding := suggestingAnswer(t, svc, d, dash, text(declinesPanel))
	answered := runOfFinding(t, d, finding)
	ask := func(prompt string) string {
		return startTurn(t, svc, user, revisions.StartRequest{Prompt: text(prompt), Kind: text("ask"), DashboardID: text(dash.String()), ChatID: text(answered.chat)}).String()
	}
	queued := ask("is checkout slow?")
	failed := ask("are refunds failing?")
	setState(t, d, failed, "failed")
	var before int
	if err := d.SQLDB(t).QueryRowContext(t.Context(), "SELECT count(*) FROM agent_requests").Scan(&before); err != nil {
		t.Fatal(err)
	}

	router := answerActionsRouterAs(t, svc, bea)
	for _, id := range []string{answered.id, queued, failed} {
		if rec := call(router, http.MethodGet, "/api/v1/observe/"+id, ""); rec.Code != http.StatusOK {
			t.Fatalf("an Admin reading the private ask %s: %d %s", id, rec.Code, rec.Body)
		}
	}
	for name, w := range map[string]struct {
		method, path, body string
		want               int
	}{
		"follow up":            {http.MethodPost, "/api/v1/observe", `{"prompt":"and refunds?","chat_id":"` + answered.chat + `"}`, http.StatusForbidden},
		"revise in the chat":   {http.MethodPost, "/api/v1/observe", `{"prompt":"add a refunds panel","kind":"revise","dashboard_id":"` + dash.String() + `","chat_id":"` + answered.chat + `"}`, http.StatusForbidden},
		"apply the suggestion": {http.MethodPost, "/api/v1/observe", `{"kind":"revise","dashboard_id":"` + dash.String() + `","applies_finding_id":"` + finding + `"}`, http.StatusNotFound},
		"share the answer":     {http.MethodPost, "/api/v1/observe/" + answered.id + "/share", "", http.StatusNotFound},
		"save the answer":      {http.MethodPost, "/api/v1/observe/" + answered.id + "/observation", "", http.StatusNotFound},
		"cancel":               {http.MethodPost, "/api/v1/observe/" + queued + "/cancel", "", http.StatusNotFound},
		"retry":                {http.MethodPost, "/api/v1/observe/" + failed + "/retry", "", http.StatusNotFound},
		"continue":             {http.MethodPost, "/api/v1/observe/" + failed + "/continue", "", http.StatusNotFound},
		"pin":                  {http.MethodPatch, "/api/v1/observe/" + answered.id, `{"pinned":true}`, http.StatusNotFound},
		"archive the run":      {http.MethodDelete, "/api/v1/observe/" + answered.id, "", http.StatusNotFound},
		"archive the chat":     {http.MethodDelete, "/api/v1/chats/" + answered.chat, "", http.StatusForbidden},
	} {
		if rec := call(router, w.method, w.path, w.body); rec.Code != w.want {
			t.Errorf("%s over HTTP by an Admin: %d %s, want %d", name, rec.Code, rec.Body, w.want)
		}
	}

	registry := tools.New(tools.Deps{Runs: revisions.NewRunDesk(svc, revisions.ObserveOptions{}), Logger: quietLogger()})
	for name, c := range map[string]struct{ tool, input string }{
		"follow up":            {"start_run", `{"prompt":"and refunds?","chat_id":"` + answered.chat + `"}`},
		"revise in the chat":   {"start_run", `{"prompt":"add a refunds panel","kind":"revise","dashboard_id":"` + dash.String() + `","chat_id":"` + answered.chat + `"}`},
		"apply the suggestion": {"start_run", `{"kind":"revise","dashboard_id":"` + dash.String() + `","applies_finding_id":"` + finding + `"}`},
		"cancel":               {"cancel_run", `{"id":"` + queued + `"}`},
		"retry":                {"retry_run", `{"id":"` + failed + `"}`},
	} {
		res, err := registry.Call(mcpContext(t, bea), c.tool, []byte(c.input))
		if err != nil || !res.IsError {
			t.Errorf("%s over MCP by an Admin: %+v, %v; want a tool error", name, res, err)
		}
	}

	var after int
	if err := d.SQLDB(t).QueryRowContext(t.Context(), "SELECT count(*) FROM agent_requests").Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Errorf("runs after the Admin's refused actions = %d, want %d", after, before)
	}
	var state string
	if err := d.SQLDB(t).QueryRowContext(t.Context(), "SELECT state FROM agent_requests WHERE id = $1::uuid", queued).Scan(&state); err != nil || state != "queued" {
		t.Errorf("the queued private ask is %q after the Admin's cancels, %v", state, err)
	}
}
