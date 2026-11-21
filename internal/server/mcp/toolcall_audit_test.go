package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/opaquetoken"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

type auditedMCP struct {
	db       *dbtest.Database
	recorder *audit.Recorder
	edge     *gin.Engine
}

func mountAudited(t *testing.T, h http.Handler) *auditedMCP {
	t.Helper()
	database := dbtest.New(t)
	recorder := audit.New(audit.Config{DB: database.DB})
	audit.Install(recorder)
	t.Cleanup(func() { audit.Install(nil) })
	mw, err := recorder.Middleware()
	if err != nil {
		t.Fatal(err)
	}
	edge := gin.New()
	edge.Any("/mcp", mw, gin.WrapH(h))
	return &auditedMCP{db: database, recorder: recorder, edge: edge}
}

func (a *auditedMCP) post(body string, headers map[string]string, remote string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/mcp", strings.NewReader(body))
	if remote != "" {
		req.RemoteAddr = remote
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", "2025-11-25")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	a.edge.ServeHTTP(rec, req)
	return rec
}

func (a *auditedMCP) call(t *testing.T, header, tool string, args any) {
	t.Helper()
	rec := a.post(rpc(1, "tools/call", map[string]any{"name": tool, "arguments": args}), map[string]string{"Authorization": header}, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("tools/call = %d: %s", rec.Code, rec.Body)
	}
}

type mcpEvent struct {
	action, outcome, reason, actorKind, actorName, initiator, authMethod, credentialID, origin, originClient, tool, targetKind, targetID, runID, data string
	status                                                                                                                                            int
}

func (a *auditedMCP) events(t *testing.T, actions ...string) []mcpEvent {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a.recorder.Run(ctx)
	rows, err := a.db.SQLDB(t).QueryContext(t.Context(), `SELECT action, outcome, coalesce(reason, ''), actor_kind, coalesce(actor_name, ''), initiator, coalesce(auth_method, ''), coalesce(credential_id, ''),
  origin, coalesce(origin_client, ''), coalesce(tool, ''), coalesce(target_kind, ''), coalesce(target_id, ''), coalesce(run_id::text, ''), data::text, coalesce(status, 0)
FROM audit_events WHERE action = ANY($1) ORDER BY occurred_at, id`, actions)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var events []mcpEvent
	for rows.Next() {
		var e mcpEvent
		if err := rows.Scan(&e.action, &e.outcome, &e.reason, &e.actorKind, &e.actorName, &e.initiator, &e.authMethod, &e.credentialID, &e.origin, &e.originClient, &e.tool, &e.targetKind, &e.targetID, &e.runID, &e.data, &e.status); err != nil {
			t.Fatal(err)
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return events
}

func (a *auditedMCP) only(t *testing.T, action string) mcpEvent {
	t.Helper()
	events := a.events(t, action)
	if len(events) != 1 {
		t.Fatalf("%s events = %+v, want exactly one", action, events)
	}
	return events[0]
}

func (a *auditedMCP) mentions(t *testing.T, secret string) int {
	t.Helper()
	var n int
	err := a.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM audit_events WHERE position($1 in row_to_json(audit_events)::text) > 0`, secret).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func answering(res tools.Result, err error) func(context.Context, string, json.RawMessage) (tools.Result, error) {
	return func(context.Context, string, json.RawMessage) (tools.Result, error) { return res, err }
}

func TestAToolCallWithAnOAuthGrantIsRecordedForThePersonAndTheirClient(t *testing.T) {
	f := newFixture(t)
	a := mountAudited(t, f.handler)
	a.call(t, oauthTakes, "get_spec_guide", nil)
	want := mcpEvent{action: "agent.tool_call", outcome: "success", actorKind: "user", actorName: "Ada", initiator: "person", authMethod: "oauth", credentialID: "grant-1", origin: "mcp", originClient: "Claude Code", tool: "get_spec_guide", targetKind: "tool", targetID: "get_spec_guide", data: "{}"}
	if got := a.only(t, "agent.tool_call"); got != want {
		t.Fatalf("event = %+v, want %+v", got, want)
	}
}

type keyScopingAuth struct{ inner Authenticator }

func (k keyScopingAuth) Authenticate(ctx context.Context, header string) (Credential, bool, error) {
	cred, ok, err := k.inner.Authenticate(ctx, header)
	if ok && !cred.OAuth && cred.Run == nil && !cred.ClaimOnly {
		auth.Authenticated(ctx, cred.Principal, audit.AuthAPIKey, "key-1")
	}
	return cred, ok, err
}

func TestAToolCallWithAKeyIsRecordedForTheServiceAccount(t *testing.T) {
	f := newFixture(t)
	f.handler.(*handler).call = answering(tools.Result{Content: map[string]string{"ok": "true"}}, nil)
	f.handler.(*handler).authn = keyScopingAuth{inner: f.authn}
	a := mountAudited(t, f.handler)
	a.call(t, goodKey, "get_spec_guide", nil)
	got := a.only(t, "agent.tool_call")
	if got.actorKind != "service_account" || got.actorName != "ci" || got.authMethod != "api_key" || got.credentialID != "key-1" || got.origin != "mcp" || got.initiator != "person" || got.outcome != "success" {
		t.Fatalf("event = %+v", got)
	}
}

func TestAToolCallInsideARunCarriesTheRun(t *testing.T) {
	f := newRunFixture(t, tools.ModeCreate)
	a := mountAudited(t, f.handler)
	rec := a.post(rpc(1, "tools/call", map[string]any{"name": "get_spec_guide", "arguments": map[string]any{}}), map[string]string{"Authorization": runToken}, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("tools/call = %d: %s", rec.Code, rec.Body)
	}
	got := a.only(t, "agent.tool_call")
	if got.runID != runRequestID || got.authMethod != "api_key" || got.actorName != "Rae" || got.initiator != "agent" || got.origin != "mcp" || got.tool != "get_spec_guide" {
		t.Fatalf("event = %+v", got)
	}
}

func TestEverythingTheBuiltInAgentDoesWithItsRunKeyIsRecordedAsTheAgent(t *testing.T) {
	f := newAssignedRunFixture(t, tools.ModeCreate, revisions.AssignBuiltin)
	f.handler.call = func(ctx context.Context, _ string, _ json.RawMessage) (tools.Result, error) {
		by := audit.By{Actor: audit.Actor{Kind: audit.ActorUser, Name: "Rae"}, Origin: audit.Origin{Kind: "cli"}}
		if err := audit.RecordNow(ctx, audit.Event{Action: "run.finish", By: &by}); err != nil {
			return tools.Result{}, err
		}
		return tools.Result{Content: map[string]string{"ok": "true"}}, nil
	}
	a := mountAudited(t, f.handler)
	rec := a.post(rpc(1, "tools/call", map[string]any{"name": "get_spec_guide", "arguments": map[string]any{}}), map[string]string{"Authorization": runToken}, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("tools/call = %d: %s", rec.Code, rec.Body)
	}
	for _, action := range []string{"agent.tool_call", "run.finish"} {
		if got := a.only(t, action); got.origin != "agent" || got.runID != runRequestID {
			t.Errorf("%s = %+v, want origin agent in run %s", action, got, runRequestID)
		}
	}
}

func TestARunKeyHeldByAnOutsideAgentIsStillRecordedAsMCP(t *testing.T) {
	for _, assignee := range []revisions.AssigneeKind{revisions.AssignGrant, revisions.AssignServiceAccount} {
		f := newAssignedRunFixture(t, tools.ModeCreate, assignee)
		a := mountAudited(t, f.handler)
		rec := a.post(rpc(1, "tools/call", map[string]any{"name": "get_spec_guide", "arguments": map[string]any{}}), map[string]string{"Authorization": runToken}, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("tools/call = %d: %s", rec.Code, rec.Body)
		}
		if got := a.only(t, "agent.tool_call"); got.origin != "mcp" {
			t.Errorf("%s: %+v, want origin mcp", assignee, got)
		}
	}
}

func TestARefusedToolCallIsRecordedAsDeniedWithItsCode(t *testing.T) {
	f := newFixture(t)
	f.handler.(*handler).call = answering(tools.Result{IsError: true, Content: map[string]string{"code": "forbidden_role", "message": "needs editor"}}, nil)
	a := mountAudited(t, f.handler)
	a.call(t, goodKey, "get_spec_guide", nil)
	got := a.only(t, "agent.tool_call")
	if got.outcome != "denied" || got.reason != "forbidden_role" {
		t.Fatalf("event = %+v", got)
	}
}

func TestAToolErrorAndAToolFailureAreRecordedAsFailed(t *testing.T) {
	f := newFixture(t)
	a := mountAudited(t, f.handler)
	f.handler.(*handler).call = answering(tools.Result{IsError: true, Content: map[string]string{"code": "not_found"}}, nil)
	a.call(t, goodKey, "get_spec_guide", map[string]any{"note": "secret-argument-value"})
	f.handler.(*handler).call = answering(tools.Result{}, errors.New("database exploded with secret-detail"))
	a.call(t, goodKey, "get_spec_guide", nil)
	events := a.events(t, "agent.tool_call")
	if len(events) != 2 || events[0].outcome != "failed" || events[0].reason != "not_found" || events[1].outcome != "failed" || events[1].reason == "" {
		t.Fatalf("events = %+v", events)
	}
	if n := a.mentions(t, "secret-argument-value"); n != 0 {
		t.Fatalf("%d audit rows carry a tool argument", n)
	}
	if n := a.mentions(t, "secret-detail"); n != 0 {
		t.Fatalf("%d audit rows carry the failure text", n)
	}
}

func TestTheSidecarsClaimPlumbingIsNotRecordedAsToolCalls(t *testing.T) {
	h, _, _ := newSidecarHandler(t)
	a := mountAudited(t, h)
	body := rpc(1, "tools/call", map[string]any{"name": "claim_run", "arguments": map[string]any{"wait_seconds": 0}})
	rec := a.post(body, map[string]string{"Authorization": sidecarToken}, "127.0.0.1:50000")
	if rec.Code != http.StatusOK {
		t.Fatalf("claim_run = %d: %s", rec.Code, rec.Body)
	}
	if events := a.events(t, "agent.tool_call"); len(events) != 0 {
		t.Fatalf("events = %+v, want none", events)
	}
}

func TestARejectedMCPCredentialIsRecordedWithTheMCPOriginAndItsKind(t *testing.T) {
	f := newFixture(t)
	a := mountAudited(t, f.handler)
	token, _, err := opaquetoken.New("sok_")
	if err != nil {
		t.Fatal(err)
	}
	rec := a.post(rpc(1, "tools/list", map[string]any{}), map[string]string{"Authorization": "Bearer " + token}, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
	got := a.only(t, "auth.credential_rejected")
	want := mcpEvent{action: "auth.credential_rejected", outcome: "denied", reason: "unknown_credential", actorKind: "anonymous", initiator: "person", authMethod: "api_key", origin: "mcp", targetKind: "credential", targetID: "", data: `{"count": 1}`, status: 401}
	if got != want {
		t.Fatalf("event = %+v, want %+v", got, want)
	}
	if n := a.mentions(t, token); n != 0 {
		t.Fatalf("%d audit rows carry the rejected key", n)
	}
}

func TestASidecarTokenFromANonLoopbackPeerIsRecordedAsRejected(t *testing.T) {
	h, _, _ := newSidecarHandler(t)
	a := mountAudited(t, h)
	rec := a.post(rpc(1, "tools/list", map[string]any{}), map[string]string{"Authorization": sidecarToken}, "10.0.0.5:50000")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
	got := a.only(t, "auth.credential_rejected")
	if got.origin != "mcp" || got.authMethod != "api_key" || got.outcome != "denied" {
		t.Fatalf("event = %+v", got)
	}
}

func TestAnMCPRequestWithoutACredentialIsRecordedAsRefusedFromMCP(t *testing.T) {
	f := newFixture(t)
	a := mountAudited(t, f.handler)
	rec := a.post(rpc(1, "tools/list", map[string]any{}), nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
	got := a.only(t, "request.refuse")
	if got.origin != "mcp" || got.authMethod != "none" || got.outcome != "denied" || got.status != 401 {
		t.Fatalf("event = %+v", got)
	}
}

func TestAnMCPCallRefusedForTooManyRequestsIsRecordedWithTheMCPOriginAndItsKey(t *testing.T) {
	f := newFixture(t)
	h := f.handler.(*handler)
	h.authn = keyScopingAuth{inner: f.authn}
	key := principalKey(auth.Principal{Kind: auth.KindServiceAccount, ID: "sa-1"})
	h.slots.mu.Lock()
	h.slots.held[key] = maxInFlight
	h.slots.mu.Unlock()
	a := mountAudited(t, h)
	rec := a.post(rpc(1, "tools/list", map[string]any{}), map[string]string{"Authorization": goodKey}, "")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	got := a.only(t, "request.refuse")
	if got.outcome != "denied" || got.status != 429 || got.origin != "mcp" || got.authMethod != "api_key" || got.actorKind != "service_account" || got.credentialID != "key-1" {
		t.Fatalf("event = %+v", got)
	}
}

func TestAToolCallThatQueriedRecordsEachDatasourceAsAQueryRunWithItsTextAndFoldsARepeat(t *testing.T) {
	f := newFixture(t)
	long := strings.Repeat("a", 2*audit.MaxQueryBytes)
	f.handler.(*handler).call = func(ctx context.Context, _ string, input json.RawMessage) (tools.Result, error) {
		var in struct{ Query string }
		if err := json.Unmarshal(input, &in); err != nil {
			return tools.Result{}, err
		}
		from := time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)
		audit.NoteQuery(ctx, audit.Query{Datasource: "prom", Text: in.Query, From: from, To: from.Add(15 * time.Minute), Rows: 4})
		return tools.Result{Content: map[string]string{"ok": "true"}}, nil
	}
	a := mountAudited(t, f.handler)
	a.call(t, oauthTakes, "query_range", map[string]string{"query": "up"})
	a.call(t, oauthTakes, "query_range", map[string]string{"query": "up"})
	a.call(t, oauthTakes, "query_range", map[string]string{"query": long})
	events := a.events(t, "query.run")
	if len(events) != 2 {
		t.Fatalf("query.run events = %+v, want one for up and one for the long query, the repeat of up folded", events)
	}
	first := events[0]
	if first.outcome != "success" || first.tool != "query_range" || first.origin != "mcp" || first.targetKind != "datasource" || first.targetID != "prom" || first.actorName != "Ada" {
		t.Fatalf("event = %+v", first)
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(first.data), &data); err != nil {
		t.Fatal(err)
	}
	if data["query"] != "up" || data["row_count"] != float64(4) || data["from"] != "2026-11-21T10:00:00Z" || data["to"] != "2026-11-21T10:15:00Z" {
		t.Fatalf("data = %v", data)
	}
	if err := json.Unmarshal([]byte(events[1].data), &data); err != nil {
		t.Fatal(err)
	}
	if got, _ := data["query"].(string); len(got) != audit.MaxQueryBytes+len(" [truncated]") {
		t.Fatalf("a %d byte query was recorded as %d bytes", len(long), len(got))
	}
	if got := len(a.events(t, "agent.tool_call")); got != 3 {
		t.Fatalf("tool calls = %d, want every call recorded", got)
	}
}
