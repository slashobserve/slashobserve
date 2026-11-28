package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/claims"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

type fakeDraftQueue struct {
	job     *DraftJob
	reports []DraftReport
	reply   DraftReply
	err     error
}

func (f *fakeDraftQueue) Claim(context.Context, time.Duration) (*DraftJob, error) { return f.job, nil }

func (f *fakeDraftQueue) Finish(_ context.Context, r DraftReport) (DraftReply, error) {
	f.reports = append(f.reports, r)
	return f.reply, f.err
}

var builtInClaimer = &claims.Claimer{Kind: revisions.AssignBuiltin, ID: "server-1"}

func callDraftTool(t *testing.T, queue DraftQueue, claimer *claims.Claimer, name, input string) Result {
	t.Helper()
	ctx := auth.WithPrincipal(t.Context(), auth.Principal{Kind: auth.KindUser, ID: "u1", Name: "n", Role: auth.RoleViewer})
	if claimer != nil {
		ctx = claims.WithClaimer(ctx, *claimer)
	}
	res, err := New(Deps{DraftQueue: queue}).CallClaim(ctx, name, []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestOnlyTheServersOwnAgentTakesDrafts(t *testing.T) {
	queue := &fakeDraftQueue{job: &DraftJob{ID: "d1"}}
	for name, claimer := range map[string]*claims.Claimer{"no claimer": nil, "a connected agent": outsideClaimer} {
		for _, call := range []struct{ tool, input string }{{nameClaimDraft, `{}`}, {nameFinishDraft, `{"id":"d1","failure":"error"}`}} {
			if res := callDraftTool(t, queue, claimer, call.tool, call.input); !res.IsError || !strings.Contains(content(t, res), codeNotAWorker) {
				t.Errorf("%s calling %s = %s, want %s", name, call.tool, res.Content, codeNotAWorker)
			}
		}
	}
	if len(queue.reports) != 0 {
		t.Fatalf("reports reached the queue: %+v", queue.reports)
	}
}

func TestClaimDraftAnswersTheJobOrNull(t *testing.T) {
	res := callDraftTool(t, &fakeDraftQueue{job: &DraftJob{ID: "d1", Prompt: "p", TimeoutSeconds: 60}}, builtInClaimer, nameClaimDraft, `{"wait_seconds":0}`)
	var got struct {
		Draft *DraftJob `json:"draft"`
	}
	if res.IsError || json.Unmarshal([]byte(content(t, res)), &got) != nil || got.Draft == nil || got.Draft.ID != "d1" {
		t.Fatalf("claim = %s", content(t, res))
	}
	res = callDraftTool(t, &fakeDraftQueue{}, builtInClaimer, nameClaimDraft, `{}`)
	if res.IsError || content(t, res) != `{"draft":null}` {
		t.Fatalf("idle claim = %s", content(t, res))
	}
}

func TestFinishDraftSendsATurnWithItsUsageOrAFailure(t *testing.T) {
	queue := &fakeDraftQueue{reply: DraftReply{Results: []DraftResult{{CallID: "c1", IsError: true, Content: "fix"}}}}
	res := callDraftTool(t, queue, builtInClaimer, nameFinishDraft,
		`{"id":"d1","turn":{"stop":"tool_use","tool_calls":[{"id":"c1","name":"submit_query","input":{"query":"x"}}]},"usage":{"input_tokens":40,"output_tokens":4},"duration_ms":1200}`)
	if res.IsError || !strings.Contains(content(t, res), `"results":[{"call_id":"c1","content":"fix","is_error":true}]`) {
		t.Fatalf("finish = %s", content(t, res))
	}
	got := queue.reports[0]
	if got.ID != "d1" || got.Turn == nil || got.Turn.Usage.InputTokens != 40 || got.Turn.ToolCalls[0].Name != "submit_query" || got.DurationMS != 1200 {
		t.Fatalf("report = %+v", got)
	}

	queue = &fakeDraftQueue{reply: DraftReply{Done: true}}
	if res := callDraftTool(t, queue, builtInClaimer, nameFinishDraft, `{"id":"d1","failure":"rate_limited"}`); res.IsError || content(t, res) != `{"done":true}` {
		t.Fatalf("finish with a failure = %s", content(t, res))
	}
	if queue.reports[0].Failure != DraftFailureRateLimited || queue.reports[0].Turn != nil {
		t.Fatalf("report = %+v", queue.reports[0])
	}

	for _, input := range []string{`{"id":"d1"}`, `{"id":"d1","failure":"error","turn":{"stop":"end_turn"}}`} {
		if res := callDraftTool(t, &fakeDraftQueue{}, builtInClaimer, nameFinishDraft, input); !res.IsError {
			t.Errorf("finish %s = %s, want an error", input, content(t, res))
		}
	}
	if res := callDraftTool(t, &fakeDraftQueue{err: ErrDraftEnded}, builtInClaimer, nameFinishDraft, `{"id":"d1","failure":"error"}`); !res.IsError || !strings.Contains(content(t, res), codeNotFound) {
		t.Fatalf("finish of an ended draft = %s", content(t, res))
	}
}
