package agent_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
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
	want := tools.Run{Mode: tools.ModeRevise, DashboardID: dashID, ParentVersion: 3, RequestID: revisionRequestID}
	if !calls[0].scoped || calls[0].run != want || calls[0].principal != principal {
		t.Fatalf("call = %+v, want run %+v for %+v", calls[0], want, principal)
	}
	started := r.provider.started()
	if len(started) != 1 || started[0].Prompt != "show this per route" ||
		!strings.Contains(started[0].Context, fmt.Sprintf("The request is a comment on panel %s", panelID)) {
		t.Fatalf("request = %+v", started)
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
			rig:  func(t *testing.T) *rig { return newRig(t, session(stopTurn(llm.StopEndTurn))) },
			want: agent.ErrNoSave.Error(),
		},
		"provider failure": {
			rig: func(t *testing.T) *rig {
				return newRig(t, session(scriptedTurn{err: fmt.Errorf("%w: status 500: %s", llm.ErrProvider, leak)}))
			},
			want: "model call failed",
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
			if err == nil || err.Error() != "model call failed" {
				t.Fatalf("err = %v, want the fixed model call message", err)
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
