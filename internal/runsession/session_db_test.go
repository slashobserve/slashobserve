package runsession

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/route"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func TestASessionDispatchesEveryCallAndCountsIt(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	dispatched := &dispatchCount{}
	for n := range 8 {
		answer := r.call(req, "list_metrics", `{"q":"`+strconv.Itoa(n)+`"}`, nil, dispatched.wrap(readOK))
		if answer.IsError || len(answer.Notes) != 0 {
			t.Fatalf("read %d = %s, notes %q", n+1, answer.Content, answer.Notes)
		}
	}
	for range 3 {
		if same := r.call(req, "list_metrics", `{"q":"a"}`, nil, dispatched.wrap(readOK)); same.IsError || len(same.Notes) != 0 {
			t.Fatalf("an identical read = %s, notes %q", same.Content, same.Notes)
		}
	}
	turn := &Turn{ID: "t1", Calls: 2, Saves: 2}
	first := r.call(req, "update_panels", `{"n":1}`, turn, dispatched.wrap(buildSaved(r, "a", 3)))
	second := r.call(req, "update_panels", `{"n":2}`, turn, dispatched.wrap(buildSaved(r, "a", 3)))
	if first.IsError || second.IsError || dispatched.n != 13 {
		t.Fatalf("saves %s %s, dispatched %d", first.Content, second.Content, dispatched.n)
	}
	if second.Meta.Build == nil || second.Meta.Build.Saves != 2 || second.Meta.Call == nil || second.Meta.Call.Edits == nil {
		t.Fatalf("the lifecycle meta of a save = %+v", second.Meta)
	}
	if state := r.storedState(req); state.Counts.ToolCalls != 13 || state.Counts.BuildSaves != 2 {
		t.Fatalf("counts = %+v", state.Counts)
	}
	if r.stored(req).State != string(revisions.StateRunning) {
		t.Fatal("the session ended the run")
	}
}

func TestASessionKeepsTheLifecycleOfItsSaves(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	r.startBuild(req, "a")
	r.store.bump()
	r.store.rebaseOK = true
	answer := r.call(req, "update_panels", `{"panel":"b"}`, nil, versionConflict(2))
	if settledOf(answer) != "saved" || r.store.commits != 1 {
		t.Fatalf("conflict answer meta %+v, commits %d", answer.Meta, r.store.commits)
	}
}

func TestTheBriefIsStructuredOnlyAndPinsTheParent(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	r.startBuild(req, "a")
	got, err := r.svc.Brief(context.Background(), req, "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Brief == nil || got.Brief.Build == nil || got.Reason != "restart" {
		t.Fatalf("brief = %+v", got)
	}
	scope, err := r.svc.Scope(context.Background(), req)
	if err != nil || scope.ParentVersion != 1 {
		t.Fatalf("scope = %+v, %v", scope, err)
	}
}

func (r *rig) hinted(req revisions.Request, name, input string, hints *Hints, dispatch func(context.Context) (tools.Result, error)) Answer {
	r.t.Helper()
	answer, err := r.svc.Call(context.Background(), req, Call{Name: name, Input: json.RawMessage(input), Hints: hints}, dispatch)
	if err != nil {
		r.t.Fatalf("call %s: %v", name, err)
	}
	return answer
}

func TestASweepReadRecordsNoStepAndCountsNothing(t *testing.T) {
	r := newRig(t)
	req := r.investigate()
	req.Investigation = true
	swept := r.hinted(req, "query_range", `{"q":"up"}`, &Hints{Sweep: true}, readOK)
	if swept.IsError || len(swept.Notes) != 0 {
		t.Fatalf("sweep read = %s, notes %q", swept.Content, swept.Notes)
	}
	state := r.storedState(req)
	if state.Counts.ToolCalls != 0 || state.Usage.ToolMS != 0 {
		t.Fatalf("a sweep read counted: tool calls %d, tool ms %d", state.Counts.ToolCalls, state.Usage.ToolMS)
	}
	if steps := r.steps.messages(); len(steps) != 0 {
		t.Fatalf("a sweep read recorded steps %q", steps)
	}
	r.hinted(req, "query_range", `{"q":"up"}`, nil, readOK)
	if state := r.storedState(req); state.Counts.ToolCalls != 1 {
		t.Fatalf("an agent read: tool calls %d", state.Counts.ToolCalls)
	}
}

func TestACallHintNamesItsStep(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	r.hinted(req, "list_metrics", `{"q":"a"}`, &Hints{Step: "Reused the result of an identical call"}, readOK)
	if steps := r.steps.messages(); len(steps) != 1 || steps[0] != "Reused the result of an identical call" {
		t.Fatalf("steps = %q", steps)
	}
}

func TestAnInvestigationKeepsTheFirstTaskItsAgentReports(t *testing.T) {
	r := newRig(t)
	req := r.investigate()
	req.Investigation = true
	measure, logs := route.Measure, route.Logs
	for _, task := range []*route.Task{&measure, &logs} {
		if err := r.svc.NoteAgent(context.Background(), req, tools.AgentReport{Task: task}); err != nil {
			t.Fatal(err)
		}
	}
	scope, err := r.svc.Scope(context.Background(), req)
	if err != nil || scope.Task != route.Measure {
		t.Fatalf("scope task = %q, %v", scope.Task, err)
	}
}

func TestOnlyAnInvestigationTakesAReportedTask(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	measure := route.Measure
	if err := r.svc.NoteAgent(context.Background(), req, tools.AgentReport{Task: &measure}); err != nil {
		t.Fatal(err)
	}
	if state := r.storedState(req); state.Task == route.Measure {
		t.Fatalf("task = %q", state.Task)
	}
}

func (r *rig) storedState(req revisions.Request) State {
	r.t.Helper()
	var raw []byte
	if err := r.d.SQLDB(r.t).QueryRow("SELECT state FROM run_sessions WHERE agent_request_id = $1", mustUUID(r.t, req.ID)).Scan(&raw); err != nil {
		r.t.Fatal(err)
	}
	var state State
	if err := json.Unmarshal(raw, &state); err != nil {
		r.t.Fatal(err)
	}
	return state
}
