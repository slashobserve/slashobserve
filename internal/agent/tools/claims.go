package tools

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/claims"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	nameClaimRun    = "claim_run"
	nameReportRun   = "report_run"
	nameFinishRun   = "finish_run"
	nameGetRunBrief = "get_run_brief"

	defaultClaimWait = 30
	maxClaimWait     = 30
	maxRunTextRunes  = 500
	maxLiveTextRunes = 4000

	failureLiveTextAppend = "live_text_append_failed"

	codeNotAWorker = "not_a_worker"
	codeNotYourRun = "not_your_run"
	codeRunEnded   = "run_ended"

	descClaimRun = "Take the next SlashObserve run assigned to you, waiting up to wait_seconds for one. run is null when none is waiting: call again. A claimed run has its prompt and target, a session url and token for a second MCP connection that acts for the person who asked, and the observe prompt with the run's kind, which is the system text for the run. While you work, call report_run at least every 30 seconds, and call finish_run when you are done. Only connected agents and service accounts that take runs can claim runs."

	descReportRun = "Keep the lease of a run you claimed and say what you are doing. The lease ends 60 seconds after the last call. The answer has the run's state and the saved panel progress; when stop is true the run was canceled or has ended: stop and call finish_run. Optional: text is the end of what your model is writing, shown as live text and never stored; usage is your cumulative model use, and the server stops the run at its cost limit."

	descFinishRun = "End a run you claimed. Send outcome succeeded after you saved its result with the run session's tools; a run that saved a dashboard version, a proposal or a finding ends succeeded whatever you send, and one that saved nothing ends failed. Send outcome failed with a message when you could not. usage is your own model and cost, shown on the run as you report it. Optional: commit saves the finished dashboard first; restarted is true when the dashboard changed meanwhile and the run restarted from its latest version, so call get_run_brief and go on."

	descGetRunBrief = "Optional. The first message the server's own agent starts each model session with: the system text, the user message and the run's saved progress. Starting a session with it resets the read guard and pins the parent version; an agent that does not call it keeps working from claim_run's prompt."

	schemaClaimRun = `{"type":"object","properties":{` +
		`"wait_seconds":{"type":"integer","minimum":0,"maximum":30,"description":"How long to wait for a run, 0 to 30 seconds. Default 30."}},"additionalProperties":false}`

	schemaRunUsage = `{"type":"object","description":"Your cumulative model use for this run.","properties":{` +
		`"provider":{"type":"string"},"model":{"type":"string"},` +
		`"input_tokens":{"type":"integer","minimum":0},"output_tokens":{"type":"integer","minimum":0},` +
		`"cache_read_tokens":{"type":"integer","minimum":0},"cache_write_tokens":{"type":"integer","minimum":0},` +
		`"cost_usd":{"type":"number","minimum":0},` +
		`"model_calls":{"type":"integer","minimum":0},"retries":{"type":"integer","minimum":0},` +
		`"compactions":{"type":"integer","minimum":0},"renewals":{"type":"integer","minimum":0},` +
		`"model_ms":{"type":"integer","minimum":0},` +
		`"max_save_output_tokens":{"type":"integer","minimum":0}},"additionalProperties":false}`

	schemaGetRunBrief = `{"type":"object","properties":{` +
		`"run_id":{"type":"string","format":"uuid","description":"The run id from claim_run."},` +
		`"attempt":{"type":"integer","minimum":1,"description":"The attempt from claim_run."},` +
		`"reason":{"type":"string","enum":["restart","output_limit","pending","conflict","continue","renewed","call_timeout","no_finding","no_answer"],"description":"Why a model session starts again; leave it out for the first."}},` +
		`"required":["run_id","attempt"],"additionalProperties":false}`

	schemaReportRun = `{"type":"object","properties":{` +
		`"run_id":{"type":"string","format":"uuid","description":"The run id from claim_run."},` +
		`"attempt":{"type":"integer","minimum":1,"description":"The attempt from claim_run."},` +
		`"note":{"type":"string","maxLength":500,"description":"One line on what you are doing, shown in the run's steps."},` +
		`"text":{"type":"string","maxLength":4000,"description":"The end of what your model is writing, shown as live text while the run is running and never stored."},` +
		`"usage":` + schemaRunUsage + `},` +
		`"required":["run_id","attempt"],"additionalProperties":false}`

	schemaFinishRun = `{"type":"object","properties":{` +
		`"run_id":{"type":"string","format":"uuid","description":"The run id from claim_run."},` +
		`"attempt":{"type":"integer","minimum":1,"description":"The attempt from claim_run."},` +
		`"outcome":{"type":"string","enum":["succeeded","failed"],"description":"succeeded once saved, failed when you could not."},` +
		`"message":{"type":"string","maxLength":500,"description":"Why the run failed, shown on the run."},` +
		`"failure_code":{"type":"string","description":"Model-side failure: call_timeout, rate_limited, provider_error, output_limit, no_tool_call, no_finding, refused, no_fixture or internal."},` +
		`"commit":{"type":"boolean","description":"Save the finished dashboard first."},` +
		`"usage":` + schemaRunUsage + `},` +
		`"required":["run_id","attempt","outcome"],"additionalProperties":false}`
)

var errClaimsNotConfigured = errors.New("claims are not configured")

type ClaimService interface {
	Claim(ctx context.Context, c claims.Claimer, wait time.Duration) (claims.Offer, error)
	ClaimedRun(ctx context.Context, c claims.Claimer, runID string, attempt int) (revisions.Request, error)
	Report(ctx context.Context, c claims.Claimer, in claims.ReportInput) (claims.ReportAnswer, error)
}

type LiveText interface {
	Append(ctx context.Context, runID string, attempt int, text string) error
}

func init() { registerEntries((*Registry).claimEntries) }

func (r *Registry) claimEntries() []entry {
	claim := func(name, desc, schema string, handle func(context.Context, json.RawMessage) (Result, error)) entry {
		return entry{Tool{Name: name, Description: desc, InputSchema: json.RawMessage(schema), MCPOnly: true, Claim: true, MinRole: auth.RoleViewer}, handle}
	}
	return []entry{
		claim(nameClaimRun, descClaimRun, schemaClaimRun, r.claimRun),
		claim(nameReportRun, descReportRun, schemaReportRun, r.reportRun),
		claim(nameFinishRun, descFinishRun, schemaFinishRun, r.finishRun),
		claim(nameGetRunBrief, descGetRunBrief, schemaGetRunBrief, r.getRunBrief),
	}
}

type claimRunInput struct {
	WaitSeconds *int `json:"wait_seconds"`
}

type runUsage struct {
	Provider            string   `json:"provider"`
	Model               string   `json:"model"`
	InputTokens         int64    `json:"input_tokens"`
	OutputTokens        int64    `json:"output_tokens"`
	CacheReadTokens     int64    `json:"cache_read_tokens"`
	CacheWriteTokens    int64    `json:"cache_write_tokens"`
	CostUSD             *float64 `json:"cost_usd"`
	ModelCalls          int      `json:"model_calls"`
	Retries             int      `json:"retries"`
	Compactions         int      `json:"compactions"`
	Renewals            int      `json:"renewals"`
	ModelMS             int64    `json:"model_ms"`
	MaxSaveOutputTokens int64    `json:"max_save_output_tokens"`
}

func (u *runUsage) reported() *revisions.Usage {
	if u == nil {
		return nil
	}
	return &revisions.Usage{
		Provider: u.Provider, Model: u.Model, InputTokens: u.InputTokens, OutputTokens: u.OutputTokens,
		CacheReadTokens: u.CacheReadTokens, CacheWriteTokens: u.CacheWriteTokens, CostUSD: u.CostUSD,
		ModelCalls: u.ModelCalls, Retries: u.Retries, Compactions: u.Compactions, Renewals: u.Renewals,
		ModelMS: u.ModelMS, MaxSaveOutputTokens: u.MaxSaveOutputTokens,
	}
}

type reportRunInput struct {
	RunID   string    `json:"run_id"`
	Attempt int       `json:"attempt"`
	Note    string    `json:"note"`
	Text    string    `json:"text"`
	Usage   *runUsage `json:"usage"`
}

type reportRunOut struct {
	claims.ReportAnswer
	Build *BuildProgress `json:"build"`
}

type finishRunInput struct {
	RunID       string    `json:"run_id"`
	Attempt     int       `json:"attempt"`
	Outcome     string    `json:"outcome"`
	Message     string    `json:"message"`
	FailureCode string    `json:"failure_code"`
	Commit      bool      `json:"commit"`
	Usage       *runUsage `json:"usage"`
}

type getRunBriefInput struct {
	RunID   string `json:"run_id"`
	Attempt int    `json:"attempt"`
	Reason  string `json:"reason"`
}

func (r *Registry) claimRun(ctx context.Context, input json.RawMessage) (Result, error) {
	var in claimRunInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	wait, res, ok := boundedInt(in.WaitSeconds, defaultClaimWait, 0, maxClaimWait, "wait_seconds")
	if !ok {
		return res, nil
	}
	return r.asClaimer(ctx, func(claimer claims.Claimer) (Result, error) {
		offer, err := r.deps.Claims.Claim(ctx, claimer, time.Duration(wait)*time.Second)
		if err != nil {
			return claimFailure(err)
		}
		if offer.Run == nil {
			return success("No run waiting", offer), nil
		}
		return success("Claimed a run", offer), nil
	})
}

func (r *Registry) getRunBrief(ctx context.Context, input json.RawMessage) (Result, error) {
	var in getRunBriefInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	if res, ok := checkRunRef(in.RunID, in.Attempt); !ok {
		return res, nil
	}
	return r.asClaimedRun(ctx, in.RunID, in.Attempt, func(claimer claims.Claimer, req revisions.Request) (Result, error) {
		answer, err := r.deps.RunSessions.Brief(ctx, req, in.Reason)
		if err != nil {
			return claimFailure(err)
		}
		return success("Read a run's brief", answer), nil
	})
}

func (r *Registry) reportRun(ctx context.Context, input json.RawMessage) (Result, error) {
	var in reportRunInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	if res, ok := checkRunRef(in.RunID, in.Attempt); !ok {
		return res, nil
	}
	if res, ok := checkRunText("note", in.Note, maxRunTextRunes); !ok {
		return res, nil
	}
	if res, ok := checkRunText("text", in.Text, maxLiveTextRunes); !ok {
		return res, nil
	}
	return r.asClaimedRun(ctx, in.RunID, in.Attempt, func(claimer claims.Claimer, req revisions.Request) (Result, error) {
		answer, err := r.report(ctx, claimer, req, in)
		if err != nil {
			return claimFailure(err)
		}
		return success("Reported on a run", answer), nil
	})
}

func (r *Registry) report(ctx context.Context, claimer claims.Claimer, req revisions.Request, in reportRunInput) (reportRunOut, error) {
	ref := claims.ReportInput{RunID: in.RunID, Attempt: in.Attempt, Note: in.Note}
	answer, err := r.deps.Claims.Report(ctx, claimer, ref)
	if err != nil || answer.Stop {
		return reportRunOut{ReportAnswer: answer}, err
	}
	if usage := in.Usage.reported(); usage != nil {
		stop, err := r.deps.RunSessions.NoteUsage(ctx, req, *usage)
		if err != nil {
			return reportRunOut{}, err
		}
		if stop {
			answer, err = r.deps.Claims.Report(ctx, claimer, claims.ReportInput{RunID: in.RunID, Attempt: in.Attempt})
			answer.Stop = true
			return reportRunOut{ReportAnswer: answer}, err
		}
	}
	r.appendLiveText(ctx, req, in.Text)
	build, err := r.deps.RunSessions.Progress(ctx, req)
	return reportRunOut{ReportAnswer: answer, Build: build}, err
}

func (r *Registry) appendLiveText(ctx context.Context, req revisions.Request, text string) {
	if r.deps.LiveText == nil || text == "" {
		return
	}
	if err := r.deps.LiveText.Append(ctx, req.ID, req.Attempt, text); err != nil {
		r.deps.Logger.WarnContext(ctx, "live text not appended", logattrs.Failure(failureLiveTextAppend, err, slog.String("agent_request_id", req.ID))...)
	}
}

func (r *Registry) finishRun(ctx context.Context, input json.RawMessage) (Result, error) {
	var in finishRunInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	if res, ok := checkRunRef(in.RunID, in.Attempt); !ok {
		return res, nil
	}
	if in.Outcome != "succeeded" && in.Outcome != "failed" {
		return invalid("outcome must be succeeded or failed"), nil
	}
	if res, ok := checkRunText("message", in.Message, maxRunTextRunes); !ok {
		return res, nil
	}
	return r.asClaimedRun(ctx, in.RunID, in.Attempt, func(claimer claims.Claimer, req revisions.Request) (Result, error) {
		answer, err := r.deps.RunSessions.Finish(ctx, req, RunFinish{
			Outcome: in.Outcome, FailureCode: in.FailureCode, Message: in.Message, Commit: in.Commit, Usage: in.Usage.reported(),
		})
		if err != nil {
			return claimFailure(err)
		}
		return success("Ended a run", answer), nil
	})
}

func (r *Registry) asClaimer(ctx context.Context, use func(claims.Claimer) (Result, error)) (Result, error) {
	claimer, ok := claims.ClaimerFrom(ctx)
	if !ok {
		return toolError(codeNotAWorker, "only a connected agent or a service account that takes runs can claim runs"), nil
	}
	if r.deps.Claims == nil {
		return Result{}, errClaimsNotConfigured
	}
	return use(claimer)
}

func (r *Registry) asClaimedRun(ctx context.Context, runID string, attempt int, use func(claims.Claimer, revisions.Request) (Result, error)) (Result, error) {
	return r.asClaimer(ctx, func(claimer claims.Claimer) (Result, error) {
		if r.deps.RunSessions == nil {
			return Result{}, errClaimsNotConfigured
		}
		req, err := r.deps.Claims.ClaimedRun(ctx, claimer, runID, attempt)
		if err != nil {
			return claimFailure(err)
		}
		return use(claimer, req)
	})
}

func checkRunRef(runID string, attempt int) (Result, bool) {
	if _, err := db.ParseUUID(runID); err != nil {
		return invalid("run_id must be a UUID"), false
	}
	if attempt < 1 {
		return invalid("attempt must be at least 1"), false
	}
	return Result{}, true
}

func checkRunText(field, value string, maxRunes int) (Result, bool) {
	if utf8.RuneCountInString(value) > maxRunes {
		return invalid("%s must be at most %d characters", field, maxRunes), false
	}
	return Result{}, true
}

func claimFailure(err error) (Result, error) {
	var proposalRequired *dashboards.ProposalRequiredError
	switch {
	case errors.As(err, &proposalRequired):
		return saveFailure(err)
	case errors.Is(err, claims.ErrNotAWorker):
		return toolError(codeNotAWorker, "this service account does not take runs; an admin can turn on takes_runs"), nil
	case errors.Is(err, claims.ErrNotYourRun):
		return toolError(codeNotYourRun, "that run is not yours at that attempt; claim_run gives the run_id and attempt"), nil
	case errors.Is(err, claims.ErrInvalidOutcome):
		return invalid("%s", claims.ErrInvalidOutcome.Error()), nil
	case errors.Is(err, ErrInvalidReason), errors.Is(err, ErrInvalidFailureCode), errors.Is(err, ErrCommitNeedsSuccess):
		return invalid("%s", err.Error()), nil
	case errors.Is(err, ErrRunEnded):
		return toolError(codeRunEnded, ErrRunEnded.Error()), nil
	}
	return Result{}, err
}
