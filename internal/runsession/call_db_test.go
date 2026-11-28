package runsession

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func contentCode(t *testing.T, a Answer) string {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(a.Content, &body); err != nil {
		t.Fatal(err)
	}
	return body.Code
}

func hasNote(a Answer, fragment string) bool {
	return slices.ContainsFunc(a.Notes, func(n string) bool { return strings.Contains(n, fragment) })
}

func settledOf(a Answer) string {
	if a.Meta.Settled == nil {
		return ""
	}
	return string(*a.Meta.Settled)
}

func TestParentVersionStaysPinnedUntilTheBrief(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	scope, err := r.svc.Scope(context.Background(), req)
	if err != nil || scope.ParentVersion != 1 || scope.Mode != tools.ModeRevise || !scope.Build {
		t.Fatalf("scope = %+v, %v", scope, err)
	}
	r.store.bump()
	scope, err = r.svc.Scope(context.Background(), req)
	if err != nil || scope.ParentVersion != 1 {
		t.Fatalf("scope after a newer version = %+v, %v", scope, err)
	}
	if _, err := r.svc.Brief(context.Background(), req, ""); err != nil {
		t.Fatal(err)
	}
	scope, err = r.svc.Scope(context.Background(), req)
	if err != nil || scope.ParentVersion != 2 {
		t.Fatalf("scope after the brief = %+v, %v", scope, err)
	}
}

func TestResultsCarryTheRunMeta(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	read := r.read(req, 0)
	if read.Meta.Settled != nil || read.Meta.Build != nil {
		t.Fatalf("read meta = %+v", read.Meta)
	}
	built := r.call(req, "update_panels", `{"n":1}`, nil, buildSaved(r, "a", 3))
	if built.Meta.Build == nil || *built.Meta.Build != (tools.BuildProgress{Saves: 1, Panels: 1, PlannedPanels: 3}) {
		t.Fatalf("build meta = %+v", built.Meta)
	}
}

func TestAFindingAndASuggestionSettleTheirRuns(t *testing.T) {
	r := newRig(t)
	investigation := r.investigate()
	finding := r.call(investigation, "save_finding", `{"x":1}`, nil, findingSaved)
	if settledOf(finding) != "finding" {
		t.Fatalf("finding meta = %+v", finding.Meta)
	}
	suggestion := r.suggest()
	suggested := r.call(suggestion, "update_panels", `{"x":1}`, nil, suggestionSaved)
	if settledOf(suggested) != "suggested" {
		t.Fatalf("suggestion meta = %+v", suggested.Meta)
	}
}

func TestAMalformedSaveResultEndsTheRun(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	answer := r.call(req, "update_panels", `{"n":1}`, nil, func(context.Context) (tools.Result, error) {
		return tools.Result{Content: map[string]any{}}, nil
	})
	row := r.stored(req)
	if row.FailureCode.String != string(revisions.FailureMalformedResult) || settledOf(answer) != "ended" {
		t.Fatalf("run %q, answer %+v", row.FailureCode.String, answer)
	}
}

func TestAToolErrorIsReturnedAndCountedAndTheTurnStillSettles(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	_, err := r.svc.Call(context.Background(), req, Call{Name: "list_metrics", Input: json.RawMessage(`{}`)}, func(context.Context) (tools.Result, error) {
		return tools.Result{}, context.DeadlineExceeded
	})
	if err == nil {
		t.Fatal("the tool error was swallowed")
	}
	if r.read(req, 1).IsError {
		t.Fatal("the next call was refused after a tool error")
	}
}

func TestAWriteIsDispatchedWithTheObjectsAnEarlierWriteCreated(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	created := r.call(req, "create_folder", `{"title":"t"}`, nil, func(context.Context) (tools.Result, error) {
		return tools.Result{Content: map[string]string{"id": "f1"}, Created: []string{"f1"}}, nil
	})
	if created.IsError {
		t.Fatalf("create_folder = %s", created.Content)
	}
	var seen tools.WriteScope
	r.call(req, "rename_folder", `{"folder_id":"f1","title":"u"}`, nil, func(ctx context.Context) (tools.Result, error) {
		seen = tools.WriteScopeFrom(ctx)
		return tools.Result{Content: map[string]string{"id": "f1"}}, nil
	})
	if !slices.Equal(seen.Created, []string{"f1"}) {
		t.Fatalf("scope seen by the second write = %+v", seen)
	}
	last := r.steps.all()[len(r.steps.all())-1]
	if last.Phase != tools.PhaseWrite || last.Tool != "rename_folder" {
		t.Fatalf("step = %+v", last)
	}
}

func TestADispatchedCallCarriesItsCallMeta(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	built := r.call(req, "update_panels", `{"n":1}`, nil, buildSaved(r, "a", 3))
	if built.Meta.Call == nil || built.Meta.Call.Edits == nil || built.Meta.Call.Edits.Panels[0] != "a" {
		t.Fatalf("build call meta = %+v", built.Meta.Call)
	}
}

func TestTheUsageOfATurnIsKeptWithTheSession(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	for n := range 3 {
		turn := &Turn{ID: "turn-" + strconv.Itoa(n+1), Calls: 1, Saves: 1, Usage: &revisions.Usage{ModelCalls: n + 1, OutputTokens: int64(100 * (n + 1))}}
		r.call(req, "update_panels", `{"n":`+strconv.Itoa(n+1)+`}`, turn, rejectedSave)
	}
	state := r.storedState(req)
	usage := state.usage()
	if usage.ModelCalls != 3 || usage.OutputTokens != 300 || usage.FailedCalls != 0 || usage.ToolCalls != 3 {
		t.Fatalf("usage = %+v, want 3 model calls, 300 output tokens, no failed calls and 3 tool calls", usage)
	}
}

func TestACallAfterTheRunEndedAnswersRunEndedWithoutRunningTheTool(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	if _, err := r.svc.Finish(context.Background(), req, tools.RunFinish{Outcome: outcomeFailed, Message: "gave up"}); err != nil {
		t.Fatal(err)
	}
	dispatched := &dispatchCount{}
	answer := r.call(req, "list_metrics", `{"q":"a"}`, nil, dispatched.wrap(readOK))
	if !answer.IsError || contentCode(t, answer) != "run_ended" || settledOf(answer) != "ended" || dispatched.n != 0 {
		t.Fatalf("answer %+v, dispatched %d", answer, dispatched.n)
	}
}

func TestTwoServicesOnOneDatabaseShareOneSession(t *testing.T) {
	r := newRig(t)
	other := New(Config{DB: r.d.DB, Runs: r.runs, Claims: r.claims, Builds: r.store, Dashboards: r.store, Statuses: healthy{}, Steps: r.steps})
	req := r.revise()
	services := []*Service{r.svc, other}
	for n := range 6 {
		if _, err := services[n%2].Call(context.Background(), req, Call{Name: "list_metrics", Input: json.RawMessage(`{"q":"` + strconv.Itoa(n) + `"}`)}, readOK); err != nil {
			t.Fatal(err)
		}
	}
	if got := r.storedState(req).Counts.ToolCalls; got != 6 {
		t.Fatalf("tool calls = %d, want the 6 of both services", got)
	}
}
