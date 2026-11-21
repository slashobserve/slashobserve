package revisions_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/agentsview"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/internal/search"
)

const privateWord = "quokkaledger"

type privateWalk struct {
	t        *testing.T
	svc      *revisions.Service
	d        *dbtest.Database
	registry *tools.Registry
	seen     map[string][]string
}

func newPrivateWalk(t *testing.T, svc *revisions.Service, d *dbtest.Database) *privateWalk {
	store := activity.NewStore(d.DB, folders.NewAccess(d.DB))
	registry := tools.New(tools.Deps{Runs: revisions.NewRunDesk(svc, revisions.ObserveOptions{}), Activity: store, Logger: quietLogger()})
	return &privateWalk{t: t, svc: svc, d: d, registry: registry, seen: map[string][]string{}}
}

func (w *privateWalk) engineFor(who auth.Principal) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	access := folders.NewAccess(w.d.DB)
	api := auth.NewRoutes(r.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(datasource.WithAccess(auth.WithPrincipal(c.Request.Context(), who), datasource.Access{By: who}))
	}))
	revisions.NewObserveHandler(w.svc, revisions.ObserveOptions{}).Register(api)
	agentsview.NewHandler(w.d.DB, agentsview.Observe{Enabled: true}, quietLogger()).Register(api)
	activity.NewHandler(activity.NewStore(w.d.DB, access), quietLogger()).Register(api)
	search.NewHandler(search.New(w.d.DB, access, &url.URL{Scheme: "http", Host: "localhost"}), quietLogger()).Register(api)
	return r
}

func (w *privateWalk) http(who auth.Principal, path string, want int) string {
	w.t.Helper()
	rec := call(w.engineFor(who), http.MethodGet, path, "")
	if rec.Code != want {
		w.t.Fatalf("%s: GET %s answered %d, want %d: %s", who.Name, path, rec.Code, want, rec.Body)
	}
	w.seen[who.Name] = append(w.seen[who.Name], rec.Body.String())
	return rec.Body.String()
}

func (w *privateWalk) mcp(who auth.Principal, tool, input string, wantError bool) string {
	w.t.Helper()
	res, err := w.registry.Call(auth.WithPrincipal(w.t.Context(), who), tool, []byte(input))
	if err != nil {
		w.t.Fatalf("%s: %s: %v", who.Name, tool, err)
	}
	body, err := json.Marshal(res.Content)
	if err != nil {
		w.t.Fatal(err)
	}
	if res.IsError != wantError {
		w.t.Fatalf("%s: %s %s answered error %v, want %v: %s", who.Name, tool, input, res.IsError, wantError, body)
	}
	w.seen[who.Name] = append(w.seen[who.Name], string(body))
	return string(body)
}

func (w *privateWalk) chatReadOnly(who auth.Principal, chat string) bool {
	w.t.Helper()
	var read struct {
		Chat struct {
			ReadOnly bool `json:"read_only"`
		} `json:"chat"`
	}
	if err := json.Unmarshal([]byte(w.http(who, "/api/v1/chats/"+chat, http.StatusOK)), &read); err != nil {
		w.t.Fatal(err)
	}
	return read.Chat.ReadOnly
}

func mustContain(t *testing.T, who, what, body string, ids ...string) {
	t.Helper()
	for _, id := range ids {
		if !strings.Contains(body, id) {
			t.Errorf("%s: %s does not show %s: %s", who, what, id, body)
		}
	}
}

func mustNotContain(t *testing.T, who, what, body string, ids ...string) {
	t.Helper()
	for _, id := range ids {
		if strings.Contains(body, id) {
			t.Errorf("%s: %s shows %s: %s", who, what, id, body)
		}
	}
}

func failAllRuns(t *testing.T, svc *revisions.Service, d *dbtest.Database, ids ...string) {
	t.Helper()
	stop := startWorker(t, svc, d, revisions.WorkerOptions{})
	for _, id := range ids {
		waitFor(t, "run "+id+" to fail", func() bool { return storedRun(t, svc, id).State == string(revisions.StateFailed) })
	}
	stop()
}

func TestAPrivateTurnIsReadByItsOwnerAndAdminsAndNobodyElse(t *testing.T) {
	svc, d := newService(t, reviserFunc(func(context.Context, revisions.Request) (revisions.Result, error) {
		return revisions.Result{FailureCode: revisions.FailureRateLimited}, errors.New("failed")
	}))
	eve := auth.Principal{Kind: auth.KindUser, ID: strangerID, Name: "Eve", Role: auth.RoleEditor}
	for _, p := range []auth.Principal{user, bea, eve, vic} {
		d.Exec(t, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, name) VALUES ($1::uuid, 'https://idp', $2, $3, $2)`, p.ID, p.Name, p.Name+"@example.com")
	}
	dash := seedDashboard(t, d, 1)
	home := startTurn(t, svc, user, revisions.StartRequest{Prompt: text("which logs mention " + privateWord)})
	followUp := startTurn(t, svc, user, revisions.StartRequest{Prompt: text("add a latency panel"), DashboardID: text(dash.String())})
	inv := investigate(t, svc, revisions.InvestigateParams{DashboardID: dash, Prompt: "why is " + privateWord + " failing"}).ID
	homeChat, followUpChat := chatOfTurn(t, d, home), chatOfTurn(t, d, followUp)
	failAllRuns(t, svc, d, home.String(), followUp.String(), inv.String())

	w := newPrivateWalk(t, svc, d)
	private := []string{home.String(), inv.String()}
	for _, tc := range []struct {
		who  auth.Principal
		full bool
	}{{user, true}, {bea, true}, {eve, false}, {vic, false}} {
		name := tc.who.Name
		chats, runs, mcpRuns, chatsTool := "/api/v1/chats", "/api/v1/observe", `{}`, `{}`
		if tc.who.Role.AtLeast(auth.RoleAdmin) {
			chats, runs, mcpRuns, chatsTool = chats+"?scope=all", runs+"?scope=all", `{"scope":"all"}`, `{"scope":"all"}`
		}
		if !tc.full {
			w.http(tc.who, "/api/v1/chats?scope=all", http.StatusForbidden)
			w.http(tc.who, "/api/v1/observe?scope=all", http.StatusForbidden)
			w.mcp(tc.who, "list_runs", `{"scope":"all"}`, true)
			w.mcp(tc.who, "list_chats", `{"scope":"all"}`, true)
		}
		listed := w.http(tc.who, chats, http.StatusOK) + w.mcp(tc.who, "list_chats", chatsTool, false)
		listedRuns := w.http(tc.who, runs, http.StatusOK) + w.mcp(tc.who, "list_runs", mcpRuns, false)
		agents := w.http(tc.who, "/api/v1/agents/runs?kind=builtin", http.StatusOK)
		feed := w.http(tc.who, "/api/v1/activity", http.StatusOK) + w.mcp(tc.who, "list_activity", `{}`, false)
		inbox := w.http(tc.who, "/api/v1/inbox", http.StatusOK)
		found := w.http(tc.who, "/api/v1/search?q="+privateWord, http.StatusOK)

		w.http(tc.who, "/api/v1/observe/"+followUp.String(), http.StatusOK)
		mustNotContain(t, name, "search", found, private...)
		mustContain(t, name, "activity", feed, inv.String())

		if tc.full {
			mustContain(t, name, "the chat list", listed, homeChat, followUpChat)
			mustContain(t, name, "the run list", listedRuns, private...)
			mustContain(t, name, "the Agents page", agents, private...)
			mustContain(t, name, "activity", feed, home.String())
			for _, id := range private {
				mustContain(t, name, "the run", w.http(tc.who, "/api/v1/observe/"+id, http.StatusOK)+w.mcp(tc.who, "get_run", `{"id":"`+id+`"}`, false), privateWord)
			}
			mustContain(t, name, "the chat", w.mcp(tc.who, "get_chat", `{"id":"`+homeChat+`"}`, false), privateWord)
			if got, want := w.chatReadOnly(tc.who, homeChat), tc.who.ID != user.ID; got != want {
				t.Errorf("%s: the chat read_only = %v, want %v", name, got, want)
			}
			w.chatReadOnly(tc.who, followUpChat)
			if tc.who.ID == user.ID {
				mustContain(t, name, "the inbox", inbox, private...)
			} else {
				mustNotContain(t, name, "the inbox", inbox, private...)
			}
			continue
		}

		mustNotContain(t, name, "the chat list", listed, homeChat, followUpChat)
		mustNotContain(t, name, "the run list", listedRuns, private...)
		mustNotContain(t, name, "the Agents page", agents, private...)
		mustNotContain(t, name, "activity", feed, home.String())
		mustNotContain(t, name, "the inbox", inbox, private...)
		for _, id := range private {
			w.http(tc.who, "/api/v1/observe/"+id, http.StatusNotFound)
			w.mcp(tc.who, "get_run", `{"id":"`+id+`"}`, true)
		}
		for _, chat := range []string{homeChat, followUpChat} {
			w.http(tc.who, "/api/v1/chats/"+chat, http.StatusNotFound)
			w.mcp(tc.who, "get_chat", `{"id":"`+chat+`"}`, true)
		}
		for _, body := range w.seen[name] {
			mustNotContain(t, name, "a read", body, privateWord)
		}
	}
}
