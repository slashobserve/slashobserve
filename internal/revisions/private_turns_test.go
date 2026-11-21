package revisions_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/agentsview"
	"github.com/slashobserve/slashobserve/internal/audit"
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

func chatIDIn(t *testing.T, body string) *string {
	t.Helper()
	var read struct {
		ChatID  *string `json:"chat_id"`
		Request *struct {
			ChatID *string `json:"chat_id"`
		} `json:"request"`
	}
	if err := json.Unmarshal([]byte(body), &read); err != nil {
		t.Fatal(err)
	}
	if read.Request != nil {
		return read.Request.ChatID
	}
	return read.ChatID
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
	inv := investigate(t, svc, revisions.InvestigateParams{DashboardID: dash, Prompt: "why is checkout failing"}).ID
	homeChat, followUpChat := chatOfTurn(t, d, home), chatOfTurn(t, d, followUp)
	failAllRuns(t, svc, d, home.String(), followUp.String(), inv.String())

	w := newPrivateWalk(t, svc, d)
	private := []string{home.String()}
	shared := []string{followUp.String(), inv.String()}
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

		for _, id := range shared {
			w.http(tc.who, "/api/v1/observe/"+id, http.StatusOK)
			w.mcp(tc.who, "get_run", `{"id":"`+id+`"}`, false)
		}
		mustNotContain(t, name, "search", found, private...)
		mustContain(t, name, "activity", feed, shared...)

		if tc.full {
			mustContain(t, name, "the chat list", listed, homeChat, followUpChat)
			mustContain(t, name, "the run list", listedRuns, home.String(), inv.String())
			mustContain(t, name, "the Agents page", agents, home.String(), inv.String())
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
				mustContain(t, name, "the inbox", inbox, home.String(), inv.String())
			} else {
				mustNotContain(t, name, "the inbox", inbox, home.String(), inv.String())
			}
			continue
		}

		mustNotContain(t, name, "the chat list", listed, homeChat, followUpChat)
		mustNotContain(t, name, "the run list", listedRuns, private...)
		mustNotContain(t, name, "the Agents page", agents, private...)
		mustNotContain(t, name, "activity", feed, home.String())
		mustNotContain(t, name, "the inbox", inbox, home.String(), inv.String())
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

func recordedReads(t *testing.T, d *dbtest.Database) []string {
	t.Helper()
	rows, err := d.SQLDB(t).QueryContext(t.Context(), `SELECT concat_ws(' ', action, coalesce(actor_id::text, '-'), outcome, coalesce(nullif(target_id, ''), '-'), coalesce(chat_id::text, '-'), coalesce(data->>'owner_id', data->>'scope', '-'))
		FROM audit_events WHERE action IN ('run.view', 'run.list', 'chat.view', 'chat.list')`)
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

func TestAReadOfSomeoneElsesRunOrChatIsRecordedOncePerReaderInTheWindowAndARefusedReadEveryTime(t *testing.T) {
	d := dbtest.New(t)
	svc := newTestService(d.DB, succeedWith(2), quietLogger(), revisions.WithBuilds(&recordingBuilds{}))
	run := startTurn(t, svc, user, revisions.StartRequest{Prompt: text("which logs exist")}).String()
	chat := run
	eve := auth.Principal{Kind: auth.KindUser, ID: strangerID, Name: "Eve", Role: auth.RoleEditor}
	recorder := audit.New(audit.Config{DB: d.DB, Logger: quietLogger()})
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
	registry := tools.New(tools.Deps{Runs: revisions.NewRunDesk(svc, revisions.ObserveOptions{}), Logger: quietLogger()})
	for _, read := range []struct {
		who  auth.Principal
		want int
	}{{user, http.StatusOK}, {bea, http.StatusOK}, {eve, http.StatusNotFound}} {
		gin.SetMode(gin.TestMode)
		engine := gin.New()
		who := read.who
		revisions.NewObserveHandler(svc, revisions.ObserveOptions{}).Register(auth.NewRoutes(engine.Group("/api/v1", recorded, func(c *gin.Context) {
			ctx := auth.WithPrincipal(c.Request.Context(), who)
			audit.Update(ctx, func(s *audit.Scope) { s.Actor = auth.AuditActor(who) })
			c.Request = c.Request.WithContext(ctx)
		})))
		for _, path := range []string{"/api/v1/chats/" + chat, "/api/v1/observe/" + run, "/api/v1/observe/" + run + "/build"} {
			if rec := call(engine, http.MethodGet, path, ""); rec.Code != read.want {
				t.Fatalf("%s read %s with %d: %s", who.Name, path, rec.Code, rec.Body)
			}
		}
		lists := []string{"/api/v1/chats", "/api/v1/observe"}
		if who.Role.AtLeast(auth.RoleAdmin) {
			lists = append(lists, "/api/v1/chats?scope=all", "/api/v1/observe?scope=all")
		}
		for _, path := range lists {
			if rec := call(engine, http.MethodGet, path, ""); rec.Code != http.StatusOK {
				t.Fatalf("%s listed %s with %d: %s", who.Name, path, rec.Code, rec.Body)
			}
		}
		toolCtx := audit.Open(auth.WithPrincipal(t.Context(), who), audit.Scope{Actor: auth.AuditActor(who), Initiator: audit.InitiatorPerson, Origin: audit.Origin{Kind: "mcp"}})
		calls := [][2]string{{"get_chat", `{"id":"` + chat + `"}`}, {"get_run", `{"id":"` + run + `"}`}, {"get_run_build", `{"id":"` + run + `"}`}, {"list_chats", `{}`}, {"list_runs", `{}`}}
		if who.Role.AtLeast(auth.RoleAdmin) {
			calls = append(calls, [2]string{"list_chats", `{"scope":"all"}`}, [2]string{"list_runs", `{"scope":"all"}`})
		}
		for _, c := range calls {
			if _, err := registry.Call(toolCtx, c[0], []byte(c[1])); err != nil {
				t.Fatalf("%s: %s: %v", who.Name, c[0], err)
			}
		}
	}
	stop()
	<-done

	want := []string{
		"chat.list " + adminID + " success - - all",
		"chat.view " + adminID + " success " + chat + " " + chat + " " + userID,
		"chat.view " + strangerID + " denied " + chat + " - -",
		"chat.view " + strangerID + " denied " + chat + " - -",
		"run.list " + adminID + " success - - all",
		"run.view " + adminID + " success " + run + " " + chat + " " + userID,
		"run.view " + strangerID + " denied " + run + " - -",
		"run.view " + strangerID + " denied " + run + " - -",
		"run.view " + strangerID + " denied " + run + " - -",
		"run.view " + strangerID + " denied " + run + " - -",
	}
	slices.Sort(want)
	if got := recordedReads(t, d); !slices.Equal(got, want) {
		t.Fatalf("recorded reads =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestAReaderOfTheDashboardAChatSavedSeesNothingElseOfTheChat(t *testing.T) {
	svc, d := newService(t, reviserFunc(func(context.Context, revisions.Request) (revisions.Result, error) {
		return revisions.Result{FailureCode: revisions.FailureRateLimited}, errors.New("failed")
	}))
	eve := auth.Principal{Kind: auth.KindUser, ID: strangerID, Name: "Eve", Role: auth.RoleEditor}
	question := startTurn(t, svc, user, revisions.StartRequest{Prompt: text("which logs mention " + privateWord)})
	d.Exec(t, `UPDATE agent_requests SET state = 'succeeded', status = 'succeeded', finished_at = now() WHERE id = $1::uuid`, question.String())
	build := startTurn(t, svc, user, revisions.StartRequest{Prompt: text("now build a dashboard of it"), ChatID: text(question.String())})
	failAllRuns(t, svc, d, build.String())
	if got := storedRun(t, svc, build.String()); !strings.Contains(got.ThreadContext.String, privateWord) {
		t.Fatalf("the second turn stored no transcript of the first: %q", got.ThreadContext.String)
	}
	d.Exec(t, `UPDATE agent_requests SET dashboard_id = $2::uuid WHERE id = $1::uuid`, build.String(), seedDashboard(t, d, 1).String())

	w := newPrivateWalk(t, svc, d)
	for _, body := range []string{w.http(eve, "/api/v1/observe/"+build.String(), http.StatusOK), w.mcp(eve, "get_run", `{"id":"`+build.String()+`"}`, false)} {
		if chat := chatIDIn(t, body); chat != nil {
			t.Errorf("a reader of the dashboard is told the chat %s", *chat)
		}
		mustNotContain(t, eve.Name, "the saved turn", body, privateWord, question.String())
	}
	w.http(eve, "/api/v1/observe/"+question.String(), http.StatusNotFound)
	w.http(eve, "/api/v1/chats/"+question.String(), http.StatusNotFound)
	w.mcp(eve, "get_chat", `{"id":"`+question.String()+`"}`, true)
	mustNotContain(t, eve.Name, "their own turns of the chat", w.http(eve, "/api/v1/observe?chat_id="+question.String(), http.StatusOK), build.String(), question.String())
	for _, who := range []auth.Principal{user, bea} {
		for _, body := range []string{w.http(who, "/api/v1/observe/"+build.String(), http.StatusOK), w.mcp(who, "get_run", `{"id":"`+build.String()+`"}`, false)} {
			if chat := chatIDIn(t, body); chat == nil || *chat != question.String() {
				t.Errorf("%s reads the chat id %v, want %s", who.Name, chat, question)
			}
		}
	}
}

func TestADashboardReaderOfAChatsFirstTurnIsNotToldTheChatsTitle(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	eve := auth.Principal{Kind: auth.KindUser, ID: strangerID, Name: "Eve", Role: auth.RoleEditor}
	first := startTurn(t, svc, user, revisions.StartRequest{Prompt: text("build a checkout dashboard")})
	d.Exec(t, `UPDATE agent_requests SET dashboard_id = $2::uuid, title = $3 WHERE id = $1::uuid`, first.String(), seedDashboard(t, d, 1).String(), privateWord)
	d.Exec(t, `UPDATE chats SET title = $2 WHERE id = $1::uuid`, first.String(), privateWord)

	w := newPrivateWalk(t, svc, d)
	for _, body := range []string{w.http(eve, "/api/v1/observe/"+first.String(), http.StatusOK), w.mcp(eve, "get_run", `{"id":"`+first.String()+`"}`, false)} {
		mustNotContain(t, eve.Name, "the first turn", body, privateWord)
	}
	mustContain(t, user.Name, "the first turn", w.http(user, "/api/v1/observe/"+first.String(), http.StatusOK), privateWord)
}

func TestARunInAnObservationIsReadByEveryoneWhoReadsTheObservation(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	eve := auth.Principal{Kind: auth.KindUser, ID: strangerID, Name: "Eve", Role: auth.RoleEditor}
	for _, p := range []auth.Principal{user, eve} {
		d.Exec(t, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, name) VALUES ($1::uuid, 'https://idp', $2, $3, $2)`, p.ID, p.Name, p.Name+"@example.com")
	}
	shared := observeFor(t, svc, user, "what does the thread say")
	private := observeFor(t, svc, user, "what do my notes say")
	observationIn(t, d, "SELECT general_folder_id()", "Checkout errors", shared)
	observationIn(t, d, "SELECT id FROM folders WHERE owner_id = '"+userID+"'::uuid", "Ada's notes", private)

	w := newPrivateWalk(t, svc, d)
	for _, body := range []string{w.http(eve, "/api/v1/observe/"+shared, http.StatusOK), w.mcp(eve, "get_run", `{"id":"`+shared+`"}`, false)} {
		if chat := chatIDIn(t, body); chat != nil {
			t.Errorf("a reader of the observation is told the chat %s", *chat)
		}
	}
	w.http(eve, "/api/v1/observe/"+private, http.StatusNotFound)
	w.mcp(eve, "get_run", `{"id":"`+private+`"}`, true)
}
