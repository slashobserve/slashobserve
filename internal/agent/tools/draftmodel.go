package tools

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/claims"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	nameClaimDraft  = "claim_draft"
	nameFinishDraft = "finish_draft"

	DraftStopToolUse   = "tool_use"
	DraftStopEndTurn   = "end_turn"
	DraftStopMaxTokens = "max_tokens"
	DraftStopRefusal   = "refusal"

	DraftFailureCallTimeout   = "call_timeout"
	DraftFailureRateLimited   = "rate_limited"
	DraftFailureProviderError = "provider_error"
	DraftFailureNoFixture     = "no_fixture"
	DraftFailureError         = "error"

	descClaimDraft = "Server's own agent only. Take the next query draft waiting for a model, waiting up to wait_seconds for one. draft is null when none is waiting: call again. A draft has its system text, the question as prompt, the datasource facts as context, the two tools the model answers with, and the model settings. Send each model turn with finish_draft."

	descFinishDraft = "Server's own agent only. Send the model's turn of a draft you claimed, or the failure that stopped the model call. The answer is done when the draft ended, or the tool results to send the model for its next turn, after which you call finish_draft again. A draft ends within timeout_seconds of its claim."

	schemaClaimDraft = `{"type":"object","properties":{` +
		`"wait_seconds":{"type":"integer","minimum":0,"maximum":30,"description":"How long to wait for a draft, 0 to 30 seconds. Default 30."}},"additionalProperties":false}`

	schemaFinishDraft = `{"type":"object","properties":{` +
		`"id":{"type":"string","minLength":1,"description":"The draft id from claim_draft."},` +
		`"turn":{"type":"object","properties":{` +
		`"stop":{"type":"string","enum":["tool_use","end_turn","max_tokens","refusal"]},` +
		`"tool_calls":{"type":"array","maxItems":16,"items":{"type":"object","properties":{` +
		`"id":{"type":"string"},"name":{"type":"string"},"input":{"type":"object"}},"required":["id","name","input"],"additionalProperties":false}}},` +
		`"required":["stop"],"additionalProperties":false,"description":"The model's turn."},` +
		`"failure":{"type":"string","enum":["call_timeout","rate_limited","provider_error","no_fixture","error"],"description":"Why the model call failed, instead of turn."},` +
		`"usage":{"type":"object","properties":{` +
		`"input_tokens":{"type":"integer","minimum":0},"output_tokens":{"type":"integer","minimum":0},` +
		`"cache_read_tokens":{"type":"integer","minimum":0},"cache_write_tokens":{"type":"integer","minimum":0}},"additionalProperties":false,"description":"The model use of this turn."},` +
		`"duration_ms":{"type":"integer","minimum":0,"description":"How long the model call took, retries included."}},` +
		`"required":["id"],"additionalProperties":false}`
)

var (
	ErrDraftStopped = errors.New("the agent stopped before it answered; try again")
	ErrDraftEnded   = errors.New("draft not found; it has ended")
)

type DraftToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type DraftRequest struct {
	System  string
	Tools   []DraftToolSpec
	Prompt  string
	Context string
}

type DraftCall struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

type DraftResult struct {
	CallID  string `json:"call_id"`
	Content string `json:"content"`
	IsError bool   `json:"is_error"`
}

type DraftUsage struct {
	InputTokens      int64 `json:"input_tokens"`
	OutputTokens     int64 `json:"output_tokens"`
	CacheReadTokens  int64 `json:"cache_read_tokens"`
	CacheWriteTokens int64 `json:"cache_write_tokens"`
}

func (u DraftUsage) Add(o DraftUsage) DraftUsage {
	return DraftUsage{
		InputTokens:      u.InputTokens + o.InputTokens,
		OutputTokens:     u.OutputTokens + o.OutputTokens,
		CacheReadTokens:  u.CacheReadTokens + o.CacheReadTokens,
		CacheWriteTokens: u.CacheWriteTokens + o.CacheWriteTokens,
	}
}

type DraftTurn struct {
	Stop      string      `json:"stop"`
	ToolCalls []DraftCall `json:"tool_calls"`
	Usage     DraftUsage  `json:"-"`
}

type DraftModelError struct{ Failure string }

func (e *DraftModelError) Error() string { return "model call failed: " + e.Failure }

type DraftModel interface {
	Available() bool
	Start(ctx context.Context, req DraftRequest) (DraftSession, error)
}

type DraftSession interface {
	Next(ctx context.Context, results []DraftResult) (DraftTurn, error)
	Close()
}

type DraftJob struct {
	ID             string                `json:"id"`
	System         string                `json:"system"`
	Prompt         string                `json:"prompt"`
	Context        string                `json:"context"`
	Tools          []DraftToolSpec       `json:"tools"`
	Model          *claims.ModelSettings `json:"model"`
	TimeoutSeconds int                   `json:"timeout_seconds"`
}

type DraftReport struct {
	ID         string
	Turn       *DraftTurn
	Failure    string
	DurationMS int64
}

type DraftReply struct {
	Done    bool          `json:"done"`
	Results []DraftResult `json:"results,omitempty"`
}

type DraftQueue interface {
	Claim(ctx context.Context, wait time.Duration) (*DraftJob, error)
	Finish(ctx context.Context, report DraftReport) (DraftReply, error)
}

func init() { registerEntries((*Registry).draftClaimEntries) }

func (r *Registry) draftClaimEntries() []entry {
	claim := func(name, desc, schema string, handle func(context.Context, json.RawMessage) (Result, error)) entry {
		return entry{Tool{Name: name, Description: desc, InputSchema: json.RawMessage(schema), Claim: true, AgentOnly: true, MinRole: auth.RoleViewer}, handle}
	}
	return []entry{
		claim(nameClaimDraft, descClaimDraft, schemaClaimDraft, r.claimDraft),
		claim(nameFinishDraft, descFinishDraft, schemaFinishDraft, r.finishDraft),
	}
}

type claimDraftAnswer struct {
	Draft *DraftJob `json:"draft"`
}

func (r *Registry) claimDraft(ctx context.Context, input json.RawMessage) (Result, error) {
	var in claimRunInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	wait, res, ok := boundedInt(in.WaitSeconds, defaultClaimWait, 0, maxClaimWait, "wait_seconds")
	if !ok {
		return res, nil
	}
	return r.asDraftWorker(ctx, func(queue DraftQueue) (Result, error) {
		job, err := queue.Claim(ctx, time.Duration(wait)*time.Second)
		if err != nil {
			return Result{}, err
		}
		if job == nil {
			return success("No draft waiting", claimDraftAnswer{}), nil
		}
		return success("Claimed a draft", claimDraftAnswer{Draft: job}), nil
	})
}

type finishDraftInput struct {
	ID         string      `json:"id"`
	Turn       *DraftTurn  `json:"turn"`
	Failure    string      `json:"failure"`
	Usage      *DraftUsage `json:"usage"`
	DurationMS *int        `json:"duration_ms"`
}

func (r *Registry) finishDraft(ctx context.Context, input json.RawMessage) (Result, error) {
	var in finishDraftInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	if in.ID == "" {
		return invalid("id is required"), nil
	}
	if (in.Turn == nil) == (in.Failure == "") {
		return invalid("send turn or failure, not both"), nil
	}
	if in.Turn != nil && in.Usage != nil {
		in.Turn.Usage = *in.Usage
	}
	report := DraftReport{ID: in.ID, Turn: in.Turn, Failure: in.Failure}
	if in.DurationMS != nil {
		report.DurationMS = int64(*in.DurationMS)
	}
	return r.asDraftWorker(ctx, func(queue DraftQueue) (Result, error) {
		reply, err := queue.Finish(ctx, report)
		if errors.Is(err, ErrDraftEnded) {
			return toolError(codeNotFound, ErrDraftEnded.Error()), nil
		}
		if err != nil {
			return Result{}, err
		}
		if reply.Done {
			return success("Ended a draft", reply), nil
		}
		return success("Answered a draft's model turn", reply), nil
	})
}

func (r *Registry) asDraftWorker(ctx context.Context, use func(DraftQueue) (Result, error)) (Result, error) {
	claimer, ok := claims.ClaimerFrom(ctx)
	if !ok || claimer.Kind != revisions.AssignBuiltin {
		return toolError(codeNotAWorker, "only the server's own agent takes drafts"), nil
	}
	if r.deps.DraftQueue == nil {
		return toolError(codeNotAWorker, "no model is configured on this server"), nil
	}
	return use(r.deps.DraftQueue)
}
