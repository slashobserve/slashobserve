package runsession

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/grounding"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func (r *rig) harness(req revisions.Request) revisions.Request {
	r.t.Helper()
	if err := r.svc.BeginHarness(context.Background(), req); err != nil {
		r.t.Fatal(err)
	}
	return req
}

func TestAHarnessRunSkipsTheGuardsAdmissionAndNudges(t *testing.T) {
	r := newRig(t)
	req := r.harness(r.revise())
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
		t.Fatalf("the lifecycle meta of a harness save = %+v", second.Meta)
	}
	if state := r.storedState(req); !state.AgentHarness || state.Guard.Counts.ToolCalls != 13 {
		t.Fatalf("state: harness %v, tool calls %d", state.AgentHarness, state.Guard.Counts.ToolCalls)
	}
	if r.stored(req).State != string(revisions.StateRunning) {
		t.Fatal("a harness run was ended by a guard")
	}
}

func TestAHarnessRunKeepsTheLifecycleOfItsSaves(t *testing.T) {
	r := newRig(t)
	req := r.harness(r.revise())
	r.startBuild(req, "a")
	r.store.bump()
	r.store.rebaseOK = true
	answer := r.call(req, "update_panels", `{"panel":"b"}`, nil, versionConflict(2))
	if settledOf(answer) != "saved" || r.store.commits != 1 {
		t.Fatalf("conflict answer meta %+v, commits %d", answer.Meta, r.store.commits)
	}
}

func TestAHarnessBriefHasNoTextButTheStructuredBriefAndPinsTheParent(t *testing.T) {
	r := newRig(t)
	req := r.harness(r.revise())
	r.startBuild(req, "a")
	got, err := r.svc.Brief(context.Background(), req, "")
	if err != nil {
		t.Fatal(err)
	}
	if got.System != "" || got.Message != "" || got.Brief == nil || got.Brief.Build == nil || got.Reason != "restart" {
		t.Fatalf("brief = %+v", got)
	}
	scope, err := r.svc.Scope(context.Background(), req)
	if err != nil || scope.ParentVersion != 1 {
		t.Fatalf("scope = %+v, %v", scope, err)
	}
}

func TestAHarnessInvestigationIsNeitherRoutedNorSweptNorGrounded(t *testing.T) {
	r := newRig(t, withInvestigationLoop(), func(c *Config) { c.FindingGrounding = grounding.Enforce })
	r.svc.SweepWith(leadSweeper{})
	req := r.investigate()
	req.Investigation = true
	r.harness(req)
	if _, err := r.svc.Brief(context.Background(), req, ""); err != nil {
		t.Fatal(err)
	}
	var unverified int
	saved := r.call(req, "save_finding", `{"markdown":"Checkout p95 is 2.1 s."}`, nil, func(ctx context.Context) (tools.Result, error) {
		unverified = len(tools.UncheckedReads(ctx)) + tools.ReadsRefused(ctx)
		return findingSaved(ctx)
	})
	if saved.IsError || settledOf(saved) != "finding" || unverified != 0 {
		t.Fatalf("save = %s, meta %+v", saved.Content, saved.Meta)
	}
	state := r.storedState(req)
	if state.Task != "" || state.Triage != nil || state.Grounding != nil || state.Guard.Loop != nil {
		t.Fatalf("state: task %q, triage %+v, grounding %v, loop %+v", state.Task, state.Triage, state.Grounding != nil, state.Guard.Loop)
	}
	for _, m := range r.steps.messages() {
		if strings.HasPrefix(m, "Routed the question") || strings.HasPrefix(m, "Swept the datasources") {
			t.Fatalf("a harness run recorded %q", m)
		}
	}
}

func TestWithoutTheHarnessTheInvestigationIsRoutedSweptAndGrounded(t *testing.T) {
	r := newRig(t, withInvestigationLoop(), func(c *Config) { c.FindingGrounding = grounding.Enforce })
	r.svc.SweepWith(leadSweeper{})
	req := r.investigate()
	req.Investigation = true
	if _, err := r.svc.Brief(context.Background(), req, ""); err != nil {
		t.Fatal(err)
	}
	state := r.storedState(req)
	if state.AgentHarness || state.Triage == nil || state.Grounding == nil {
		t.Fatalf("state: harness %v, triage %+v, grounding %v", state.AgentHarness, state.Triage, state.Grounding != nil)
	}
	routed := false
	for _, m := range r.steps.messages() {
		routed = routed || strings.HasPrefix(m, "Routed the question")
	}
	if !routed {
		t.Fatalf("steps = %q", r.steps.messages())
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

func TestAHarnessSweepReadRecordsNoStepAndCountsNothing(t *testing.T) {
	r := newRig(t)
	req := r.investigate()
	req.Investigation = true
	r.harness(req)
	swept := r.hinted(req, "query_range", `{"q":"up"}`, &Hints{Sweep: true}, readOK)
	if swept.IsError || len(swept.Notes) != 0 {
		t.Fatalf("sweep read = %s, notes %q", swept.Content, swept.Notes)
	}
	state := r.storedState(req)
	if state.Guard.Counts.ToolCalls != 0 || state.DataReads != 0 || state.Usage.ToolMS != 0 {
		t.Fatalf("a sweep read counted: tool calls %d, data reads %d, tool ms %d", state.Guard.Counts.ToolCalls, state.DataReads, state.Usage.ToolMS)
	}
	if steps := r.steps.messages(); len(steps) != 0 {
		t.Fatalf("a sweep read recorded steps %q", steps)
	}
	r.hinted(req, "query_range", `{"q":"up"}`, nil, readOK)
	if state := r.storedState(req); state.Guard.Counts.ToolCalls != 1 || state.DataReads != 1 {
		t.Fatalf("an agent read: tool calls %d, data reads %d", state.Guard.Counts.ToolCalls, state.DataReads)
	}
}

func TestAHarnessNamesTheStepOfACall(t *testing.T) {
	r := newRig(t)
	req := r.harness(r.revise())
	r.hinted(req, "list_metrics", `{"q":"a"}`, &Hints{Step: "Reused the result of an identical call"}, readOK)
	if steps := r.steps.messages(); len(steps) != 1 || steps[0] != "Reused the result of an identical call" {
		t.Fatalf("steps = %q", steps)
	}
}

func TestOutsideTheHarnessCallHintsChangeNothing(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	r.hinted(req, "list_metrics", `{"q":"a"}`, &Hints{Sweep: true, Step: "a step the agent chose"}, readOK)
	if state := r.storedState(req); state.Guard.Counts.ToolCalls != 1 {
		t.Fatalf("tool calls = %d", state.Guard.Counts.ToolCalls)
	}
	if steps := r.steps.messages(); len(steps) != 1 || steps[0] == "a step the agent chose" {
		t.Fatalf("steps = %q", steps)
	}
}
