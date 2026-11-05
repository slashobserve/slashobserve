package agent_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/agent"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/llm"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

var (
	windowFrom = time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	windowTo   = time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
)

func (r *rig) investigateTask() agent.Task {
	task := r.task()
	task.Prompt = "why is checkout p99 up"
	task.Investigate = true
	return task
}

func findingSaved(id string) tools.Result {
	return tools.Result{Content: map[string]any{"code": "saved", "finding_id": id, "url": "/findings/" + id, "replayed": false, "evidence": 1}, Step: "Saved finding " + id}
}

func toolNamesOf(specs []llm.ToolSpec) []string {
	names := make([]string, len(specs))
	for i, s := range specs {
		names[i] = s.Name
	}
	return names
}

func TestInvestigationReadsThenEndsOnTheSavedFinding(t *testing.T) {
	r := newRig(t, session(
		toolTurn(call("c1", "query_range", `{"query":"up"}`)),
		toolTurn(call("c2", "save_finding", `{"markdown":"checkout-0 is slow"}`)),
	))
	r.tools.onSave("save_finding", findingSaved("f1"))
	r.builds.seed(dashboards.Build{Spec: buildSpec(t, "r1"), Saves: 1})
	task := r.investigateTask()
	task.DashboardID, task.PanelID, task.From, task.To = dashID, panelID, &windowFrom, &windowTo

	out, err := r.agent.Run(context.Background(), task)
	if err != nil || out.FindingID != "f1" || out.Version != 0 {
		t.Fatalf("outcome = %+v, err %v", out, err)
	}
	if r.builds.commits != 0 || r.builds.discards != 0 || r.builds.rebases != 0 {
		t.Errorf("builds touched: %d commits, %d discards, %d rebases", r.builds.commits, r.builds.discards, r.builds.rebases)
	}
	started := r.provider.started()
	if len(started) != 1 {
		t.Fatalf("sessions = %d", len(started))
	}
	if got, want := toolNamesOf(started[0].Tools), []string{"get_spec_guide", "lint_query", "list_datasources", "query_range", "get_panels", "save_finding"}; !slices.Equal(got, want) {
		t.Errorf("offered %v, want %v", got, want)
	}
	if !strings.HasPrefix(started[0].System, "# Investigating\n") || !strings.Contains(started[0].System, "# Authoring guide\n") {
		t.Errorf("system = %.200q", started[0].System)
	}
	for _, want := range []string{"Window: 2026-11-01T09:00:00Z to 2026-11-01T10:00:00Z", `The question is about panel p_p99 titled "p99 latency"`, "dashboard " + dashID + ", version 3"} {
		if !strings.Contains(started[0].Context, want) {
			t.Errorf("context lacks %q:\n%s", want, started[0].Context)
		}
	}
	if strings.Contains(started[0].Context, "<build>") {
		t.Errorf("the context shows a build:\n%s", started[0].Context)
	}
	calls := r.tools.recorded()
	want := tools.Run{Mode: tools.ModeInvestigate, DashboardID: dashID, ParentVersion: latestVersion, RequestID: requestID, PanelID: panelID, Prompt: task.Prompt, Attempt: 1, From: windowFrom, To: windowTo}
	if len(calls) != 2 || calls[1].run != want {
		t.Fatalf("save_finding ran in %+v, want %+v", calls[len(calls)-1].run, want)
	}
}

func TestBuildSessionsKeepTheirSystemText(t *testing.T) {
	r := newRig(t, session(toolTurn(call("c1", "create_dashboard", `{}`))), session(toolTurn(call("c1", "save_finding", `{}`))))
	r.tools.onSave("create_dashboard", savedResult(dashID, 1))
	r.tools.onSave("save_finding", findingSaved("f1"))
	if _, err := r.agent.Run(context.Background(), r.task()); err != nil {
		t.Fatal(err)
	}
	if _, err := r.agent.Run(context.Background(), r.investigateTask()); err != nil {
		t.Fatal(err)
	}
	started := r.provider.started()
	if strings.Contains(started[0].System, "# Investigating") || started[0].System == started[1].System {
		t.Fatal("a create session got the investigate system text")
	}
	if !strings.Contains(started[1].Context, "Window: the last hour") || strings.Contains(started[1].Context, "dashboard ") {
		t.Errorf("context without a dashboard or window:\n%s", started[1].Context)
	}
}

func TestAnInvestigationAnsweringInTextIsNudgedOnceThenFails(t *testing.T) {
	s := session(stopTurn(llm.StopEndTurn), stopTurn(llm.StopEndTurn))
	r := newRig(t, s)
	req := revisions.Request{ID: requestID, Prompt: "why is checkout slow", Source: revisions.SourceInvestigate, Attempt: 1, RequestedBy: principal}
	result, err := agent.NewReviser(r.agent, nil).Revise(context.Background(), req)
	if err == nil || err.Error() != agent.ErrNoFinding.Error() || result.FailureCode != revisions.FailureNoFinding {
		t.Fatalf("result = %+v, err %v", result, err)
	}
	if len(s.compactions) != 1 || !strings.Contains(s.compactions[0].Message, "Call save_finding now with your answer") {
		t.Fatalf("compactions = %+v", s.compactions)
	}
}

func TestThreeRejectedFindingsInARowEndTheInvestigation(t *testing.T) {
	r := newRig(t, session(
		toolTurn(call("c1", "save_finding", `{"n":1}`)),
		toolTurn(call("c2", "save_finding", `{"n":2}`)),
		toolTurn(call("c3", "save_finding", `{"n":3}`)),
	))
	r.tools.onSave("save_finding", invalidSpecResult("no_data"), invalidSpecResult("no_data"), invalidSpecResult("no_data"))
	_, err := r.agent.Run(context.Background(), r.investigateTask())
	if !errors.Is(err, agent.ErrRepairExhausted) {
		t.Fatalf("err = %v", err)
	}
	steps := r.recordedSteps()
	if last := steps[len(steps)-1]; last.Message != "Stopped: 3 findings in a row were rejected" {
		t.Errorf("last step = %+v", last)
	}
}

func TestAtTheTwelfthCallOnlySaveFindingRuns(t *testing.T) {
	var turns []scriptedTurn
	for i := range 12 {
		turns = append(turns, toolTurn(call(fmt.Sprintf("c%d", i), "query_range", fmt.Sprintf(`{"n":%d}`, i))))
	}
	turns = append(turns, toolTurn(call("save", "save_finding", `{}`)))
	s := session(turns...)
	r := newRig(t, s)
	r.tools.onSave("save_finding", findingSaved("f1"))
	out, err := r.agent.Run(context.Background(), r.investigateTask())
	if err != nil || out.FindingID != "f1" {
		t.Fatalf("outcome = %+v, err %v", out, err)
	}
	if got := s.results(11)[0].Content; !strings.Contains(got, "One more read at most, then call save_finding.") {
		t.Errorf("the eleventh read carries no nudge: %s", got)
	}
	refused := s.results(12)[0]
	if !refused.IsError || !strings.Contains(refused.Content, "12 model calls without a finding; only save_finding can run now.") {
		t.Fatalf("the twelfth read = %+v", refused)
	}
	if n := strings.Count(strings.Join(r.tools.names(), ","), "query_range"); n != 11 {
		t.Errorf("query_range ran %d times", n)
	}
}

type recordedRuns struct {
	mu    sync.Mutex
	kinds []string
}

func (m *recordedRuns) AgentRun(kind, _ string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.kinds = append(m.kinds, kind)
}

type succeeding struct{}

func (succeeding) Revise(context.Context, revisions.Request) (revisions.Result, error) {
	return revisions.Result{}, nil
}

func TestMetricsCountInvestigationsByTheirKind(t *testing.T) {
	m := &recordedRuns{}
	metered := agent.WithMetrics(succeeding{}, m)
	for _, req := range []revisions.Request{
		{Source: revisions.SourceInvestigate, DashboardID: dashID},
		{Source: revisions.SourceObserve},
		{Source: revisions.SourcePrompt, DashboardID: dashID},
	} {
		if _, err := metered.Revise(context.Background(), req); err != nil {
			t.Fatal(err)
		}
	}
	if want := []string{"investigate", "create", "revise"}; !slices.Equal(m.kinds, want) {
		t.Fatalf("kinds = %v, want %v", m.kinds, want)
	}
}
