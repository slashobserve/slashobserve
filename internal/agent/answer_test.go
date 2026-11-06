package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/llm"
)

const findingAfterPanels = "this run already saved panels; finish the dashboard instead of saving a finding"

func refusalMessage(t *testing.T, res llm.ToolResult) string {
	t.Helper()
	var body struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(res.Content), &body); err != nil || !res.IsError || body.Code != "invalid_request" {
		t.Fatalf("result = %+v", res)
	}
	return body.Message
}

var buildModes = []tools.Mode{tools.ModeCreate, tools.ModeRevise}

func (r *rig) taskIn(mode tools.Mode) agent.Task {
	if mode == tools.ModeRevise {
		return r.reviseTask()
	}
	return r.task()
}

func TestAnAnswerToAQuestionEndsTheRunOnTheFinding(t *testing.T) {
	for _, mode := range buildModes {
		t.Run(string(mode), func(t *testing.T) {
			r := newRig(t, session(
				toolTurn(call("c1", "list_labels", `{"datasource":"loki"}`)),
				toolTurn(call("c2", "save_finding", `{"markdown":"Loki holds checkout and payments logs."}`)),
			))
			r.tools.onSave("save_finding", findingSaved("f1"))
			task := r.taskIn(mode)
			task.Prompt = "what all logs are available?"

			out, err := r.agent.Run(context.Background(), task)
			if err != nil || out.FindingID != "f1" || out.Version != 0 {
				t.Fatalf("outcome = %+v, err %v", out, err)
			}
			if r.builds.commits != 0 || r.builds.discards != 0 || r.builds.rebases != 0 {
				t.Errorf("builds touched: %d commits, %d discards, %d rebases", r.builds.commits, r.builds.discards, r.builds.rebases)
			}
			calls := r.tools.recorded()
			if len(calls) != 2 || calls[1].name != "save_finding" || calls[1].run.Mode != mode || calls[1].run.Prompt != task.Prompt {
				t.Fatalf("calls = %+v", calls)
			}
		})
	}
}

func TestAnAnswerAfterASavedPanelIsRefused(t *testing.T) {
	s := session(
		toolTurn(call("c1", "create_dashboard", `{"n":1}`)),
		toolTurn(call("c2", "save_finding", `{"markdown":"answer"}`)),
		stopTurn(llm.StopEndTurn),
	)
	r := newRig(t, s)
	r.tools.on("create_dashboard", func(context.Context, json.RawMessage) (tools.Result, error) {
		r.builds.seed(dashboards.Build{Spec: buildSpec(t, "r1"), Saves: 1})
		return buildSavedResult(1), nil
	})

	out, err := r.agent.Run(context.Background(), r.task())
	if err != nil || out.FindingID != "" || out.Version != 1 || r.builds.commits != 1 {
		t.Fatalf("outcome = %+v, err %v, commits %d", out, err, r.builds.commits)
	}
	if slices.Contains(r.tools.names(), "save_finding") {
		t.Fatalf("the finding handler ran: %v", r.tools.names())
	}
	if got := refusalMessage(t, s.results(2)[0]); got != findingAfterPanels {
		t.Fatalf("refusal = %q", got)
	}
}

func TestAnAnswerInAResumedBuildIsRefusedInTheFirstTurn(t *testing.T) {
	for _, mode := range buildModes {
		t.Run(string(mode), func(t *testing.T) {
			s := session(
				toolTurn(call("c1", "save_finding", `{"markdown":"answer"}`)),
				stopTurn(llm.StopEndTurn),
			)
			r := newRig(t, s)
			r.builds.seed(dashboards.Build{Spec: buildSpec(t, "r1"), BaseVersion: 3, Saves: 1})

			out, err := r.agent.Run(context.Background(), r.taskIn(mode))
			if err != nil || out.FindingID != "" || r.builds.commits != 1 {
				t.Fatalf("outcome = %+v, err %v, commits %d", out, err, r.builds.commits)
			}
			if slices.Contains(r.tools.names(), "save_finding") {
				t.Fatalf("the finding handler ran: %v", r.tools.names())
			}
			if got := refusalMessage(t, s.results(1)[0]); got != findingAfterPanels {
				t.Fatalf("refusal = %q", got)
			}
		})
	}
}

const answerNudge = "Your answer is not shown to anyone. Call save_finding with it, or save a panel."

func TestAnAnswerInTextIsNudgedOnceTowardsAFinding(t *testing.T) {
	for _, mode := range buildModes {
		t.Run(string(mode), func(t *testing.T) {
			s := session(
				stopTurn(llm.StopEndTurn),
				toolTurn(call("c1", "save_finding", `{"markdown":"Loki holds checkout logs."}`)),
			)
			r := newRig(t, s)
			r.tools.onSave("save_finding", findingSaved("f1"))

			out, err := r.agent.Run(context.Background(), r.taskIn(mode))
			if err != nil || out.FindingID != "f1" || out.Compactions != 1 || r.builds.commits != 0 {
				t.Fatalf("outcome = %+v, err %v", out, err)
			}
			if len(s.compactions) != 1 || !strings.Contains(s.compactions[0].Message, answerNudge) {
				t.Fatalf("compactions = %+v", s.compactions)
			}
		})
	}
}

func TestAnAnswerInTextTwiceFailsWithoutAToolCall(t *testing.T) {
	for _, mode := range buildModes {
		t.Run(string(mode), func(t *testing.T) {
			s := session(stopTurn(llm.StopEndTurn), stopTurn(llm.StopEndTurn))
			r := newRig(t, s)

			out, err := r.agent.Run(context.Background(), r.taskIn(mode))
			if !errors.Is(err, agent.ErrNoSave) || out.Usage.Calls != 2 || len(s.compactions) != 1 {
				t.Fatalf("outcome = %+v, err %v, compactions %d", out, err, len(s.compactions))
			}
		})
	}
}
