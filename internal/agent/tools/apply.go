package tools

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/internal/threads"
)

const (
	nameApplySuggestedChange = "apply_suggested_change"

	descApplySuggestedChange = "Ask the agent to make the change an answer in a dashboard comment thread suggests. An answer to a question about a dashboard may carry suggested_change, one sentence on the change the person asked for or the nearest safe one; the question's run never changes the dashboard. This posts \"Revise the dashboard: <suggested change>\" in the thread as you and starts a run, yours, given to the agent that answered, that revises the dashboard with that sentence as its prompt: it saves a new version when you can edit the dashboard and opens a proposal when you can only comment. finding_id is the id of the run's finding, from get_run; it must be an answer given in this thread. A repeat call while that run is queued or running, or after it succeeded, posts nothing and returns the first run with replayed true."

	schemaApplySuggestedChange = `{"type":"object","properties":{"comment_id":{"type":"string","format":"uuid","description":"The dashboard comment thread the answer was given in, or the id of any of its messages."},"finding_id":{"type":"string","format":"uuid","description":"The answer whose suggested change to apply: the id of the run's finding, from get_run."}},"required":["comment_id","finding_id"],"additionalProperties":false}`
)

func init() { registerEntries((*Registry).applyEntries) }

func (r *Registry) applyEntries() []entry {
	return []entry{
		{Tool{Name: nameApplySuggestedChange, Description: descApplySuggestedChange, InputSchema: json.RawMessage(schemaApplySuggestedChange), MCPOnly: true, MinRole: auth.RoleViewer}, r.applySuggestedChange},
	}
}

func (r *Registry) applySuggestedChange(ctx context.Context, input json.RawMessage) (Result, error) {
	principal, res, ok, err := r.commentCaller(ctx)
	if err != nil || !ok {
		return res, err
	}
	var in struct {
		CommentID string `json:"comment_id"`
		FindingID string `json:"finding_id"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	if r.deps.Asks == nil {
		return Result{}, errCommentsNotConfigured
	}
	threadID, res, ok := parseCommentID(in.CommentID)
	if !ok {
		return res, nil
	}
	findingID, err := db.ParseUUID(in.FindingID)
	if err != nil {
		return invalid("finding_id must be a UUID"), nil
	}
	sent, err := r.deps.Asks.Apply(ctx, principal, threadID, findingID)
	if res, ok := applyRefusal(ctx, err); ok {
		return res, nil
	}
	if err != nil {
		return commentFailure(err)
	}
	step := startedStep("Asked to revise the dashboard", sent.Run)
	if sent.Replayed {
		step = "The suggested change is already applied by run " + sent.Run.ID
	}
	return success(step, commentResult(commentRef{sent.Thread, sent.Message}, sent)), nil
}

func applyRefusal(ctx context.Context, err error) (Result, bool) {
	switch {
	case err == nil:
		return Result{}, false
	case errors.Is(err, threads.ErrAnswerNotHere):
		return toolError(codeNotFound, "finding_id must be an answer given in this thread"), true
	case errors.Is(err, revisions.ErrUnavailable), errors.Is(err, revisions.ErrTooManyRevisions), errors.Is(err, revisions.ErrAssigneeUnavailable), errors.Is(err, revisions.ErrForbidden), errors.Is(err, revisions.ErrFindingApplied),
		errors.Is(err, folders.ErrRestrictedAnswer), errors.Is(err, folders.ErrProvisioned):
		res, _ := runFailure(ctx, err)
		return res, true
	}
	return Result{}, false
}
