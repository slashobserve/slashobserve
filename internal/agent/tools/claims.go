package tools

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/internal/agent/route"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/claims"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

var usageNoteTimeout = 4 * time.Second

const (
	nameClaimRun    = "claim_run"
	nameReportRun   = "report_run"
	nameFinishRun   = "finish_run"
	nameGetRunBrief = "get_run_brief"
	nameReleaseRun  = "release_run"

	defaultClaimWait = 30
	maxClaimWait     = 30
	maxRunTextRunes  = 500
	maxLiveTextRunes = 4000
	maxStepToolRunes = 64

	failureLiveTextAppend = "live_text_append_failed"
	failureUsageNotNoted  = "usage_not_noted"
	failureHarnessRelease = "harness_release_failed"

	codeNotAWorker = "not_a_worker"
	codeNotYourRun = "not_your_run"
	codeRunEnded   = "run_ended"

	descClaimRun = "Take the next SlashObserve run assigned to you, waiting up to wait_seconds for one. run is null when none is waiting: call again. A claimed run has its prompt and target, a session url and token for a second MCP connection that acts for the person who asked, and the observe prompt with the run's kind, which is the system text for the run. While you work, call report_run at least every 30 seconds, and call finish_run when you are done. Only connected agents and service accounts that take runs can claim runs."

	descReportRun = "Keep the lease of a run you claimed and say what you are doing. The lease ends 60 seconds after the last call. The answer has the run's state and the saved panel progress; when stop is true the run was canceled or has ended: stop and call finish_run. Optional: text is the end of what your model is writing, shown as live text and never stored; usage is your cumulative model use, and the server stops the run at its cost limit."

	descFinishRun = "End a run you claimed. Send outcome succeeded after you saved its result with the run session's tools; a run that saved a version, a proposal or a finding ends succeeded whatever you send, and one that saved nothing ends failed. Send outcome failed with a message when you could not. usage is your model use, shown on the run. Optional: commit saves the finished dashboard first; restarted is true when the dashboard changed meanwhile and the run restarted from its latest version, so call get_run_brief and go on."

	descGetRunBrief = "Optional. The first message the server's own agent starts each model session with: the system text, the user message, the run's saved progress and, in brief, what they are written from as fields. Starting a session with it resets the read guard and pins the parent version; an agent that does not call it keeps working from claim_run's prompt."

	descReleaseRun = "Hand a run you claimed back to the queue when you cannot work it, for example because you are stopping. The run is queued again with its saved progress and steps, and it does not count as failed; the next claim_run of an agent it is assigned to takes it. The answer has the run's state: queued, canceled when its requester had canceled it, or the state it had already ended in. Call finish_run instead when the run is done or cannot be done."

	schemaClaimRun = `{"type":"object","properties":{` +
		`"wait_seconds":{"type":"integer","minimum":0,"maximum":30,"description":"How long to wait for a run, 0 to 30 seconds. Default 30."},` +
		`"harness":{"type":"boolean","description":"Only for the server's own agent, which keeps the run's guards itself."}},"additionalProperties":false}`

	schemaRunUsage = `{"type":"object","description":"Your cumulative model use.","properties":{` +
		`"provider":{"type":"string"},"model":{"type":"string"},` +
		`"input_tokens":{"type":"integer","minimum":0},"output_tokens":{"type":"integer","minimum":0},` +
		`"cache_read_tokens":{"type":"integer","minimum":0},"cache_write_tokens":{"type":"integer","minimum":0},` +
		`"cost_usd":{"type":"number","minimum":0},` +
		`"model_calls":{"type":"integer","minimum":0},"failed_calls":{"type":"integer","minimum":0},"retries":{"type":"integer","minimum":0},` +
		`"compactions":{"type":"integer","minimum":0},"renewals":{"type":"integer","minimum":0},` +
		`"model_ms":{"type":"integer","minimum":0},` +
		`"max_save_output_tokens":{"type":"integer","minimum":0},` +
		`"nudges":{"type":"integer","minimum":0},"save_now_refusals":{"type":"integer","minimum":0},"stalled_turns":{"type":"integer","minimum":0},` +
		`"one_panel_refusals":{"type":"integer","minimum":0},"repair_rounds":{"type":"integer","minimum":0},` +
		`"hypotheses":{"type":"array","maxItems":6,"items":{"type":"object"},"description":"Your hypothesis ledger, as note_hypotheses takes it."},` +
		`"call_limit":{"type":"integer","minimum":0},"stage_at_save":{"type":"string","enum":["orient","hypothesize","test","conclude"]}},"additionalProperties":false}`

	schemaGetRunBrief = `{"type":"object","properties":{` +
		`"run_id":{"type":"string","format":"uuid","description":"The run id from claim_run."},` +
		`"attempt":{"type":"integer","minimum":1,"description":"The attempt from claim_run."},` +
		`"claim_epoch":{"type":"integer","minimum":1,"description":"The claim_epoch from claim_run."},` +
		`"reason":{"type":"string","enum":["restart","output_limit","pending","conflict","continue","renewed","call_timeout","no_finding","no_answer"],"description":"Why a model session starts again; leave it out for the first."}},` +
		`"required":["run_id","attempt"],"additionalProperties":false}`

	schemaReportRun = `{"type":"object","properties":{` +
		`"run_id":{"type":"string","format":"uuid","description":"The run id from claim_run."},` +
		`"attempt":{"type":"integer","minimum":1,"description":"The attempt from claim_run."},` +
		`"claim_epoch":{"type":"integer","minimum":1,"description":"The claim_epoch from claim_run."},` +
		`"note":{"type":"string","maxLength":500,"description":"One line on what you are doing, shown in the run's steps."},` +
		`"text":{"type":"string","maxLength":4000,"description":"The end of what your model is writing, shown as live text while the run is running and never stored."},` +
		`"usage":` + schemaRunUsage + `,` +
		`"agent_state":{"type":"object","properties":{"session":{"description":"For this attempt."},"carry":{"description":"Also for the next attempt."}},"additionalProperties":false,"description":"Your own state, at most 256 KiB, replacing the last; get_run_brief returns it."},` +
		`"steps":{"type":"array","maxItems":20,"items":{"type":"object","properties":{` +
		`"phase":{"type":"string","enum":["discover","lint","query","save","write","ledger","repair","stage"]},` +
		`"tool":{"type":"string","maxLength":64},"status":{"type":"string","enum":["ok","error"]},` +
		`"message":{"type":"string","maxLength":500}},"required":["phase","status","message"],"additionalProperties":false},` +
		`"description":"Steps of your own for the run's steps."},` +
		`"task":{"type":"string","description":"Server's own agent only: the investigation's task type."}},` +
		`"required":["run_id","attempt"],"additionalProperties":false}`

	schemaReleaseRun = `{"type":"object","properties":{` +
		`"run_id":{"type":"string","format":"uuid","description":"The run id from claim_run."},` +
		`"attempt":{"type":"integer","minimum":1,"description":"The attempt from claim_run."},` +
		`"claim_epoch":{"type":"integer","minimum":1,"description":"The claim_epoch from claim_run."}},` +
		`"required":["run_id","attempt"],"additionalProperties":false}`

	schemaFinishRun = `{"type":"object","properties":{` +
		`"run_id":{"type":"string","format":"uuid","description":"The run id from claim_run."},` +
		`"attempt":{"type":"integer","minimum":1,"description":"The attempt from claim_run."},` +
		`"claim_epoch":{"type":"integer","minimum":1,"description":"The claim_epoch from claim_run."},` +
		`"outcome":{"type":"string","enum":["succeeded","failed"],"description":"succeeded once saved, failed when you could not."},` +
		`"message":{"type":"string","maxLength":500,"description":"Why the run failed, shown on the run."},` +
		`"failure_code":{"type":"string","description":"Model-side failure: call_timeout, rate_limited, provider_error, output_limit, no_tool_call, no_finding, refused, no_fixture or internal; or a guard of your own: validation_exhausted, no_progress or restricted_answer, with message."},` +
		`"commit":{"type":"boolean","description":"Save the finished dashboard first."},` +
		`"usage":` + schemaRunUsage + `},` +
		`"required":["run_id","attempt","outcome"],"additionalProperties":false}`
)

var errClaimsNotConfigured = errors.New("claims are not configured")

type ClaimService interface {
	Claim(ctx context.Context, c claims.Claimer, wait time.Duration) (claims.Offer, error)
	ClaimedRun(ctx context.Context, c claims.Claimer, runID string, attempt, claimEpoch int) (revisions.Request, error)
	Report(ctx context.Context, c claims.Claimer, in claims.ReportInput) (claims.ReportAnswer, error)
	Release(ctx context.Context, c claims.Claimer, runID string, attempt, claimEpoch int) (claims.ReleaseAnswer, error)
}

type LiveText interface {
	Append(ctx context.Context, runID string, attempt int, text string) error
}

func init() { registerEntries((*Registry).claimEntries) }

func (r *Registry) claimEntries() []entry {
	claim := func(name, desc, schema string, handle func(context.Context, json.RawMessage) (Result, error)) entry {
		return entry{Tool{Name: name, Description: desc, InputSchema: json.RawMessage(schema), Claim: true, MinRole: auth.RoleViewer}, handle}
	}
	brief := claim(nameGetRunBrief, descGetRunBrief, schemaGetRunBrief, r.getRunBrief)
	brief.tool.ReadOnly = true
	return []entry{
		claim(nameClaimRun, descClaimRun, schemaClaimRun, r.claimRun),
		claim(nameReportRun, descReportRun, schemaReportRun, r.reportRun),
		claim(nameFinishRun, descFinishRun, schemaFinishRun, r.finishRun),
		claim(nameReleaseRun, descReleaseRun, schemaReleaseRun, r.releaseRun),
		brief,
	}
}

type claimRunInput struct {
	WaitSeconds *int `json:"wait_seconds"`
	Harness     bool `json:"harness"`
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
	FailedCalls         int      `json:"failed_calls"`
	Retries             int      `json:"retries"`
	Compactions         int      `json:"compactions"`
	Renewals            int      `json:"renewals"`
	ModelMS             int64    `json:"model_ms"`
	MaxSaveOutputTokens int64    `json:"max_save_output_tokens"`

	Nudges           int               `json:"nudges"`
	SaveNowRefusals  int               `json:"save_now_refusals"`
	StalledTurns     int               `json:"stalled_turns"`
	OnePanelRefusals int               `json:"one_panel_refusals"`
	RepairRounds     int               `json:"repair_rounds"`
	Hypotheses       []hypothesisInput `json:"hypotheses"`
	CallLimit        int               `json:"call_limit"`
	StageAtSave      string            `json:"stage_at_save"`
}

func (u *runUsage) reported() (*revisions.Usage, Result, bool) {
	if u == nil {
		return nil, Result{}, true
	}
	var ledger []revisions.Hypothesis
	if len(u.Hypotheses) > 0 {
		var problem string
		if ledger, problem = checkLedger(u.Hypotheses); problem != "" {
			return nil, invalid("usage.%s", problem), false
		}
	}
	if !slices.Contains(reportedStages, u.StageAtSave) {
		return nil, invalid("usage.stage_at_save must be one of %s", strings.Join(reportedStages[1:], ", ")), false
	}
	return &revisions.Usage{
		Provider: u.Provider, Model: u.Model, InputTokens: u.InputTokens, OutputTokens: u.OutputTokens,
		CacheReadTokens: u.CacheReadTokens, CacheWriteTokens: u.CacheWriteTokens, CostUSD: u.CostUSD,
		ModelCalls: u.ModelCalls, FailedCalls: u.FailedCalls, Retries: u.Retries, Compactions: u.Compactions, Renewals: u.Renewals,
		ModelMS: u.ModelMS, MaxSaveOutputTokens: u.MaxSaveOutputTokens,
		Nudges: u.Nudges, SaveNowRefusals: u.SaveNowRefusals, StalledTurns: u.StalledTurns, OnePanelRefusals: u.OnePanelRefusals,
		RepairRounds: u.RepairRounds, Hypotheses: ledger, CallLimit: u.CallLimit, StageAtSave: u.StageAtSave,
	}, Result{}, true
}

var reportedStages = []string{"", "orient", "hypothesize", "test", "conclude"}

type runRefInput struct {
	RunID      string `json:"run_id"`
	Attempt    int    `json:"attempt"`
	ClaimEpoch int    `json:"claim_epoch"`
}

type reportRunInput struct {
	runRefInput
	Note       string         `json:"note"`
	Text       string         `json:"text"`
	Usage      *runUsage      `json:"usage"`
	AgentState *AgentState    `json:"agent_state"`
	Steps      []ReportedStep `json:"steps"`
	Task       *string        `json:"task"`
}

type reportRunOut struct {
	claims.ReportAnswer
	Build *BuildProgress `json:"build"`
}

type finishRunInput struct {
	runRefInput
	Outcome     string    `json:"outcome"`
	Message     string    `json:"message"`
	FailureCode string    `json:"failure_code"`
	Commit      bool      `json:"commit"`
	Usage       *runUsage `json:"usage"`
}

type getRunBriefInput struct {
	runRefInput
	Reason string `json:"reason"`
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
		if in.Harness && claimer.Kind != revisions.AssignBuiltin {
			return invalid("harness is only for the server's own agent"), nil
		}
		offer, err := r.deps.Claims.Claim(ctx, claimer, time.Duration(wait)*time.Second)
		if err != nil {
			return claimFailure(ctx, err)
		}
		if offer.Run == nil {
			return success("No run waiting", offer), nil
		}
		if in.Harness {
			if err := r.beginHarness(ctx, claimer, *offer.Run); err != nil {
				return claimFailure(ctx, err)
			}
		}
		return success("Claimed a run", offer), nil
	})
}

func (r *Registry) beginHarness(ctx context.Context, claimer claims.Claimer, run claims.OfferedRun) error {
	if r.deps.RunSessions == nil {
		return errClaimsNotConfigured
	}
	req, err := r.deps.Claims.ClaimedRun(ctx, claimer, run.ID, run.Attempt, run.ClaimEpoch)
	if err == nil {
		err = r.deps.RunSessions.BeginHarness(ctx, req)
	}
	if err != nil {
		if _, releaseErr := r.deps.Claims.Release(context.WithoutCancel(ctx), claimer, run.ID, run.Attempt, run.ClaimEpoch); releaseErr != nil {
			r.deps.Logger.WarnContext(ctx, "run not released after its harness session failed to open", logattrs.Failure(failureHarnessRelease, releaseErr, slog.String("agent_request_id", run.ID))...)
		}
	}
	return err
}

func (r *Registry) getRunBrief(ctx context.Context, input json.RawMessage) (Result, error) {
	var in getRunBriefInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	if res, ok := checkRunRef(in.runRefInput); !ok {
		return res, nil
	}
	return r.asClaimedRun(ctx, in.runRefInput, func(claimer claims.Claimer, req revisions.Request) (Result, error) {
		answer, err := r.deps.RunSessions.Brief(ctx, req, in.Reason)
		if err != nil {
			return claimFailure(ctx, err)
		}
		return success("Read a run's brief", answer), nil
	})
}

func (r *Registry) reportRun(ctx context.Context, input json.RawMessage) (Result, error) {
	var in reportRunInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	if res, ok := checkRunRef(in.runRefInput); !ok {
		return res, nil
	}
	if res, ok := checkRunText("note", in.Note, maxRunTextRunes); !ok {
		return res, nil
	}
	if res, ok := checkRunText("text", in.Text, maxLiveTextRunes); !ok {
		return res, nil
	}
	if res, ok := checkAgentReport(in.AgentState, in.Steps); !ok {
		return res, nil
	}
	task, res, ok := reportedTask(in.Task)
	if !ok {
		return res, nil
	}
	usage, res, ok := in.Usage.reported()
	if !ok {
		return res, nil
	}
	return r.asClaimedRun(ctx, in.runRefInput, func(claimer claims.Claimer, req revisions.Request) (Result, error) {
		answer, err := r.report(ctx, claimer, req, in, usage, task)
		if err != nil {
			return claimFailure(ctx, err)
		}
		return success("Reported on a run", answer), nil
	})
}

func (r *Registry) report(ctx context.Context, claimer claims.Claimer, req revisions.Request, in reportRunInput, usage *revisions.Usage, task *route.Task) (reportRunOut, error) {
	ref := claims.ReportInput{RunID: in.RunID, Attempt: in.Attempt, ClaimEpoch: in.ClaimEpoch, Note: in.Note}
	answer, err := r.deps.Claims.Report(ctx, claimer, ref)
	if err != nil || answer.Stop {
		return reportRunOut{ReportAnswer: answer}, err
	}
	if task != nil && claimer.Kind != revisions.AssignBuiltin {
		task = nil
	}
	if in.AgentState != nil || len(in.Steps) > 0 || task != nil {
		err := r.deps.RunSessions.NoteAgent(ctx, req, AgentReport{State: in.AgentState, Steps: in.Steps, Task: task})
		if err != nil && !errors.Is(err, ErrRunEnded) {
			return reportRunOut{}, err
		}
	}
	if usage != nil {
		stop, err := r.noteUsage(ctx, req, *usage)
		if err != nil {
			return reportRunOut{}, err
		}
		if stop {
			answer, err = r.deps.Claims.Report(ctx, claimer, claims.ReportInput{RunID: in.RunID, Attempt: in.Attempt, ClaimEpoch: in.ClaimEpoch})
			answer.Stop = true
			return reportRunOut{ReportAnswer: answer}, err
		}
	}
	r.appendLiveText(ctx, req, in.Text)
	build, err := r.deps.RunSessions.Progress(ctx, req)
	return reportRunOut{ReportAnswer: answer, Build: build}, err
}

func (r *Registry) noteUsage(ctx context.Context, req revisions.Request, usage revisions.Usage) (bool, error) {
	noteCtx, cancel := context.WithTimeout(ctx, usageNoteTimeout)
	defer cancel()
	stop, err := r.deps.RunSessions.NoteUsage(noteCtx, req, usage)
	if err != nil && noteCtx.Err() != nil && ctx.Err() == nil {
		r.deps.Logger.WarnContext(ctx, "usage not noted, the lease was renewed", logattrs.Failure(failureUsageNotNoted, err, slog.String("agent_request_id", req.ID))...)
		return false, nil
	}
	return stop, err
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
	if res, ok := checkRunRef(in.runRefInput); !ok {
		return res, nil
	}
	if in.Outcome != "succeeded" && in.Outcome != "failed" {
		return invalid("outcome must be succeeded or failed"), nil
	}
	if res, ok := checkRunText("message", in.Message, maxRunTextRunes); !ok {
		return res, nil
	}
	usage, res, ok := in.Usage.reported()
	if !ok {
		return res, nil
	}
	return r.asClaimedRun(ctx, in.runRefInput, func(claimer claims.Claimer, req revisions.Request) (Result, error) {
		answer, err := r.deps.RunSessions.Finish(ctx, req, RunFinish{
			Outcome: in.Outcome, FailureCode: in.FailureCode, Message: in.Message, Commit: in.Commit, Usage: usage,
		})
		if err != nil {
			return claimFailure(ctx, err)
		}
		return success("Ended a run", answer), nil
	})
}

func (r *Registry) releaseRun(ctx context.Context, input json.RawMessage) (Result, error) {
	var in runRefInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	if res, ok := checkRunRef(in); !ok {
		return res, nil
	}
	return r.asClaimer(ctx, func(claimer claims.Claimer) (Result, error) {
		answer, err := r.deps.Claims.Release(ctx, claimer, in.RunID, in.Attempt, in.ClaimEpoch)
		if err != nil {
			return claimFailure(ctx, err)
		}
		return success("Released a run", answer), nil
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

func (r *Registry) asClaimedRun(ctx context.Context, ref runRefInput, use func(claims.Claimer, revisions.Request) (Result, error)) (Result, error) {
	return r.asClaimer(ctx, func(claimer claims.Claimer) (Result, error) {
		if r.deps.RunSessions == nil {
			return Result{}, errClaimsNotConfigured
		}
		req, err := r.deps.Claims.ClaimedRun(ctx, claimer, ref.RunID, ref.Attempt, ref.ClaimEpoch)
		if err != nil {
			return claimFailure(ctx, err)
		}
		return use(claimer, req)
	})
}

func checkRunRef(in runRefInput) (Result, bool) {
	if _, err := db.ParseUUID(in.RunID); err != nil {
		return invalid("run_id must be a UUID"), false
	}
	if in.Attempt < 1 {
		return invalid("attempt must be at least 1"), false
	}
	if in.ClaimEpoch < 0 {
		return invalid("claim_epoch must be at least 1"), false
	}
	return Result{}, true
}

var reportedPhases = []string{PhaseDiscover, PhaseLint, PhaseQuery, PhaseSave, PhaseWrite, PhaseLedger, "repair", "stage"}

func checkAgentReport(state *AgentState, steps []ReportedStep) (Result, bool) {
	if state != nil && len(state.Session)+len(state.Carry) > MaxAgentStateBytes {
		return invalid("agent_state must be at most %d bytes", MaxAgentStateBytes), false
	}
	if len(steps) > MaxReportedSteps {
		return invalid("steps takes at most %d steps", MaxReportedSteps), false
	}
	for i, step := range steps {
		switch {
		case !slices.Contains(reportedPhases, step.Phase):
			return invalid("steps[%d].phase must be one of %s", i, strings.Join(reportedPhases, ", ")), false
		case step.Status != "ok" && step.Status != "error":
			return invalid("steps[%d].status must be ok or error", i), false
		case utf8.RuneCountInString(step.Tool) > maxStepToolRunes:
			return invalid("steps[%d].tool must be at most %d characters", i, maxStepToolRunes), false
		case utf8.RuneCountInString(step.Message) > maxRunTextRunes:
			return invalid("steps[%d].message must be at most %d characters", i, maxRunTextRunes), false
		}
	}
	return Result{}, true
}

func reportedTask(raw *string) (*route.Task, Result, bool) {
	if raw == nil {
		return nil, Result{}, true
	}
	task, ok := route.Parse(*raw)
	if !ok || task == route.Full {
		return nil, invalid("task must be one of %s", strings.Join(taskNames(), ", ")), false
	}
	return &task, Result{}, true
}

func taskNames() []string {
	names := make([]string, len(route.Tasks))
	for i, t := range route.Tasks {
		names[i] = string(t)
	}
	return names
}

func checkRunText(field, value string, maxRunes int) (Result, bool) {
	if utf8.RuneCountInString(value) > maxRunes {
		return invalid("%s must be at most %d characters", field, maxRunes), false
	}
	return Result{}, true
}

func claimFailure(ctx context.Context, err error) (Result, error) {
	var proposalRequired *dashboards.ProposalRequiredError
	switch {
	case errors.As(err, &proposalRequired):
		return saveFailure(ctx, err)
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
