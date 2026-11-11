package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	mcplib "github.com/mark3labs/mcp-go/mcp"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/origin"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/internal/runsession"
)

const (
	runToken       = "Bearer run-token"
	requesterToken = "Bearer requester"
	runRequestID   = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
)

var requester = auth.Principal{Kind: auth.KindUser, ID: "u-requester", Name: "Rae", Role: auth.RoleViewer}

type runAuth struct{ req revisions.Request }

func (a runAuth) Authenticate(_ context.Context, header string) (Credential, bool, error) {
	switch header {
	case runToken:
		req := a.req
		return Credential{Principal: requester, Run: &req}, true, nil
	case requesterToken:
		return Credential{Principal: requester}, true, nil
	}
	return Credential{}, false, nil
}

type fakeCall struct {
	req  revisions.Request
	call runsession.Call
}

type fakeRuns struct {
	mode     tools.Mode
	scopeErr error
	notes    []string
	meta     runsession.RunMeta

	mu    sync.Mutex
	calls []fakeCall
}

func (r *fakeRuns) Scope(_ context.Context, req revisions.Request) (tools.Run, error) {
	if r.scopeErr != nil {
		return tools.Run{}, r.scopeErr
	}
	return tools.Run{Mode: r.mode, RequestID: req.ID, Attempt: req.Attempt, Build: r.mode != tools.ModeInvestigate}, nil
}

func (r *fakeRuns) Call(ctx context.Context, req revisions.Request, call runsession.Call, dispatch func(context.Context) (tools.Result, error)) (runsession.Answer, error) {
	r.mu.Lock()
	r.calls = append(r.calls, fakeCall{req: req, call: call})
	r.mu.Unlock()
	res, err := dispatch(ctx)
	if err != nil {
		return runsession.Answer{}, err
	}
	body, err := json.Marshal(res.Content)
	if err != nil {
		return runsession.Answer{}, err
	}
	return runsession.Answer{Content: body, IsError: res.IsError, Notes: r.notes, Meta: r.meta}, nil
}

func (r *fakeRuns) recorded() []fakeCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.calls)
}

type runFixture struct {
	handler *handler
	reg     *tools.Registry
	runs    *fakeRuns
	logs    *bytes.Buffer
}

func newRunFixture(t *testing.T, mode tools.Mode) *runFixture {
	t.Helper()
	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewTextHandler(&syncWriter{w: logs}, nil))
	reg := tools.New(tools.Deps{Logger: logger})
	base, err := url.Parse(publicURL)
	if err != nil {
		t.Fatal(err)
	}
	req := revisions.Request{
		ID: runRequestID, Attempt: 2, RequestedBy: requester,
		Origin: origin.Origin{Kind: origin.Web, Client: "browser"},
	}
	runs := &fakeRuns{mode: mode}
	h := NewHandler(reg, runAuth{req: req}, Options{
		PublicURL: base, Version: "test", Logger: logger, Runs: runs,
	}).(*handler)
	return &runFixture{handler: h, reg: reg, runs: runs, logs: logs}
}

func (f *runFixture) post(token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", "2025-11-25")
	req.Header.Set("Authorization", token)
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	return rec
}

type listedTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

func (f *runFixture) listTools(t *testing.T) map[string]listedTool {
	t.Helper()
	rec := f.post(runToken, rpc(1, "tools/list", map[string]any{}))
	if rec.Code != http.StatusOK {
		t.Fatalf("tools/list status %d: %s", rec.Code, rec.Body.String())
	}
	var result struct {
		Tools []listedTool `json:"tools"`
	}
	if err := json.Unmarshal(decodeRPC(t, rec).Result, &result); err != nil {
		t.Fatal(err)
	}
	out := map[string]listedTool{}
	for _, tool := range result.Tools {
		out[tool.Name] = tool
	}
	return out
}

func TestRunSessionListsTheToolsOfTheBuiltInAgentForItsKind(t *testing.T) {
	for _, mode := range []tools.Mode{tools.ModeCreate, tools.ModeRevise, tools.ModeInvestigate} {
		t.Run(string(mode), func(t *testing.T) {
			f := newRunFixture(t, mode)
			listed := f.listTools(t)
			want := map[string]tools.Tool{}
			for _, tool := range f.reg.List() {
				if tool.OfferedIn(mode) {
					want[tool.Name] = tool.ForAgent()
				}
			}
			if len(listed) != len(want) {
				t.Errorf("listed %d tools, want %d", len(listed), len(want))
			}
			for name, tool := range want {
				got, ok := listed[name]
				if !ok {
					t.Errorf("%s is not listed", name)
					continue
				}
				if got.Description != tool.Description || !jsonEqual(got.InputSchema, tool.InputSchema) {
					t.Errorf("%s does not carry the agent wording", name)
				}
			}
			for _, name := range []string{"claim_run", "report_run", "finish_run", "get_run", "list_folders"} {
				if _, ok := listed[name]; ok {
					t.Errorf("%s is listed in a run session", name)
				}
			}
		})
	}
}

func jsonEqual(a, b json.RawMessage) bool {
	var x, y any
	return json.Unmarshal(a, &x) == nil && json.Unmarshal(b, &y) == nil && jsonText(x) == jsonText(y)
}

func jsonText(v any) string {
	out, _ := json.Marshal(v)
	return string(out)
}

func TestRunSessionOfAnInvestigationSavesFindingsNotPanels(t *testing.T) {
	listed := newRunFixture(t, tools.ModeInvestigate).listTools(t)
	if _, ok := listed["save_finding"]; !ok {
		t.Error("save_finding is not listed")
	}
	if _, ok := listed["update_panels"]; ok {
		t.Error("update_panels is listed in an investigate run")
	}
	build := newRunFixture(t, tools.ModeRevise).listTools(t)
	if _, ok := build["update_panels"]; !ok {
		t.Error("update_panels is not listed in a revision")
	}
}

func TestRunSessionServesNoPromptsAndNoInstructions(t *testing.T) {
	f := newRunFixture(t, tools.ModeRevise)
	rec := f.post(runToken, rpc(1, "prompts/list", map[string]any{}))
	resp := decodeRPC(t, rec)
	if resp.Error == nil || resp.Error.Code != -32601 {
		t.Fatalf("prompts/list = %s, want a method-not-found error", rec.Body.String())
	}
	rec = f.post(runToken, rpc(2, "initialize", map[string]any{
		"protocolVersion": "2025-11-25", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "agent", "version": "1"},
	}))
	if strings.Contains(rec.Body.String(), `"instructions"`) {
		t.Errorf("initialize carries instructions: %s", rec.Body.String())
	}
	if rec.Header().Get("Mcp-Session-Id") != "" {
		t.Errorf("a run session labels its client: %q", rec.Header().Get("Mcp-Session-Id"))
	}
}

func TestRunSessionCallActsForTheRequesterInsideTheRunAndGoesThroughTheRunSessions(t *testing.T) {
	const metaKey = "io.modelcontextprotocol/clientInfo"
	f := newRunFixture(t, tools.ModeRevise)
	var gotRun tools.Run
	var gotScoped bool
	var gotBy auth.Principal
	var gotOrigin origin.Origin
	f.handler.call = func(ctx context.Context, _ string, _ json.RawMessage) (tools.Result, error) {
		gotRun, gotScoped = tools.RunFrom(ctx)
		gotBy, _ = auth.FromContext(ctx)
		gotOrigin = origin.From(ctx)
		return tools.Result{Content: map[string]string{"ok": "true"}, Step: "Linted a query"}, nil
	}
	params := map[string]any{
		"name": "lint_query", "arguments": map[string]any{},
		"_meta": map[string]any{metaKey: map[string]any{"name": "evil", "version": "1"}},
	}
	rec := f.post(runToken, rpc(1, "tools/call", params))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if !gotScoped || gotRun.Mode != tools.ModeRevise || gotRun.RequestID != runRequestID || gotRun.Attempt != 2 || !gotRun.Build {
		t.Errorf("run scope = %+v, %v", gotRun, gotScoped)
	}
	if gotBy.ID != requester.ID || gotBy.Kind != requester.Kind {
		t.Errorf("principal = %+v, want the requester", gotBy)
	}
	if gotOrigin.Kind != origin.MCP || gotOrigin.Client != "browser" {
		t.Errorf("origin = %+v, want the run's client and not the one the session reports", gotOrigin)
	}
	calls := f.runs.recorded()
	if len(calls) != 1 || calls[0].req.ID != runRequestID || calls[0].req.Attempt != 2 || calls[0].call.Name != "lint_query" || calls[0].call.Turn != nil {
		t.Fatalf("calls = %+v", calls)
	}
	if !strings.Contains(f.logs.String(), "agent_request_id="+runRequestID) {
		t.Errorf("the call log lacks the run id: %s", f.logs.String())
	}
}

func callResult(t *testing.T, rec *httptest.ResponseRecorder) mcplib.CallToolResult {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var result mcplib.CallToolResult
	if err := json.Unmarshal(decodeRPC(t, rec).Result, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestRunSessionPassesTheTurnKeyAndIgnoresMalformedOnes(t *testing.T) {
	cases := map[string]struct {
		meta any
		want *runsession.Turn
	}{
		"a turn":              {map[string]any{"id": "t1", "calls": 3, "saves": 1}, &runsession.Turn{ID: "t1", Calls: 3, Saves: 1}},
		"not an object":       {"t1", nil},
		"a number for the id": {map[string]any{"id": 4, "calls": 1}, nil},
		"calls as text":       {map[string]any{"id": "t1", "calls": "3"}, nil},
		"no key":              {nil, nil},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newRunFixture(t, tools.ModeRevise)
			f.handler.call = func(context.Context, string, json.RawMessage) (tools.Result, error) {
				return tools.Result{Content: map[string]string{"ok": "true"}}, nil
			}
			params := map[string]any{"name": "lint_query", "arguments": map[string]any{}}
			if tc.meta != nil {
				params["_meta"] = map[string]any{turnMetaKey: tc.meta}
			}
			f.post(runToken, rpc(1, "tools/call", params))
			calls := f.runs.recorded()
			if len(calls) != 1 {
				t.Fatalf("calls = %+v", calls)
			}
			got := calls[0].call.Turn
			if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
				t.Fatalf("turn = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestRunSessionResultCarriesTheRunKeyAndTheNotesButNotInItsStructuredContent(t *testing.T) {
	f := newRunFixture(t, tools.ModeRevise)
	settled := "saved"
	f.runs.notes = []string{"read less"}
	f.runs.meta = runsession.RunMeta{Settled: &settled, Build: &tools.BuildProgress{Saves: 2, Panels: 3, PlannedPanels: 5}}
	f.handler.call = func(context.Context, string, json.RawMessage) (tools.Result, error) {
		return tools.Result{Content: map[string]string{"ok": "true"}}, nil
	}
	result := callResult(t, f.post(runToken, rpc(1, "tools/call", map[string]any{"name": "lint_query", "arguments": map[string]any{}})))
	text, _ := result.Content[0].(mcplib.TextContent)
	if text.Text != "{\"ok\":\"true\"}\n\n<note>read less</note>" {
		t.Errorf("text = %q", text.Text)
	}
	if got, _ := json.Marshal(result.StructuredContent); string(got) != `{"ok":"true"}` {
		t.Errorf("structured content = %s", got)
	}
	got, _ := json.Marshal(result.Meta.AdditionalFields[runMetaKey])
	if string(got) != `{"build":{"panels":3,"planned_panels":5,"saves":2},"settled":"saved"}` {
		t.Errorf("run meta = %s", got)
	}
}

func TestRunSessionResultWithNothingSettledCarriesNullsAndNonRunResultsCarryNoKey(t *testing.T) {
	f := newRunFixture(t, tools.ModeRevise)
	f.handler.call = func(context.Context, string, json.RawMessage) (tools.Result, error) {
		return tools.Result{Content: map[string]string{"ok": "true"}}, nil
	}
	params := map[string]any{"name": "lint_query", "arguments": map[string]any{}}
	inRun := callResult(t, f.post(runToken, rpc(1, "tools/call", params)))
	got, _ := json.Marshal(inRun.Meta.AdditionalFields[runMetaKey])
	if string(got) != `{"build":null,"settled":null}` {
		t.Errorf("run meta = %s", got)
	}
	outside := callResult(t, f.post(requesterToken, rpc(2, "tools/call", params)))
	if outside.Meta != nil {
		if _, ok := outside.Meta.AdditionalFields[runMetaKey]; ok {
			t.Errorf("a call outside a run carries the run key: %+v", outside.Meta)
		}
	}
}

func TestRunSessionAnswersAnInternalErrorWithoutItsText(t *testing.T) {
	f := newRunFixture(t, tools.ModeRevise)
	f.handler.call = func(context.Context, string, json.RawMessage) (tools.Result, error) {
		return tools.Result{}, errors.New("secret-detail")
	}
	result := callResult(t, f.post(runToken, rpc(1, "tools/call", map[string]any{"name": "update_panels", "arguments": map[string]any{}})))
	text, _ := result.Content[0].(mcplib.TextContent)
	if !result.IsError || text.Text != internalErrorMsg {
		t.Fatalf("result = %+v", result)
	}
	if strings.Contains(f.logs.String(), "secret-detail") {
		t.Error("the logs carry the error text")
	}
}

func TestRunSessionHasItsOwnInFlightSlot(t *testing.T) {
	f := newRunFixture(t, tools.ModeRevise)
	entered := make(chan struct{}, maxInFlight+1)
	release := make(chan struct{})
	f.handler.call = func(context.Context, string, json.RawMessage) (tools.Result, error) {
		entered <- struct{}{}
		<-release
		return tools.Result{Content: map[string]string{"ok": "true"}}, nil
	}
	statuses := make(chan int, maxInFlight+1)
	var running sync.WaitGroup
	send := func(token string) {
		running.Add(1)
		go func() {
			defer running.Done()
			statuses <- f.post(token, rpc(1, "tools/call", map[string]any{"name": "lint_query", "arguments": map[string]any{}})).Code
		}()
	}
	for range maxInFlight {
		send(requesterToken)
	}
	send(runToken)
	for range maxInFlight + 1 {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("a call never reached the tool: the run session shares the requester's slots")
		}
	}
	close(release)
	running.Wait()
	close(statuses)
	for status := range statuses {
		if status != http.StatusOK {
			t.Errorf("status %d", status)
		}
	}
}

func TestRunSessionRefusesARunWhoseScopeCannotBeBuilt(t *testing.T) {
	f := newRunFixture(t, tools.ModeRevise)
	f.runs.scopeErr = errors.New("secret-detail")
	rec := f.post(runToken, rpc(1, "tools/list", map[string]any{}))
	assertErrorBody(t, rec, http.StatusInternalServerError, "internal")
	if strings.Contains(rec.Body.String(), "secret-detail") || strings.Contains(f.logs.String(), "secret-detail") {
		t.Error("the scope error text leaked")
	}
	if !strings.Contains(f.logs.String(), "run_scope_failed") || !strings.Contains(f.logs.String(), runRequestID) {
		t.Errorf("the failure is not logged by code and run id: %s", f.logs.String())
	}
}

func TestRunToolWithoutARunScopeIsRefused(t *testing.T) {
	f := newRunFixture(t, tools.ModeRevise)
	called := false
	f.handler.call = func(context.Context, string, json.RawMessage) (tools.Result, error) {
		called = true
		return tools.Result{}, nil
	}
	ctx := auth.WithPrincipal(context.Background(), requester)
	res, _ := f.handler.callRunTool("lint_query")(ctx, mcplib.CallToolRequest{})
	if called || res == nil || !res.IsError {
		t.Fatalf("a run tool ran without a run scope: called %v, result %+v", called, res)
	}
}

func TestIsClaimPoll(t *testing.T) {
	call := func(name string) string { return rpc(1, "tools/call", map[string]any{"name": name}) }
	cases := map[string]struct {
		body string
		want bool
	}{
		"a claim_run call":        {call("claim_run"), true},
		"another tool":            {call("get_spec_guide"), false},
		"initialize":              {rpc(1, "initialize", map[string]any{"clientInfo": map[string]any{"name": "claim_run"}}), false},
		"a method named after it": {rpc(1, "claim_run", map[string]any{"name": "claim_run"}), false},
		"a batch":                 {"[" + call("claim_run") + "]", false},
		"garbage":                 {"claim_run {", false},
		"empty":                   {"", false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := isClaimPoll([]byte(tc.body)); got != tc.want {
				t.Fatalf("isClaimPoll = %v, want %v", got, tc.want)
			}
		})
	}
}
