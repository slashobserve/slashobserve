package agent_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/agent"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/llm"
	"github.com/slashobserve/slashobserve/internal/llm/replay"
	"github.com/slashobserve/slashobserve/pkg/spec"
	"github.com/slashobserve/slashobserve/spec/examples"
)

const (
	dashID    = "0190b5c0-0000-7000-8000-000000000001"
	panelID   = "p_p99"
	panelName = "p99 latency"
)

var principal = auth.Principal{Kind: auth.KindUser, ID: "user-1", Name: "Ada"}

type scriptedTurn struct {
	turn llm.Turn
	err  error
}

type scriptedSession struct {
	mu       sync.Mutex
	script   []scriptedTurn
	next     int
	received [][]llm.ToolResult
}

func (s *scriptedSession) Next(ctx context.Context, results []llm.ToolResult) (llm.Turn, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return llm.Turn{}, err
	}
	s.received = append(s.received, results)
	if s.next >= len(s.script) {
		return llm.Turn{}, errors.New("test script exhausted: the agent made an unexpected model call")
	}
	t := s.script[s.next]
	s.next++
	return t.turn, t.err
}

func (s *scriptedSession) modelCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.received)
}

func (s *scriptedSession) results(call int) []llm.ToolResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.received[call]
}

type scriptedProvider struct {
	mu       sync.Mutex
	sessions []*scriptedSession
	requests []llm.Request
	startErr error
}

func (p *scriptedProvider) Name() string  { return "fake" }
func (p *scriptedProvider) Model() string { return "fake-model" }

func (p *scriptedProvider) Start(_ context.Context, req llm.Request) (llm.Session, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.requests = append(p.requests, req)
	if p.startErr != nil {
		return nil, p.startErr
	}
	if len(p.sessions) < len(p.requests) {
		return nil, errors.New("test script exhausted: the agent started an unexpected session")
	}
	return p.sessions[len(p.requests)-1], nil
}

func (p *scriptedProvider) started() []llm.Request {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]llm.Request(nil), p.requests...)
}

type recordedCall struct {
	name      string
	input     string
	run       tools.Run
	scoped    bool
	principal auth.Principal
}

type handler func(ctx context.Context, input json.RawMessage) (tools.Result, error)

type fakeTools struct {
	mu       sync.Mutex
	handlers map[string]handler
	calls    []recordedCall
}

var fakeToolList = []tools.Tool{
	{Name: "create_dashboard", Description: "create", InputSchema: json.RawMessage(`{"type":"object"}`)},
	{Name: "get_spec_guide", Description: "guide", InputSchema: json.RawMessage(`{"type":"object"}`), ReadOnly: true},
	{Name: "lint_query", Description: "lint", InputSchema: json.RawMessage(`{"type":"object"}`), ReadOnly: true},
	{Name: "list_datasources", Description: "datasources", InputSchema: json.RawMessage(`{"type":"object"}`), ReadOnly: true},
	{Name: "query_range", Description: "range", InputSchema: json.RawMessage(`{"type":"object"}`), ReadOnly: true},
	{Name: "update_dashboard", Description: "update", InputSchema: json.RawMessage(`{"type":"object"}`)},
}

func (f *fakeTools) List() []tools.Tool { return fakeToolList }

func (f *fakeTools) Call(ctx context.Context, name string, input json.RawMessage) (tools.Result, error) {
	run, scoped := tools.RunFrom(ctx)
	p, _ := auth.FromContext(ctx)
	f.mu.Lock()
	f.calls = append(f.calls, recordedCall{name: name, input: string(input), run: run, scoped: scoped, principal: p})
	h := f.handlers[name]
	f.mu.Unlock()
	if h != nil {
		return h(ctx, input)
	}
	return tools.Result{Content: map[string]any{"ok": true}, Step: "Ran " + name}, nil
}

func (f *fakeTools) on(name string, h handler) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.handlers == nil {
		f.handlers = map[string]handler{}
	}
	f.handlers[name] = h
}

func (f *fakeTools) onSave(name string, results ...tools.Result) {
	var mu sync.Mutex
	f.on(name, func(context.Context, json.RawMessage) (tools.Result, error) {
		mu.Lock()
		defer mu.Unlock()
		if len(results) == 0 {
			return tools.Result{}, errors.New("test script exhausted: unexpected save")
		}
		r := results[0]
		results = results[1:]
		return r, nil
	})
}

func (f *fakeTools) recorded() []recordedCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]recordedCall(nil), f.calls...)
}

func (f *fakeTools) names() []string {
	var out []string
	for _, c := range f.recorded() {
		out = append(out, c.name)
	}
	return out
}

type fakeDashboards struct {
	mu     sync.Mutex
	number int
	err    error
	gets   int
	spec   *spec.Dashboard
}

func (d *fakeDashboards) Get(_ context.Context, id pgtype.UUID) (dashboards.Dashboard, dashboards.Version, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.gets++
	if d.err != nil {
		return dashboards.Dashboard{}, dashboards.Version{}, d.err
	}
	return dashboards.Dashboard{ID: dashID, LatestVersion: d.number},
		dashboards.Version{VersionMeta: dashboards.VersionMeta{Number: d.number}, Spec: d.spec}, nil
}

func (d *fakeDashboards) setNumber(n int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.number = n
}

func (d *fakeDashboards) getCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.gets
}

type fakeHealth struct{}

func (fakeHealth) Health(context.Context) []datasource.Status {
	return []datasource.Status{
		{UID: "logs", Type: "clickhouse", Healthy: false},
		{UID: "prom", Type: "prometheus", Healthy: true},
	}
}

func usage() llm.Usage {
	return llm.Usage{InputTokens: 100, OutputTokens: 10, CacheReadTokens: 5, CacheWriteTokens: 2, Calls: 1}
}

func toolTurn(calls ...llm.ToolCall) scriptedTurn {
	return scriptedTurn{turn: llm.Turn{ToolCalls: calls, Stop: llm.StopToolUse, Usage: usage()}}
}

func stopTurn(stop llm.StopReason) scriptedTurn {
	return scriptedTurn{turn: llm.Turn{Text: "done", Stop: stop, Usage: usage()}}
}

func call(id, name, input string) llm.ToolCall {
	return llm.ToolCall{ID: id, Name: name, Input: json.RawMessage(input)}
}

func savedResult(id string, version int) tools.Result {
	return tools.Result{
		Content: map[string]any{"dashboard_id": id, "version": version, "url": "/dashboards/" + id, "panels": []any{}},
		Step:    fmt.Sprintf("Saved version %d", version),
	}
}

func invalidSpecResult(codes ...string) tools.Result {
	issues := make([]map[string]string, len(codes))
	for i, code := range codes {
		issues[i] = map[string]string{"path": "/panels/0", "code": code, "message": "SECRET datasource text"}
	}
	return tools.Result{
		IsError: true,
		Content: map[string]any{"code": "invalid_spec", "issues": issues},
		Step:    fmt.Sprintf("Save rejected: %d issues", len(codes)),
	}
}

func conflictResult(latest int) tools.Result {
	return tools.Result{
		IsError: true,
		Content: map[string]any{"code": "version_conflict", "latest_version": latest},
		Step:    "Save rejected: version conflict",
	}
}

type rig struct {
	t         *testing.T
	provider  *scriptedProvider
	tools     *fakeTools
	dash      *fakeDashboards
	agent     *agent.Agent
	logs      *bytes.Buffer
	steps     []agent.Step
	stepsLock sync.Mutex
}

func newRig(t *testing.T, sessions ...*scriptedSession) *rig {
	t.Helper()
	parsed, err := spec.Parse(examples.CheckoutServiceHealth)
	if err != nil {
		t.Fatal(err)
	}
	r := &rig{
		t:        t,
		provider: &scriptedProvider{sessions: sessions},
		tools:    &fakeTools{},
		dash:     &fakeDashboards{number: 3, spec: parsed},
		logs:     &bytes.Buffer{},
	}
	r.agent = agent.New(agent.Config{
		Provider:    r.provider,
		Tools:       r.tools,
		Dashboards:  r.dash,
		Datasources: fakeHealth{},
		Logger:      slog.New(slog.NewJSONHandler(r.logs, nil)),
	})
	return r
}

func session(turns ...scriptedTurn) *scriptedSession { return &scriptedSession{script: turns} }

func (r *rig) task() agent.Task {
	return agent.Task{
		Prompt:    "checkout health by pod",
		Principal: principal,
		Progress: func(s agent.Step) {
			r.stepsLock.Lock()
			defer r.stepsLock.Unlock()
			r.steps = append(r.steps, s)
		},
	}
}

func (r *rig) reviseTask() agent.Task {
	task := r.task()
	task.DashboardID = dashID
	task.RequestID = "req-1"
	return task
}

func (r *rig) recordedSteps() []agent.Step {
	r.stepsLock.Lock()
	defer r.stepsLock.Unlock()
	return append([]agent.Step(nil), r.steps...)
}

func (r *rig) finishLog() map[string]any {
	r.t.Helper()
	for line := range strings.SplitSeq(r.logs.String(), "\n") {
		var entry map[string]any
		if json.Unmarshal([]byte(line), &entry) == nil && entry["msg"] == "agent run finished" {
			return entry
		}
	}
	r.t.Fatalf("no finish line in logs: %s", r.logs.String())
	return nil
}

func phases(steps []agent.Step) []string {
	var out []string
	for _, s := range steps {
		out = append(out, s.Phase)
	}
	return out
}

func TestCreateHappyPath(t *testing.T) {
	s := session(
		toolTurn(call("c1", "list_datasources", `{}`), call("c2", "get_spec_guide", `{}`)),
		toolTurn(call("c3", "lint_query", `{"n":1}`), call("c4", "lint_query", `{"n":2}`)),
		toolTurn(call("c5", "create_dashboard", `{"spec":{}}`)),
	)
	r := newRig(t, s)
	r.tools.onSave("create_dashboard", savedResult(dashID, 1))

	out, err := r.agent.Run(context.Background(), r.task())
	if err != nil {
		t.Fatal(err)
	}
	if out.DashboardID != dashID || out.Version != 1 || out.Replayed || out.RepairRounds != 0 {
		t.Fatalf("outcome = %+v", out)
	}
	if out.Provider != "fake" || out.Model != "fake-model" {
		t.Fatalf("provider/model = %q/%q", out.Provider, out.Model)
	}
	want := llm.Usage{InputTokens: 300, OutputTokens: 30, CacheReadTokens: 15, CacheWriteTokens: 6, Calls: 3}
	if out.Usage != want {
		t.Fatalf("usage = %+v, want %+v", out.Usage, want)
	}
	if got := s.modelCalls(); got != 3 {
		t.Fatalf("model calls = %d, want 3: a successful save must end the run", got)
	}
	if got := strings.Join(phases(r.recordedSteps()), ","); got != "discover,discover,lint,lint,save" {
		t.Fatalf("phases = %s", got)
	}
	for _, step := range r.recordedSteps() {
		if step.Status != "ok" || step.At.IsZero() || step.DurationMS < 0 || step.Message != "Ran "+step.Tool && step.Tool != "create_dashboard" {
			t.Fatalf("unexpected step %+v", step)
		}
	}
	last := r.recordedSteps()[4]
	if last.Tool != "create_dashboard" || last.Message != "Saved version 1" {
		t.Fatalf("save step = %+v", last)
	}

	second := s.results(1)
	if len(second) != 2 || second[0].CallID != "c1" || second[1].CallID != "c2" || second[0].IsError || second[0].Content != `{"ok":true}` {
		t.Fatalf("results sent back = %+v", second)
	}

	for _, c := range r.tools.recorded() {
		if !c.scoped || c.run.Mode != tools.ModeCreate || c.run.DashboardID != "" || c.run.ParentVersion != 0 || c.run.RequestID != "" {
			t.Fatalf("call %s run scope = %+v scoped=%v", c.name, c.run, c.scoped)
		}
		if c.principal != principal {
			t.Fatalf("call %s principal = %+v", c.name, c.principal)
		}
	}
	if r.dash.getCount() != 0 {
		t.Fatal("create mode must not load a dashboard")
	}

	fin := r.finishLog()
	if fin["mode"] != "create" || fin["dashboard_id"] != dashID || fin["version"] != float64(1) || fin["model_calls"] != float64(3) ||
		fin["repair_rounds"] != float64(0) || fin["provider"] != "fake" || fin["model"] != "fake-model" || fin["input_tokens"] != float64(300) {
		t.Fatalf("finish line = %v", fin)
	}
	if _, ok := fin["error"]; ok {
		t.Fatal("successful run logged an error")
	}
}

func TestRequestCarriesStableSystemToolsPromptAndContext(t *testing.T) {
	r := newRig(t,
		session(toolTurn(call("c1", "create_dashboard", `{}`))),
		session(toolTurn(call("c1", "create_dashboard", `{}`))),
	)
	r.tools.onSave("create_dashboard", savedResult(dashID, 1), savedResult(dashID, 1))
	task := r.task()
	task.Prompt = "  checkout health by pod \n"
	for range 2 {
		if _, err := r.agent.Run(context.Background(), task); err != nil {
			t.Fatal(err)
		}
	}
	reqs := r.provider.started()
	if len(reqs) != 2 {
		t.Fatalf("sessions started = %d", len(reqs))
	}
	if reqs[0].System != reqs[1].System {
		t.Fatal("system text differs between runs")
	}
	for _, want := range []string{
		"You are SlashObserve's dashboard agent.",
		"\n\n# Authoring guide\n",
		"\n\n# ObserveSpec v0 JSON Schema\n```json\n",
		"\n```\n\n# Example Spec (format only)\n```json\n",
	} {
		if !strings.Contains(reqs[0].System, want) {
			t.Fatalf("system text lacks %q", want)
		}
	}
	if reqs[0].Prompt != "checkout health by pod" {
		t.Fatalf("prompt = %q", reqs[0].Prompt)
	}
	wantContext := "Configured datasources:\n- logs (clickhouse): unhealthy\n- prom (prometheus): healthy\n\nCreate a new dashboard for the request."
	if reqs[0].Context != wantContext {
		t.Fatalf("context = %q", reqs[0].Context)
	}
	if len(reqs[0].Tools) != len(fakeToolList) {
		t.Fatalf("tools = %d", len(reqs[0].Tools))
	}
	for i, spec := range reqs[0].Tools {
		if spec.Name != fakeToolList[i].Name || spec.Description != fakeToolList[i].Description || string(spec.InputSchema) != string(fakeToolList[i].InputSchema) {
			t.Fatalf("tool %d = %+v", i, spec)
		}
	}
}

func TestRepairAfterRejectedSave(t *testing.T) {
	s := session(
		toolTurn(call("c1", "create_dashboard", `{"n":1}`)),
		toolTurn(call("c2", "create_dashboard", `{"n":2}`)),
		toolTurn(call("c3", "create_dashboard", `{"n":3}`)),
	)
	r := newRig(t, s)
	r.tools.onSave("create_dashboard", invalidSpecResult("unknown_field"), invalidSpecResult("bad_query", "bad_query"), savedResult(dashID, 1))

	out, err := r.agent.Run(context.Background(), r.task())
	if err != nil {
		t.Fatal(err)
	}
	if out.RepairRounds != 2 || out.Version != 1 || out.Usage.Calls != 3 {
		t.Fatalf("outcome = %+v", out)
	}
	steps := r.recordedSteps()
	if got := strings.Join(phases(steps), ","); got != "save,repair,repair" {
		t.Fatalf("phases = %s", got)
	}
	if steps[0].Status != "error" || steps[0].Message != "Save rejected: 1 issues" || steps[2].Status != "ok" {
		t.Fatalf("steps = %+v", steps)
	}
	rejected := s.results(1)
	if len(rejected) != 1 || !rejected[0].IsError || !strings.Contains(rejected[0].Content, `"invalid_spec"`) {
		t.Fatalf("model did not see the rejection: %+v", rejected)
	}
	if fin := r.finishLog(); fin["repair_rounds"] != float64(2) {
		t.Fatalf("finish line = %v", fin)
	}
}

func TestRepairBudgetIsThreeRounds(t *testing.T) {
	s := session(
		toolTurn(call("c1", "create_dashboard", `{}`)),
		toolTurn(call("c2", "create_dashboard", `{}`)),
		toolTurn(call("c3", "create_dashboard", `{}`)),
		toolTurn(call("c4", "create_dashboard", `{}`)),
		toolTurn(call("c5", "create_dashboard", `{}`)),
	)
	r := newRig(t, s)
	r.tools.onSave("create_dashboard", invalidSpecResult("a_code"), invalidSpecResult("a_code"), invalidSpecResult("b_code", "c_code"), invalidSpecResult("d_code", "d_code", "e_code", "f_code", "g_code"), savedResult(dashID, 1))

	out, err := r.agent.Run(context.Background(), r.task())
	if !errors.Is(err, agent.ErrRepairExhausted) {
		t.Fatalf("err = %v", err)
	}
	if got := s.modelCalls(); got != 4 {
		t.Fatalf("model calls = %d, want 4 (first save plus three repairs)", got)
	}
	if out.RepairRounds != 3 || out.Version != 0 || out.Usage.Calls != 4 || out.Provider != "fake" {
		t.Fatalf("outcome = %+v", out)
	}
	if !strings.Contains(err.Error(), "d_code, e_code, f_code") || strings.Contains(err.Error(), "g_code") || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("error should list at most three issue codes and no messages: %q", err.Error())
	}
	if got := strings.Join(phases(r.recordedSteps()), ","); got != "save,repair,repair,repair" {
		t.Fatalf("phases = %s", got)
	}
	fin := r.finishLog()
	if fin["error"] == nil || fin["repair_rounds"] != float64(3) {
		t.Fatalf("finish line = %v", fin)
	}
}

func TestOneSaveRunsPerTurnAndResultsKeepCallOrder(t *testing.T) {
	s := session(
		toolTurn(call("a", "create_dashboard", `{"n":1}`), call("b", "list_datasources", `{}`), call("c", "create_dashboard", `{"n":2}`)),
		toolTurn(call("d", "create_dashboard", `{"n":3}`)),
	)
	r := newRig(t, s)
	r.tools.onSave("create_dashboard", invalidSpecResult("unknown_field"), savedResult(dashID, 1))

	out, err := r.agent.Run(context.Background(), r.task())
	if err != nil {
		t.Fatal(err)
	}
	if out.Version != 1 || out.RepairRounds != 1 {
		t.Fatalf("outcome = %+v", out)
	}
	if got := strings.Join(r.tools.names(), ","); got != "list_datasources,create_dashboard,create_dashboard" {
		t.Fatalf("tool execution order = %s: reads run first and only one save runs per turn", got)
	}
	got := s.results(1)
	if len(got) != 3 || got[0].CallID != "a" || got[1].CallID != "b" || got[2].CallID != "c" {
		t.Fatalf("results = %+v", got)
	}
	if !got[0].IsError || got[1].IsError || !got[2].IsError || !strings.Contains(got[2].Content, "only one save per turn") {
		t.Fatalf("results = %+v", got)
	}
	steps := r.recordedSteps()
	if len(steps) != 4 || steps[2].Phase != "save" || steps[2].Status != "error" || steps[3].Phase != "repair" {
		t.Fatalf("steps = %+v", steps)
	}
}

func TestSuccessfulSaveSkipsLaterSavesInTheTurn(t *testing.T) {
	s := session(toolTurn(call("a", "create_dashboard", `{}`), call("b", "create_dashboard", `{}`)))
	r := newRig(t, s)
	r.tools.onSave("create_dashboard", savedResult(dashID, 1))

	if _, err := r.agent.Run(context.Background(), r.task()); err != nil {
		t.Fatal(err)
	}
	if got := r.tools.names(); len(got) != 1 {
		t.Fatalf("tool calls = %v, want exactly one save", got)
	}
	if got := s.modelCalls(); got != 1 {
		t.Fatalf("model calls = %d", got)
	}
}

func TestReadOnlyCallsRunConcurrentlyUpToFour(t *testing.T) {
	var inFlight, peak atomic.Int64
	r := newRig(t, session(
		toolTurn(
			call("1", "query_range", `{}`), call("2", "query_range", `{}`), call("3", "query_range", `{}`),
			call("4", "query_range", `{}`), call("5", "query_range", `{}`), call("6", "query_range", `{}`),
		),
		toolTurn(call("7", "create_dashboard", `{}`)),
	))
	r.tools.on("query_range", func(context.Context, json.RawMessage) (tools.Result, error) {
		n := inFlight.Add(1)
		defer inFlight.Add(-1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(40 * time.Millisecond)
		return tools.Result{Content: map[string]any{"ok": true}, Step: "queried"}, nil
	})
	r.tools.onSave("create_dashboard", savedResult(dashID, 1))

	if _, err := r.agent.Run(context.Background(), r.task()); err != nil {
		t.Fatal(err)
	}
	if got := peak.Load(); got < 2 || got > 4 {
		t.Fatalf("peak concurrency = %d, want between 2 and 4", got)
	}
	steps := r.recordedSteps()
	if len(steps) != 7 || steps[0].Phase != "query" {
		t.Fatalf("steps = %+v", steps)
	}
}

func TestReviseHappyPath(t *testing.T) {
	s := session(
		toolTurn(call("c1", "get_spec_guide", `{}`)),
		toolTurn(call("c2", "update_dashboard", `{"spec":{}}`)),
	)
	r := newRig(t, s)
	r.tools.onSave("update_dashboard", func() tools.Result {
		res := savedResult(dashID, 4)
		res.Content.(map[string]any)["replayed"] = true
		return res
	}())
	task := r.reviseTask()
	task.PanelID = panelID

	out, err := r.agent.Run(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}
	if out.DashboardID != dashID || out.Version != 4 || !out.Replayed {
		t.Fatalf("outcome = %+v", out)
	}
	for _, c := range r.tools.recorded() {
		want := tools.Run{Mode: tools.ModeRevise, DashboardID: dashID, ParentVersion: 3, RequestID: "req-1"}
		if !c.scoped || c.run != want {
			t.Fatalf("call %s run = %+v scoped=%v, want %+v", c.name, c.run, c.scoped, want)
		}
	}
	reqs := r.provider.started()
	for _, want := range []string{
		"- prom (prometheus): healthy",
		"You are revising dashboard " + dashID + ", currently version 3.\n```json\n{",
		"Checkout service health",
		fmt.Sprintf("The request is a comment on panel %s titled %q.", panelID, panelName),
	} {
		if !strings.Contains(reqs[0].Context, want) {
			t.Fatalf("context lacks %q:\n%s", want, reqs[0].Context)
		}
	}
	if strings.Contains(reqs[0].Context, "Create a new dashboard") {
		t.Fatal("revise context offers to create a dashboard")
	}
	if fin := r.finishLog(); fin["mode"] != "revise" || fin["version"] != float64(4) {
		t.Fatalf("finish line = %v", fin)
	}
}

func TestReviseCommentOnPanelMissingFromLatestVersion(t *testing.T) {
	r := newRig(t, session(toolTurn(call("c1", "update_dashboard", `{}`))))
	r.tools.onSave("update_dashboard", savedResult(dashID, 4))
	task := r.reviseTask()
	task.PanelID = "p_gone"

	if _, err := r.agent.Run(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if got := r.provider.started()[0].Context; !strings.Contains(got, "Panel p_gone from the comment is not in the latest version.") {
		t.Fatalf("context = %s", got)
	}
}

func TestReviseConflictRestartsOnceFromTheLatestVersion(t *testing.T) {
	first := session(toolTurn(call("c1", "update_dashboard", `{}`)), toolTurn(call("never", "update_dashboard", `{}`)))
	second := session(
		toolTurn(call("c2", "get_spec_guide", `{}`)),
		toolTurn(call("c3", "update_dashboard", `{}`)),
	)
	r := newRig(t, first, second)
	r.tools.on("update_dashboard", func() handler {
		var n atomic.Int64
		return func(context.Context, json.RawMessage) (tools.Result, error) {
			if n.Add(1) == 1 {
				r.dash.setNumber(4)
				return conflictResult(4), nil
			}
			return savedResult(dashID, 5), nil
		}
	}())

	out, err := r.agent.Run(context.Background(), r.reviseTask())
	if err != nil {
		t.Fatal(err)
	}
	if out.Version != 5 || out.RepairRounds != 0 {
		t.Fatalf("outcome = %+v", out)
	}
	if first.modelCalls() != 1 {
		t.Fatalf("the conflicted session continued: %d model calls", first.modelCalls())
	}
	if out.Usage.Calls != 3 || out.Usage.InputTokens != 300 {
		t.Fatalf("usage should accumulate across sessions: %+v", out.Usage)
	}
	reqs := r.provider.started()
	if len(reqs) != 2 || !strings.Contains(reqs[0].Context, "currently version 3.") || !strings.Contains(reqs[1].Context, "currently version 4.") {
		t.Fatalf("contexts = %q", []string{reqs[0].Context, reqs[1].Context})
	}
	var parents []int
	for _, c := range r.tools.recorded() {
		parents = append(parents, c.run.ParentVersion)
	}
	if fmt.Sprint(parents) != "[3 4 4]" {
		t.Fatalf("parent versions = %v", parents)
	}
	if r.dash.getCount() != 2 {
		t.Fatalf("dashboard loads = %d", r.dash.getCount())
	}
}

func TestReviseSecondConflictFails(t *testing.T) {
	r := newRig(t,
		session(toolTurn(call("c1", "update_dashboard", `{}`))),
		session(toolTurn(call("c2", "update_dashboard", `{}`))),
		session(toolTurn(call("c3", "update_dashboard", `{}`))),
	)
	r.tools.onSave("update_dashboard", conflictResult(4), conflictResult(5))

	out, err := r.agent.Run(context.Background(), r.reviseTask())
	if !errors.Is(err, agent.ErrConflict) {
		t.Fatalf("err = %v", err)
	}
	if got := len(r.provider.started()); got != 2 {
		t.Fatalf("sessions = %d, want exactly one restart", got)
	}
	if out.Usage.Calls != 2 || out.Provider != "fake" || out.DashboardID != dashID {
		t.Fatalf("outcome = %+v", out)
	}
}

func TestCreateConflictIsAnOrdinaryRejection(t *testing.T) {
	r := newRig(t, session(
		toolTurn(call("c1", "create_dashboard", `{}`)),
		toolTurn(call("c2", "create_dashboard", `{}`)),
	))
	r.tools.onSave("create_dashboard", conflictResult(2), savedResult(dashID, 1))

	out, err := r.agent.Run(context.Background(), r.task())
	if err != nil || out.RepairRounds != 1 || len(r.provider.started()) != 1 {
		t.Fatalf("out = %+v err = %v sessions = %d", out, err, len(r.provider.started()))
	}
}

func TestReviseSaveForMissingDashboardFails(t *testing.T) {
	r := newRig(t, session(toolTurn(call("c1", "update_dashboard", `{}`))))
	r.tools.onSave("update_dashboard", tools.Result{IsError: true, Content: map[string]any{"code": "not_found"}, Step: "Save rejected: dashboard not found"})

	_, err := r.agent.Run(context.Background(), r.reviseTask())
	if !errors.Is(err, dashboards.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestRefusalFailsTheRunAndKeepsUsage(t *testing.T) {
	r := newRig(t, session(stopTurn(llm.StopRefusal)))
	out, err := r.agent.Run(context.Background(), r.task())
	if !errors.Is(err, agent.ErrRefused) {
		t.Fatalf("err = %v", err)
	}
	if out.Usage.Calls != 1 || out.Usage.InputTokens != 100 || out.Provider != "fake" || out.Model != "fake-model" {
		t.Fatalf("outcome = %+v", out)
	}
	if fin := r.finishLog(); !strings.Contains(fmt.Sprint(fin["error"]), "declined") {
		t.Fatalf("finish line = %v", fin)
	}
}

func TestFinishingWithoutSavingFails(t *testing.T) {
	cases := []struct {
		name string
		stop llm.StopReason
		want string
	}{
		{"end turn", llm.StopEndTurn, ""},
		{"max tokens", llm.StopMaxTokens, "after reaching max tokens"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t, session(toolTurn(call("c1", "get_spec_guide", `{}`)), stopTurn(tc.stop)))
			out, err := r.agent.Run(context.Background(), r.task())
			if !errors.Is(err, agent.ErrNoSave) {
				t.Fatalf("err = %v", err)
			}
			if tc.want != "" && !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %q, want it to mention %q", err, tc.want)
			}
			if tc.want == "" && err.Error() != agent.ErrNoSave.Error() {
				t.Fatalf("err = %q", err)
			}
			if out.Usage.Calls != 2 {
				t.Fatalf("usage = %+v", out.Usage)
			}
		})
	}
}

func TestTurnLimit(t *testing.T) {
	turns := make([]scriptedTurn, 25)
	for i := range turns {
		turns[i] = toolTurn(call(fmt.Sprint("c", i), "get_spec_guide", `{}`))
	}
	s := session(turns...)
	r := newRig(t, s)

	out, err := r.agent.Run(context.Background(), r.task())
	if !errors.Is(err, agent.ErrTurnLimit) {
		t.Fatalf("err = %v", err)
	}
	if s.modelCalls() != 20 || out.Usage.Calls != 20 || len(r.tools.names()) != 20 {
		t.Fatalf("model calls = %d, usage = %+v, tool calls = %d", s.modelCalls(), out.Usage, len(r.tools.names()))
	}
}

func TestTurnLimitStillAllowsASaveOnTheLastTurn(t *testing.T) {
	turns := make([]scriptedTurn, 20)
	for i := range turns {
		turns[i] = toolTurn(call(fmt.Sprint("c", i), "get_spec_guide", `{}`))
	}
	turns[19] = toolTurn(call("last", "create_dashboard", `{}`))
	r := newRig(t, session(turns...))
	r.tools.onSave("create_dashboard", savedResult(dashID, 1))

	if out, err := r.agent.Run(context.Background(), r.task()); err != nil || out.Version != 1 {
		t.Fatalf("out = %+v err = %v", out, err)
	}
}

func TestCancellationBetweenTurns(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := session(toolTurn(call("c1", "get_spec_guide", `{}`)), toolTurn(call("c2", "create_dashboard", `{}`)))
	r := newRig(t, s)
	r.tools.on("get_spec_guide", func(context.Context, json.RawMessage) (tools.Result, error) {
		cancel()
		return tools.Result{Content: map[string]any{}, Step: "Read the guide"}, nil
	})

	out, err := r.agent.Run(ctx, r.task())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if out.Usage.Calls != 1 {
		t.Fatalf("usage = %+v", out.Usage)
	}
	for _, name := range r.tools.names() {
		if name == "create_dashboard" {
			t.Fatal("a cancelled run saved")
		}
	}
}

func TestCancellationDuringAToolCallIsNotAnInternalError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := newRig(t, session(toolTurn(call("c1", "get_spec_guide", `{}`))))
	r.tools.on("get_spec_guide", func(ctx context.Context, _ json.RawMessage) (tools.Result, error) {
		cancel()
		return tools.Result{}, errors.New("query failed: " + ctx.Err().Error())
	})

	_, err := r.agent.Run(ctx, r.task())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}

func TestDeadlineExpiryIsReturnedAsTheContextError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	r := newRig(t, session(toolTurn(call("c1", "get_spec_guide", `{}`)), toolTurn(call("c2", "create_dashboard", `{}`))))
	r.tools.on("get_spec_guide", func(ctx context.Context, _ json.RawMessage) (tools.Result, error) {
		<-ctx.Done()
		return tools.Result{Content: map[string]any{}, Step: "slow"}, nil
	})

	if _, err := r.agent.Run(ctx, r.task()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
}

func TestInternalToolErrorAbortsWithASafeMessage(t *testing.T) {
	cause := errors.New("pq: password authentication failed for user app at db.internal:5432")
	r := newRig(t, session(
		toolTurn(call("c1", "create_dashboard", `{"spec":"UNIQUE-INPUT"}`)),
		toolTurn(call("never", "create_dashboard", `{}`)),
	))
	r.tools.on("create_dashboard", func(context.Context, json.RawMessage) (tools.Result, error) {
		return tools.Result{}, cause
	})

	out, err := r.agent.Run(context.Background(), r.task())
	if !errors.Is(err, cause) {
		t.Fatalf("err = %v", err)
	}
	if err.Error() != "the create_dashboard tool failed with an internal error" {
		t.Fatalf("error text = %q", err.Error())
	}
	if out.Usage.Calls != 1 {
		t.Fatalf("usage = %+v", out.Usage)
	}
	steps := r.recordedSteps()
	if len(steps) != 1 || steps[0].Status != "error" || steps[0].Message != "internal error" || steps[0].Phase != "save" {
		t.Fatalf("steps = %+v", steps)
	}
	if strings.Contains(r.logs.String(), "UNIQUE-INPUT") {
		t.Fatal("tool input reached the log")
	}
}

func TestInternalErrorInAReadAbortsBeforeTheSave(t *testing.T) {
	r := newRig(t, session(toolTurn(call("c1", "get_spec_guide", `{}`), call("c2", "create_dashboard", `{}`))))
	r.tools.on("get_spec_guide", func(context.Context, json.RawMessage) (tools.Result, error) {
		return tools.Result{}, errors.New("boom")
	})

	if _, err := r.agent.Run(context.Background(), r.task()); err == nil {
		t.Fatal("expected an error")
	}
	if got := r.tools.names(); len(got) != 1 {
		t.Fatalf("tool calls = %v: the save must not run after an internal error", got)
	}
}

func TestSaveErrorWithoutACodeFails(t *testing.T) {
	r := newRig(t, session(toolTurn(call("c1", "create_dashboard", `{}`))))
	r.tools.onSave("create_dashboard", tools.Result{IsError: true, Content: map[string]any{"message": "no code here"}, Step: "odd"})

	_, err := r.agent.Run(context.Background(), r.task())
	if err == nil || !strings.Contains(err.Error(), "unreadable result") {
		t.Fatalf("err = %v", err)
	}
}

func TestToolErrorsGoBackToTheModel(t *testing.T) {
	s := session(
		toolTurn(call("c1", "lint_query", `{}`)),
		toolTurn(call("c2", "create_dashboard", `{}`)),
	)
	r := newRig(t, s)
	r.tools.on("lint_query", func(context.Context, json.RawMessage) (tools.Result, error) {
		return tools.Result{IsError: true, Content: map[string]any{"code": "invalid_request", "message": "bad"}, Step: "Rejected lint_query input: invalid_request"}, nil
	})
	r.tools.onSave("create_dashboard", savedResult(dashID, 1))

	if _, err := r.agent.Run(context.Background(), r.task()); err != nil {
		t.Fatal(err)
	}
	got := s.results(1)
	if len(got) != 1 || !got[0].IsError || got[0].Content != `{"code":"invalid_request","message":"bad"}` {
		t.Fatalf("results = %+v", got)
	}
	steps := r.recordedSteps()
	if steps[0].Phase != "lint" || steps[0].Status != "error" || steps[0].Message != "Rejected lint_query input: invalid_request" {
		t.Fatalf("steps = %+v", steps)
	}
}

func TestProviderFailures(t *testing.T) {
	apiErr := fmt.Errorf("%w: fake returned status 500", llm.ErrProvider)
	t.Run("next", func(t *testing.T) {
		r := newRig(t, session(scriptedTurn{err: apiErr}))
		_, err := r.agent.Run(context.Background(), r.task())
		if !errors.Is(err, llm.ErrProvider) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("start", func(t *testing.T) {
		r := newRig(t)
		r.provider.startErr = apiErr
		_, err := r.agent.Run(context.Background(), r.task())
		if !errors.Is(err, llm.ErrProvider) {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestProgressIsOptional(t *testing.T) {
	r := newRig(t, session(toolTurn(call("c1", "get_spec_guide", `{}`)), toolTurn(call("c2", "create_dashboard", `{}`))))
	r.tools.onSave("create_dashboard", savedResult(dashID, 1))
	task := r.task()
	task.Progress = nil
	if _, err := r.agent.Run(context.Background(), task); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidTasksNeverReachTheModel(t *testing.T) {
	cases := map[string]func(*agent.Task){
		"empty prompt":        func(t *agent.Task) { t.Prompt = " \n\t" },
		"long prompt":         func(t *agent.Task) { t.Prompt = strings.Repeat("é", 4001) },
		"no principal":        func(t *agent.Task) { t.Principal = auth.Principal{} },
		"unknown kind":        func(t *agent.Task) { t.Principal.Kind = "robot" },
		"bad dashboard id":    func(t *agent.Task) { t.DashboardID = "not-a-uuid" },
		"panel without board": func(t *agent.Task) { t.PanelID = panelID },
		"bad panel id":        func(t *agent.Task) { t.DashboardID = dashID; t.PanelID = "bad panel!" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			r := newRig(t, session())
			task := r.task()
			mutate(&task)
			out, err := r.agent.Run(context.Background(), task)
			if !errors.Is(err, agent.ErrInvalidTask) {
				t.Fatalf("err = %v", err)
			}
			if len(r.provider.started()) != 0 || r.dash.getCount() != 0 || out.Provider != "fake" {
				t.Fatalf("started=%d loads=%d out=%+v", len(r.provider.started()), r.dash.getCount(), out)
			}
		})
	}
}

func TestPromptAtTheLimitIsAccepted(t *testing.T) {
	r := newRig(t, session(toolTurn(call("c1", "create_dashboard", `{}`))))
	r.tools.onSave("create_dashboard", savedResult(dashID, 1))
	task := r.task()
	task.Prompt = strings.Repeat("é", 4000)
	if _, err := r.agent.Run(context.Background(), task); err != nil {
		t.Fatal(err)
	}
}

func TestReviseLoadFailures(t *testing.T) {
	t.Run("not found", func(t *testing.T) {
		r := newRig(t, session())
		r.dash.err = dashboards.ErrNotFound
		_, err := r.agent.Run(context.Background(), r.reviseTask())
		if !errors.Is(err, dashboards.ErrNotFound) || err.Error() != "dashboard not found" {
			t.Fatalf("err = %v", err)
		}
		if len(r.provider.started()) != 0 {
			t.Fatal("a session started for a missing dashboard")
		}
	})
	t.Run("store error", func(t *testing.T) {
		cause := errors.New("connection refused to 10.0.0.5:5432")
		r := newRig(t, session())
		r.dash.err = cause
		_, err := r.agent.Run(context.Background(), r.reviseTask())
		if !errors.Is(err, cause) || strings.Contains(err.Error(), "10.0.0.5") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestLogsCarryNeitherPromptNorToolData(t *testing.T) {
	r := newRig(t, session(
		toolTurn(call("c1", "get_spec_guide", `{"q":"UNIQUE-INPUT"}`)),
		toolTurn(call("c2", "create_dashboard", `{"spec":"UNIQUE-INPUT"}`)),
	))
	r.tools.on("get_spec_guide", func(context.Context, json.RawMessage) (tools.Result, error) {
		return tools.Result{Content: map[string]any{"v": "UNIQUE-RESULT"}, Step: "Read"}, nil
	})
	r.tools.onSave("create_dashboard", invalidSpecResult("x_code"))
	task := r.task()
	task.Prompt = "UNIQUE-PROMPT show me everything"

	if _, err := r.agent.Run(context.Background(), task); err == nil {
		t.Fatal("expected a failure")
	}
	logs := r.logs.String()
	for _, secret := range []string{"UNIQUE-PROMPT", "UNIQUE-INPUT", "UNIQUE-RESULT", "SECRET"} {
		if strings.Contains(logs, secret) {
			t.Fatalf("log leaks %s: %s", secret, logs)
		}
	}
}

func writeFixture(t *testing.T, dir, name string, fixture any) {
	t.Helper()
	body, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func replayRig(t *testing.T, prompt string, turns []map[string]any) *rig {
	t.Helper()
	dir := t.TempDir()
	writeFixture(t, dir, "run.json", map[string]any{"prompt": prompt, "turns": turns})
	provider, err := replay.New(dir, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	r := newRig(t)
	r.agent = agent.New(agent.Config{
		Provider:    provider,
		Tools:       r.tools,
		Dashboards:  r.dash,
		Datasources: fakeHealth{},
		Logger:      slog.New(slog.NewJSONHandler(r.logs, nil)),
	})
	return r
}

func TestReplayFixtureHappyPathEndsOnTheSave(t *testing.T) {
	r := replayRig(t, "checkout health by pod", []map[string]any{
		{"tool_calls": []map[string]any{{"name": "list_datasources"}, {"name": "get_spec_guide"}}},
		{"tool_calls": []map[string]any{{"name": "lint_query", "input": map[string]any{"q": 1}}}, "expect": []map[string]any{{"is_error": false}, {"is_error": false}}},
		{"tool_calls": []map[string]any{{"name": "create_dashboard", "input": map[string]any{"spec": map[string]any{}}}}, "expect": []map[string]any{{"is_error": false, "contains": "ok"}}},
		{"text": "never requested: the run ends on the save"},
	})
	r.tools.onSave("create_dashboard", savedResult(dashID, 1))

	out, err := r.agent.Run(context.Background(), r.task())
	if err != nil {
		t.Fatal(err)
	}
	if out.Provider != "replay" || out.Version != 1 || out.Usage.Calls != 3 {
		t.Fatalf("outcome = %+v", out)
	}
}

func TestReplayFixtureWithARejectedFirstSaveRepairs(t *testing.T) {
	r := replayRig(t, "checkout health by pod", []map[string]any{
		{"tool_calls": []map[string]any{{"name": "create_dashboard", "input": map[string]any{"spec": map[string]any{"v": 1}}}}},
		{"tool_calls": []map[string]any{{"name": "create_dashboard", "input": map[string]any{"spec": map[string]any{"v": 2}}}}, "expect": []map[string]any{{"is_error": true, "contains": "invalid_spec"}}},
	})
	r.tools.onSave("create_dashboard", invalidSpecResult("unknown_field"), savedResult(dashID, 1))

	out, err := r.agent.Run(context.Background(), r.task())
	if err != nil {
		t.Fatal(err)
	}
	if out.RepairRounds != 1 || out.Version != 1 {
		t.Fatalf("outcome = %+v", out)
	}
}

func TestReplayFixtureThatNeverSavesFails(t *testing.T) {
	r := replayRig(t, "checkout health by pod", []map[string]any{
		{"tool_calls": []map[string]any{{"name": "get_spec_guide"}}},
		{"text": "I could not find anything."},
	})
	out, err := r.agent.Run(context.Background(), r.task())
	if !errors.Is(err, agent.ErrNoSave) {
		t.Fatalf("err = %v", err)
	}
	if out.Usage.Calls != 2 {
		t.Fatalf("usage = %+v", out.Usage)
	}
}

func TestReplayFixtureExpectingTheWrongToolResultFails(t *testing.T) {
	r := replayRig(t, "checkout health by pod", []map[string]any{
		{"tool_calls": []map[string]any{{"name": "get_spec_guide"}}},
		{"tool_calls": []map[string]any{{"name": "create_dashboard"}}, "expect": []map[string]any{{"is_error": true}}},
	})
	r.tools.onSave("create_dashboard", savedResult(dashID, 1))
	if _, err := r.agent.Run(context.Background(), r.task()); err == nil {
		t.Fatal("expected the fixture's expectation to fail the run")
	}
}
