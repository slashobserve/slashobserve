package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/llm"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

var _ revisions.Reviser = (*agent.Reviser)(nil)

const revisionRequestID = "0190b5c0-0000-7000-8000-0000000000aa"

func revisionRequest() revisions.Request {
	return revisions.Request{
		ID:          revisionRequestID,
		DashboardID: dashID,
		Source:      revisions.SourceComment,
		Prompt:      "show this per route",
		PanelID:     panelID,
		BaseVersion: 2,
		Attempt:     2,
		RequestedBy: principal,
	}
}

func TestReviserRunsTheAgentInReviseModeForTheRequest(t *testing.T) {
	r := newRig(t, session(toolTurn(call("c1", "update_dashboard", `{"spec":{}}`))))
	r.tools.onSave("update_dashboard", savedResult(dashID, 4))

	result, err := agent.NewReviser(r.agent, nil).Revise(context.Background(), revisionRequest())
	if err != nil || result.Version != 4 {
		t.Fatalf("result = %+v, err %v", result, err)
	}
	calls := r.tools.recorded()
	if len(calls) != 1 {
		t.Fatalf("calls = %v", r.tools.names())
	}
	want := tools.Run{Mode: tools.ModeRevise, DashboardID: dashID, ParentVersion: 3, RequestID: revisionRequestID, Prompt: "show this per route", Build: true, Attempt: 2}
	if !calls[0].scoped || calls[0].run != want || !reflect.DeepEqual(calls[0].principal, principal) {
		t.Fatalf("call = %+v, want run %+v for %+v", calls[0], want, principal)
	}
	started := r.provider.started()
	if len(started) != 1 || started[0].Prompt != "show this per route" ||
		!strings.Contains(started[0].Context, fmt.Sprintf("The request is a comment on panel %s", panelID)) {
		t.Fatalf("request = %+v", started)
	}
}

func TestReviserTellsTheAgentAPromptIsAboutAPanel(t *testing.T) {
	r := newRig(t, session(toolTurn(call("c1", "update_dashboard", `{"spec":{}}`))))
	r.tools.onSave("update_dashboard", savedResult(dashID, 4))
	req := revisionRequest()
	req.Source = revisions.SourcePrompt

	if _, err := agent.NewReviser(r.agent, nil).Revise(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if got := r.provider.started()[0].Context; !strings.Contains(got, fmt.Sprintf("The request is about panel %s", panelID)) {
		t.Fatalf("context = %s", got)
	}
}

func TestReviserStoresOnlyTheAgentsOwnMessages(t *testing.T) {
	leak := "SECRET sk-live-123 db.internal:5432"
	for name, c := range map[string]struct {
		rig  func(t *testing.T) *rig
		want string
	}{
		"refusal": {
			rig:  func(t *testing.T) *rig { return newRig(t, session(stopTurn(llm.StopRefusal))) },
			want: agent.ErrRefused.Error(),
		},
		"no save": {
			rig: func(t *testing.T) *rig {
				return newRig(t, session(stopTurn(llm.StopEndTurn), stopTurn(llm.StopEndTurn)))
			},
			want: agent.ErrNoSave.Error(),
		},
		"provider failure": {
			rig: func(t *testing.T) *rig {
				return newRig(t, session(scriptedTurn{err: fmt.Errorf("%w: status 500: %s", llm.ErrProvider, leak)}))
			},
			want: "the model provider returned an error",
		},
		"missing replay fixture": {
			rig: func(t *testing.T) *rig {
				r := newRig(t)
				r.provider.startErr = llm.ErrNoFixture
				return r
			},
			want: "model call failed: no replay fixture",
		},
		"internal tool error": {
			rig: func(t *testing.T) *rig {
				r := newRig(t, session(toolTurn(call("c1", "update_dashboard", `{}`))))
				r.tools.onSave("update_dashboard")
				return r
			},
			want: "the update_dashboard tool failed with an internal error",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := agent.NewReviser(c.rig(t).agent, nil).Revise(context.Background(), revisionRequest())
			if err == nil || err.Error() != c.want {
				t.Fatalf("err = %v, want %q", err, c.want)
			}
			if strings.Contains(err.Error(), "SECRET") {
				t.Fatalf("the stored error leaks provider text: %v", err)
			}
		})
	}
}

func TestReviserStoresNoProviderTextWhenTheProviderTimesOut(t *testing.T) {
	endpoint := `Post "https://llm.internal.example/v1/messages?key=sk-live-123": `
	for name, sentinel := range map[string]error{
		"deadline": context.DeadlineExceeded,
		"canceled": context.Canceled,
	} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t, session(scriptedTurn{err: fmt.Errorf("%s%w", endpoint, sentinel)}))
			_, err := agent.NewReviser(r.agent, nil).Revise(context.Background(), revisionRequest())
			if err == nil || err.Error() != "the model provider returned an error" {
				t.Fatalf("err = %v, want the fixed provider error message", err)
			}
			if !errors.Is(err, sentinel) {
				t.Fatalf("err = %v lost %v", err, sentinel)
			}
		})
	}
}

func TestReviserPassesCancellationThrough(t *testing.T) {
	r := newRig(t, session(toolTurn(call("c1", "get_spec_guide", `{}`))))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := agent.NewReviser(r.agent, nil).Revise(ctx, revisionRequest())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}

func TestReviserNamesTheFailureOfEveryRun(t *testing.T) {
	provider500 := llm.NewCallError("fake", llm.ProviderError, 500)
	for name, c := range map[string]struct {
		rig         func(t *testing.T) *rig
		wantMessage string
		wantCode    revisions.FailureCode
		wantUsage   bool
	}{
		"rate limited": {
			rig: func(t *testing.T) *rig {
				return newRig(t, session(scriptedTurn{err: llm.NewCallError("fake", llm.RateLimited, 429)}))
			},
			wantMessage: "the model provider is rate limiting requests",
			wantCode:    revisions.FailureRateLimited,
			wantUsage:   true,
		},
		"call timeout": {
			rig: func(t *testing.T) *rig {
				return newRig(t, session(timedOut(), timedOut()))
			},
			wantMessage: "the model did not answer in time",
			wantCode:    revisions.FailureCallTimeout,
			wantUsage:   true,
		},
		"provider error": {
			rig:         func(t *testing.T) *rig { return newRig(t, session(scriptedTurn{err: provider500})) },
			wantMessage: "the model provider returned an error",
			wantCode:    revisions.FailureProviderError,
			wantUsage:   true,
		},
		"unclassified provider error": {
			rig: func(t *testing.T) *rig {
				return newRig(t, session(scriptedTurn{err: fmt.Errorf("%w: openai returned no choices", llm.ErrProvider)}))
			},
			wantMessage: "the model provider returned an error",
			wantCode:    revisions.FailureProviderError,
			wantUsage:   true,
		},
		"output limit": {
			rig: func(t *testing.T) *rig {
				return newRig(t, session(stopTurn(llm.StopMaxTokens), stopTurn(llm.StopMaxTokens)))
			},
			wantMessage: agent.ErrNoSave.Error() + " after reaching max tokens",
			wantCode:    revisions.FailureOutputLimit,
			wantUsage:   true,
		},
		"no tool call": {
			rig: func(t *testing.T) *rig {
				return newRig(t, session(stopTurn(llm.StopEndTurn), stopTurn(llm.StopEndTurn)))
			},
			wantMessage: agent.ErrNoSave.Error(),
			wantCode:    revisions.FailureNoToolCall,
			wantUsage:   true,
		},
		"refusal": {
			rig:         func(t *testing.T) *rig { return newRig(t, session(stopTurn(llm.StopRefusal))) },
			wantMessage: agent.ErrRefused.Error(),
			wantCode:    revisions.FailureRefused,
			wantUsage:   true,
		},
		"missing replay fixture": {
			rig: func(t *testing.T) *rig {
				r := newRig(t)
				r.provider.startErr = llm.ErrNoFixture
				return r
			},
			wantMessage: "model call failed: no replay fixture",
			wantCode:    revisions.FailureNoFixture,
		},
	} {
		t.Run(name, func(t *testing.T) {
			result, err := agent.NewReviser(c.rig(t).agent, nil).Revise(context.Background(), revisionRequest())
			if err == nil || err.Error() != c.wantMessage {
				t.Fatalf("err = %v, want %q", err, c.wantMessage)
			}
			if result.FailureCode != c.wantCode {
				t.Fatalf("failure code = %q, want %q", result.FailureCode, c.wantCode)
			}
			if (result.Usage != nil) != c.wantUsage {
				t.Fatalf("usage = %+v, want non-null %v", result.Usage, c.wantUsage)
			}
		})
	}
}

func TestReviserLeavesTheFailureCodeEmptyOnSuccess(t *testing.T) {
	r := newRig(t, session(toolTurn(call("c1", "update_dashboard", `{"spec":{}}`))))
	r.tools.onSave("update_dashboard", savedResult(dashID, 4))
	result, err := agent.NewReviser(r.agent, nil).Revise(context.Background(), revisionRequest())
	if err != nil || result.FailureCode != "" || result.Usage == nil || result.Usage.ToolCalls != 1 || result.Usage.MaxSaveOutputTokens != 10 {
		t.Fatalf("result = %+v (usage %+v), err %v", result, result.Usage, err)
	}
}

func TestReviserReportsTheRunCostInTheUsage(t *testing.T) {
	run := func(t *testing.T, configure func(*agent.Config)) *revisions.Usage {
		t.Helper()
		r := newRigWithConfig(t, configure, session(toolTurn(call("c1", "update_dashboard", `{"spec":{}}`))))
		r.tools.onSave("update_dashboard", savedResult(dashID, 4))
		result, err := agent.NewReviser(r.agent, nil).Revise(context.Background(), revisionRequest())
		if err != nil || result.Usage == nil {
			t.Fatalf("result = %+v, err %v", result, err)
		}
		return result.Usage
	}

	priced := run(t, func(c *agent.Config) { c.Prices = fakeModelPrices(t) })
	const want = (100*2.00 + 10*10.00 + 5*0.20 + 2*2.50) / 1e6
	if priced.CostUSD == nil || math.Abs(*priced.CostUSD-want) > 1e-12 {
		t.Fatalf("cost = %v, want %v", priced.CostUSD, want)
	}
	if unpriced := run(t, func(*agent.Config) {}); unpriced.CostUSD != nil {
		t.Fatalf("cost = %v, want none for a model with no price", *unpriced.CostUSD)
	}
}

func TestReviserReportsBuildSavesForACompactionInTheUsage(t *testing.T) {
	plan := planOf("r1", "r2")
	r := newRig(t, session(toolTurn(call("c1", "update_panels", `{"n":1}`)), stopTurn(llm.StopMaxTokens), toolTurn(call("c2", "update_panels", `{"n":2,"finish":true}`))))
	r.buildSaves(t, r.rows(t, plan, "r1"), saved(4))

	result, err := agent.NewReviser(r.agent, nil).Revise(context.Background(), revisionRequest())
	if err != nil || result.Version != 4 || result.Usage == nil || result.Usage.BuildSaves != 2 {
		t.Fatalf("result = %+v (usage %+v), err %v", result, result.Usage, err)
	}
}

func TestReviserNamesNoProgressAndStoresItsFixedMessage(t *testing.T) {
	r := newRig(t, session(
		toolTurn(call("c1", "update_panels", `{"n":1}`)),
		toolTurn(call("c2", "update_panels", `{"n":1}`)),
		toolTurn(call("c3", "update_panels", `{"n":1}`)),
	))
	r.buildSaves(t, r.rows(t, planOf("r1", "r2"), "r1"))

	result, err := agent.NewReviser(r.agent, nil).Revise(context.Background(), revisionRequest())
	if err == nil || err.Error() != agent.ErrNoProgress.Error() || result.FailureCode != revisions.FailureNoProgress {
		t.Fatalf("result = %+v, err %v", result, err)
	}
}

func TestReviserCarriesTheContinuationAndItsNoteIntoTheSeed(t *testing.T) {
	for _, tc := range []struct {
		name      string
		continues bool
		note      string
		attempt   int
		sentence  string
	}{
		{"first attempt of a continuation with a note", true, "add a latency row", 1, "the person asked you to continue it from the build above"},
		{"first attempt of a continuation without a note", true, "", 1, "the person asked you to continue it from the build above"},
		{"second attempt of a continuation", true, "add a latency row", 2, "The server restarted while you worked."},
		{"ordinary request", false, "", 1, "The server restarted while you worked."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t, session(toolTurn(call("c1", "update_panels", `{}`))))
			r.builds.seed(agentBuild(t, "r1"))
			r.tools.onSave("update_panels", savedResult(dashID, 4))
			req := revisionRequest()
			req.Attempt, req.Continues, req.ContinueNote = tc.attempt, tc.continues, tc.note

			if _, err := agent.NewReviser(r.agent, nil).Revise(context.Background(), req); err != nil {
				t.Fatal(err)
			}
			got := r.provider.started()[0].Context
			noteLine := "The person's note for this run: " + tc.note
			wantNotes := 0
			if tc.note != "" {
				wantNotes = 1
			}
			if !strings.Contains(got, tc.sentence) || strings.Count(got, "The person's note for this run:") != wantNotes || (wantNotes == 1 && !strings.Contains(got, noteLine)) {
				t.Fatalf("context:\n%s", got)
			}
		})
	}
}

func TestReviseReportsALostBuildAsALostLease(t *testing.T) {
	r := newRig(t, session(toolTurn(call("c1", "create_dashboard", `{}`))))
	r.tools.on("create_dashboard", func(context.Context, json.RawMessage) (tools.Result, error) {
		return tools.Result{}, fmt.Errorf("tool create_dashboard: %w", dashboards.ErrBuildLost)
	})

	result, err := agent.NewReviser(r.agent, nil).Revise(context.Background(), revisionRequest())
	if !errors.Is(err, revisions.ErrLeaseLost) || result.FailureCode != revisions.FailureLeaseExpired {
		t.Fatalf("result = %+v, err %v", result, err)
	}
}
