package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/forbidden"
	"github.com/slashobserve/slashobserve/internal/lifecycle"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/internal/threads"
)

const (
	nameGetRun = "get_run"

	descGetRun = "Read one run of the built-in agent by id: its state, result_kind, allowed_actions, attempt, the last 20 steps and its usage, the same object the REST API returns for one observe request. Run ids come from list_activity events of type run.*."

	schemaGetRun = `{"type":"object","properties":{"id":{"type":"string","format":"uuid","description":"The run id, the target id of a run.* event from list_activity."}},"required":["id"],"additionalProperties":false}`

	nameStartRun    = "start_run"
	nameCancelRun   = "cancel_run"
	nameRetryRun    = "retry_run"
	nameGetRunBuild = "get_run_build"

	descStartRun    = "Ask the built-in agent to create a new dashboard, or to revise one when dashboard_id is given, and get the run back at once with its id; the run is queued and works in the background, so follow it with get_run until its state is succeeded or failed. It spends the instance's model, acts as you with your role and folder access, and a Commenter's revision becomes a proposal. A person can have 3 new-dashboard runs queued or running and a dashboard 5 revisions; the answers are too_many_requests, too_many_revisions and agent_busy, and agent_unavailable means no model is configured. It is not safe to repeat: every call queues another run, so read get_run or list_activity before asking again. Investigations are a person's decision and cannot be started here; if you can do the work yourself, use the other tools instead."
	descCancelRun   = "Stop one run of the built-in agent that you started. A queued run is canceled at once, a running one is asked to stop and ends canceled within a few seconds with the panels it already saved kept. Safe to repeat: a run that is already canceled, succeeded or failed answers with its current state, not an error. Only the person or key that started a run can cancel it (forbidden)."
	descGetRunBuild = "Read the dashboard a run of the built-in agent has saved so far: its Spec, how many panels and saves it holds and how many it planned, updated_at, and dashboard_id and base_version when it revises one. Only the person or key that started the run and an Admin can read it; a run with no saved work answers not_found. Read it to see what a run made before you keep, redo or replace it; nothing here changes it."
	descRetryRun    = "Start a succeeded, failed or canceled run of the built-in agent again with the same prompt; the new run is yours and queues at once, with a new id to follow with get_run. A queued or running run answers invalid_transition with allowed_actions. It is not safe to repeat: each call queues another run. A run that was an investigation cannot be retried here."

	schemaStartRun  = `{"type":"object","properties":{"prompt":{"type":"string","minLength":1,"maxLength":4000,"description":"What to create or change, 1 to 4000 characters."},"dashboard_id":{"type":"string","format":"uuid","description":"The dashboard to revise; omit to create a new one."},"base_version":{"type":"integer","minimum":1,"description":"A version that exists on the dashboard; only with dashboard_id, and the latest by default."},"follows_id":{"type":"string","format":"uuid","description":"An earlier run of yours that has no dashboard, to continue its chat so the agent reads the earlier questions and answers; not with dashboard_id."}},"required":["prompt"],"additionalProperties":false}`
	schemaRunTarget = `{"type":"object","properties":{"id":{"type":"string","format":"uuid","description":"The run id, from start_run, get_run or list_activity."}},"required":["id"],"additionalProperties":false}`

	runStepLimit = 20
)

var errRunsNotConfigured = errors.New("runs are not configured")

type RunDesk interface {
	Run(ctx context.Context, id pgtype.UUID, by auth.Principal, maxSteps int) (json.RawMessage, error)
	Start(ctx context.Context, by auth.Principal, req revisions.StartRequest) (json.RawMessage, error)
	Cancel(ctx context.Context, id pgtype.UUID, by auth.Principal) (json.RawMessage, error)
	Retry(ctx context.Context, id pgtype.UUID, by auth.Principal) (json.RawMessage, error)
	Build(ctx context.Context, id pgtype.UUID, by auth.Principal) (json.RawMessage, error)
}

func init() { registerEntries((*Registry).runEntries) }

func (r *Registry) runEntries() []entry {
	return []entry{
		{Tool{Name: nameGetRun, Description: descGetRun, InputSchema: json.RawMessage(schemaGetRun), MCPOnly: true, ReadOnly: true, MinRole: auth.RoleViewer}, r.getRun},
		{Tool{Name: nameStartRun, Description: descStartRun, InputSchema: json.RawMessage(schemaStartRun), MCPOnly: true, MinRole: auth.RoleViewer}, r.startRun},
		{Tool{Name: nameGetRunBuild, Description: descGetRunBuild, InputSchema: json.RawMessage(schemaRunTarget), MCPOnly: true, ReadOnly: true, MinRole: auth.RoleViewer}, r.getRunBuild},
		{Tool{Name: nameCancelRun, Description: descCancelRun, InputSchema: json.RawMessage(schemaRunTarget), MCPOnly: true, MinRole: auth.RoleViewer}, r.cancelRun},
		{Tool{Name: nameRetryRun, Description: descRetryRun, InputSchema: json.RawMessage(schemaRunTarget), MCPOnly: true, MinRole: auth.RoleViewer}, r.retryRun},
	}
}

type getRunInput struct {
	ID string `json:"id"`
}

func (r *Registry) getRun(ctx context.Context, input json.RawMessage) (Result, error) {
	if r.deps.Runs == nil {
		return Result{}, errRunsNotConfigured
	}
	var in getRunInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	id, err := db.ParseUUID(in.ID)
	if err != nil {
		return invalid("id must be a UUID"), nil
	}
	principal, _ := auth.FromContext(ctx)
	raw, err := r.deps.Runs.Run(ctx, id, principal, runStepLimit)
	if err != nil {
		switch {
		case errors.Is(err, revisions.ErrRequestNotFound), errors.Is(err, revisions.ErrNotFound),
			errors.Is(err, revisions.ErrDashboardNotFound), errors.Is(err, folders.ErrNotFound):
			return toolError(codeNotFound, "run not found"), nil
		case errors.Is(err, folders.ErrForbidden), errors.Is(err, folders.ErrNoAccess), errors.Is(err, revisions.ErrForbidden):
			return toolError(codeForbiddenRole, roleDeniedMessage), nil
		}
		return Result{}, err
	}
	return success("Read run "+in.ID, raw), nil
}

type startRunInput struct {
	Prompt      *string `json:"prompt"`
	DashboardID *string `json:"dashboard_id"`
	BaseVersion *int    `json:"base_version"`
	FollowsID   *string `json:"follows_id"`
}

func (r *Registry) startRun(ctx context.Context, input json.RawMessage) (Result, error) {
	if r.deps.Runs == nil {
		return Result{}, errRunsNotConfigured
	}
	var in startRunInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	principal, _ := auth.FromContext(ctx)
	raw, err := r.deps.Runs.Start(ctx, principal, revisions.StartRequest{Prompt: in.Prompt, DashboardID: in.DashboardID, BaseVersion: in.BaseVersion, FollowsID: in.FollowsID})
	if err != nil {
		return runFailure(err)
	}
	return success("Started a run", raw), nil
}

func (r *Registry) cancelRun(ctx context.Context, input json.RawMessage) (Result, error) {
	if r.deps.Runs == nil {
		return Result{}, errRunsNotConfigured
	}
	id, res, ok := r.runTarget(input)
	if !ok {
		return res, nil
	}
	principal, _ := auth.FromContext(ctx)
	raw, err := r.deps.Runs.Cancel(ctx, id, principal)
	if errors.Is(err, revisions.ErrRequestFinished) {
		current, readErr := r.deps.Runs.Run(ctx, id, principal, 0)
		if readErr != nil {
			return runFailure(readErr)
		}
		return success("Run "+id.String()+" had already finished", current), nil
	}
	if err != nil {
		return runFailure(err)
	}
	return success("Canceled run "+id.String(), raw), nil
}

func (r *Registry) retryRun(ctx context.Context, input json.RawMessage) (Result, error) {
	if r.deps.Runs == nil {
		return Result{}, errRunsNotConfigured
	}
	id, res, ok := r.runTarget(input)
	if !ok {
		return res, nil
	}
	principal, _ := auth.FromContext(ctx)
	current, err := r.deps.Runs.Run(ctx, id, principal, 0)
	if err != nil {
		return runFailure(err)
	}
	var source struct {
		Source string `json:"source"`
	}
	if json.Unmarshal(current, &source) == nil && source.Source == string(revisions.SourceInvestigate) {
		return toolError(threads.RefusalPersonStarts, "an investigation is a person's decision; ask the person to retry it in the web app"), nil
	}
	raw, err := r.deps.Runs.Retry(ctx, id, principal)
	if err != nil {
		return runFailure(err)
	}
	return success("Retried run "+id.String(), raw), nil
}

func (r *Registry) runTarget(input json.RawMessage) (pgtype.UUID, Result, bool) {
	var in getRunInput
	if res, ok := decodeInput(input, &in); !ok {
		return pgtype.UUID{}, res, false
	}
	id, err := db.ParseUUID(in.ID)
	if err != nil {
		return pgtype.UUID{}, invalid("id must be a UUID"), false
	}
	return id, Result{}, true
}

func runFailure(err error) (Result, error) {
	var invalidStart *revisions.StartError
	var refusal *lifecycle.Refusal
	switch {
	case errors.As(err, &invalidStart):
		return invalid("%s", invalidStart.Message), nil
	case errors.As(err, &refusal):
		allowed := refusal.Allowed
		if allowed == nil {
			allowed = []string{}
		}
		return failure("", invalidTransitionOut{
			Code:           codeInvalidTransition,
			Message:        fmt.Sprintf("%s is not allowed while the run is %s%s", refusal.Action, refusal.State, allowedNow(refusal.Allowed)),
			State:          refusal.State,
			AllowedActions: allowed,
		}), nil
	case errors.Is(err, revisions.ErrUnavailable):
		return toolError("agent_unavailable", "no model is configured on this server, so the built-in agent cannot run"), nil
	case errors.Is(err, revisions.ErrTooManyRevisions):
		return toolError("too_many_revisions", err.Error()), nil
	case errors.Is(err, revisions.ErrTooManyRequests):
		return toolError("too_many_requests", err.Error()), nil
	case errors.Is(err, revisions.ErrAgentBusy):
		return toolError("agent_busy", err.Error()), nil
	case errors.Is(err, revisions.ErrUnknownPanel):
		return toolError("unknown_panel", err.Error()), nil
	case errors.Is(err, revisions.ErrDashboardVersionNotFound):
		return toolError(codeVersionNotFound, err.Error()), nil
	}
	if reply, ok := revisions.ReplyFor(err); ok {
		if reply.Code == forbidden.Code {
			return toolError(codeForbiddenRole, reply.Message), nil
		}
		return toolError(reply.Code, reply.Message), nil
	}
	return Result{}, err
}

func (r *Registry) getRunBuild(ctx context.Context, input json.RawMessage) (Result, error) {
	if r.deps.Runs == nil {
		return Result{}, errRunsNotConfigured
	}
	id, res, ok := r.runTarget(input)
	if !ok {
		return res, nil
	}
	principal, _ := auth.FromContext(ctx)
	raw, err := r.deps.Runs.Build(ctx, id, principal)
	if err != nil {
		return runFailure(err)
	}
	return success("Read the build of run "+id.String(), raw), nil
}
