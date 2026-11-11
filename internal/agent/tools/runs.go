package tools

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	nameGetRun = "get_run"

	descGetRun = "Read one run of the built-in agent by id: its state, result_kind, allowed_actions, attempt, the last 20 steps and its usage, the same object the REST API returns for one observe request. Run ids come from list_activity events of type run.*."

	schemaGetRun = `{"type":"object","properties":{"id":{"type":"string","format":"uuid","description":"The run id, the target id of a run.* event from list_activity."}},"required":["id"],"additionalProperties":false}`

	runStepLimit = 20
)

var errRunsNotConfigured = errors.New("runs are not configured")

type RunReader interface {
	Run(ctx context.Context, id pgtype.UUID, by auth.Principal, maxSteps int) (json.RawMessage, error)
}

func init() { registerEntries((*Registry).runEntries) }

func (r *Registry) runEntries() []entry {
	return []entry{
		{Tool{Name: nameGetRun, Description: descGetRun, InputSchema: json.RawMessage(schemaGetRun), MCPOnly: true, ReadOnly: true, MinRole: auth.RoleViewer}, r.getRun},
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
