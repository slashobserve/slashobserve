package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/claims"
	"github.com/slashobserve/slashobserve/internal/origin"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

type folderEvent struct {
	Action, ActorKind, ActorID, ActorName, ActorRole, Initiator, Origin, Client, Tool, RunID, TargetName string
}

func (f *accessFixture) folderEvents(t *testing.T) []folderEvent {
	t.Helper()
	rows, err := f.db.SQLDB(t).QueryContext(t.Context(), `SELECT action, actor_kind, coalesce(actor_id::text, ''), coalesce(actor_name, ''), coalesce(actor_role, ''), initiator, origin,
  coalesce(origin_client, ''), coalesce(tool, ''), coalesce(run_id::text, ''), coalesce(target_name, '')
FROM audit_events WHERE action LIKE 'folder.%' ORDER BY seq`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var events []folderEvent
	for rows.Next() {
		var e folderEvent
		if err := rows.Scan(&e.Action, &e.ActorKind, &e.ActorID, &e.ActorName, &e.ActorRole, &e.Initiator, &e.Origin, &e.Client, &e.Tool, &e.RunID, &e.TargetName); err != nil {
			t.Fatal(err)
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return events
}

func (f *accessFixture) audited(t *testing.T) http.Handler {
	t.Helper()
	recorded, err := audit.New(audit.Config{DB: f.db.DB}).Middleware()
	if err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	engine.Any("/mcp", recorded, gin.WrapH(f.handler))
	return engine
}

func (f *accessFixture) overMCP(t *testing.T, by auth.Principal, tool string, args map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(rpc(1, "tools/call", map[string]any{"name": tool, "arguments": args})))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", "2025-11-25")
	req.Header.Set("Authorization", bearer(by))
	rec := httptest.NewRecorder()
	f.audited(t).ServeHTTP(rec, req)
	resp := decodeRPC(t, rec)
	var res toolResult
	if rec.Code != http.StatusOK || resp.Error != nil || json.Unmarshal(resp.Result, &res) != nil || res.IsError {
		t.Fatalf("tools/call %s = %d %s", tool, rec.Code, rec.Body)
	}
}

func TestAFolderChangedOverMCPIsAttributedToThePersonAndTheClient(t *testing.T) {
	f := newAccessFixture(t)
	ada := f.person(t, "Ada", auth.RoleEditor)
	f.overMCP(t, ada, "create_folder", map[string]any{"title": "Payments"})
	f.overMCP(t, ada, "rename_folder", map[string]any{"folder_id": f.folderID(t, "Payments"), "title": "Billing"})

	base := folderEvent{ActorKind: "user", ActorID: ada.ID, ActorName: "Ada", ActorRole: "editor", Initiator: "person", Origin: "mcp", Client: "Claude Code"}
	create, rename := base, base
	create.Action, create.Tool, create.TargetName = "folder.create", "create_folder", "Payments"
	rename.Action, rename.Tool, rename.TargetName = "folder.rename", "rename_folder", "Billing"
	got := f.folderEvents(t)
	if len(got) != 2 || got[0] != create || got[1] != rename {
		t.Fatalf("events = %+v, want %+v and %+v", got, create, rename)
	}
}

func TestAFolderChangedInsideARunIsAttributedToThePersonTheAgentActsFor(t *testing.T) {
	f := newAccessFixture(t)
	ada := f.person(t, "Ada", auth.RoleEditor)
	const run = "44444444-4444-4444-8444-444444444444"
	ctx := auth.WithPrincipal(context.Background(), ada)
	ctx = origin.With(ctx, origin.Origin{Kind: origin.Web})
	ctx = tools.WithRun(ctx, tools.Run{RequestID: run})
	res, err := f.registry.Call(ctx, "create_folder", json.RawMessage(`{"title":"From a run"}`))
	if err != nil {
		t.Fatalf("create_folder in a run: %v", err)
	}
	if res.IsError {
		t.Fatalf("create_folder in a run = %+v", res.Content)
	}

	want := folderEvent{Action: "folder.create", ActorKind: "user", ActorID: ada.ID, ActorName: "Ada", ActorRole: "editor", Initiator: "agent", Origin: "web", Tool: "create_folder", RunID: run, TargetName: "From a run"}
	got := f.folderEvents(t)
	if len(got) != 1 || got[0] != want {
		t.Fatalf("events = %+v, want %+v", got, want)
	}
}

func TestEveryEventOfARunTokenSessionNamesTheAgentThatTookTheRun(t *testing.T) {
	agent := audit.Agent{ID: uuid.NullUUID{UUID: uuid.MustParse("33333333-3333-4333-8333-333333333333"), Valid: true}, Name: "Claude Code"}
	ada := auth.Principal{Kind: auth.KindUser, ID: "00000000-0000-4000-8000-0000000000ad", Name: "Ada", Role: auth.RoleEditor}
	ctx := audit.Open(context.Background(), audit.Scope{Initiator: audit.InitiatorPerson})
	attribute(ctx, Credential{Principal: ada, Run: &revisions.Request{ID: "44444444-4444-4444-8444-444444444444", Assignee: revisions.AssignGrant}, Agent: agent})
	scope, _ := audit.ScopeOf(ctx)
	if scope.Agent != agent || !scope.RunID.Valid {
		t.Fatalf("scope agent = %+v, run = %v, want %+v in the run", scope.Agent, scope.RunID, agent)
	}

	outside := audit.Open(context.Background(), audit.Scope{Initiator: audit.InitiatorPerson})
	attribute(outside, Credential{Principal: ada, OAuth: true, Client: "Claude Code", Claimer: &claims.Claimer{Kind: revisions.AssignGrant}, Agent: agent})
	if scope, _ := audit.ScopeOf(outside); scope.Agent != (audit.Agent{}) {
		t.Fatalf("scope agent outside a run = %+v, want none", scope.Agent)
	}
}
