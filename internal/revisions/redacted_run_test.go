package revisions_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

type runRead struct {
	path     string
	body     string
	redacted bool
	version  *int
	status   string
	asker    string
	actions  []string
}

var chatRunOptions = revisions.ObserveOptions{LiveText: func(string, int) (string, bool) { return "still reading " + privateWord, true }}

func chatRunEngine(svc *revisions.Service, who auth.Principal) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := auth.NewRoutes(engine.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(datasource.WithAccess(auth.WithPrincipal(c.Request.Context(), who), datasource.Access{By: who}))
	}))
	revisions.NewObserveHandler(svc, chatRunOptions).Register(api)
	svc.Register(api)
	return engine
}

func readsOfAChatRun(t *testing.T, svc *revisions.Service, dash, run string, who auth.Principal) []runRead {
	t.Helper()
	opts := chatRunOptions
	engine := chatRunEngine(svc, who)
	var reads []runRead
	type shown struct {
		Redacted       bool                  `json:"redacted"`
		ResultVersion  *int                  `json:"result_version"`
		Status         string                `json:"status"`
		RequestedBy    struct{ Name string } `json:"requested_by"`
		AllowedActions []string              `json:"allowed_actions"`
	}
	add := func(path, body string, view shown) {
		reads = append(reads, runRead{path: path, body: body, redacted: view.Redacted, version: view.ResultVersion, status: view.Status, asker: view.RequestedBy.Name, actions: view.AllowedActions})
	}
	get := func(path string) string {
		rec := call(engine, http.MethodGet, path, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: GET %s answered %d: %s", who.Name, path, rec.Code, rec.Body)
		}
		return rec.Body.String()
	}
	decode := func(raw string, into any) {
		if err := json.Unmarshal([]byte(raw), into); err != nil {
			t.Fatal(err)
		}
	}

	path := "/api/v1/observe/" + run
	body := get(path)
	var one struct{ Request shown }
	decode(body, &one)
	add(path, body, one.Request)

	registry := tools.New(tools.Deps{Runs: revisions.NewRunDesk(svc, opts), Logger: quietLogger()})
	res, err := registry.Call(auth.WithPrincipal(t.Context(), who), "get_run", []byte(`{"id":"`+run+`"}`))
	if err != nil || res.IsError {
		t.Fatalf("%s: get_run: %v %+v", who.Name, err, res)
	}
	raw, err := json.Marshal(res.Content)
	if err != nil {
		t.Fatal(err)
	}
	var tool shown
	decode(string(raw), &tool)
	add("get_run", string(raw), tool)

	path = "/api/v1/dashboards/" + dash + "/revisions"
	body = get(path)
	var list struct{ Revisions []json.RawMessage }
	decode(body, &list)
	for _, item := range list.Revisions {
		var rev struct {
			ID string `json:"id"`
			shown
		}
		decode(string(item), &rev)
		if rev.ID == run {
			add(path, body, rev.shown)
		}
	}
	path += "/" + run
	body = get(path)
	var envelope struct{ Revision shown }
	decode(body, &envelope)
	add(path, body, envelope.Revision)
	if len(reads) != 4 {
		t.Fatalf("%s: the dashboard's revisions do not list run %s", who.Name, run)
	}
	return reads
}

func TestAnotherEditorReadsARunOfSomeonesPrivateChatWithoutItsWords(t *testing.T) {
	svc, d := newService(t, reviserFunc(func(context.Context, revisions.Request) (revisions.Result, error) {
		return revisions.Result{}, errors.New("unused")
	}))
	eve := auth.Principal{Kind: auth.KindUser, ID: strangerID, Name: "Eve", Role: auth.RoleEditor}
	dash := seedDashboard(t, d, 2)
	run := startTurn(t, svc, user, revisions.StartRequest{Prompt: text("add a panel for " + privateWord), Kind: text(revisions.KindRevise), DashboardID: text(dash.String())}).String()
	d.Exec(t, `UPDATE agent_requests SET title = $2, continue_note = $2, state = 'running', status = 'running', attempts = 1, started_at = now(),
		steps = jsonb_build_array(jsonb_build_object('at', now(), 'attempt', 1, 'phase', 'discover', 'tool', 'query_range', 'status', 'ok', 'message', $2::text, 'duration_ms', 5))
		WHERE id = $1::uuid`, run, "the "+privateWord+" note")

	check := func(when string, wantVersion bool) {
		t.Helper()
		for _, who := range []auth.Principal{user, bea, eve} {
			redacted := who.ID == eve.ID
			for _, read := range readsOfAChatRun(t, svc, dash.String(), run, who) {
				if read.redacted != redacted || strings.Contains(read.body, privateWord) == redacted {
					t.Errorf("%s, %s: %s is redacted %v and shows the chat's words %v, want redacted %v", when, who.Name, read.path, read.redacted, strings.Contains(read.body, privateWord), redacted)
				}
				if read.asker != user.Name || read.status == "" || (read.version != nil) != wantVersion {
					t.Errorf("%s, %s: %s shows asker %q, status %q and version %v", when, who.Name, read.path, read.asker, read.status, read.version)
				}
			}
		}
	}
	check("running", false)
	d.Exec(t, `UPDATE agent_requests SET state = 'succeeded', status = 'succeeded', finished_at = now(), result_version = 2 WHERE id = $1::uuid`, run)
	check("saved", true)
}

func TestAReaderWhoSeesARunRedactedCannotRetryItsWordsIntoARunOfTheirOwn(t *testing.T) {
	svc, d := newService(t, reviserFunc(func(context.Context, revisions.Request) (revisions.Result, error) {
		return revisions.Result{}, errors.New("unused")
	}))
	eve := auth.Principal{Kind: auth.KindUser, ID: strangerID, Name: "Eve", Role: auth.RoleEditor}
	dash := seedDashboard(t, d, 1)
	run := startTurn(t, svc, user, revisions.StartRequest{Prompt: text("add a panel for " + privateWord), Kind: text(revisions.KindRevise), DashboardID: text(dash.String())}).String()
	setState(t, d, run, "failed")

	for _, who := range []auth.Principal{user, eve} {
		mayRetry := who.ID == user.ID
		for _, read := range readsOfAChatRun(t, svc, dash.String(), run, who) {
			if slices.Contains(read.actions, "retry") != mayRetry {
				t.Errorf("%s: %s offers %v, want retry %v", who.Name, read.path, read.actions, mayRetry)
			}
		}
	}
	rec := call(chatRunEngine(svc, eve), http.MethodPost, "/api/v1/observe/"+run+"/retry", "")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "invalid_transition") || strings.Contains(rec.Body.String(), privateWord) {
		t.Fatalf("a redacted reader's retry answered %d: %s", rec.Code, rec.Body)
	}
	if rec := call(chatRunEngine(svc, user), http.MethodPost, "/api/v1/observe/"+run+"/retry", ""); rec.Code != http.StatusAccepted {
		t.Fatalf("the requester's retry answered %d: %s", rec.Code, rec.Body)
	}
}
