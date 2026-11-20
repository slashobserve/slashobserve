package tools

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/links"
)

const (
	nameSaveRunAnswer = "save_run_answer"

	codeNoAnswer = "no_answer"

	descSaveRunAnswer = "Save the answer of a run as an observation: a plain question asked in chat or in a comment is answered in the chat only, and this files that answer as an observation, with its text and evidence, in the folder of the dashboard it was about, else in your default folder. Only the one who asked may. Returns the observation id and url; a repeat call returns the same observation with replayed true."

	schemaSaveRunAnswer = `{"type":"object","properties":{"run_id":{"type":"string","format":"uuid","description":"The run whose answer to save: the id of the request, from get_run or the chat url."}},"required":["run_id"],"additionalProperties":false}`
)

type runAnswerSavedOut struct {
	ObservationID string `json:"observation_id"`
	URL           string `json:"url"`
	Replayed      bool   `json:"replayed"`
}

func init() { registerEntries((*Registry).runAnswerEntries) }

func (r *Registry) runAnswerEntries() []entry {
	return []entry{
		{Tool{Name: nameSaveRunAnswer, Description: descSaveRunAnswer, InputSchema: json.RawMessage(schemaSaveRunAnswer), MCPOnly: true, MinRole: auth.RoleViewer}, r.saveRunAnswer},
	}
}

func (r *Registry) saveRunAnswer(ctx context.Context, input json.RawMessage) (Result, error) {
	if r.deps.Findings == nil {
		return Result{}, errFindingsNotConfigured
	}
	var in struct {
		RunID string `json:"run_id"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	id, err := db.ParseUUID(in.RunID)
	if err != nil {
		return invalid("run_id must be a UUID"), nil
	}
	principal, _ := auth.FromContext(ctx)
	f, replayed, err := r.deps.Findings.SaveAsObservation(ctx, principal, id)
	switch {
	case errors.Is(err, findings.ErrNotFound):
		return toolError(codeNoAnswer, "the run has no answer you can save"), nil
	case errors.Is(err, findings.ErrRequesterOnly):
		return toolError(codeForbiddenRole, "only the one who asked can save the answer"), nil
	case errors.Is(err, folders.ErrForbidden), errors.Is(err, folders.ErrNoAccess):
		return toolError(codeForbiddenRole, roleDeniedMessage), nil
	case err != nil:
		return Result{}, err
	}
	return success("Saved the answer of run "+in.RunID+" as observation "+f.ObservationID, runAnswerSavedOut{ObservationID: f.ObservationID, URL: links.Observation(r.deps.PublicURL, f.ObservationID), Replayed: replayed}), nil
}
