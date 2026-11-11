package tools

import (
	"context"
	"encoding/json"
	"errors"
	"time"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/claims"
	"github.com/slashobserve/slashobserve/internal/db"
)

const (
	nameClaimRun  = "claim_run"
	nameReportRun = "report_run"
	nameFinishRun = "finish_run"

	defaultClaimWait = 30
	maxClaimWait     = 30
	maxRunTextRunes  = 500

	codeNotAWorker = "not_a_worker"
	codeNotYourRun = "not_your_run"

	descClaimRun = "Take the next SlashObserve run assigned to you, waiting up to wait_seconds for one. run is null when none is waiting: call again. A claimed run has its prompt and target, a session url and token for a second MCP connection that acts for the person who asked, and the observe prompt with the run's kind, which is the system text for the run. While you work, call report_run at least every 30 seconds, and call finish_run when you are done. Only connected agents and service accounts that take runs can claim runs."

	descReportRun = "Keep the lease of a run you claimed and say what you are doing. The lease ends 60 seconds after the last call. The answer has the run's state; when stop is true the run was canceled or has ended: stop and call finish_run."

	descFinishRun = "End a run you claimed. Send outcome succeeded after you saved its result with the run session's tools; a run that saved a dashboard version or a finding ends succeeded whatever you send, and one that saved nothing ends failed. Send outcome failed with a message when you could not do it. usage is your own model and cost, shown on the run as you report it."

	schemaClaimRun = `{"type":"object","properties":{` +
		`"wait_seconds":{"type":"integer","minimum":0,"maximum":30,"description":"How long to wait for a run, 0 to 30 seconds. Default 30."}},"additionalProperties":false}`

	schemaReportRun = `{"type":"object","properties":{` +
		`"run_id":{"type":"string","format":"uuid","description":"The run id from claim_run."},` +
		`"attempt":{"type":"integer","minimum":1,"description":"The attempt from claim_run."},` +
		`"note":{"type":"string","maxLength":500,"description":"One line on what you are doing, shown in the run's steps."}},` +
		`"required":["run_id","attempt"],"additionalProperties":false}`

	schemaFinishRun = `{"type":"object","properties":{` +
		`"run_id":{"type":"string","format":"uuid","description":"The run id from claim_run."},` +
		`"attempt":{"type":"integer","minimum":1,"description":"The attempt from claim_run."},` +
		`"outcome":{"type":"string","enum":["succeeded","failed"],"description":"succeeded after you saved the result, failed when you could not."},` +
		`"message":{"type":"string","maxLength":500,"description":"Why the run failed, shown on the run."},` +
		`"usage":{"type":"object","description":"Your own model use for this run, shown as you report it.","properties":{` +
		`"provider":{"type":"string","description":"The model provider."},` +
		`"model":{"type":"string","description":"The model name."},` +
		`"input_tokens":{"type":"integer","minimum":0,"description":"Input tokens used."},` +
		`"output_tokens":{"type":"integer","minimum":0,"description":"Output tokens used."},` +
		`"cost_usd":{"type":"number","minimum":0,"description":"Cost in US dollars."}},"additionalProperties":false}},` +
		`"required":["run_id","attempt","outcome"],"additionalProperties":false}`
)

var errClaimsNotConfigured = errors.New("claims are not configured")

func init() { registerEntries((*Registry).claimEntries) }

func (r *Registry) claimEntries() []entry {
	return []entry{
		{Tool{Name: nameClaimRun, Description: descClaimRun, InputSchema: json.RawMessage(schemaClaimRun), MCPOnly: true, MinRole: auth.RoleViewer}, r.claimRun},
		{Tool{Name: nameReportRun, Description: descReportRun, InputSchema: json.RawMessage(schemaReportRun), MCPOnly: true, MinRole: auth.RoleViewer}, r.reportRun},
		{Tool{Name: nameFinishRun, Description: descFinishRun, InputSchema: json.RawMessage(schemaFinishRun), MCPOnly: true, MinRole: auth.RoleViewer}, r.finishRun},
	}
}

type claimRunInput struct {
	WaitSeconds *int `json:"wait_seconds"`
}

type reportRunInput struct {
	RunID   string `json:"run_id"`
	Attempt int    `json:"attempt"`
	Note    string `json:"note"`
}

type finishRunInput struct {
	RunID   string              `json:"run_id"`
	Attempt int                 `json:"attempt"`
	Outcome string              `json:"outcome"`
	Message string              `json:"message"`
	Usage   *claims.FinishUsage `json:"usage"`
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

func (r *Registry) reportRun(ctx context.Context, input json.RawMessage) (Result, error) {
	var in reportRunInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	if res, ok := checkRunRef(in.RunID, in.Attempt); !ok {
		return res, nil
	}
	if res, ok := checkRunText("note", in.Note); !ok {
		return res, nil
	}
	return r.asClaimer(ctx, func(claimer claims.Claimer) (Result, error) {
		answer, err := r.deps.Claims.Report(ctx, claimer, claims.ReportInput{RunID: in.RunID, Attempt: in.Attempt, Note: in.Note})
		if err != nil {
			return claimFailure(err)
		}
		return success("Reported on a run", answer), nil
	})
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
	if res, ok := checkRunText("message", in.Message); !ok {
		return res, nil
	}
	return r.asClaimer(ctx, func(claimer claims.Claimer) (Result, error) {
		answer, err := r.deps.Claims.Finish(ctx, claimer, claims.FinishInput{RunID: in.RunID, Attempt: in.Attempt, Outcome: in.Outcome, Message: in.Message, Usage: in.Usage})
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

func checkRunRef(runID string, attempt int) (Result, bool) {
	if _, err := db.ParseUUID(runID); err != nil {
		return invalid("run_id must be a UUID"), false
	}
	if attempt < 1 {
		return invalid("attempt must be at least 1"), false
	}
	return Result{}, true
}

func checkRunText(field, value string) (Result, bool) {
	if utf8.RuneCountInString(value) > maxRunTextRunes {
		return invalid("%s must be at most %d characters", field, maxRunTextRunes), false
	}
	return Result{}, true
}

func claimFailure(err error) (Result, error) {
	switch {
	case errors.Is(err, claims.ErrNotAWorker):
		return toolError(codeNotAWorker, "this service account does not take runs; an admin can turn on takes_runs"), nil
	case errors.Is(err, claims.ErrNotYourRun):
		return toolError(codeNotYourRun, "that run is not yours at that attempt; claim_run gives the run_id and attempt"), nil
	case errors.Is(err, claims.ErrInvalidOutcome):
		return invalid("%s", claims.ErrInvalidOutcome.Error()), nil
	}
	return Result{}, err
}
