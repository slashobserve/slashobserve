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
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/origin"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

type buildsFailingWith struct{ err error }

func (b buildsFailingWith) ViewBuild(context.Context, auth.Principal, pgtype.UUID) (revisions.BuildView, error) {
	return revisions.BuildView{}, b.err
}

func (b buildsFailingWith) SaveStoppedBuild(context.Context, auth.Principal, pgtype.UUID) (revisions.SavedBuild, error) {
	return revisions.SavedBuild{}, b.err
}

type startsFailingWith struct {
	*revisions.RunDesk
	err error
}

func (s startsFailingWith) Start(context.Context, auth.Principal, revisions.StartRequest) (json.RawMessage, error) {
	return nil, s.err
}

func recordedEngine(t *testing.T, svc *revisions.Service, recorder *audit.Recorder, who auth.Principal) *gin.Engine {
	t.Helper()
	recorded, err := recorder.Middleware()
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	revisions.NewObserveHandler(svc, revisions.ObserveOptions{}).Register(auth.NewRoutes(engine.Group("/api/v1", recorded, func(c *gin.Context) {
		ctx := auth.WithPrincipal(c.Request.Context(), who)
		audit.Update(ctx, func(s *audit.Scope) { s.Actor = auth.AuditActor(who) })
		c.Request = c.Request.WithContext(ctx)
	})))
	return engine
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
	svc := newTestService(d.DB, succeedWith(2), quietLogger(), revisions.WithBuilds(buildsFailingWith{revisions.ErrNoBuild}))
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
	registry := tools.New(tools.Deps{Runs: revisions.NewRunDesk(svc, revisions.ObserveOptions{}), Logger: quietLogger()})
	eve := auth.Principal{Kind: auth.KindUser, ID: strangerID, Name: "Eve", Role: auth.RoleEditor}
	const noSuch = "00000000-0000-4000-8000-000000000009"

	refuse := func(name string, who auth.Principal, body string, want int) {
		t.Helper()
		if rec := call(recordedEngine(t, svc, recorder, who), http.MethodPost, "/api/v1/observe", body); rec.Code != want {
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

func TestAnAnswerOnlyRunsBuildOverHTTPIsNoRefusedReadAsOverMCP(t *testing.T) {
	d := dbtest.New(t)
	svc := newTestService(d.DB, succeedWith(2), quietLogger(), revisions.WithBuilds(buildsFailingWith{revisions.ErrNoBuild}))
	run := startTurn(t, svc, user, revisions.StartRequest{Prompt: text("which logs exist")}).String()
	recorder, flush := recordingAudit(t, d)

	for _, who := range []auth.Principal{user, bea} {
		if rec := call(recordedEngine(t, svc, recorder, who), http.MethodGet, "/api/v1/observe/"+run+"/build", ""); rec.Code != http.StatusNotFound {
			t.Fatalf("%s: GET build answered %d, want 404: %s", who.Name, rec.Code, rec.Body)
		}
	}
	flush()

	want := []string{
		"run.view " + userID + " success " + run + " " + run + " " + userID,
		"run.view " + adminID + " success " + run + " " + run + " " + userID,
	}
	slices.Sort(want)
	if got := recordedReads(t, d); !slices.Equal(got, want) {
		t.Fatalf("recorded reads = %q, want %q", got, want)
	}
}

func TestARefusedBuildOverHTTPIsARefusedReadAsOverMCP(t *testing.T) {
	d := dbtest.New(t)
	svc := newTestService(d.DB, succeedWith(2), quietLogger(), revisions.WithBuilds(buildsFailingWith{revisions.ErrBuildNotFound}))
	run := startTurn(t, svc, user, revisions.StartRequest{Prompt: text("which logs exist")}).String()
	recorder, flush := recordingAudit(t, d)

	if rec := call(recordedEngine(t, svc, recorder, user), http.MethodGet, "/api/v1/observe/"+run+"/build", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("GET build answered %d, want 404: %s", rec.Code, rec.Body)
	}
	flush()

	want := []string{
		"run.view " + userID + " denied " + run + " - -",
		"run.view " + userID + " success " + run + " " + run + " " + userID,
	}
	if got := recordedReads(t, d); !slices.Equal(got, want) {
		t.Fatalf("recorded reads = %q, want %q", got, want)
	}
}

func TestAPromptWithNoVisibleTextIsRefusedWithItsReasonOverHTTP(t *testing.T) {
	d := dbtest.New(t)
	svc := newTestService(d.DB, succeedWith(2), quietLogger())

	rec := call(newObserveRouter(svc), http.MethodPost, "/api/v1/observe", `{"prompt":"\u200b"}`)
	var body struct {
		Code   string `json:"code"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != http.StatusBadRequest || body.Code != "invalid_request" || body.Reason != "text_invisible_only" {
		t.Fatalf("POST answered %d %s, want 400 invalid_request with reason text_invisible_only", rec.Code, rec.Body)
	}
}

func TestAStartThatFailsOverMCPLeavesTheRunQueueEventHTTPLeaves(t *testing.T) {
	d := dbtest.New(t)
	svc := newTestService(d.DB, succeedWith(2), quietLogger())
	_, flush := recordingAudit(t, d)
	desk := revisions.NewRunDesk(svc, revisions.ObserveOptions{})
	internal := tools.New(tools.Deps{Runs: startsFailingWith{desk, errors.New("the store is down")}, Logger: quietLogger()})
	gone := tools.New(tools.Deps{Runs: startsFailingWith{desk, context.Canceled}, Logger: quietLogger()})
	left, leave := context.WithCancel(mcpContext(t, user))
	leave()

	for name, call := range map[string]func() (tools.Result, error){
		"an internal failure": func() (tools.Result, error) {
			return internal.Call(mcpContext(t, user), "start_run", []byte(`{"prompt":"x"}`))
		},
		"a caller who went away": func() (tools.Result, error) {
			return gone.Call(left, "start_run", []byte(`{"prompt":"x"}`))
		},
	} {
		if res, err := call(); err == nil && !res.IsError {
			t.Fatalf("%s: start_run = %+v; want an error", name, res)
		}
	}
	flush()

	want := []queueEvent{{outcome: "failed", reason: "internal", status: "500"}, {outcome: "failed", reason: "-", status: "499"}}
	got := queueEvents(t, d, true)
	slices.SortFunc(got, func(a, b queueEvent) int { return strings.Compare(a.status, b.status) })
	slices.SortFunc(want, func(a, b queueEvent) int { return strings.Compare(a.status, b.status) })
	if !slices.Equal(got, want) {
		t.Fatalf("run.queue events over MCP = %+v, want %+v", got, want)
	}
}

func TestARefusedBuildOfAReadableRunIsStillARefusedRead(t *testing.T) {
	d := dbtest.New(t)
	svc := newTestService(d.DB, succeedWith(2), quietLogger(), revisions.WithBuilds(buildsFailingWith{revisions.ErrBuildNotFound}))
	run := startTurn(t, svc, user, revisions.StartRequest{Prompt: text("which logs exist")}).String()
	_, flush := recordingAudit(t, d)
	registry := tools.New(tools.Deps{Runs: revisions.NewRunDesk(svc, revisions.ObserveOptions{}), Logger: quietLogger()})

	res, err := registry.Call(mcpContext(t, bea), "get_run_build", []byte(`{"id":"`+run+`"}`))
	if err != nil || !res.IsError {
		t.Fatalf("get_run_build = %+v, %v; want a tool error", res, err)
	}
	flush()

	want := []string{
		"run.view " + adminID + " success " + run + " " + run + " " + userID,
		"run.view " + adminID + " denied " + run + " - -",
	}
	slices.Sort(want)
	if got := recordedReads(t, d); !slices.Equal(got, want) {
		t.Fatalf("recorded reads = %q, want %q", got, want)
	}
}

func TestADashboardRevisionOrSuggestionReadIsARunRead(t *testing.T) {
	svc, d := newSuggestService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, "add a latency panel")
	suggestion := suggestFor(t, svc, dash, user)
	_, flush := recordingAudit(t, d)

	if _, err := svc.Get(mcpContext(t, bea), dash, mustUUID(t, rev.ID)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.List(mcpContext(t, bea), dash, 10, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetSuggestion(mcpContext(t, user), dash, suggestion.ID, user); err != nil {
		t.Fatal(err)
	}
	flush()

	want := []string{
		"run.list " + adminID + " success " + dash.String() + " - dashboard",
		"run.view " + adminID + " success " + rev.ID + " - " + userID,
		"run.view " + userID + " success " + suggestion.ID.String() + " - " + userID,
	}
	slices.Sort(want)
	if got := recordedReads(t, d); !slices.Equal(got, want) {
		t.Fatalf("recorded reads =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
