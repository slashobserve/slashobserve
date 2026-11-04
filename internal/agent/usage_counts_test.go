package agent_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/llm"
)

type counts struct {
	BuildSaves, Reseeds, Renewals, Compactions, Nudges, SaveNowRefusals, StalledTurns, OnePanelRefusals, Retries int
}

func countsOf(out agent.Outcome) counts {
	return counts{
		BuildSaves: out.BuildSaves, Reseeds: out.Reseeds, Renewals: out.Renewals, Compactions: out.Compactions,
		Nudges: out.Nudges, SaveNowRefusals: out.SaveNowRefusals, StalledTurns: out.StalledTurns,
		OnePanelRefusals: out.OnePanelRefusals, Retries: out.Retries,
	}
}

func twoPanelRefusal() func() tools.Result {
	return func() tools.Result {
		return tools.Result{
			IsError: true,
			Content: map[string]any{"code": "invalid_request", "message": "one panel per save: this save adds or changes 2 panels (a, b); save them one at a time"},
			Step:    "Rejected update_panels input: one panel per save",
			Refusal: tools.RefusalOnePanel,
		}
	}
}

func bigGuide(r *rig, kib int) {
	big := strings.Repeat("x", kib<<10)
	r.tools.on("get_spec_guide", func(context.Context, json.RawMessage) (tools.Result, error) {
		return tools.Result{Content: map[string]any{"text": big}}, nil
	})
}

func TestUsageCountersOfEachDecisionPoint(t *testing.T) {
	cases := []struct {
		name       string
		renewBytes int
		sessions   func() []*scriptedSession
		saves      func(r *rig) []func() tools.Result
		prepare    func(r *rig)
		want       counts
	}{
		{
			name: "six reads then a save is one save-now refusal, one stalled turn and the nudge before it",
			sessions: func() []*scriptedSession {
				return []*scriptedSession{session(append(readTurns("r", 6), toolTurn(call("s", "create_dashboard", `{"n":1}`)), stopTurn(llm.StopEndTurn))...)}
			},
			saves: func(r *rig) []func() tools.Result { return []func() tools.Result{r.rows(t, 1, "r1")} },
			want:  counts{BuildSaves: 1, Nudges: 1, SaveNowRefusals: 1, StalledTurns: 1},
		},
		{
			name: "five reads then a save is only the nudge",
			sessions: func() []*scriptedSession {
				return []*scriptedSession{session(append(readTurns("r", 5), toolTurn(call("s", "create_dashboard", `{"n":1}`)), stopTurn(llm.StopEndTurn))...)}
			},
			saves: func(r *rig) []func() tools.Result { return []func() tools.Result{r.rows(t, 1, "r1")} },
			want:  counts{BuildSaves: 1, Nudges: 1},
		},
		{
			name: "a two-panel save then two single saves is one one-panel refusal",
			sessions: func() []*scriptedSession {
				return []*scriptedSession{session(
					toolTurn(call("s1", "create_dashboard", `{"n":1}`)),
					toolTurn(call("s2", "update_panels", `{"n":2}`)),
					toolTurn(call("s3", "update_panels", `{"n":3}`)),
					toolTurn(call("s4", "update_panels", `{"n":4,"finish":true}`)),
				)}
			},
			saves: func(r *rig) []func() tools.Result {
				return []func() tools.Result{r.rows(t, 3, "r1"), twoPanelRefusal(), r.rows(t, 3, "r1", "r2"), saved(1)}
			},
			want: counts{BuildSaves: 3, OnePanelRefusals: 1},
		},
		{
			name:       "a renewal is one compaction and one renewal, and no reseed",
			renewBytes: 64 << 10,
			sessions: func() []*scriptedSession {
				return []*scriptedSession{session(toolTurn(readOf("big", 1)), toolTurn(call("s1", "update_panels", `{"n":1}`)), toolTurn(call("s2", "update_panels", `{"n":2,"finish":true}`)))}
			},
			prepare: func(r *rig) { bigGuide(r, 70) },
			saves:   func(r *rig) []func() tools.Result { return []func() tools.Result{r.rows(t, 2, "r1"), saved(1)} },
			want:    counts{BuildSaves: 2, Renewals: 1, Compactions: 1},
		},
		{
			name: "the first cut-off is a compaction and not a renewal",
			sessions: func() []*scriptedSession {
				return []*scriptedSession{session(stopTurn(llm.StopMaxTokens), toolTurn(call("s", "create_dashboard", `{"n":1}`)), stopTurn(llm.StopEndTurn))}
			},
			saves: func(r *rig) []func() tools.Result { return []func() tools.Result{r.rows(t, 1, "r1")} },
			want:  counts{BuildSaves: 1, Compactions: 1},
		},
		{
			name: "an early stop with panels still planned is a reseed and no compaction",
			sessions: func() []*scriptedSession {
				return []*scriptedSession{
					session(toolTurn(call("s1", "create_dashboard", `{"n":1}`)), stopTurn(llm.StopEndTurn)),
					session(stopTurn(llm.StopEndTurn)),
				}
			},
			saves: func(r *rig) []func() tools.Result { return []func() tools.Result{r.rows(t, 2, "r1")} },
			want:  counts{BuildSaves: 1, Reseeds: 1},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRigWithRenewBytes(t, c.renewBytes, c.sessions()...)
			if c.prepare != nil {
				c.prepare(r)
			}
			r.buildSaves(t, c.saves(r)...)

			out, err := r.agent.Run(context.Background(), r.task())
			if err != nil {
				t.Fatalf("err = %v, outcome %+v", err, out)
			}
			if got := countsOf(out); got != c.want {
				t.Fatalf("counts = %+v, want %+v", got, c.want)
			}
		})
	}
}

func TestStalledTurnsAreCountedWhenTheRunEndsWithNoProgress(t *testing.T) {
	r := newRig(t, session(readTurns("r", 8)...))

	out, err := r.agent.Run(context.Background(), r.task())
	if err == nil {
		t.Fatal("run succeeded")
	}
	want := counts{Nudges: 1, SaveNowRefusals: 2, StalledTurns: 2}
	if got := countsOf(out); got != want {
		t.Fatalf("counts = %+v, want %+v", got, want)
	}
}

func TestAProviderRetryCountsOneRetry(t *testing.T) {
	r := newRig(t)
	provider := &sessionProvider{sessions: []llm.Session{
		&attemptingSession{statuses: []int{http.StatusServiceUnavailable, http.StatusOK}, turn: llm.Turn{ToolCalls: []llm.ToolCall{call("c1", "create_dashboard", `{}`)}, Stop: llm.StopToolUse, Usage: usage()}},
	}}
	r.tools.onSave("create_dashboard", savedResult(dashID, 1))
	ag := agent.New(agent.Config{Provider: provider, Tools: r.tools, Dashboards: r.dash, Builds: r.builds, Datasources: fakeHealth{}, Logger: slog.New(slog.NewJSONHandler(r.logs, nil))})

	out, err := ag.Run(context.Background(), r.task())
	if err != nil {
		t.Fatal(err)
	}
	if out.Retries != 1 {
		t.Fatalf("retries = %d, want 1", out.Retries)
	}
}

func TestTheFinishLogAndTheUsageCarryTheNewCounters(t *testing.T) {
	turns := append(readTurns("r", 6), toolTurn(call("s", "update_panels", `{"n":1,"finish":true}`)))
	r := newRig(t, session(turns...))
	r.buildSaves(t, saved(4))

	result, err := agent.NewReviser(r.agent, nil).Revise(context.Background(), revisionRequest())
	if err != nil || result.Usage == nil {
		t.Fatalf("result = %+v, err %v", result, err)
	}
	u := result.Usage
	if u.Nudges != 1 || u.SaveNowRefusals != 1 || u.StalledTurns != 1 || u.Renewals != 0 || u.Compactions != 0 || u.OnePanelRefusals != 0 {
		t.Fatalf("usage = %+v", u)
	}
	entry := r.finishLog()
	for key, want := range map[string]float64{"nudges": 1, "save_now_refusals": 1, "stalled_turns": 1, "renewals": 0, "compactions": 0, "one_panel_refusals": 0} {
		if entry[key] != want {
			t.Fatalf("finish log %s = %v, want %v", key, entry[key], want)
		}
	}
}
