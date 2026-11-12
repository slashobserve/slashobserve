package agent_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/agent"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/llm"
	"github.com/slashobserve/slashobserve/internal/llm/pricing"
	"github.com/slashobserve/slashobserve/internal/llm/replay"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/pkg/spec"
	"github.com/slashobserve/slashobserve/spec/examples"
)

const (
	requestID = "0190b5c0-0000-7000-8000-0000000000bb"
	dashID    = "0190b5c0-0000-7000-8000-000000000001"
	panelID   = "p_p99"
	panelName = "p99 latency"

	latestVersion = 3
)

var (
	_ agent.ToolSet          = (*tools.Registry)(nil)
	_ agent.DashboardReader  = (*dashboards.Service)(nil)
	_ agent.BuildStore       = (*dashboards.Service)(nil)
	_ agent.DatasourceHealth = (*datasource.Registry)(nil)
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

	compactions []llm.Compaction
}

func (s *scriptedSession) Compact(c llm.Compaction) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.compactions = append(s.compactions, c)
	return nil
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
	{Name: "update_panels", Description: "patch", InputSchema: json.RawMessage(`{"type":"object"}`)},
	{Name: "get_panels", Description: "panels", InputSchema: json.RawMessage(`{"type":"object"}`), ReadOnly: true},
	{Name: "save_finding", Description: "finding", InputSchema: json.RawMessage(`{"type":"object"}`), AgentOnly: true, Answers: true},
}

func offeredIn(mode tools.Mode) []tools.Tool {
	var out []tools.Tool
	for _, tool := range fakeToolList {
		if tool.OfferedIn(mode) {
			out = append(out, tool)
		}
	}
	return out
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
	byNum  map[int]*spec.Dashboard
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

func (d *fakeDashboards) GetVersion(_ context.Context, _ pgtype.UUID, version int) (dashboards.Dashboard, dashboards.Version, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.err != nil {
		return dashboards.Dashboard{}, dashboards.Version{}, d.err
	}
	if version < 1 || version > d.number {
		return dashboards.Dashboard{}, dashboards.Version{}, dashboards.ErrNotFound
	}
	content := d.spec
	if byNum, ok := d.byNum[version]; ok {
		content = byNum
	}
	return dashboards.Dashboard{ID: dashID, LatestVersion: d.number},
		dashboards.Version{VersionMeta: dashboards.VersionMeta{Number: version}, Spec: content}, nil
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
	builds    *fakeBuilds
	agent     *agent.Agent
	logs      *bytes.Buffer
	steps     []agent.Step
	stepsLock sync.Mutex
}

func newRig(t *testing.T, sessions ...*scriptedSession) *rig {
	t.Helper()
	return newRigWithRenewBytes(t, 0, sessions...)
}

func newRigWithRenewBytes(t *testing.T, renewBytes int, sessions ...*scriptedSession) *rig {
	t.Helper()
	return newRigWithConfig(t, func(c *agent.Config) { c.SessionRenewBytes = renewBytes }, sessions...)
}

func newRigWithConfig(t *testing.T, configure func(*agent.Config), sessions ...*scriptedSession) *rig {
	t.Helper()
	parsed, err := spec.Parse(examples.CheckoutServiceHealth)
	if err != nil {
		t.Fatal(err)
	}
	r := &rig{
		t:        t,
		provider: &scriptedProvider{sessions: sessions},
		tools:    &fakeTools{},
		dash:     &fakeDashboards{number: latestVersion, spec: parsed},
		builds:   &fakeBuilds{},
		logs:     &bytes.Buffer{},
	}
	config := agent.Config{
		Provider:    r.provider,
		Tools:       r.tools,
		Dashboards:  r.dash,
		Builds:      r.builds,
		Datasources: fakeHealth{},
		Logger:      slog.New(slog.NewJSONHandler(r.logs, nil)),
	}
	configure(&config)
	r.agent = agent.New(config)
	return r
}

func (r *rig) logEntry(msg string) map[string]any {
	r.t.Helper()
	for line := range strings.SplitSeq(r.logs.String(), "\n") {
		var entry map[string]any
		if json.Unmarshal([]byte(line), &entry) == nil && entry["msg"] == msg {
			return entry
		}
	}
	r.t.Fatalf("no %q log line in %s", msg, r.logs.String())
	return nil
}

func session(turns ...scriptedTurn) *scriptedSession { return &scriptedSession{script: turns} }

func (r *rig) task() agent.Task {
	return agent.Task{
		Prompt:    "checkout health by pod",
		RequestID: requestID,
		Attempt:   1,
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
	task.RequestID = requestID
	return task
}

func (r *rig) recordedSteps() []agent.Step {
	r.stepsLock.Lock()
	defer r.stepsLock.Unlock()
	return append([]agent.Step(nil), r.steps...)
}

func (r *rig) finishLog() map[string]any {
	r.t.Helper()
	return r.logEntry("agent run finished")
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
		if !c.scoped || c.run.Mode != tools.ModeCreate || c.run.DashboardID != "" || c.run.ParentVersion != 0 || c.run.RequestID != requestID || !c.run.Build || c.run.Attempt != 1 {
			t.Fatalf("call %s run scope = %+v scoped=%v", c.name, c.run, c.scoped)
		}
		if !reflect.DeepEqual(c.principal, principal) {
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
	if _, ok := fin["error_code"]; ok {
		t.Fatal("successful run logged an error")
	}
}

func fakeModelPrices(t *testing.T) pricing.Table {
	t.Helper()
	table, err := pricing.Parse([]byte("models:\n  fake-model: { input: 2.00, output: 10.00, cache_read: 0.20, cache_write: 2.50 }\n"))
	if err != nil {
		t.Fatal(err)
	}
	return table
}

func TestOutcomeCarriesTheEstimatedCostOfTheRun(t *testing.T) {
	run := func(t *testing.T, prices pricing.Table) agent.Outcome {
		t.Helper()
		s := session(
			toolTurn(call("c1", "list_datasources", `{}`)),
			toolTurn(call("c2", "lint_query", `{"n":1}`)),
			toolTurn(call("c3", "create_dashboard", `{"spec":{}}`)),
		)
		r := newRigWithConfig(t, func(c *agent.Config) { c.Prices = prices }, s)
		r.tools.onSave("create_dashboard", savedResult(dashID, 1))
		out, err := r.agent.Run(context.Background(), r.task())
		if err != nil {
			t.Fatal(err)
		}
		return out
	}

	t.Run("a priced model", func(t *testing.T) {
		out := run(t, fakeModelPrices(t))
		const want = (300*2.00 + 30*10.00 + 15*0.20 + 6*2.50) / 1e6
		if out.CostUSD == nil || math.Abs(*out.CostUSD-want) > 1e-12 {
			t.Fatalf("cost = %v, want %v", out.CostUSD, want)
		}
	})
	t.Run("a model with no price", func(t *testing.T) {
		if out := run(t, pricing.Table{}); out.CostUSD != nil {
			t.Fatalf("cost = %v, want none", *out.CostUSD)
		}
	})
}

func budgetRig(t *testing.T, maxCostUSD float64, turns ...scriptedTurn) (*rig, *scriptedSession) {
	t.Helper()
	s := session(turns...)
	r := newRigWithConfig(t, func(c *agent.Config) {
		c.Prices = fakeModelPrices(t)
		c.MaxRunCostUSD = maxCostUSD
	}, s)
	r.tools.onSave("create_dashboard", savedResult(dashID, 1))
	return r, s
}

func TestRunStopsBeforeTheCallThatTheCostLimitForbids(t *testing.T) {
	const perTurnUSD = (100*2.00 + 10*10.00 + 5*0.20 + 2*2.50) / 1e6
	cases := map[string]struct {
		limit     float64
		wantCalls int
	}{
		"limit below the first turn":    {perTurnUSD / 2, 1},
		"limit equal to the first turn": {perTurnUSD, 1},
		"limit crossed by the second":   {perTurnUSD * 1.5, 2},
		"limit above the whole script":  {perTurnUSD * 10, 3},
		"no limit":                      {0, 3},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r, s := budgetRig(t, tc.limit,
				toolTurn(call("c1", "list_datasources", `{}`)),
				toolTurn(call("c2", "lint_query", `{"n":1}`)),
				toolTurn(call("c3", "create_dashboard", `{"spec":{}}`)),
			)
			out, err := r.agent.Run(context.Background(), r.task())
			if got := s.modelCalls(); got != tc.wantCalls {
				t.Fatalf("model calls = %d, want %d", got, tc.wantCalls)
			}
			if tc.wantCalls == 3 {
				if err != nil || out.Version != 1 {
					t.Fatalf("version %d, err %v: a limit never reached must not stop the run", out.Version, err)
				}
				return
			}
			if !errors.Is(err, agent.ErrBudgetExhausted) {
				t.Fatalf("err = %v, want ErrBudgetExhausted", err)
			}
			if out.Usage.Calls != tc.wantCalls || out.CostUSD == nil {
				t.Fatalf("outcome = %+v: the calls made must still be counted and priced", out)
			}
			if got := len(r.tools.names()); got != tc.wantCalls {
				t.Fatalf("tools run = %v: the crossing turn's tool calls must still run", r.tools.names())
			}
		})
	}
}

func TestRunStoppedByTheCostLimitFailsWithItsOwnCodeAndKeepsItsMessage(t *testing.T) {
	r, _ := budgetRig(t, 1e-6, toolTurn(call("c1", "list_datasources", `{}`)))
	result, err := agent.NewReviser(r.agent, nil).Revise(context.Background(), revisionRequest())
	if string(result.FailureCode) != "budget_exhausted" {
		t.Fatalf("failure code = %q", result.FailureCode)
	}
	if err == nil || err.Error() != agent.ErrBudgetExhausted.Error() {
		t.Fatalf("stored error = %v, want %q", err, agent.ErrBudgetExhausted)
	}
	if result.Usage == nil || result.Usage.CostUSD == nil {
		t.Fatalf("usage = %+v: a stopped run still reports its cost", result.Usage)
	}
}

func TestSaveInTheTurnThatCrossesTheCostLimitStillCommits(t *testing.T) {
	r, s := budgetRig(t, 1e-6, toolTurn(call("c1", "create_dashboard", `{"spec":{}}`)))
	out, err := r.agent.Run(context.Background(), r.task())
	if err != nil || out.Version != 1 || s.modelCalls() != 1 {
		t.Fatalf("version %d, calls %d, err %v: the finishing save must not be thrown away", out.Version, s.modelCalls(), err)
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
	if reqs[0].Prompt != "checkout health by pod" {
		t.Fatalf("prompt = %q", reqs[0].Prompt)
	}
	wantContext := "Configured datasources:\n- logs (clickhouse): unhealthy\n- prom (prometheus): healthy\n\nCreate a new dashboard for the request, or answer it with save_finding if it only asks a question."
	if reqs[0].Context != wantContext {
		t.Fatalf("context = %q", reqs[0].Context)
	}
	offered := offeredIn(tools.ModeCreate)
	if len(reqs[0].Tools) != len(offered) {
		t.Fatalf("tools = %d", len(reqs[0].Tools))
	}
	for i, spec := range reqs[0].Tools {
		if spec.Name != offered[i].Name || spec.Description != offered[i].Description || string(spec.InputSchema) != string(offered[i].InputSchema) {
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

func TestThreeRejectedSavesInARowEndTheRun(t *testing.T) {
	s := session(
		toolTurn(call("c1", "create_dashboard", `{"n":1}`)),
		toolTurn(call("c2", "create_dashboard", `{"n":2}`)),
		toolTurn(call("c3", "create_dashboard", `{"n":3}`)),
		toolTurn(call("c4", "create_dashboard", `{"n":4}`)),
	)
	r := newRig(t, s)
	r.tools.onSave("create_dashboard", invalidSpecResult("a_code"), invalidSpecResult("b_code", "c_code"), invalidSpecResult("d_code", "d_code", "e_code", "f_code", "g_code"), savedResult(dashID, 1))

	out, err := r.agent.Run(context.Background(), r.task())
	if !errors.Is(err, agent.ErrRepairExhausted) {
		t.Fatalf("err = %v", err)
	}
	if got := s.modelCalls(); got != 3 {
		t.Fatalf("model calls = %d, want 3 rejected saves", got)
	}
	if out.RepairRounds != 2 || out.Version != 0 || out.Usage.Calls != 3 || out.Provider != "fake" {
		t.Fatalf("outcome = %+v", out)
	}
	if !strings.Contains(err.Error(), "d_code, e_code, f_code") || strings.Contains(err.Error(), "g_code") || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("error should list at most three issue codes and no messages: %q", err.Error())
	}
	steps := r.recordedSteps()
	if got := strings.Join(phases(steps[:3]), ","); got != "save,repair,repair" || len(steps) != 4 {
		t.Fatalf("phases = %s, steps = %+v", got, steps)
	}
	if stop := steps[3]; stop.Status != "error" || stop.Message != "Stopped: 3 saves in a row were rejected" {
		t.Fatalf("stop step = %+v", stop)
	}
	fin := r.finishLog()
	if fin["error_code"] != "validation_exhausted" || fin["error"] != nil || fin["repair_rounds"] != float64(2) {
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
			call("1", "query_range", `{"n":1}`), call("2", "query_range", `{"n":2}`), call("3", "query_range", `{"n":3}`),
			call("4", "query_range", `{"n":4}`), call("5", "query_range", `{"n":5}`), call("6", "query_range", `{"n":6}`),
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
	task.Source = revisions.SourceComment

	out, err := r.agent.Run(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}
	if out.DashboardID != dashID || out.Version != 4 || !out.Replayed {
		t.Fatalf("outcome = %+v", out)
	}
	for _, c := range r.tools.recorded() {
		want := tools.Run{Mode: tools.ModeRevise, DashboardID: dashID, ParentVersion: 3, RequestID: requestID, Prompt: task.Prompt, Build: true, Attempt: 1}
		if !c.scoped || c.run != want {
			t.Fatalf("call %s run = %+v scoped=%v, want %+v", c.name, c.run, c.scoped, want)
		}
	}
	reqs := r.provider.started()
	for _, want := range []string{
		"- prom (prometheus): healthy",
		"You are revising dashboard " + dashID + ", currently version 3. Change it for the request, or answer it with save_finding if it only asks a question.\n```json\n{",
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
	task.Source = revisions.SourceComment

	if _, err := r.agent.Run(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if got := r.provider.started()[0].Context; !strings.Contains(got, "Panel p_gone from the comment is not in the latest version.") {
		t.Fatalf("context = %s", got)
	}
}

func TestRevisePromptOnAPanelDoesNotCallItAComment(t *testing.T) {
	for name, c := range map[string]struct {
		panel string
		want  string
	}{
		"known panel":   {panelID, fmt.Sprintf("The request is about panel %s titled %q.", panelID, panelName)},
		"missing panel": {"p_gone", "Panel p_gone from the request is not in the latest version."},
	} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t, session(toolTurn(call("c1", "update_dashboard", `{}`))))
			r.tools.onSave("update_dashboard", savedResult(dashID, 4))
			task := r.reviseTask()
			task.PanelID = c.panel
			task.Source = revisions.SourcePrompt

			if _, err := r.agent.Run(context.Background(), task); err != nil {
				t.Fatal(err)
			}
			got := r.provider.started()[0].Context
			if !strings.Contains(got, c.want) || strings.Contains(got, "comment") {
				t.Fatalf("context = %s", got)
			}
		})
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

func TestReviseConflictWithoutANewVersionGoesBackToTheModel(t *testing.T) {
	s := session(
		toolTurn(call("c1", "update_dashboard", `{"parent_version":2}`)),
		toolTurn(call("c2", "update_dashboard", `{}`)),
	)
	r := newRig(t, s)
	r.tools.onSave("update_dashboard", conflictResult(3), savedResult(dashID, 4))

	out, err := r.agent.Run(context.Background(), r.reviseTask())
	if err != nil {
		t.Fatal(err)
	}
	if out.Version != 4 || out.RepairRounds != 1 {
		t.Fatalf("outcome = %+v", out)
	}
	if got := len(r.provider.started()); got != 1 {
		t.Fatalf("sessions = %d, want no restart while the dashboard did not move", got)
	}
	if r.dash.getCount() != 1 {
		t.Fatalf("dashboard loads = %d", r.dash.getCount())
	}
	got := s.results(1)
	if len(got) != 1 || !got[0].IsError || !strings.Contains(got[0].Content, "version_conflict") {
		t.Fatalf("the conflict should reach the model: %+v", got)
	}
}

func TestReviseRepeatedConflictWithoutANewVersionExhaustsRepairs(t *testing.T) {
	turns := make([]scriptedTurn, 3)
	for i := range turns {
		turns[i] = toolTurn(call(fmt.Sprint("c", i), "update_dashboard", fmt.Sprintf(`{"parent_version":2,"n":%d}`, i)))
	}
	r := newRig(t, session(turns...))
	r.tools.onSave("update_dashboard", conflictResult(3), conflictResult(3), conflictResult(3))

	_, err := r.agent.Run(context.Background(), r.reviseTask())
	if !errors.Is(err, agent.ErrRepairExhausted) || !strings.Contains(err.Error(), "version_conflict") {
		t.Fatalf("err = %v", err)
	}
	if errors.Is(err, agent.ErrConflict) || len(r.provider.started()) != 1 {
		t.Fatalf("err = %v sessions = %d", err, len(r.provider.started()))
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
		toolTurn(call("c1", "create_dashboard", `{"n":1}`)),
		toolTurn(call("c2", "create_dashboard", `{"n":2}`)),
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
	if fin := r.finishLog(); fin["error_code"] != "refused" || fin["error"] != nil {
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
			turns := []scriptedTurn{toolTurn(call("c1", "get_spec_guide", `{}`)), stopTurn(tc.stop), stopTurn(tc.stop)}
			r := newRig(t, session(turns...))
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
			if want := len(turns); out.Usage.Calls != want {
				t.Fatalf("usage = %+v, want %d calls", out.Usage, want)
			}
		})
	}
}

func TestMaxTokensWithToolCallsRunsNothing(t *testing.T) {
	truncated := scriptedTurn{turn: llm.Turn{
		ToolCalls: []llm.ToolCall{call("c1", "query_range", `{"half`), call("c2", "create_dashboard", `{"spec":`)},
		Stop:      llm.StopMaxTokens,
		Usage:     usage(),
	}}
	r := newRig(t, session(truncated, truncated))
	r.tools.onSave("create_dashboard", savedResult(dashID, 1))

	out, err := r.agent.Run(context.Background(), r.task())
	if !errors.Is(err, agent.ErrNoSave) || !strings.Contains(err.Error(), "after reaching max tokens") {
		t.Fatalf("err = %v", err)
	}
	if got := r.tools.names(); len(got) != 0 {
		t.Fatalf("tools ran on a truncated turn: %v", got)
	}
	if out.Version != 0 || out.Usage.Calls != 2 {
		t.Fatalf("outcome = %+v", out)
	}
}

func TestStepTimestampsMarkWhenEachReadFinished(t *testing.T) {
	r := newRig(t, session(
		toolTurn(call("slow", "query_range", `{"n":1}`), call("fast", "lint_query", `{"n":2}`)),
		toolTurn(call("s", "create_dashboard", `{}`)),
	))
	r.tools.on("query_range", func(context.Context, json.RawMessage) (tools.Result, error) {
		time.Sleep(150 * time.Millisecond)
		return tools.Result{Content: map[string]any{"ok": true}, Step: "slow"}, nil
	})
	r.tools.onSave("create_dashboard", savedResult(dashID, 1))

	if _, err := r.agent.Run(context.Background(), r.task()); err != nil {
		t.Fatal(err)
	}
	steps := r.recordedSteps()
	if steps[0].Tool != "query_range" || steps[1].Tool != "lint_query" {
		t.Fatalf("steps should stay in call order: %+v", steps)
	}
	if gap := steps[0].At.Sub(steps[1].At); gap < 100*time.Millisecond {
		t.Fatalf("the slow call finished %v after the fast one, want at least 100ms: %+v", gap, steps)
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
	cause := errors.New("pq: password authentication failed for user app at db.internal:5432 running SELECT UNIQUE-QUERY")
	r := newRig(t, session(
		toolTurn(call("c1", "create_dashboard", `{"spec":"UNIQUE-INPUT"}`)),
		toolTurn(call("never", "create_dashboard", `{}`)),
	))
	r.tools.on("create_dashboard", func(context.Context, json.RawMessage) (tools.Result, error) {
		return tools.Result{}, cause
	})
	task := r.task()
	task.RequestID = requestID

	out, err := r.agent.Run(context.Background(), task)
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
	logs := r.logs.String()
	for _, leak := range []string{"UNIQUE-INPUT", "UNIQUE-QUERY", "password", "db.internal"} {
		if strings.Contains(logs, leak) {
			t.Fatalf("log leaks %q: %s", leak, logs)
		}
	}
	failed := r.logEntry("agent tool failed")
	if failed["tool"] != "create_dashboard" || failed["error_code"] != "tool_failed" || failed["agent_request_id"] != requestID ||
		failed["error_type"] != "*errors.errorString" || failed["error"] != nil || failed["request_id"] != nil {
		t.Fatalf("tool failure log = %v", failed)
	}
}

func TestToolFailureLogCarriesTheErrorCodeButNeverTheQueryText(t *testing.T) {
	tests := []struct {
		name     string
		tool     string
		err      error
		code     string
		typ      string
		sqlstate string
	}{
		{
			name: "postgres error from a dashboard tool",
			tool: "create_dashboard",
			err: fmt.Errorf("tool create_dashboard: %w", &pgconn.PgError{
				Code:    "23505",
				Message: "duplicate key value violates unique constraint for UNIQUE-QUERY",
				Detail:  "Key (spec)=(UNIQUE-QUERY) already exists",
			}),
			code:     "tool_failed",
			typ:      "*pgconn.PgError",
			sqlstate: "23505",
		},
		{
			name: "oversize result",
			tool: "query_range",
			err:  fmt.Errorf("tool query_range: %w: 20000 bytes for UNIQUE-QUERY", tools.ErrResultTooLarge),
			code: "result_too_large",
			typ:  "*errors.errorString",
		},
		{
			name: "panic",
			tool: "query_range",
			err:  fmt.Errorf("tool query_range: %w: UNIQUE-QUERY", tools.ErrPanicked),
			code: "tool_panicked",
			typ:  "*errors.errorString",
		},
		{
			name: "error that only wraps a context error",
			tool: "query_range",
			err:  fmt.Errorf("tool query_range: UNIQUE-QUERY: %w", context.DeadlineExceeded),
			code: "tool_failed",
			typ:  "context.deadlineExceededError",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t, session(toolTurn(call("c1", tc.tool, `{"query":"UNIQUE-QUERY"}`))))
			r.tools.on(tc.tool, func(context.Context, json.RawMessage) (tools.Result, error) {
				return tools.Result{}, tc.err
			})
			task := r.task()
			task.RequestID = requestID

			if _, err := r.agent.Run(context.Background(), task); !errors.Is(err, tc.err) {
				t.Fatalf("err = %v", err)
			}
			logs := r.logs.String()
			for _, leak := range []string{"UNIQUE-QUERY", "duplicate key", "already exists", "20000 bytes"} {
				if strings.Contains(logs, leak) {
					t.Fatalf("log leaks %q: %s", leak, logs)
				}
			}
			failed := r.logEntry("agent tool failed")
			if failed["tool"] != tc.tool || failed["error_code"] != tc.code || failed["agent_request_id"] != requestID ||
				failed["error_type"] != tc.typ || failed["sqlstate"] != nilIfEmpty(tc.sqlstate) || failed["error"] != nil {
				t.Fatalf("tool failure log = %v", failed)
			}
			if finish := r.finishLog(); finish["error_code"] != tc.code || finish["agent_request_id"] != requestID {
				t.Fatalf("finish line = %v", finish)
			}
		})
	}
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
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

func nodeExporterFull(t *testing.T) *spec.Dashboard {
	t.Helper()
	data, err := os.ReadFile("../../pkg/grafana/testdata/golden/rfmoz/node-exporter-full.json")
	if err != nil {
		t.Fatal(err)
	}
	var golden struct {
		Spec json.RawMessage `json:"spec"`
	}
	if err := json.Unmarshal(data, &golden); err != nil {
		t.Fatal(err)
	}
	d, err := spec.Parse(golden.Spec)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestReviseOfALargeDashboardStartsWithAnOutline(t *testing.T) {
	r := newRig(t, session(toolTurn(call("c1", "update_panels", `{}`))))
	r.dash.spec = nodeExporterFull(t)
	r.tools.onSave("update_panels", savedResult(dashID, 4))

	out, err := r.agent.Run(context.Background(), r.reviseTask())
	if err != nil {
		t.Fatal(err)
	}
	if out.Version != 4 {
		t.Fatalf("outcome = %+v", out)
	}
	reqs := r.provider.started()
	if len(reqs) != 1 {
		t.Fatalf("sessions = %d", len(reqs))
	}
	got := reqs[0].Context
	if !strings.Contains(got, "so this is its outline") || strings.Contains(got, "```json\n{") {
		t.Fatalf("context is not an outline: %.400s", got)
	}
}

func TestUpdatePanelsRepairIsCountedAsARepairRound(t *testing.T) {
	r := newRig(t, session(
		toolTurn(call("c1", "update_panels", `{"upsert":[{}]}`)),
		toolTurn(call("c2", "update_panels", `{"upsert":[{"id":"p"}]}`)),
	))
	r.tools.onSave("update_panels", invalidSpecResult("query_empty"), savedResult(dashID, 4))

	out, err := r.agent.Run(context.Background(), r.reviseTask())
	if err != nil {
		t.Fatal(err)
	}
	if out.Version != 4 || out.RepairRounds != 1 {
		t.Fatalf("outcome = %+v", out)
	}
	if got := strings.Join(phases(r.recordedSteps()), ","); got != "save,repair" {
		t.Fatalf("phases = %s", got)
	}
}

func TestUpdatePanelsConflictRestartsOnce(t *testing.T) {
	r := newRig(t,
		session(toolTurn(call("c1", "update_panels", `{}`))),
		session(toolTurn(call("c2", "update_panels", `{}`))),
	)
	var saves atomic.Int64
	r.tools.on("update_panels", func(context.Context, json.RawMessage) (tools.Result, error) {
		if saves.Add(1) == 1 {
			r.dash.setNumber(4)
			return conflictResult(4), nil
		}
		return savedResult(dashID, 5), nil
	})

	out, err := r.agent.Run(context.Background(), r.reviseTask())
	if err != nil {
		t.Fatal(err)
	}
	if out.Version != 5 || out.RepairRounds != 0 || len(r.provider.started()) != 2 {
		t.Fatalf("outcome = %+v, sessions = %d", out, len(r.provider.started()))
	}
}

func TestUpdatePanelsSecondConflictFails(t *testing.T) {
	r := newRig(t,
		session(toolTurn(call("c1", "update_panels", `{}`))),
		session(toolTurn(call("c2", "update_panels", `{}`))),
	)
	r.tools.onSave("update_panels", conflictResult(4), conflictResult(5))

	if _, err := r.agent.Run(context.Background(), r.reviseTask()); !errors.Is(err, agent.ErrConflict) {
		t.Fatalf("err = %v", err)
	}
}

func TestUpdatePanelsSaveOutputIsTheSavingTurnAlone(t *testing.T) {
	r := newRig(t, session(
		scriptedTurn{turn: llm.Turn{Stop: llm.StopToolUse, Usage: llm.Usage{OutputTokens: 900, Calls: 1}, ToolCalls: []llm.ToolCall{call("c1", "get_panels", `{"ids":["p1"]}`)}}},
		scriptedTurn{turn: llm.Turn{Stop: llm.StopToolUse, Usage: llm.Usage{OutputTokens: 120, Calls: 1}, ToolCalls: []llm.ToolCall{call("c2", "update_panels", `{}`)}}},
	))
	r.tools.onSave("update_panels", savedResult(dashID, 4))

	out, err := r.agent.Run(context.Background(), r.reviseTask())
	if err != nil {
		t.Fatal(err)
	}
	if out.MaxSaveOutputTokens != 120 {
		t.Fatalf("max save output tokens = %d, want 120", out.MaxSaveOutputTokens)
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
		if strings.Contains(r.logs.String(), "10.0.0.5") {
			t.Fatalf("log leaks the driver error: %s", r.logs.String())
		}
		failed := r.logEntry("agent could not load the dashboard")
		if failed["error_code"] != "load_failed" || failed["agent_request_id"] != requestID || failed["error_type"] != "*errors.errorString" ||
			failed["error"] != nil || failed["request_id"] != nil {
			t.Fatalf("load failure log = %v", failed)
		}
	})
}

func TestLogsCarryNeitherPromptNorToolData(t *testing.T) {
	r := newRig(t, session(
		toolTurn(call("c1", "get_spec_guide", `{"q":"UNIQUE-INPUT"}`)),
		stopTurn(llm.StopMaxTokens),
		toolTurn(call("c2", "create_dashboard", `{"spec":"UNIQUE-INPUT"}`)),
	))
	r.tools.on("get_spec_guide", func(context.Context, json.RawMessage) (tools.Result, error) {
		return tools.Result{
			Content:    map[string]any{"v": "UNIQUE-RESULT"},
			Step:       "Read",
			Discovered: &tools.Discovery{Datasource: "prom", Of: "metrics", Names: []string{"UNIQUE-METRIC"}},
		}, nil
	})
	r.tools.onSave("create_dashboard", invalidSpecResult("x_code"))
	task := r.task()
	task.Prompt = "UNIQUE-PROMPT show me everything"

	if _, err := r.agent.Run(context.Background(), task); err == nil {
		t.Fatal("expected a failure")
	}
	logs := r.logs.String()
	for _, secret := range []string{"UNIQUE-PROMPT", "UNIQUE-INPUT", "UNIQUE-RESULT", "UNIQUE-METRIC", "SECRET"} {
		if strings.Contains(logs, secret) {
			t.Fatalf("log leaks %s: %s", secret, logs)
		}
	}
}

func TestRunFinishedLogNeverCarriesToolOrModelText(t *testing.T) {
	leaks := []string{"UNIQUE-PROMPT", "UNIQUE-INPUT", "UNIQUE-TOOL", "UNIQUE-MODEL", "SECRET", "10.0.0.5"}
	type scenario struct {
		name string
		code string
		typ  string
		rig  func(t *testing.T) *rig
		task func(r *rig) agent.Task
		ctx  func() (context.Context, context.CancelFunc)
	}
	toolErr := func(err error) func(t *testing.T) *rig {
		return func(t *testing.T) *rig {
			r := newRig(t, session(toolTurn(call("c1", "create_dashboard", `{"spec":"UNIQUE-INPUT"}`))))
			r.tools.on("create_dashboard", func(context.Context, json.RawMessage) (tools.Result, error) {
				return tools.Result{}, err
			})
			return r
		}
	}
	oneTurn := func(turn scriptedTurn) func(t *testing.T) *rig {
		return func(t *testing.T) *rig { return newRig(t, session(turn)) }
	}
	reviseTask := func(r *rig) agent.Task { return r.reviseTask() }
	tests := []scenario{
		{
			name: "internal tool error",
			code: "tool_failed",
			typ:  "*errors.errorString",
			rig:  toolErr(fmt.Errorf("%w: SELECT UNIQUE-TOOL at 10.0.0.5", datasource.ErrUnavailable)),
		},
		{
			name: "tool error that only wraps a deadline",
			code: "tool_failed",
			typ:  "context.deadlineExceededError",
			rig:  toolErr(fmt.Errorf("tool create_dashboard: UNIQUE-TOOL: %w", context.DeadlineExceeded)),
		},
		{
			name: "tool error that only wraps a cancellation",
			code: "tool_failed",
			typ:  "*errors.errorString",
			rig:  toolErr(fmt.Errorf("tool create_dashboard: UNIQUE-TOOL: %w", context.Canceled)),
		},
		{
			name: "oversize tool result",
			code: "result_too_large",
			typ:  "*errors.errorString",
			rig:  toolErr(fmt.Errorf("tool create_dashboard: %w: UNIQUE-TOOL", tools.ErrResultTooLarge)),
		},
		{
			name: "tool panic",
			code: "tool_panicked",
			typ:  "*errors.errorString",
			rig:  toolErr(fmt.Errorf("tool create_dashboard: %w: UNIQUE-TOOL", tools.ErrPanicked)),
		},
		{
			name: "unreadable save result",
			code: "malformed_result",
			typ:  "*errors.errorString",
			rig: func(t *testing.T) *rig {
				r := newRig(t, session(toolTurn(call("c1", "create_dashboard", `{"spec":"UNIQUE-INPUT"}`))))
				r.tools.onSave("create_dashboard", tools.Result{Content: "UNIQUE-TOOL not an object"})
				return r
			},
		},
		{
			name: "rejected saves exhaust the repair budget",
			code: "validation_exhausted",
			typ:  "",
			rig: func(t *testing.T) *rig {
				turns := make([]scriptedTurn, 3)
				for i := range turns {
					turns[i] = toolTurn(call(fmt.Sprint("c", i), "create_dashboard", fmt.Sprintf(`{"spec":"UNIQUE-INPUT","n":%d}`, i)))
				}
				r := newRig(t, session(turns...))
				r.tools.onSave("create_dashboard", invalidSpecResult("a_code"), invalidSpecResult("a_code"), invalidSpecResult("a_code"))
				return r
			},
		},
		{
			name: "model ends without saving",
			code: "no_tool_call",
			typ:  "",
			rig: func(t *testing.T) *rig {
				words := scriptedTurn{turn: llm.Turn{Text: "UNIQUE-MODEL words", Stop: llm.StopEndTurn, Usage: usage()}}
				return newRig(t, session(words, words))
			},
		},
		{
			name: "model refuses",
			code: "refused",
			typ:  "",
			rig:  oneTurn(scriptedTurn{turn: llm.Turn{Text: "UNIQUE-MODEL words", Stop: llm.StopRefusal, Usage: usage()}}),
		},
		{
			name: "model call fails",
			code: "provider_error",
			typ:  "*errors.errorString",
			rig:  oneTurn(scriptedTurn{err: llm.ErrProvider}),
		},
		{
			name: "model call fails with unsanitized provider text",
			code: "provider_error",
			typ:  "*errors.errorString",
			rig:  oneTurn(scriptedTurn{err: errors.New("provider echoed UNIQUE-MODEL words and UNIQUE-PROMPT")}),
		},
		{
			name: "model call rate limited",
			code: "rate_limited",
			typ:  "*errors.errorString",
			rig:  oneTurn(scriptedTurn{err: llm.NewCallError("fake", llm.RateLimited, 429)}),
		},
		{
			name: "model call timed out",
			code: "call_timeout",
			typ:  "*errors.errorString",
			rig:  func(t *testing.T) *rig { return newRig(t, session(timedOut(), timedOut())) },
		},
		{
			name: "model hits the output limit",
			code: "output_limit",
			typ:  "",
			rig: func(t *testing.T) *rig {
				cut := scriptedTurn{turn: llm.Turn{Text: "UNIQUE-MODEL words", Stop: llm.StopMaxTokens, Usage: usage()}}
				return newRig(t, session(cut, cut))
			},
		},
		{
			name: "conflict",
			code: "conflict",
			typ:  "",
			rig: func(t *testing.T) *rig {
				r := newRig(t,
					session(toolTurn(call("c1", "update_dashboard", `{"spec":"UNIQUE-INPUT"}`))),
					session(toolTurn(call("c2", "update_dashboard", `{"spec":"UNIQUE-INPUT"}`))),
				)
				r.tools.onSave("update_dashboard", conflictResult(4), conflictResult(5))
				return r
			},
			task: reviseTask,
		},
		{
			name: "invalid task",
			code: "invalid_task",
			typ:  "",
			rig:  func(t *testing.T) *rig { return newRig(t, session()) },
			task: func(r *rig) agent.Task {
				task := r.task()
				task.PanelID = "p_1"
				return task
			},
		},
		{
			name: "dashboard not found",
			code: "not_found",
			typ:  "",
			rig: func(t *testing.T) *rig {
				r := newRig(t, session())
				r.dash.err = dashboards.ErrNotFound
				return r
			},
			task: reviseTask,
		},
		{
			name: "dashboard load fails",
			code: "load_failed",
			typ:  "*errors.errorString",
			rig: func(t *testing.T) *rig {
				r := newRig(t, session())
				r.dash.err = errors.New("connection refused to 10.0.0.5:5432")
				return r
			},
			task: reviseTask,
		},
		{
			name: "canceled",
			code: "canceled",
			typ:  "",
			rig:  func(t *testing.T) *rig { return newRig(t, session()) },
			ctx: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx, cancel
			},
		},
		{
			name: "deadline exceeded",
			code: "run_timeout",
			typ:  "",
			rig:  func(t *testing.T) *rig { return newRig(t, session()) },
			ctx: func() (context.Context, context.CancelFunc) {
				return context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := tc.rig(t)
			task := r.task()
			if tc.task != nil {
				task = tc.task(r)
			}
			task.Prompt = "UNIQUE-PROMPT show me everything"
			if task.RequestID == "" {
				task.RequestID = requestID
			}
			ctx, cancel := context.Background(), context.CancelFunc(func() {})
			if tc.ctx != nil {
				ctx, cancel = tc.ctx()
			}
			defer cancel()

			_, runErr := r.agent.Run(ctx, task)
			if runErr == nil {
				t.Fatal("expected a failure")
			}
			if tc.code == "provider_error" && !strings.HasPrefix(runErr.Error(), "model call failed: ") {
				t.Fatalf("the returned error changed: %q", runErr)
			}
			entry := r.finishLog()
			if want := finishLevelFor(tc.code); entry["level"] != want {
				t.Fatalf("finish line level = %v, want %s for %s", entry["level"], want, tc.code)
			}
			if entry["error_code"] != tc.code || entry["agent_request_id"] != task.RequestID || entry["error"] != nil {
				t.Fatalf("finish line = %v, want error_code %q, agent_request_id %q and no error text", entry, tc.code, task.RequestID)
			}
			if tc.typ == "" && (entry["error_type"] != nil || entry["sqlstate"] != nil) {
				t.Fatalf("a sentinel-only outcome logged error attributes: %v", entry)
			}
			if tc.typ != "" && entry["error_type"] != tc.typ {
				t.Fatalf("finish line error_type = %v, want %q", entry["error_type"], tc.typ)
			}
			finish, err := json.Marshal(entry)
			if err != nil {
				t.Fatal(err)
			}
			for _, leak := range leaks {
				if strings.Contains(string(finish), leak) {
					t.Fatalf("finish line leaks %q: %s", leak, finish)
				}
			}
		})
	}
}

func finishLevelFor(code string) string {
	switch code {
	case "canceled":
		return "INFO"
	case "tool_failed", "tool_panicked", "result_too_large", "malformed_result", "load_failed", "context_failed", "internal":
		return "ERROR"
	}
	return "WARN"
}

func TestSuccessfulRunFinishesAtInfo(t *testing.T) {
	r := newRig(t, session(toolTurn(call("c1", "create_dashboard", `{}`))))
	r.tools.onSave("create_dashboard", savedResult(dashID, 1))
	if _, err := r.agent.Run(context.Background(), r.task()); err != nil {
		t.Fatal(err)
	}
	if level := r.finishLog()["level"]; level != "INFO" {
		t.Fatalf("level = %v, want INFO", level)
	}
}

func debugAgent(r *rig) {
	r.agent = agent.New(agent.Config{
		Provider:    r.provider,
		Tools:       r.tools,
		Dashboards:  r.dash,
		Builds:      r.builds,
		Datasources: fakeHealth{},
		Logger:      slog.New(slog.NewJSONHandler(r.logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
	})
}

func TestToolCallFinishedLineHoldsNoInputOrResult(t *testing.T) {
	for _, debug := range []bool{false, true} {
		t.Run(fmt.Sprintf("debug=%v", debug), func(t *testing.T) {
			r := newRig(t, session(
				toolTurn(call("c1", "get_spec_guide", `{"q":"UNIQUE-INPUT"}`), call("c2", "UNIQUE-TOOLNAME", `{}`)),
				toolTurn(call("c3", "create_dashboard", `{"spec":"UNIQUE-INPUT"}`)),
			))
			if debug {
				debugAgent(r)
			}
			r.tools.on("get_spec_guide", func(context.Context, json.RawMessage) (tools.Result, error) {
				return tools.Result{Content: map[string]any{"v": "UNIQUE-RESULT"}, Step: "Read"}, nil
			})
			r.tools.onSave("create_dashboard", savedResult(dashID, 1))
			task := r.task()
			task.RequestID = requestID
			task.Prompt = "UNIQUE-PROMPT show me everything"
			if _, err := r.agent.Run(context.Background(), task); err != nil {
				t.Fatal(err)
			}

			var lines []map[string]any
			for line := range strings.SplitSeq(r.logs.String(), "\n") {
				var entry map[string]any
				if json.Unmarshal([]byte(line), &entry) == nil && entry["msg"] == "tool call finished" {
					lines = append(lines, entry)
				}
			}
			if !debug {
				if len(lines) != 0 {
					t.Fatalf("tool call lines at INFO: %v", lines)
				}
				return
			}
			if len(lines) != 3 {
				t.Fatalf("%d tool call lines, want 3: %s", len(lines), r.logs)
			}
			tools := map[string]bool{}
			for _, line := range lines {
				if line["level"] != "DEBUG" || line["agent_request_id"] != requestID || line["is_error"] != false {
					t.Errorf("line = %v", line)
				}
				if _, ok := line["duration"]; !ok {
					t.Errorf("line has no duration: %v", line)
				}
				if len(line) != 7 {
					t.Errorf("line has unexpected attributes: %v", line)
				}
				tools[fmt.Sprint(line["tool"])] = true
			}
			if !tools["get_spec_guide"] || !tools["create_dashboard"] || !tools["unknown_tool"] {
				t.Errorf("tools = %v", tools)
			}
			for _, secret := range []string{"UNIQUE-PROMPT", "UNIQUE-INPUT", "UNIQUE-RESULT", "UNIQUE-TOOLNAME"} {
				if strings.Contains(r.logs.String(), secret) {
					t.Fatalf("log leaks %s: %s", secret, r.logs)
				}
			}
		})
	}
}

func TestToolCallFinishedFlagsAnErroringCall(t *testing.T) {
	r := newRig(t, session(toolTurn(call("c1", "create_dashboard", `{"spec":"UNIQUE-INPUT"}`))))
	debugAgent(r)
	r.tools.onSave("create_dashboard", invalidSpecResult("x_code"))
	if _, err := r.agent.Run(context.Background(), r.task()); err == nil {
		t.Fatal("expected a failure")
	}
	if line := r.logEntry("tool call finished"); line["is_error"] != true || line["tool"] != "create_dashboard" {
		t.Fatalf("line = %v", line)
	}
}

func TestProviderErrorMessageIsLoggedOnlyWhenPresent(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    any
	}{
		{"present", "provider says UNIQUE-ECHO", "provider says UNIQUE-ECHO"},
		{"absent", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callErr := llm.NewCallError("fake", llm.ProviderError, 400)
			callErr.ProviderMessage = tt.message
			r := newRig(t, session(scriptedTurn{err: callErr}))
			task := r.task()
			task.Prompt = "UNIQUE-PROMPT show me everything"
			task.RequestID = requestID
			if _, err := r.agent.Run(context.Background(), task); err == nil {
				t.Fatal("expected a failure")
			}
			lines := r.llmCallLogs()
			if len(lines) != 1 || lines[0]["level"] != "WARN" || lines[0]["provider_error_message"] != tt.want {
				t.Fatalf("llm call lines = %v", lines)
			}
			if strings.Contains(r.logs.String(), "UNIQUE-PROMPT") {
				t.Fatalf("log leaks the prompt: %s", r.logs)
			}
			if finish := r.finishLog(); finish["provider_error_message"] != nil || strings.Contains(fmt.Sprint(finish), "UNIQUE-ECHO") {
				t.Fatalf("the run finished line carries the provider message: %v", finish)
			}
		})
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
	provider, err := replay.New(dir, 0, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	r := newRig(t)
	r.agent = agent.New(agent.Config{
		Provider:    provider,
		Tools:       r.tools,
		Dashboards:  r.dash,
		Builds:      r.builds,
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
	if out.Usage.Calls != 3 || out.Compactions != 1 {
		t.Fatalf("outcome = %+v", out)
	}
}

func TestReplayFixtureExpectingTheWrongToolResultFails(t *testing.T) {
	r := replayRig(t, "checkout health by pod", []map[string]any{
		{"tool_calls": []map[string]any{{"name": "get_spec_guide"}}},
		{"tool_calls": []map[string]any{{"name": "create_dashboard"}}, "expect": []map[string]any{{"is_error": true}}},
	})
	r.tools.onSave("create_dashboard", savedResult(dashID, 1))
	_, err := r.agent.Run(context.Background(), r.task())
	if err == nil {
		t.Fatal("expected the fixture's expectation to fail the run")
	}
	if strings.Contains(err.Error(), dashID) {
		t.Fatalf("the error quotes the tool result: %v", err)
	}
}

func TestAgentDoesNotOfferMCPOnlyTools(t *testing.T) {
	registry := tools.New(tools.Deps{})
	provider := &scriptedProvider{sessions: []*scriptedSession{session(stopTurn(llm.StopRefusal))}}
	a := agent.New(agent.Config{
		Provider:    provider,
		Tools:       registry,
		Dashboards:  &fakeDashboards{number: 3},
		Builds:      &fakeBuilds{},
		Datasources: fakeHealth{},
		Logger:      slog.New(slog.DiscardHandler),
	})
	if _, err := a.Run(context.Background(), agent.Task{Prompt: "checkout health by pod", RequestID: requestID, Principal: principal}); !errors.Is(err, agent.ErrRefused) {
		t.Fatalf("err = %v", err)
	}
	offered := map[string]bool{}
	for _, spec := range provider.started()[0].Tools {
		offered[spec.Name] = true
		for _, tool := range registry.List() {
			if tool.MCPOnly && strings.Contains(spec.Description+string(spec.InputSchema), tool.Name) {
				t.Errorf("%s is offered with text that mentions %s", spec.Name, tool.Name)
			}
		}
	}
	mcpOnly := 0
	for _, tool := range registry.List() {
		if tool.MCPOnly {
			mcpOnly++
		}
		if tool.OfferedIn(tools.ModeCreate) != offered[tool.Name] {
			t.Errorf("%s: offered in create = %v, offered to the model = %v", tool.Name, tool.OfferedIn(tools.ModeCreate), offered[tool.Name])
		}
	}
	for _, name := range []string{"list_comments", "list_folders", "create_comment", "edit_comment", "reopen_comment", "reply_to_comment", "resolve_comment", "create_finding", "list_dashboard_health", "list_annotations", "create_annotation", "draft_query", "list_activity", "get_run", "search_workspace", "move_finding", "save_search", "list_saved_searches", "claim_run", "report_run", "finish_run", "create_observation", "get_observation", "list_observations", "add_block", "update_block", "resolve_observation", "keep_block", "discard_block", "publish_rca", "list_proposals", "get_proposal", "merge_proposal", "discard_proposal"} {
		if offered[name] {
			t.Errorf("%s is offered to the model", name)
		}
	}
	if !offered["get_spec_guide"] || !offered["save_finding"] || !offered["list_findings"] || mcpOnly != 33 || len(offered) != len(registry.List())-34 {
		t.Fatalf("mcp-only tools = %d, offered = %d of %d", mcpOnly, len(offered), len(registry.List()))
	}
}

func (r *rig) llmCallLogs() []map[string]any {
	r.t.Helper()
	var out []map[string]any
	for line := range strings.SplitSeq(r.logs.String(), "\n") {
		var entry map[string]any
		if json.Unmarshal([]byte(line), &entry) == nil && entry["msg"] == "llm call finished" {
			out = append(out, entry)
		}
	}
	return out
}

type attemptingSession struct {
	statuses []int
	turn     llm.Turn
	err      error
}

func (s *attemptingSession) Compact(llm.Compaction) error { return nil }

func (s *attemptingSession) Next(ctx context.Context, _ []llm.ToolResult) (llm.Turn, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://provider.invalid/v1", nil)
	if err != nil {
		return llm.Turn{}, err
	}
	for _, status := range s.statuses {
		resp, err := llm.TraceAttempt(req, func(*http.Request) (*http.Response, error) {
			header := http.Header{}
			if status == http.StatusTooManyRequests {
				header.Set("Retry-After", "2")
			}
			return &http.Response{StatusCode: status, Header: header, Body: http.NoBody}, nil
		})
		if err != nil {
			return llm.Turn{}, err
		}
		_ = resp.Body.Close()
	}
	return s.turn, s.err
}

func TestEveryModelCallLogsOneLineWithItsOutcome(t *testing.T) {
	r := newRig(t, session(
		toolTurn(call("c1", "get_spec_guide", `{"q":"UNIQUE-INPUT"}`)),
		toolTurn(call("c2", "create_dashboard", `{"spec":"UNIQUE-INPUT"}`)),
	))
	r.tools.onSave("create_dashboard", savedResult(dashID, 1))
	task := r.task()
	task.RequestID = requestID
	task.Prompt = "UNIQUE-PROMPT show me everything"

	if _, err := r.agent.Run(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	lines := r.llmCallLogs()
	if len(lines) != 2 {
		t.Fatalf("%d llm call lines, want one per model call: %s", len(lines), r.logs)
	}
	for _, line := range lines {
		want := map[string]any{
			"level": "INFO", "agent_request_id": requestID, "provider": "fake", "model": "fake-model",
			"attempts": float64(0), "retries": float64(0), "status": float64(0),
			"input_tokens": float64(100), "output_tokens": float64(10), "cache_read_tokens": float64(5), "cache_write_tokens": float64(2),
			"stop_reason": "tool_use",
		}
		for key, value := range want {
			if line[key] != value {
				t.Errorf("%s = %v, want %v", key, line[key], value)
			}
		}
		if _, ok := line["duration"]; !ok {
			t.Errorf("line has no duration: %v", line)
		}
		if line["failure_code"] != nil {
			t.Errorf("a successful call logged a failure code: %v", line)
		}
	}
	for _, secret := range []string{"UNIQUE-PROMPT", "UNIQUE-INPUT"} {
		if strings.Contains(r.logs.String(), secret) {
			t.Fatalf("log leaks %s: %s", secret, r.logs)
		}
	}
}

func TestFailedFirstModelCallLogsItsFailureAndKeepsUsage(t *testing.T) {
	callErr := llm.NewCallError("fake", llm.RateLimited, 429)
	callErr.Type, callErr.Code, callErr.Param, callErr.RequestID = "rate_limit_error", "rate_limited", "model", "req_abc"
	r := newRig(t, session(scriptedTurn{err: callErr}))
	task := r.task()
	task.RequestID = requestID
	task.Prompt = "UNIQUE-PROMPT show me everything"

	out, err := r.agent.Run(context.Background(), task)
	if err == nil {
		t.Fatal("expected a failure")
	}
	if out.FailedCalls != 1 || out.Usage.Calls != 0 {
		t.Fatalf("outcome = %+v, want one failed call and no counted call", out)
	}
	lines := r.llmCallLogs()
	if len(lines) != 1 {
		t.Fatalf("%d llm call lines, want 1: %s", len(lines), r.logs)
	}
	want := map[string]any{
		"level": "WARN", "agent_request_id": requestID, "failure_code": "rate_limited",
		"provider_error_type": "rate_limit_error", "provider_error_code": "rate_limited", "provider_error_param": "model", "request_id": "req_abc",
	}
	for key, value := range want {
		if lines[0][key] != value {
			t.Errorf("%s = %v, want %v", key, lines[0][key], value)
		}
	}
	for _, key := range []string{"input_tokens", "stop_reason"} {
		if lines[0][key] != nil {
			t.Errorf("a failed call logged %s", key)
		}
	}
	if strings.Contains(r.logs.String(), "UNIQUE-PROMPT") {
		t.Fatalf("log leaks the prompt: %s", r.logs)
	}

	result, err := agent.NewReviser(newRig(t, session(scriptedTurn{err: callErr})).agent, nil).Revise(context.Background(), revisionRequest())
	if err == nil || result.FailureCode != revisions.FailureRateLimited {
		t.Fatalf("result = %+v, err %v", result, err)
	}
	if result.Usage == nil || result.Usage.FailedCalls != 1 || result.Usage.ModelCalls != 0 {
		t.Fatalf("usage = %+v, want a non-null usage with one failed call", result.Usage)
	}
}

func TestProviderAttemptsAreCountedAsRetriesPerCall(t *testing.T) {
	r := newRig(t)
	provider := &sessionProvider{sessions: []llm.Session{
		&attemptingSession{statuses: []int{429, 503, 200}, turn: llm.Turn{ToolCalls: []llm.ToolCall{call("c1", "create_dashboard", `{}`)}, Stop: llm.StopToolUse, Usage: usage()}},
	}}
	r.tools.onSave("create_dashboard", savedResult(dashID, 1))
	ag := agent.New(agent.Config{Provider: provider, Tools: r.tools, Dashboards: r.dash, Builds: r.builds, Datasources: fakeHealth{}, Logger: slog.New(slog.NewJSONHandler(r.logs, nil))})

	out, err := ag.Run(context.Background(), r.task())
	if err != nil {
		t.Fatal(err)
	}
	if out.Retries != 2 {
		t.Fatalf("retries = %d, want 2", out.Retries)
	}
	lines := r.llmCallLogs()
	if len(lines) != 1 || lines[0]["attempts"] != float64(3) || lines[0]["retries"] != float64(2) || lines[0]["status"] != float64(200) {
		t.Fatalf("llm call lines = %v", lines)
	}
}

func TestRetryAfterOfAFailedCallIsLogged(t *testing.T) {
	r := newRig(t)
	provider := &sessionProvider{sessions: []llm.Session{
		&attemptingSession{statuses: []int{429, 429, 429}, err: llm.NewCallError("fake", llm.RateLimited, 429)},
	}}
	ag := agent.New(agent.Config{Provider: provider, Tools: r.tools, Dashboards: r.dash, Builds: r.builds, Datasources: fakeHealth{}, Logger: slog.New(slog.NewJSONHandler(r.logs, nil))})

	out, err := ag.Run(context.Background(), r.task())
	if err == nil || out.Retries != 2 || out.FailedCalls != 1 {
		t.Fatalf("outcome = %+v, err %v", out, err)
	}
	lines := r.llmCallLogs()
	if len(lines) != 1 || lines[0]["retry_after"] != "2" || lines[0]["status"] != float64(429) || lines[0]["failure_code"] != "rate_limited" {
		t.Fatalf("llm call lines = %v", lines)
	}
}

type sessionProvider struct {
	mu       sync.Mutex
	sessions []llm.Session
	started  int
}

func (p *sessionProvider) Name() string  { return "fake" }
func (p *sessionProvider) Model() string { return "fake-model" }

func (p *sessionProvider) Start(context.Context, llm.Request) (llm.Session, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.started++
	return p.sessions[p.started-1], nil
}

func TestOutcomeSummarisesToolWorkAndTheLargestSavingTurn(t *testing.T) {
	r := newRig(t, session(
		scriptedTurn{turn: llm.Turn{Stop: llm.StopToolUse, Usage: llm.Usage{OutputTokens: 4000, Calls: 1}, ToolCalls: []llm.ToolCall{
			call("c1", "get_spec_guide", `{}`), call("c2", "list_datasources", `{}`),
		}}},
		scriptedTurn{turn: llm.Turn{Stop: llm.StopToolUse, Usage: llm.Usage{OutputTokens: 700, Calls: 1}, ToolCalls: []llm.ToolCall{
			call("c3", "create_dashboard", `{}`),
		}}},
	))
	const readDuration = 150 * time.Millisecond
	slowRead := func(context.Context, json.RawMessage) (tools.Result, error) {
		time.Sleep(readDuration)
		return tools.Result{Content: map[string]any{"ok": true}, Step: "Read"}, nil
	}
	r.tools.on("get_spec_guide", slowRead)
	r.tools.on("list_datasources", slowRead)
	r.tools.onSave("create_dashboard", savedResult(dashID, 1))

	out, err := r.agent.Run(context.Background(), r.task())
	if err != nil {
		t.Fatal(err)
	}
	if out.ToolCalls != 3 || out.MaxSaveOutputTokens != 700 || out.FailedCalls != 0 || out.Retries != 0 {
		t.Fatalf("outcome = %+v, want 3 tool calls and a largest saving turn of 700 tokens", out)
	}
	if out.ToolTime < readDuration || out.ToolTime >= 2*readDuration {
		t.Fatalf("tool time = %s, want one wall-clock read of %s, not the sum of the parallel reads", out.ToolTime, readDuration)
	}
}

func TestOutcomeCountsATruncatedSavingTurn(t *testing.T) {
	truncated := scriptedTurn{turn: llm.Turn{
		Stop: llm.StopMaxTokens, Usage: llm.Usage{OutputTokens: 16000, Calls: 1}, ToolCalls: []llm.ToolCall{call("c1", "create_dashboard", `{}`)},
	}}
	r := newRig(t, session(truncated, truncated))
	out, err := r.agent.Run(context.Background(), r.task())
	if !errors.Is(err, agent.ErrNoSave) || !strings.HasSuffix(err.Error(), " after reaching max tokens") {
		t.Fatalf("err = %v", err)
	}
	if out.MaxSaveOutputTokens != 16000 {
		t.Fatalf("max save output tokens = %d, want 16000", out.MaxSaveOutputTokens)
	}
}

func TestOutcomeTotalsSurviveAConflictRestart(t *testing.T) {
	r := newRig(t,
		session(scriptedTurn{turn: llm.Turn{Stop: llm.StopToolUse, Usage: llm.Usage{OutputTokens: 300, Calls: 1}, ToolCalls: []llm.ToolCall{call("c1", "update_dashboard", `{}`)}}}),
		session(scriptedTurn{turn: llm.Turn{Stop: llm.StopToolUse, Usage: llm.Usage{OutputTokens: 200, Calls: 1}, ToolCalls: []llm.ToolCall{call("c2", "update_dashboard", `{}`)}}}),
	)
	r.tools.onSave("update_dashboard", conflictResult(4), savedResult(dashID, 5))

	out, err := r.agent.Run(context.Background(), r.reviseTask())
	if err != nil {
		t.Fatal(err)
	}
	if out.ToolCalls != 2 || out.MaxSaveOutputTokens != 300 || out.Usage.Calls != 2 {
		t.Fatalf("outcome = %+v, want both sessions counted", out)
	}
}

func TestFailureCodesOfToolFailuresAreInTheClosedSet(t *testing.T) {
	for _, code := range []string{tools.FailureToolFailed, tools.FailureResultTooLarge, tools.FailurePanicked} {
		if !slices.Contains([]revisions.FailureCode{revisions.FailureToolFailed, revisions.FailureResultTooLarge, revisions.FailureToolPanicked}, revisions.FailureCode(code)) {
			t.Errorf("tool failure code %q is not a revisions.FailureCode", code)
		}
	}
	for failure, code := range map[llm.CallFailure]revisions.FailureCode{
		llm.CallTimeout: revisions.FailureCallTimeout, llm.RateLimited: revisions.FailureRateLimited, llm.ProviderError: revisions.FailureProviderError,
	} {
		if string(failure) != string(code) {
			t.Errorf("llm failure %q does not match failure code %q", failure, code)
		}
	}
}

type fakeBuilds struct {
	mu         sync.Mutex
	build      *dashboards.Build
	commitErr  []error
	commits    int
	discards   int
	attempts   []int
	rebaseOnto int
	rebases    int
}

func (f *fakeBuilds) Build(context.Context, pgtype.UUID) (dashboards.Build, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.build == nil {
		return dashboards.Build{}, false, nil
	}
	return *f.build, true, nil
}

func (f *fakeBuilds) CommitBuild(_ context.Context, _ auth.Principal, _ pgtype.UUID, attempt int) (dashboards.Dashboard, dashboards.Version, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.attempts = append(f.attempts, attempt)
	if len(f.commitErr) > 0 {
		err := f.commitErr[0]
		f.commitErr = f.commitErr[1:]
		if err != nil {
			return dashboards.Dashboard{}, dashboards.Version{}, false, err
		}
	}
	f.commits++
	f.build = nil
	return dashboards.Dashboard{ID: dashID}, dashboards.Version{VersionMeta: dashboards.VersionMeta{Number: 1}}, false, nil
}

func (f *fakeBuilds) DiscardBuild(_ context.Context, _ pgtype.UUID, attempt int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.discards++
	f.attempts = append(f.attempts, attempt)
	f.build = nil
	return nil
}

func (f *fakeBuilds) RebaseBuild(context.Context, auth.Principal, pgtype.UUID, int) (int, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rebases++
	if f.rebaseOnto == 0 || f.build == nil {
		return 0, false, nil
	}
	f.build.BaseVersion = f.rebaseOnto
	return f.rebaseOnto, true, nil
}

func (f *fakeBuilds) current() *dashboards.Build {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.build
}

func (f *fakeBuilds) seed(b dashboards.Build) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.build = &b
}

func buildSpec(t *testing.T, rowIDs ...string) *spec.Dashboard {
	t.Helper()
	rows := make([]string, len(rowIDs))
	for i, id := range rowIDs {
		rows[i] = fmt.Sprintf(`{"id":%q,"type":"row","title":"Row %s","gridPos":{"x":0,"y":%d,"w":24,"h":1},"panels":[{"id":"%s_p","type":"stat","title":"p","gridPos":{"x":0,"y":%d,"w":6,"h":4},"datasource":"prom","queries":[{"query":"up"}]}]}`, id, id, i*5, id, i*5+1)
	}
	raw := `{"specVersion":"0","title":"t","time":{"from":"now-15m","to":"now"},"panels":[` + strings.Join(rows, ",") + `]}`
	d, err := spec.Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func planOf(ids ...string) int {
	return len(ids)
}

func (f *fakeBuilds) saveRows(t *testing.T, planned int, built ...string) tools.Result {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	saves := 1
	if f.build != nil {
		saves = f.build.Saves + 1
	}
	f.build = &dashboards.Build{Spec: buildSpec(t, built...), PlannedPanels: planned, Saves: saves, BaseVersion: latestVersion}
	return tools.Result{
		Content: map[string]any{
			"build":   map[string]any{"saves": saves},
			"message": "saved",
		},
		Step:  fmt.Sprintf("Saved %d rows to the build", len(built)),
		Edits: tools.Edits{Panels: []string{fmt.Sprint("p", saves)}},
	}
}

func sequence(fns ...func() tools.Result) handler {
	var mu sync.Mutex
	next := 0
	return func(context.Context, json.RawMessage) (tools.Result, error) {
		mu.Lock()
		defer mu.Unlock()
		if next >= len(fns) {
			return tools.Result{}, errors.New("test script exhausted: unexpected save")
		}
		fn := fns[next]
		next++
		return fn(), nil
	}
}

func (r *rig) buildSaves(t *testing.T, fns ...func() tools.Result) {
	t.Helper()
	seq := sequence(fns...)
	r.tools.on("create_dashboard", seq)
	r.tools.on("update_panels", seq)
}

func (r *rig) rows(t *testing.T, planned int, built ...string) func() tools.Result {
	return func() tools.Result { return r.builds.saveRows(t, planned, built...) }
}

func saved(version int) func() tools.Result {
	return func() tools.Result { return savedResult(dashID, version) }
}

func rejected() func() tools.Result {
	return func() tools.Result { return invalidSpecResult("bad_query") }
}

func buildSavedResult(saves int) tools.Result {
	return tools.Result{
		Content: map[string]any{"build": map[string]any{"saves": saves}, "message": "saved"},
		Step:    fmt.Sprintf("Saved panel %d", saves),
	}
}

func (r *rig) savesUntilVersion(versionAt int) {
	var mu sync.Mutex
	saves := 0
	save := func(context.Context, json.RawMessage) (tools.Result, error) {
		mu.Lock()
		defer mu.Unlock()
		saves++
		if saves == versionAt {
			return savedResult(dashID, 1), nil
		}
		res := buildSavedResult(saves)
		res.Edits = tools.Edits{Panels: []string{fmt.Sprint("p", saves)}}
		return res, nil
	}
	r.tools.on("create_dashboard", save)
	r.tools.on("update_panels", save)
}

func TestThreeBuildSavesWithFinishOnTheThirdMakeOneVersion(t *testing.T) {
	plan := planOf("r1", "r2", "r3")
	s := session(
		toolTurn(call("c1", "create_dashboard", `{"n":1}`)),
		toolTurn(call("c2", "update_panels", `{"n":2}`)),
		toolTurn(call("c3", "update_panels", `{"n":3,"finish":true}`)),
	)
	r := newRig(t, s)
	r.buildSaves(t, r.rows(t, plan, "r1"), r.rows(t, plan, "r1", "r2"), saved(1))

	out, err := r.agent.Run(context.Background(), r.task())
	if err != nil {
		t.Fatal(err)
	}
	if out.Version != 1 || out.DashboardID != dashID || out.BuildSaves != 3 || s.modelCalls() != 3 {
		t.Fatalf("outcome = %+v, model calls %d", out, s.modelCalls())
	}
	if r.builds.commits != 0 {
		t.Fatal("the loop committed a build the tool had already committed")
	}
	steps := r.recordedSteps()
	if got := steps[0].Message; got != "Saved 1 rows to the build" {
		t.Fatalf("first step = %q", got)
	}
}

func TestTwoBuildSavesThenNoToolCallCommitOnce(t *testing.T) {
	plan := planOf("r1", "r2")
	s := session(
		toolTurn(call("c1", "create_dashboard", `{"n":1}`)),
		toolTurn(call("c2", "update_panels", `{"n":2}`)),
		stopTurn(llm.StopEndTurn),
	)
	r := newRig(t, s)
	r.buildSaves(t, r.rows(t, plan, "r1"), r.rows(t, plan, "r1", "r2"))

	out, err := r.agent.Run(context.Background(), r.task())
	if err != nil {
		t.Fatal(err)
	}
	if out.Version != 1 || out.DashboardID != dashID || out.BuildSaves != 2 || r.builds.commits != 1 || !slices.Equal(r.builds.attempts, []int{1}) {
		t.Fatalf("outcome = %+v, commits %d, attempts %v", out, r.builds.commits, r.builds.attempts)
	}
	if got := len(r.provider.started()); got != 1 {
		t.Fatalf("sessions = %d", got)
	}
}

func TestNoToolCallWithFewerPanelsThanPlannedCompactsOnceThenCommits(t *testing.T) {
	plan := planOf("r1", "r2", "r3")
	s := session(toolTurn(call("c1", "create_dashboard", `{"n":1}`)), stopTurn(llm.StopEndTurn), stopTurn(llm.StopEndTurn))
	r := newRig(t, s)
	r.buildSaves(t, r.rows(t, plan, "r1"))

	out, err := r.agent.Run(context.Background(), r.task())
	if err != nil {
		t.Fatal(err)
	}
	if out.Version != 1 || out.Compactions != 1 || out.Renewals != 0 || out.BuildSaves != 1 || r.builds.commits != 1 {
		t.Fatalf("outcome = %+v, commits %d", out, r.builds.commits)
	}
	if got := len(r.provider.started()); got != 1 {
		t.Fatalf("sessions = %d, want the same session continued", got)
	}
	if len(s.compactions) != 1 {
		t.Fatalf("compactions = %+v", s.compactions)
	}
	message := s.compactions[0].Message
	if !strings.Contains(message, "Why your conversation was shortened: You answered without a tool call, but the build has fewer panels than you planned.") ||
		!strings.Contains(message, "Panels built: 1 of 3.") {
		t.Fatalf("compaction message = %s", message)
	}
	if s.modelCalls() != 3 || len(s.results(2)) != 0 {
		t.Fatalf("model calls = %d, results after the compaction = %+v", s.modelCalls(), s.results(2))
	}
}

func TestNoToolCallWithFewerPanelsThanPlannedIsNudgedOnlyOncePerRun(t *testing.T) {
	plan := planOf("r1", "r2", "r3")
	s := session(toolTurn(call("c1", "create_dashboard", `{"n":1}`)), stopTurn(llm.StopEndTurn), stopTurn(llm.StopEndTurn), stopTurn(llm.StopEndTurn))
	r := newRig(t, s)
	r.buildSaves(t, r.rows(t, plan, "r1"))

	out, err := r.agent.Run(context.Background(), r.task())
	if err != nil || out.Version != 1 || len(s.compactions) != 1 || s.modelCalls() != 3 {
		t.Fatalf("outcome = %+v, err %v, compactions %d, model calls %d", out, err, len(s.compactions), s.modelCalls())
	}
}

func TestMaxTokensOnceCompactsWithTheOutputLimitNoteAndTwiceInARowEndsTheRun(t *testing.T) {
	const sentence = "Why your conversation was shortened: Your last answer was cut off at the output limit. Save one panel per call, and check only what that panel needs."
	t.Run("recovers", func(t *testing.T) {
		s := session(stopTurn(llm.StopMaxTokens), toolTurn(call("c1", "create_dashboard", `{}`)))
		r := newRig(t, s)
		r.tools.onSave("create_dashboard", savedResult(dashID, 1))
		out, err := r.agent.Run(context.Background(), r.task())
		if err != nil || out.Version != 1 {
			t.Fatalf("outcome = %+v, err %v", out, err)
		}
		started := r.provider.started()
		if len(started) != 1 || strings.Contains(started[0].Context, "output limit") {
			t.Fatalf("contexts = %+v", started)
		}
		if len(s.compactions) != 1 || !strings.HasSuffix(s.compactions[0].Message, sentence+"\n\n<request>\n"+r.task().Prompt+"\n</request>") {
			t.Fatalf("compactions = %+v", s.compactions)
		}
	})
	t.Run("a second cut off in a row ends it", func(t *testing.T) {
		s := session(stopTurn(llm.StopMaxTokens), stopTurn(llm.StopMaxTokens))
		r := newRig(t, s)
		out, err := r.agent.Run(context.Background(), r.task())
		if !strings.Contains(fmt.Sprint(err), "after reaching max tokens") || len(s.compactions) != 1 {
			t.Fatalf("outcome = %+v, err %v, compactions %d", out, err, len(s.compactions))
		}
	})
	t.Run("progress in between resets the count", func(t *testing.T) {
		plan := planOf("r1", "r2")
		s := session(
			stopTurn(llm.StopMaxTokens),
			toolTurn(call("c1", "create_dashboard", `{"n":1}`)), stopTurn(llm.StopMaxTokens),
			toolTurn(call("c2", "update_panels", `{"n":2,"finish":true}`)),
		)
		r := newRig(t, s)
		r.buildSaves(t, r.rows(t, plan, "r1"), saved(1))
		out, err := r.agent.Run(context.Background(), r.task())
		if err != nil || out.Version != 1 || len(s.compactions) != 2 {
			t.Fatalf("outcome = %+v, err %v, compactions %d", out, err, len(s.compactions))
		}
	})
}

func TestRejectedSavesCountInARowAndABuildSaveResetsTheCount(t *testing.T) {
	plan := planOf("r1")
	turns := make([]scriptedTurn, 6)
	for i := range turns {
		turns[i] = toolTurn(call(fmt.Sprint("c", i), "update_panels", fmt.Sprintf(`{"n":%d}`, i)))
	}
	r := newRig(t, session(turns...))
	r.buildSaves(t, rejected(), rejected(), r.rows(t, plan, "r1"), rejected(), rejected(), saved(1))

	out, err := r.agent.Run(context.Background(), r.task())
	if err != nil || out.Version != 1 || out.RepairRounds != 4 {
		t.Fatalf("outcome = %+v, err %v", out, err)
	}
}

func TestThreeRejectedSavesInARowEndAnUnfinishedBuild(t *testing.T) {
	plan := planOf("r1", "r2")
	turns := make([]scriptedTurn, 4)
	for i := range turns {
		turns[i] = toolTurn(call(fmt.Sprint("c", i), "update_panels", fmt.Sprintf(`{"n":%d}`, i)))
	}
	r := newRig(t, session(turns...))
	r.buildSaves(t, r.rows(t, plan, "r1"), rejected(), rejected(), rejected())

	_, err := r.agent.Run(context.Background(), r.task())
	if !errors.Is(err, agent.ErrRepairExhausted) {
		t.Fatalf("err = %v", err)
	}
	if r.builds.current() == nil || r.builds.commits != 0 {
		t.Fatal("a failed run must keep its build and commit nothing")
	}
}

func TestARepeatedSaveIsRefusedAndTheRunGoesOn(t *testing.T) {
	plan := planOf("r1", "r2")
	s := session(
		toolTurn(call("c1", "update_panels", `{"upsert":[{"id":"x"}]}`)),
		toolTurn(call("c2", "update_panels", "{ \"upsert\": [ {\"id\": \"x\"} ] }")),
		toolTurn(call("c3", "update_panels", `{"upsert":[{"id":"y"}],"finish":true}`)),
	)
	r := newRig(t, s)
	r.buildSaves(t, r.rows(t, plan, "r1"), saved(1))

	out, err := r.agent.Run(context.Background(), r.task())
	if err != nil || out.Version != 1 {
		t.Fatalf("outcome = %+v, err %v", out, err)
	}
	if got := r.tools.names(); len(got) != 2 {
		t.Fatalf("tool calls = %v, the repeated save must not run", got)
	}
	refused := s.results(2)
	if len(refused) != 1 || !refused[0].IsError || !strings.Contains(refused[0].Content, "this save is identical to an earlier one and changes nothing") {
		t.Fatalf("result of the repeated save = %+v", refused)
	}
	step := r.recordedSteps()[1]
	if step.Status != "error" || step.Message != "Refused update_panels: the same save was sent again" {
		t.Fatalf("refusal step = %+v", step)
	}
	if out.RepairRounds != 0 {
		t.Fatalf("a refused repeated save counted as a repair: %+v", out)
	}
}

func TestTheSecondIdenticalReadIsReplayedWithANoteAndTheThirdIsRefused(t *testing.T) {
	plan := planOf("r1")
	s := session(
		toolTurn(call("c1", "get_spec_guide", `{"a":1}`)),
		toolTurn(call("c2", "get_spec_guide", `{ "a": 1 }`)),
		toolTurn(call("c3", "get_spec_guide", `{"a":1}`)),
		toolTurn(call("c4", "create_dashboard", `{"n":1}`)),
		stopTurn(llm.StopEndTurn),
	)
	r := newRig(t, s)
	r.tools.on("get_spec_guide", func(context.Context, json.RawMessage) (tools.Result, error) {
		return tools.Result{Content: map[string]any{"guide": "GUIDE"}, Step: "Read the guide"}, nil
	})
	r.buildSaves(t, r.rows(t, plan, "r1"))

	out, err := r.agent.Run(context.Background(), r.task())
	if err != nil || out.Version != 1 {
		t.Fatalf("outcome = %+v, err %v", out, err)
	}
	if got := r.tools.names(); len(got) != 2 {
		t.Fatalf("tool calls = %v, only the first read and the save may run", got)
	}
	second := s.results(2)
	wantSecond := `{"guide":"GUIDE"}` + "\n\n<note>This is the result of the same earlier call. A third identical call is refused.</note>"
	if len(second) != 1 || second[0].Content != wantSecond || second[0].IsError {
		t.Fatalf("replayed answer = %+v", second)
	}
	third := s.results(3)
	if len(third) != 1 || !third[0].IsError || !strings.Contains(third[0].Content, "this exact call already ran 2 times; use its earlier result, change its input, or save") {
		t.Fatalf("third answer = %+v", third)
	}
	steps := r.recordedSteps()
	if steps[1].Message != "Reused the result of an identical call" || steps[1].Status != "ok" || steps[1].Tool != "get_spec_guide" {
		t.Fatalf("replay step = %+v", steps[1])
	}
	if steps[2].Status != "error" || steps[2].Message != "Refused get_spec_guide: the same read was asked 3 times" {
		t.Fatalf("refusal step = %+v", steps[2])
	}
}

func TestAFailedReadIsRetriedLive(t *testing.T) {
	s := session(
		toolTurn(call("c1", "list_datasources", `{}`)),
		toolTurn(call("c2", "list_datasources", `{}`)),
		toolTurn(call("c3", "create_dashboard", `{"n":1}`)),
		stopTurn(llm.StopEndTurn),
	)
	r := newRig(t, s)
	var mu sync.Mutex
	reads := 0
	r.tools.on("list_datasources", func(context.Context, json.RawMessage) (tools.Result, error) {
		mu.Lock()
		defer mu.Unlock()
		reads++
		if reads == 1 {
			return tools.Result{IsError: true, Content: map[string]any{"code": "datasource_unavailable"}}, nil
		}
		return tools.Result{Content: map[string]any{"datasources": []string{"prom"}}}, nil
	})
	r.buildSaves(t, r.rows(t, 1, "r1"))

	if _, err := r.agent.Run(context.Background(), r.task()); err != nil {
		t.Fatal(err)
	}
	if got := r.tools.names(); len(got) != 3 {
		t.Fatalf("tool calls = %v, both reads must run", got)
	}
	second := s.results(2)
	if len(second) != 1 || second[0].IsError || strings.Contains(second[0].Content, "<note>") {
		t.Fatalf("second answer = %+v", second)
	}
}

func TestThreeIdenticalReadTurnsThenAnotherEndWithNoProgress(t *testing.T) {
	turns := make([]scriptedTurn, 4)
	for i := range turns {
		turns[i] = toolTurn(call(fmt.Sprint("c", i), "get_spec_guide", `{"a":1}`))
	}
	s := session(turns...)
	r := newRig(t, s)

	_, err := r.agent.Run(context.Background(), r.task())
	if !errors.Is(err, agent.ErrNoProgress) {
		t.Fatalf("err = %v", err)
	}
	if s.modelCalls() != 4 || len(r.tools.names()) != 1 {
		t.Fatalf("model calls = %d, tools ran = %v", s.modelCalls(), r.tools.names())
	}
	steps := r.recordedSteps()
	if stop := steps[len(steps)-1]; stop.Status != "error" || stop.Message != "Stopped: the same read was asked 3 times" {
		t.Fatalf("stop step = %+v", stop)
	}
	if code := r.finishLog()["error_code"]; code != "no_progress" {
		t.Fatalf("error_code = %v", code)
	}
}

func TestTwoTurnsOfRepeatedSavesEndWithNoProgressAndSayWhy(t *testing.T) {
	s := session(
		toolTurn(call("c1", "update_panels", `{"n":1}`)),
		toolTurn(call("c2", "update_panels", `{"n":1}`)),
		toolTurn(call("c3", "update_panels", `{"n":1}`)),
	)
	r := newRig(t, s)
	r.buildSaves(t, r.rows(t, planOf("r1", "r2"), "r1"))

	_, err := r.agent.Run(context.Background(), r.task())
	if !errors.Is(err, agent.ErrNoProgress) {
		t.Fatalf("err = %v", err)
	}
	steps := r.recordedSteps()
	if stop := steps[len(steps)-1]; stop.Message != "Stopped: the same save was sent again and again" {
		t.Fatalf("stop step = %+v", stop)
	}
}

func TestRepeatedReadsInOneTurnCountOnce(t *testing.T) {
	s := session(
		toolTurn(call("1", "query_range", `{}`), call("2", "query_range", `{}`), call("3", "query_range", `{}`)),
		toolTurn(call("4", "create_dashboard", `{}`)),
	)
	r := newRig(t, s)
	r.tools.onSave("create_dashboard", savedResult(dashID, 1))
	if _, err := r.agent.Run(context.Background(), r.task()); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(r.tools.names(), ","); got != "query_range,create_dashboard" {
		t.Fatalf("tool calls = %s", got)
	}
	results := s.results(1)
	if len(results) != 3 || results[1] != (llm.ToolResult{CallID: "2", Content: results[0].Content, Prunable: true}) || results[2] != (llm.ToolResult{CallID: "3", Content: results[0].Content, Prunable: true}) {
		t.Fatalf("results = %+v", results)
	}
}

func TestABuildSaveMakesAnIdenticalReadRunAgain(t *testing.T) {
	plan := planOf("r1", "r2")
	r := newRig(t, session(
		toolTurn(call("1", "get_panels", `{"ids":["a"]}`)),
		toolTurn(call("2", "create_dashboard", `{"n":1}`)),
		toolTurn(call("3", "get_panels", `{"ids":["a"]}`)),
		toolTurn(call("4", "update_panels", `{"n":2,"finish":true}`)),
	))
	r.buildSaves(t, r.rows(t, plan, "r1"), saved(1))
	if _, err := r.agent.Run(context.Background(), r.task()); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(r.tools.names(), ","); got != "get_panels,create_dashboard,get_panels,update_panels" {
		t.Fatalf("calls = %s", got)
	}
}

func TestACommitConflictDiscardsTheBuildAndRestartsOnce(t *testing.T) {
	r := newRig(t,
		session(stopTurn(llm.StopEndTurn)),
		session(toolTurn(call("c1", "update_panels", `{}`))),
	)
	r.builds.seed(agentBuild(t, "r1"))
	r.builds.commitErr = []error{&dashboards.ConflictError{Latest: 4}}
	r.tools.onSave("update_panels", savedResult(dashID, 5))

	out, err := r.agent.Run(context.Background(), r.reviseTask())
	if err != nil || out.Version != 5 {
		t.Fatalf("outcome = %+v, err %v", out, err)
	}
	if r.builds.discards != 1 || r.builds.current() != nil {
		t.Fatalf("discards = %d", r.builds.discards)
	}
	started := r.provider.started()
	if len(started) != 2 || !strings.Contains(started[1].Context, "Someone saved a new version while you worked; your build was discarded. Make the change again on the dashboard above.") ||
		strings.Contains(started[1].Context, "<build>") {
		t.Fatalf("restart request = %+v", started)
	}
}

func hasStep(steps []agent.Step, message string) bool {
	return slices.ContainsFunc(steps, func(s agent.Step) bool { return s.Message == message && s.Status == "ok" })
}

func TestACommitConflictOnADisjointChangeRebasesAndCommitsWithoutANewSession(t *testing.T) {
	r := newRig(t, session(stopTurn(llm.StopEndTurn)))
	r.builds.seed(agentBuild(t, "r1"))
	r.builds.commitErr = []error{&dashboards.ConflictError{Latest: 4}}
	r.builds.rebaseOnto = 4

	out, err := r.agent.Run(context.Background(), r.reviseTask())
	if err != nil || out.Version != 1 {
		t.Fatalf("outcome = %+v, err %v", out, err)
	}
	if r.builds.commits != 1 || r.builds.discards != 0 || len(r.provider.started()) != 1 {
		t.Fatalf("commits %d, discards %d, sessions %d, want one commit on the first session", r.builds.commits, r.builds.discards, len(r.provider.started()))
	}
	if !hasStep(r.recordedSteps(), "Rebased onto version 4") {
		t.Fatalf("steps = %+v, want the rebase step", r.recordedSteps())
	}
}

func TestAFinishSaveConflictOnADisjointChangeRebasesAndCommits(t *testing.T) {
	r := newRig(t, session(toolTurn(call("c1", "update_panels", `{"finish":true}`))))
	r.builds.seed(agentBuild(t, "r1"))
	r.builds.rebaseOnto = 4
	r.tools.onSave("update_panels", conflictResult(4))

	out, err := r.agent.Run(context.Background(), r.reviseTask())
	if err != nil || out.Version != 1 {
		t.Fatalf("outcome = %+v, err %v", out, err)
	}
	if r.builds.commits != 1 || r.builds.discards != 0 || len(r.provider.started()) != 1 || r.provider.sessions[0].modelCalls() != 1 {
		t.Fatalf("commits %d, discards %d, sessions %d, model calls %d", r.builds.commits, r.builds.discards, len(r.provider.started()), r.provider.sessions[0].modelCalls())
	}
}

func TestARebasedCommitThatConflictsAgainRestartsFromTheLatest(t *testing.T) {
	r := newRig(t,
		session(stopTurn(llm.StopEndTurn)),
		session(toolTurn(call("c1", "update_panels", `{}`))),
	)
	r.builds.seed(agentBuild(t, "r1"))
	r.builds.commitErr = []error{&dashboards.ConflictError{Latest: 4}, &dashboards.ConflictError{Latest: 5}}
	r.builds.rebaseOnto = 4
	r.tools.onSave("update_panels", savedResult(dashID, 6))

	out, err := r.agent.Run(context.Background(), r.reviseTask())
	if err != nil || out.Version != 6 {
		t.Fatalf("outcome = %+v, err %v", out, err)
	}
	started := r.provider.started()
	if r.builds.discards != 1 || len(started) != 2 || !strings.Contains(started[1].Context, conflictSeedSentence) {
		t.Fatalf("discards %d, sessions %d, want one restart with the conflict seed", r.builds.discards, len(started))
	}
}

func TestASecondConflictEndsTheRunAndDiscardsTheBuildAgain(t *testing.T) {
	r := newRig(t,
		session(stopTurn(llm.StopEndTurn)),
		session(toolTurn(call("c1", "update_panels", `{}`))),
	)
	r.builds.seed(agentBuild(t, "r1"))
	r.builds.commitErr = []error{&dashboards.ConflictError{Latest: 4}}
	r.tools.onSave("update_panels", conflictResult(5))

	_, err := r.agent.Run(context.Background(), r.reviseTask())
	if !errors.Is(err, agent.ErrConflict) || r.builds.discards != 2 {
		t.Fatalf("err = %v, discards %d", err, r.builds.discards)
	}
	steps := r.recordedSteps()
	if stop := steps[len(steps)-1]; stop.Status != "error" || stop.Message != "Stopped: the dashboard changed twice while the agent was revising it" {
		t.Fatalf("stop step = %+v", stop)
	}
}

func TestARepeatedSaveAfterAConflictRestartIsNotNoProgress(t *testing.T) {
	r := newRig(t,
		session(toolTurn(call("c1", "update_panels", `{"upsert":[]}`))),
		session(toolTurn(call("c2", "update_panels", `{"upsert":[]}`))),
	)
	r.tools.onSave("update_panels", conflictResult(5), savedResult(dashID, 6))

	out, err := r.agent.Run(context.Background(), r.reviseTask())
	if err != nil || out.Version != 6 {
		t.Fatalf("outcome = %+v, err %v", out, err)
	}
}

func agentBuild(t *testing.T, planned ...string) dashboards.Build {
	t.Helper()
	return dashboards.Build{Spec: buildSpec(t, "r1"), PlannedPanels: planOf(planned...), Saves: 4, BaseVersion: latestVersion}
}

func TestABuildPresentAtTheStartSeedsARestartAndTheProviderSeesItsSaves(t *testing.T) {
	r := newRig(t, session(toolTurn(call("c1", "update_panels", `{}`))))
	r.builds.seed(agentBuild(t, "r1", "r2"))
	r.tools.onSave("update_panels", savedResult(dashID, 1))

	if out, err := r.agent.Run(context.Background(), r.task()); err != nil || out.Version != 1 {
		t.Fatalf("outcome = %+v, err %v", out, err)
	}
	started := r.provider.started()
	if len(started) != 1 || started[0].BuildSaves != 4 {
		t.Fatalf("requests = %+v", started)
	}
	for _, want := range []string{
		"This is your build of the new dashboard, not saved as a version yet.",
		"Panels built: 1 of 2.",
		"Why this is a new session: The server restarted while you worked.",
	} {
		if !strings.Contains(started[0].Context, want) {
			t.Fatalf("context lacks %q:\n%s", want, started[0].Context)
		}
	}
}

func TestAHealthyRunPastFourHundredModelCallsFinishes(t *testing.T) {
	turns := make([]scriptedTurn, 406)
	for i := range turns {
		turns[i] = toolTurn(call(fmt.Sprint("c", i), "update_panels", fmt.Sprintf(`{"n":%d}`, i)))
	}
	r := newRig(t, session(turns...))
	r.savesUntilVersion(406)

	out, err := r.agent.Run(context.Background(), r.task())
	if err != nil || out.Version != 1 || out.Usage.Calls != 406 || out.BuildSaves != 406 {
		t.Fatalf("outcome = %+v, err %v", out, err)
	}
}

func changedSave(saves int, edits tools.Edits) func() tools.Result {
	return func() tools.Result {
		return tools.Result{
			Content: map[string]any{"build": map[string]any{"saves": saves}},
			Step:    fmt.Sprintf("Saved panel %d", saves),
			Edits:   edits,
		}
	}
}

func TestEndlessEditsOfOnePanelEndWithNoProgress(t *testing.T) {
	turns := make([]scriptedTurn, 9)
	fns := make([]func() tools.Result, 9)
	for i := range turns {
		turns[i] = toolTurn(call(fmt.Sprint("c", i), "update_panels", fmt.Sprintf(`{"n":%d}`, i)))
		fns[i] = changedSave(i+1, tools.Edits{Panels: []string{"p1"}})
	}
	s := session(turns...)
	r := newRig(t, s)
	r.buildSaves(t, fns...)

	out, err := r.agent.Run(context.Background(), r.task())
	if !errors.Is(err, agent.ErrNoProgress) || out.BuildSaves != 8 || s.modelCalls() != 8 {
		t.Fatalf("outcome = %+v, err %v, model calls %d", out, err, s.modelCalls())
	}
	for save := 1; save <= 7; save++ {
		got := s.results(save)[0].Content
		if churned := strings.Contains(got, "<note>This save changed no panel that still needs work. Leave p1 as it is"); churned != (save >= 5) {
			t.Fatalf("save %d: churn note = %v in %s", save, churned, got)
		}
	}
	steps := r.recordedSteps()
	if stop := steps[len(steps)-1]; stop.Status != "error" || stop.Message != "Stopped: 4 saves in a row changed nothing new" {
		t.Fatalf("stop step = %+v", stop)
	}
	if code := r.finishLog()["error_code"]; code != "no_progress" {
		t.Fatalf("error_code = %v", code)
	}
}

func TestEndlessPanelSwapsEndWithNoProgress(t *testing.T) {
	turns := make([]scriptedTurn, 9)
	fns := make([]func() tools.Result, 9)
	for i := range turns {
		turns[i] = toolTurn(call(fmt.Sprint("c", i), "update_panels", fmt.Sprintf(`{"n":%d}`, i)))
		added := fmt.Sprint("p", i)
		fns[i] = changedSave(i+1, tools.Edits{Panels: []string{added}, Added: []string{added}, Deleted: []string{fmt.Sprint("p", i-1)}})
	}
	s := session(turns...)
	r := newRig(t, s)
	r.buildSaves(t, fns...)

	out, err := r.agent.Run(context.Background(), r.task())
	if !errors.Is(err, agent.ErrNoProgress) || out.BuildSaves != 4 || s.modelCalls() != 4 {
		t.Fatalf("outcome = %+v, err %v, model calls %d", out, err, s.modelCalls())
	}
	if got := s.results(1)[0].Content; !strings.Contains(got, "<note>This save deleted a panel and added another, which is not progress.") {
		t.Fatalf("first swap result = %s", got)
	}
	steps := r.recordedSteps()
	if stop := steps[len(steps)-1]; stop.Message != "Stopped: 4 saves in a row changed nothing new" {
		t.Fatalf("stop step = %+v", stop)
	}
}

func TestASaveThatChangesNothingCarriesTheChurnNoteAndASettingsSaveDoesNot(t *testing.T) {
	s := session(
		toolTurn(call("c1", "update_panels", `{"n":1}`)),
		toolTurn(call("c2", "update_panels", `{"dashboard":{"title":"x"}}`)),
		toolTurn(call("c3", "update_panels", `{"upsert":[{"id":"p1"}]}`)),
		toolTurn(call("c4", "update_panels", `{"upsert":[{"id":"p2"}],"finish":true}`)),
	)
	r := newRig(t, s)
	r.buildSaves(t, changedSave(1, tools.Edits{}), changedSave(2, tools.Edits{Settings: true}), changedSave(3, tools.Edits{Panels: []string{"p1"}}), saved(1))

	out, err := r.agent.Run(context.Background(), r.task())
	if err != nil || out.Version != 1 {
		t.Fatalf("outcome = %+v, err %v", out, err)
	}
	if got := s.results(1)[0].Content; !strings.HasSuffix(got, "Leave the build as it is, save the next panel, or set finish: true when every planned panel is built.</note>") {
		t.Fatalf("no-op save result = %s", got)
	}
	for save := 2; save <= 3; save++ {
		if got := s.results(save)[0].Content; strings.Contains(got, "<note>") {
			t.Fatalf("save %d that changed something carries a note: %s", save, got)
		}
	}
}

func TestSixSavesEachChangingADifferentPanelAreAllAccepted(t *testing.T) {
	turns := make([]scriptedTurn, 7)
	fns := make([]func() tools.Result, 7)
	for i := range 6 {
		turns[i] = toolTurn(call(fmt.Sprint("c", i), "update_panels", fmt.Sprintf(`{"n":%d}`, i)))
		fns[i] = changedSave(i+1, tools.Edits{Panels: []string{fmt.Sprint("p", i)}})
	}
	turns[6] = toolTurn(call("c6", "update_panels", `{"finish":true}`))
	fns[6] = saved(1)
	s := session(turns...)
	r := newRig(t, s)
	r.buildSaves(t, fns...)

	out, err := r.agent.Run(context.Background(), r.task())
	if err != nil || out.Version != 1 || out.BuildSaves != 7 {
		t.Fatalf("outcome = %+v, err %v", out, err)
	}
	for save := 1; save <= 6; save++ {
		if got := s.results(save)[0].Content; strings.Contains(got, "<note>") {
			t.Fatalf("save %d carries a churn note: %s", save, got)
		}
	}
}

func TestMoreThanSixCompactionsDoNotEndTheRunWhenEachFollowsAnAcceptedSave(t *testing.T) {
	var turns []scriptedTurn
	fns := make([]func() tools.Result, 0, 8)
	r := newRig(t)
	for i := range 8 {
		turns = append(turns, toolTurn(call(fmt.Sprint("c", i), "update_panels", fmt.Sprintf(`{"n":%d}`, i))), stopTurn(llm.StopMaxTokens))
		fns = append(fns, r.rows(t, 1, "r1"))
	}
	turns = append(turns, stopTurn(llm.StopEndTurn))
	s := session(turns...)
	r.provider.sessions = []*scriptedSession{s}
	r.buildSaves(t, fns...)

	out, err := r.agent.Run(context.Background(), r.task())
	if err != nil || out.Version != 1 || out.BuildSaves != 8 || len(s.compactions) != 8 || len(r.provider.started()) != 1 {
		t.Fatalf("outcome = %+v, err %v, compactions %d", out, err, len(s.compactions))
	}
}

func readOf(id string, n int) llm.ToolCall {
	return call(id, "get_spec_guide", fmt.Sprintf(`{"n":%d}`, n))
}

func readTurns(prefix string, n int) []scriptedTurn {
	turns := make([]scriptedTurn, n)
	for i := range turns {
		turns[i] = toolTurn(readOf(fmt.Sprint(prefix, i), i))
	}
	return turns
}

func TestReadsBesideASaveRunPastTheSixthCall(t *testing.T) {
	turns := readTurns("r", 5)
	turns = append(turns, toolTurn(readOf("late", 99), call("s", "create_dashboard", `{"n":1}`)))
	turns = append(turns, readTurns("after", 5)...)
	turns = append(turns, stopTurn(llm.StopEndTurn))
	s := session(turns...)
	r := newRig(t, s)
	r.buildSaves(t, r.rows(t, 1, "r1"))

	out, err := r.agent.Run(context.Background(), r.task())
	if err != nil || out.Version != 1 || out.BuildSaves != 1 {
		t.Fatalf("outcome = %+v, err %v", out, err)
	}
	beside := s.results(6)
	if len(beside) != 2 || beside[0].IsError || beside[1].IsError {
		t.Fatalf("results of the sixth call = %+v", beside)
	}
	if got := len(r.tools.names()); got != 5+2+5 {
		t.Fatalf("tools ran = %v", r.tools.names())
	}
	if after := s.results(7); len(after) != 1 || after[0].IsError {
		t.Fatalf("a read after the save = %+v", after)
	}
}

const noteOpen = "<note>"

func TestTheFifthCallWithoutASaveCarriesTheNudge(t *testing.T) {
	turns := readTurns("r", 5)
	turns = append(turns, toolTurn(call("s", "create_dashboard", `{"n":1}`)), stopTurn(llm.StopEndTurn))
	s := session(turns...)
	r := newRig(t, s)
	r.buildSaves(t, r.rows(t, 1, "r1"))

	if _, err := r.agent.Run(context.Background(), r.task()); err != nil {
		t.Fatal(err)
	}
	for call := 1; call <= 4; call++ {
		if got := s.results(call)[0].Content; strings.Contains(got, noteOpen) {
			t.Fatalf("result of call %d carries a note: %s", call, got)
		}
	}
	fifth := s.results(5)
	want := "\n\n<note>Save the next panel, or call save_finding if the request only asks a question, in your next call. From then on, reads run only in the same call as a save.</note>"
	if len(fifth) != 1 || !strings.HasSuffix(fifth[0].Content, want) || fifth[0].IsError {
		t.Fatalf("result of the fifth call = %+v", fifth)
	}
}

func TestTheTurnAfterARejectedSaveMayRead(t *testing.T) {
	turns := readTurns("r", 5)
	turns = append(turns,
		toolTurn(call("bad", "create_dashboard", `{"n":1}`)),
		toolTurn(call("diagnose", "query_range", `{"q":"up"}`)),
		toolTurn(call("good", "create_dashboard", `{"n":2}`)),
		stopTurn(llm.StopEndTurn),
	)
	s := session(turns...)
	r := newRig(t, s)
	r.buildSaves(t, rejected(), r.rows(t, 1, "r1"))

	out, err := r.agent.Run(context.Background(), r.task())
	if err != nil || out.Version != 1 {
		t.Fatalf("outcome = %+v, err %v", out, err)
	}
	if diagnosed := s.results(7); len(diagnosed) != 1 || diagnosed[0].IsError {
		t.Fatalf("the read after the rejected save = %+v", diagnosed)
	}
	if got := r.tools.names(); !slices.Contains(got, "query_range") {
		t.Fatalf("tools ran = %v", got)
	}
}

func TestTheDiagnoseTurnIsOnlyTheNextOne(t *testing.T) {
	turns := readTurns("r", 5)
	turns = append(turns,
		toolTurn(call("bad", "create_dashboard", `{"n":1}`)),
		toolTurn(call("diagnose", "query_range", `{"q":"up"}`)),
		toolTurn(readOf("again", 77)),
		toolTurn(call("good", "create_dashboard", `{"n":2}`)),
		stopTurn(llm.StopEndTurn),
	)
	s := session(turns...)
	r := newRig(t, s)
	r.buildSaves(t, rejected(), r.rows(t, 1, "r1"))

	if _, err := r.agent.Run(context.Background(), r.task()); err != nil {
		t.Fatal(err)
	}
	refused := s.results(8)
	if len(refused) != 1 || !refused[0].IsError || !strings.Contains(refused[0].Content, "6 model calls without a save; only a save can run now") {
		t.Fatalf("the second read after the rejected save = %+v", refused)
	}
}

func TestACompactionStartsAFreshSaveWindow(t *testing.T) {
	turns := append(readTurns("a", 5), stopTurn(llm.StopMaxTokens))
	turns = append(turns, readTurns("b", 5)...)
	turns = append(turns, toolTurn(call("s", "create_dashboard", `{"n":1}`)), stopTurn(llm.StopEndTurn))
	s := session(turns...)
	r := newRig(t, s)
	r.buildSaves(t, r.rows(t, 1, "r1"))

	out, err := r.agent.Run(context.Background(), r.task())
	if err != nil || out.Version != 1 || len(s.compactions) != 1 {
		t.Fatalf("outcome = %+v, err %v, compactions %d", out, err, len(s.compactions))
	}
	if got := len(r.tools.names()); got != 5+5+1 {
		t.Fatalf("tools ran = %v", r.tools.names())
	}
}

func TestSixReadOnlyTurnsThenASeventhEndWithNoProgress(t *testing.T) {
	s := session(readTurns("r", 8)...)
	r := newRig(t, s)

	out, err := r.agent.Run(context.Background(), r.task())
	if !errors.Is(err, agent.ErrNoProgress) {
		t.Fatalf("err = %v", err)
	}
	if s.modelCalls() != 7 || out.Usage.Calls != 7 || len(r.tools.names()) != 5 {
		t.Fatalf("model calls = %d, tools ran = %v", s.modelCalls(), r.tools.names())
	}
	steps := r.recordedSteps()
	if refusal := steps[len(steps)-3]; refusal.Status != "error" || refusal.Message != "Refused get_spec_guide: 6 model calls passed without a save" {
		t.Fatalf("refusal step = %+v", refusal)
	}
	if stop := steps[len(steps)-1]; stop.Status != "error" || stop.Message != "Stopped: the model kept reading after 6 calls without a save" {
		t.Fatalf("stop step = %+v", stop)
	}
}

func TestASuggestRunMayReadForAWhileBeforeProposing(t *testing.T) {
	turns := readTurns("r", 9)
	turns = append(turns, toolTurn(call("s", "update_dashboard", `{"spec":{}}`)))
	r := newRig(t, session(turns...))
	r.versionTwo(t)
	r.tools.onSave("update_dashboard", suggestedResult(t, panelID))

	out, err := r.agent.Run(context.Background(), r.suggestTask(t))
	if err != nil || !strings.Contains(string(out.Suggestion), panelID) || len(r.tools.names()) != 10 {
		t.Fatalf("outcome = %+v, err %v, tools ran %d", out, err, len(r.tools.names()))
	}
}

func TestAHundredAndTwentyPanelSavesWithAReadBesideEachSucceed(t *testing.T) {
	turns := make([]scriptedTurn, 120)
	for i := range turns {
		turns[i] = toolTurn(readOf(fmt.Sprint("r", i), i), call(fmt.Sprint("s", i), "update_panels", fmt.Sprintf(`{"n":%d}`, i)))
	}
	r := newRig(t, session(turns...))
	r.savesUntilVersion(120)

	out, err := r.agent.Run(context.Background(), r.task())
	if err != nil || out.Version != 1 || out.BuildSaves != 120 || out.Usage.Calls != 120 {
		t.Fatalf("outcome = %+v, err %v", out, err)
	}
}

func TestASessionRenewsAfterTheNextAcceptedSaveOncePastItsBudget(t *testing.T) {
	for _, c := range []struct {
		name       string
		renewBytes int
		resultKiB  int
		renews     bool
	}{
		{"default budget passed", 0, 300, true},
		{"default budget not passed", 0, 200, false},
		{"configured 64 KiB passed", 64 << 10, 70, true},
		{"configured 64 KiB not passed", 64 << 10, 50, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := session(
				toolTurn(readOf("big", 1)),
				toolTurn(call("s1", "update_panels", `{"n":1}`)),
				toolTurn(call("s2", "update_panels", `{"n":2,"finish":true}`)),
			)
			r := newRigWithRenewBytes(t, c.renewBytes, s)
			big := strings.Repeat("x", c.resultKiB<<10)
			r.tools.on("get_spec_guide", func(context.Context, json.RawMessage) (tools.Result, error) {
				return tools.Result{Content: map[string]any{"text": big}}, nil
			})
			r.buildSaves(t, r.rows(t, 2, "r1"), saved(1))

			out, err := r.agent.Run(context.Background(), r.task())
			if err != nil || out.Version != 1 {
				t.Fatalf("outcome = %+v, err %v", out, err)
			}
			started := r.provider.started()
			if len(started) != 1 || out.BuildSaves != 2 {
				t.Fatalf("outcome = %+v, sessions %d", out, len(started))
			}
			if !c.renews {
				if len(s.compactions) != 0 {
					t.Fatalf("compactions = %d", len(s.compactions))
				}
				return
			}
			if len(s.compactions) != 1 {
				t.Fatalf("compactions = %d", len(s.compactions))
			}
			for _, want := range []string{
				"Why your conversation was shortened: Your session was renewed to keep it short; the build above holds every panel you saved.",
				"Panels built: 1 of 2.",
				"r1_p",
			} {
				if !strings.Contains(s.compactions[0].Message, want) {
					t.Fatalf("compaction message lacks %q:\n%s", want, s.compactions[0].Message)
				}
			}
		})
	}
}

func TestARenewalDoesNotHideASaveRepeatedAcrossIt(t *testing.T) {
	r := newRig(t, session(
		toolTurn(readOf("big", 1)),
		toolTurn(call("s1", "update_panels", `{"n":1}`)),
		toolTurn(call("s2", "update_panels", `{"n":1}`)),
		toolTurn(call("s3", "update_panels", `{"n":1}`)),
	))
	r.tools.on("get_spec_guide", func(context.Context, json.RawMessage) (tools.Result, error) {
		return tools.Result{Content: map[string]any{"text": strings.Repeat("x", 300<<10)}}, nil
	})
	r.buildSaves(t, r.rows(t, 2, "r1"))

	out, err := r.agent.Run(context.Background(), r.task())
	if !errors.Is(err, agent.ErrNoProgress) {
		t.Fatalf("outcome = %+v, err %v", out, err)
	}
}

func TestALostBuildEndsTheRunAtOnce(t *testing.T) {
	r := newRig(t, session(toolTurn(call("c1", "create_dashboard", `{}`))))
	r.tools.on("create_dashboard", func(context.Context, json.RawMessage) (tools.Result, error) {
		return tools.Result{}, fmt.Errorf("tool create_dashboard: %w", dashboards.ErrBuildLost)
	})
	_, err := r.agent.Run(context.Background(), r.task())
	if !errors.Is(err, dashboards.ErrBuildLost) {
		t.Fatalf("err = %v", err)
	}
	fin := r.finishLog()
	if fin["error_code"] != "lease_expired" || fin["level"] != "WARN" {
		t.Fatalf("finish line = %v", fin)
	}
}

func TestACommitAfterALostLeaseEndsTheRun(t *testing.T) {
	r := newRig(t, session(stopTurn(llm.StopEndTurn)))
	r.builds.seed(agentBuild(t, "r1"))
	r.builds.commitErr = []error{dashboards.ErrBuildLost}

	_, err := r.agent.Run(context.Background(), r.task())
	if !errors.Is(err, dashboards.ErrBuildLost) {
		t.Fatalf("err = %v", err)
	}
	fin := r.finishLog()
	if fin["error_code"] != "lease_expired" || fin["level"] != "WARN" {
		t.Fatalf("finish line = %v", fin)
	}
}

func TestATaskWithARequestIDThatIsNotAUUIDIsInvalid(t *testing.T) {
	r := newRig(t, session())
	task := r.task()
	task.RequestID = "not-a-uuid"
	if _, err := r.agent.Run(context.Background(), task); !errors.Is(err, agent.ErrInvalidTask) {
		t.Fatalf("err = %v", err)
	}
}

const conflictSeedSentence = "Someone saved a new version while you worked; your build was discarded. Make the change again on the dashboard above."

func TestAResumedBuildOnAnOlderVersionRestartsFromTheLatest(t *testing.T) {
	r := newRig(t, session(toolTurn(call("c1", "update_panels", `{}`))))
	stale := agentBuild(t, "r1")
	stale.BaseVersion = latestVersion - 1
	r.builds.seed(stale)
	r.tools.onSave("update_panels", savedResult(dashID, 4))

	out, err := r.agent.Run(context.Background(), r.reviseTask())
	if err != nil || out.Version != 4 {
		t.Fatalf("outcome = %+v, err %v", out, err)
	}
	started := r.provider.started()
	if len(started) != 1 || !strings.Contains(started[0].Context, conflictSeedSentence) || !strings.Contains(started[0].Context, "version 3") || strings.Contains(started[0].Context, "<build>") {
		t.Fatalf("started = %+v", started)
	}
	if r.builds.discards != 1 {
		t.Fatalf("discards = %d", r.builds.discards)
	}
}

func TestAResumedBuildOnAnOlderVersionRebasesWhenItsChangesAreDisjoint(t *testing.T) {
	r := newRig(t, session(toolTurn(call("c1", "update_panels", `{}`))))
	stale := agentBuild(t, "r1")
	stale.BaseVersion = latestVersion - 1
	r.builds.seed(stale)
	r.builds.rebaseOnto = latestVersion
	r.tools.onSave("update_panels", savedResult(dashID, 4))

	out, err := r.agent.Run(context.Background(), r.reviseTask())
	if err != nil || out.Version != 4 {
		t.Fatalf("outcome = %+v, err %v", out, err)
	}
	started := r.provider.started()
	if len(started) != 1 || strings.Contains(started[0].Context, conflictSeedSentence) || !strings.Contains(started[0].Context, "<build>") {
		t.Fatalf("started = %+v, want one session on the kept build", started)
	}
	if r.builds.discards != 0 || r.builds.rebases != 1 || !hasStep(r.recordedSteps(), "Rebased onto version 3") {
		t.Fatalf("discards %d, rebases %d, steps %+v", r.builds.discards, r.builds.rebases, r.recordedSteps())
	}
}

func TestADashboardThatMovedBeforeARenewalRestartsAtTheRenewal(t *testing.T) {
	r := newRigWithRenewBytes(t, 1,
		session(toolTurn(call("s1", "update_panels", `{"n":1}`))),
		session(toolTurn(call("s2", "update_panels", `{"n":2,"finish":true}`))),
	)
	rows := r.rows(t, 2, "r1")
	r.tools.on("update_panels", sequence(
		func() tools.Result {
			res := rows()
			r.dash.setNumber(latestVersion + 1)
			return res
		},
		saved(5),
	))

	out, err := r.agent.Run(context.Background(), r.reviseTask())
	if err != nil || out.Version != 5 || out.RepairRounds != 0 {
		t.Fatalf("outcome = %+v, err %v", out, err)
	}
	started := r.provider.started()
	if len(started) != 2 || !strings.Contains(started[1].Context, conflictSeedSentence) || !strings.Contains(started[1].Context, "version 4") || strings.Contains(started[1].Context, "<build>") {
		t.Fatalf("started = %+v", started)
	}
	if r.builds.discards != 1 {
		t.Fatalf("discards = %d", r.builds.discards)
	}
}

func TestACommitConflictMidSessionIsAConflictNotARejection(t *testing.T) {
	r := newRig(t,
		session(toolTurn(call("c1", "update_panels", `{"n":1,"finish":true}`))),
		session(toolTurn(call("c2", "update_panels", `{"n":2,"finish":true}`))),
	)
	r.tools.onSave("update_panels", conflictResult(latestVersion+1), savedResult(dashID, 5))

	out, err := r.agent.Run(context.Background(), r.reviseTask())
	if err != nil || out.Version != 5 || out.RepairRounds != 0 {
		t.Fatalf("outcome = %+v, err %v", out, err)
	}
	if got := len(r.provider.started()); got != 2 {
		t.Fatalf("sessions = %d, want one restart", got)
	}
}

func TestARestartResetsRejectionsAndTheSaveWindow(t *testing.T) {
	first := session(
		toolTurn(call("a1", "update_panels", `{"n":1}`)),
		toolTurn(call("a2", "update_panels", `{"n":2}`)),
		toolTurn(call("a3", "update_panels", `{"n":3}`)),
	)
	turns := readTurns("w", 5)
	turns = append(turns,
		toolTurn(call("b1", "update_panels", `{"n":4}`)),
		toolTurn(call("b2", "update_panels", `{"n":5}`)),
		toolTurn(call("b3", "update_panels", `{"n":6}`)),
	)
	r := newRig(t, first, session(turns...))
	r.buildSaves(t, rejected(), rejected(), func() tools.Result { return conflictResult(latestVersion + 1) }, rejected(), rejected(), saved(5))

	out, err := r.agent.Run(context.Background(), r.reviseTask())
	if err != nil || out.Version != 5 {
		t.Fatalf("outcome = %+v, err %v", out, err)
	}
	if got := len(r.tools.names()); got != 3+5+3 {
		t.Fatalf("tools ran = %v", r.tools.names())
	}
}

const callTimeoutSeedSentence = "Why your conversation was shortened: Your last call ran past the time limit for one model call. Keep each answer short: save one panel per call, and check only what that panel needs."

func timedOut() scriptedTurn {
	return scriptedTurn{err: llm.NewCallError("fake", llm.CallTimeout, 0)}
}

func TestACallTimeoutCompactsOnce(t *testing.T) {
	s := session(toolTurn(readOf("r1", 1)), timedOut(), toolTurn(call("c1", "create_dashboard", `{}`)))
	r := newRig(t, s)
	r.tools.onSave("create_dashboard", savedResult(dashID, 1))

	out, err := r.agent.Run(context.Background(), r.task())
	if err != nil || out.Version != 1 {
		t.Fatalf("outcome = %+v, err %v", out, err)
	}
	started := r.provider.started()
	if len(started) != 1 || strings.Contains(started[0].Context, "time limit") {
		t.Fatalf("contexts = %+v", started)
	}
	if len(s.compactions) != 1 || !strings.Contains(s.compactions[0].Message, callTimeoutSeedSentence) {
		t.Fatalf("compactions = %+v", s.compactions)
	}
}

func TestACompactionCarriesTheCutCallsResults(t *testing.T) {
	cases := map[string]scriptedTurn{
		"output limit": stopTurn(llm.StopMaxTokens),
		"call timeout": timedOut(),
	}
	for name, cut := range cases {
		t.Run(name, func(t *testing.T) {
			s := session(toolTurn(readOf("r1", 1), readOf("r2", 2)), cut, toolTurn(call("c1", "create_dashboard", `{}`)))
			r := newRig(t, s)
			r.tools.onSave("create_dashboard", savedResult(dashID, 1))

			if _, err := r.agent.Run(context.Background(), r.task()); err != nil {
				t.Fatal(err)
			}
			if s.modelCalls() != 3 || len(s.results(1)) != 2 || !reflect.DeepEqual(s.results(1), s.results(2)) {
				t.Fatalf("results sent with the cut call %+v and the call after it %+v", s.results(1), s.results(2))
			}
		})
	}
}

func TestARenewalCarriesTheResultsOfTheSavingTurn(t *testing.T) {
	s := session(
		toolTurn(readOf("big", 1)),
		toolTurn(call("s1", "update_panels", `{"n":1}`)),
		toolTurn(call("s2", "update_panels", `{"n":2,"finish":true}`)),
	)
	r := newRigWithRenewBytes(t, 64<<10, s)
	r.tools.on("get_spec_guide", func(context.Context, json.RawMessage) (tools.Result, error) {
		return tools.Result{Content: map[string]any{"text": strings.Repeat("x", 70<<10)}}, nil
	})
	r.buildSaves(t, r.rows(t, 2, "r1"), saved(1))

	if _, err := r.agent.Run(context.Background(), r.task()); err != nil {
		t.Fatal(err)
	}
	if len(s.compactions) != 1 || s.modelCalls() != 3 || len(s.results(2)) != 1 || s.results(2)[0].CallID != "s1" {
		t.Fatalf("compactions %d, results of the call after the renewal %+v", len(s.compactions), s.results(2))
	}
}

func TestTwoCutOffsInARowEndTheRunWithTheLastCode(t *testing.T) {
	r := newRig(t, session(stopTurn(llm.StopMaxTokens), timedOut()))

	result, err := agent.NewReviser(r.agent, nil).Revise(context.Background(), revisionRequest())
	if err == nil || result.FailureCode != revisions.FailureCallTimeout {
		t.Fatalf("result = %+v, err %v", result, err)
	}
}

func TestAnOutputLimitAfterACallTimeoutEndsTheRunAndSaysWhy(t *testing.T) {
	r := newRig(t, session(timedOut(), stopTurn(llm.StopMaxTokens)))

	out, err := r.agent.Run(context.Background(), r.task())
	if !strings.Contains(fmt.Sprint(err), "after reaching max tokens") {
		t.Fatalf("outcome = %+v, err %v", out, err)
	}
	steps := r.recordedSteps()
	if stop := steps[len(steps)-1]; stop.Status != "error" || stop.Message != "Stopped: the model's answer was cut off at the output limit 2 times in a row" {
		t.Fatalf("stop step = %+v", stop)
	}
}

func TestTwoCallTimeoutsInARowSayWhyInAStep(t *testing.T) {
	r := newRig(t, session(timedOut(), timedOut()))

	if _, err := r.agent.Run(context.Background(), r.task()); err == nil {
		t.Fatal("the run succeeded")
	}
	steps := r.recordedSteps()
	if stop := steps[len(steps)-1]; stop.Message != "Stopped: a model call ran past the time limit 2 times in a row" || stop.Status != "error" {
		t.Fatalf("stop step = %+v", stop)
	}
}

func TestACallTimeoutInSuggestFailsAtOnce(t *testing.T) {
	r := newRig(t, session(timedOut()))
	r.versionTwo(t)

	_, err := r.agent.Run(context.Background(), r.suggestTask(t))
	var callErr *llm.CallError
	if !errors.As(err, &callErr) || callErr.Failure != llm.CallTimeout || len(r.provider.started()) != 1 {
		t.Fatalf("err = %v, sessions %d", err, len(r.provider.started()))
	}
	steps := r.recordedSteps()
	if stop := steps[len(steps)-1]; stop.Message != "Stopped: a model call ran past the time limit" {
		t.Fatalf("stop step = %+v", stop)
	}
}

type funcSession func(ctx context.Context, results []llm.ToolResult) (llm.Turn, error)

func (f funcSession) Compact(llm.Compaction) error { return nil }

func (f funcSession) Next(ctx context.Context, results []llm.ToolResult) (llm.Turn, error) {
	return f(ctx, results)
}

type sessionsProvider struct {
	scriptedProvider
	only llm.Session
}

func (p *sessionsProvider) Start(context.Context, llm.Request) (llm.Session, error) {
	return p.only, nil
}

func TestACanceledRunIsNotCompactedOnACallTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := newRig(t)
	r.agent = agent.New(agent.Config{
		Provider: &sessionsProvider{only: funcSession(func(context.Context, []llm.ToolResult) (llm.Turn, error) {
			cancel()
			return llm.Turn{}, llm.NewCallError("fake", llm.CallTimeout, 0)
		})},
		Tools: r.tools, Dashboards: r.dash, Builds: r.builds, Datasources: fakeHealth{}, Logger: slog.New(slog.DiscardHandler),
	})

	out, err := r.agent.Run(ctx, r.task())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("outcome = %+v, err %v", out, err)
	}
}

func discoveringRead(names ...string) handler {
	return func(context.Context, json.RawMessage) (tools.Result, error) {
		return tools.Result{
			Content:    map[string]any{"ok": true},
			Step:       "Read",
			Discovered: &tools.Discovery{Datasource: "prom", Of: "metrics", Names: names},
		}, nil
	}
}

const discoveredLine = "- prom metrics: node_load1, node_cpu_seconds_total"

func TestACompactionAfterAnOutputLimitCarriesTheDiscoveredNames(t *testing.T) {
	s := session(toolTurn(readOf("d1", 1)), stopTurn(llm.StopMaxTokens), toolTurn(call("c1", "create_dashboard", `{}`)))
	r := newRig(t, s)
	r.tools.on("get_spec_guide", discoveringRead("node_load1", "node_cpu_seconds_total"))
	r.tools.onSave("create_dashboard", savedResult(dashID, 1))

	if _, err := r.agent.Run(context.Background(), r.task()); err != nil {
		t.Fatal(err)
	}
	if got := r.provider.started()[0].Context; strings.Contains(got, "<discovered>") {
		t.Fatalf("the first session knows nothing yet: %s", got)
	}
	if len(s.compactions) != 1 {
		t.Fatalf("compactions = %d", len(s.compactions))
	}
	message := s.compactions[0].Message
	if !strings.Contains(message, "<discovered>") || !strings.Contains(message, discoveredLine) || !strings.Contains(message, "</discovered>\n\n<request>") {
		t.Fatalf("compaction message lacks the digest before the request:\n%s", message)
	}
}

func TestADiscoveryThatFailedIsNotInTheDigest(t *testing.T) {
	s := session(toolTurn(readOf("d1", 1)), stopTurn(llm.StopMaxTokens), toolTurn(call("c1", "create_dashboard", `{}`)))
	r := newRig(t, s)
	r.tools.on("get_spec_guide", func(context.Context, json.RawMessage) (tools.Result, error) {
		return tools.Result{
			IsError:    true,
			Content:    map[string]any{"code": "datasource_unavailable"},
			Discovered: &tools.Discovery{Datasource: "prom", Of: "metrics", Names: []string{"FAILED-NAME"}},
		}, nil
	})
	r.tools.onSave("create_dashboard", savedResult(dashID, 1))

	if _, err := r.agent.Run(context.Background(), r.task()); err != nil {
		t.Fatal(err)
	}
	if message := s.compactions[0].Message; strings.Contains(message, "FAILED-NAME") || strings.Contains(message, "<discovered>") {
		t.Fatalf("compaction message carries a failed discovery:\n%s", message)
	}
}

func TestARenewalCarriesTheDiscoveredNames(t *testing.T) {
	s := session(
		toolTurn(readOf("d1", 1)),
		toolTurn(call("s1", "update_panels", `{"n":1}`)),
		toolTurn(call("s2", "update_panels", `{"n":2,"finish":true}`)),
	)
	r := newRigWithRenewBytes(t, 1, s)
	r.tools.on("get_spec_guide", discoveringRead("node_load1", "node_cpu_seconds_total"))
	r.buildSaves(t, r.rows(t, 2, "r1"), saved(1))

	if _, err := r.agent.Run(context.Background(), r.task()); err != nil {
		t.Fatal(err)
	}
	if len(s.compactions) != 1 || !strings.Contains(s.compactions[0].Message, discoveredLine) {
		t.Fatalf("compactions = %+v", s.compactions)
	}
}

func TestAConflictRestartCarriesTheDiscoveredNames(t *testing.T) {
	first := session(toolTurn(readOf("d1", 1)), toolTurn(call("c1", "update_dashboard", `{}`)))
	second := session(toolTurn(call("c2", "update_dashboard", `{}`)))
	r := newRig(t, first, second)
	r.tools.on("get_spec_guide", discoveringRead("node_load1", "node_cpu_seconds_total"))
	r.tools.on("update_dashboard", sequence(
		func() tools.Result {
			r.dash.setNumber(latestVersion + 1)
			return conflictResult(latestVersion + 1)
		},
		saved(5),
	))

	out, err := r.agent.Run(context.Background(), r.reviseTask())
	if err != nil || out.Version != 5 {
		t.Fatalf("outcome = %+v, err %v", out, err)
	}
	started := r.provider.started()
	if len(started) != 2 || !strings.Contains(started[1].Context, discoveredLine) || strings.Contains(started[0].Context, "<discovered>") {
		t.Fatalf("contexts = %+v", started)
	}
}

func TestEverySessionAsksForRequestSidePruning(t *testing.T) {
	r := newRig(t, session(toolTurn(call("c1", "create_dashboard", `{}`))), session(toolTurn(call("c2", "create_dashboard", `{}`))))
	r.tools.onSave("create_dashboard", conflictResult(4), savedResult(dashID, 5))
	r.dash.setNumber(3)

	if _, err := r.agent.Run(context.Background(), r.reviseTask()); err != nil {
		t.Fatal(err)
	}
	started := r.provider.started()
	if len(started) != 2 {
		t.Fatalf("sessions = %d", len(started))
	}
	for i, req := range started {
		if req.Pruning != (llm.Pruning{KeepTurns: 4, StepBytes: 64 << 10}) {
			t.Fatalf("session %d pruning = %+v", i, req.Pruning)
		}
	}
}

type scriptedDatasource struct {
	mu      sync.Mutex
	answers []func() ([]datasource.Frame, error)
}

func (d *scriptedDatasource) Type() string                      { return "prometheus" }
func (d *scriptedDatasource) CheckHealth(context.Context) error { return nil }
func (d *scriptedDatasource) Close() error                      { return nil }
func (d *scriptedDatasource) Discover(context.Context, datasource.DiscoverRequest) (*datasource.Catalog, error) {
	return &datasource.Catalog{}, nil
}
func (d *scriptedDatasource) Lint(context.Context, string) ([]datasource.Diagnostic, error) {
	return nil, nil
}
func (d *scriptedDatasource) VariableValues(context.Context, datasource.Query, datasource.TimeRange) ([]string, error) {
	return nil, nil
}
func (d *scriptedDatasource) QueryInstant(context.Context, datasource.Query, datasource.TimeRange) ([]datasource.Frame, error) {
	return nil, nil
}

func (d *scriptedDatasource) QueryRange(context.Context, datasource.Query, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.answers) == 0 {
		return nil, nil
	}
	answer := d.answers[0]
	d.answers = d.answers[1:]
	return answer()
}

func registryRig(t *testing.T, ds datasource.Datasource, sessions ...*scriptedSession) (*agent.Agent, *scriptedProvider, *bytes.Buffer) {
	t.Helper()
	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	reg, err := datasource.NewRegistry(logger, datasource.Instance{UID: "prom", Name: "Prometheus", Datasource: ds})
	if err != nil {
		t.Fatal(err)
	}
	provider := &scriptedProvider{sessions: sessions}
	a := agent.New(agent.Config{
		Provider:    provider,
		Tools:       tools.New(tools.Deps{Datasources: reg, Runner: queryproxy.NewRunner(reg, logger), Logger: logger}),
		Dashboards:  &fakeDashboards{number: latestVersion},
		Builds:      &fakeBuilds{},
		Datasources: fakeHealth{},
		Logger:      logger,
	})
	return a, provider, logs
}

func TestAToolErrorFromAHandlerReachesTheModelAndTheRunGoesOn(t *testing.T) {
	ds := &scriptedDatasource{answers: []func() ([]datasource.Frame, error){
		func() ([]datasource.Frame, error) { return nil, errors.New("boom at 10.0.0.5 UNIQUE-SECRET") },
		func() ([]datasource.Frame, error) { return nil, nil },
	}}
	s := session(
		toolTurn(call("c1", "query_range", `{"datasource":"prom","query":"up"}`)),
		toolTurn(call("c2", "query_range", `{"datasource":"prom","query":"sum(up)"}`)),
		stopTurn(llm.StopRefusal),
	)
	a, _, logs := registryRig(t, ds, s)

	if _, err := a.Run(context.Background(), agent.Task{Prompt: "checkout health by pod", RequestID: requestID, Attempt: 1, Principal: principal}); !errors.Is(err, agent.ErrRefused) {
		t.Fatalf("err = %v, want the run to reach the model's refusal", err)
	}
	if got := s.modelCalls(); got != 3 {
		t.Fatalf("model calls = %d, want 3", got)
	}
	failed := s.results(1)
	if len(failed) != 1 || !failed[0].IsError || !strings.Contains(failed[0].Content, `"code":"tool_failed"`) ||
		!strings.Contains(failed[0].Content, "query_range failed: the datasource did not answer") {
		t.Fatalf("result after the failure = %+v", failed)
	}
	if next := s.results(2); len(next) != 1 || next[0].IsError {
		t.Fatalf("result after the retry = %+v", next)
	}
	for _, leaked := range []string{"UNIQUE-SECRET", "10.0.0.5"} {
		if strings.Contains(logs.String(), leaked) || strings.Contains(failed[0].Content, leaked) {
			t.Fatalf("%q leaked: %s", leaked, logs.String())
		}
	}
	if strings.Count(logs.String(), `"msg":"agent tool failed"`) != 1 {
		t.Fatalf("want one tool failure line: %s", logs.String())
	}
}

func TestAnInvariantFromAHandlerStillEndsTheRun(t *testing.T) {
	tests := []struct {
		name   string
		answer func() ([]datasource.Frame, error)
		tool   string
		input  string
		cause  error
		code   string
	}{
		{
			name:   "wrapped cancellation",
			answer: func() ([]datasource.Frame, error) { return nil, fmt.Errorf("upstream: %w", context.Canceled) },
			tool:   "query_range",
			input:  `{"datasource":"prom","query":"up"}`,
			cause:  context.Canceled,
			code:   "tool_failed",
		},
		{
			name:  "handler panic",
			tool:  "get_panels",
			input: `{"ids":["p1"]}`,
			cause: tools.ErrPanicked,
			code:  "tool_panicked",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds := &scriptedDatasource{}
			if tt.answer != nil {
				ds.answers = append(ds.answers, tt.answer)
			}
			s := session(toolTurn(call("c1", tt.tool, tt.input)), stopTurn(llm.StopRefusal))
			a, _, logs := registryRig(t, ds, s)

			_, err := a.Run(context.Background(), agent.Task{Prompt: "checkout health by pod", RequestID: requestID, Attempt: 1, Principal: principal})
			if !errors.Is(err, tools.ErrInvariant) || !errors.Is(err, tt.cause) {
				t.Fatalf("err = %v, want an invariant wrapping %v", err, tt.cause)
			}
			if err.Error() != "the "+tt.tool+" tool failed with an internal error" {
				t.Fatalf("error text = %q", err.Error())
			}
			if got := s.modelCalls(); got != 1 {
				t.Fatalf("model calls = %d, want the run to end after the first turn", got)
			}
			var finish map[string]any
			for line := range strings.SplitSeq(logs.String(), "\n") {
				var entry map[string]any
				if json.Unmarshal([]byte(line), &entry) == nil && entry["msg"] == "agent run finished" {
					finish = entry
				}
			}
			if finish["error_code"] != tt.code {
				t.Fatalf("finish line = %v, want error_code %s", finish, tt.code)
			}
		})
	}
}
