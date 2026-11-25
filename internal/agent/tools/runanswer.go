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
	"github.com/slashobserve/slashobserve/internal/threads"
)

const (
	nameSaveRunAnswer = "save_run_answer"
	nameShareAnswer   = "share_answer"

	codeNoAnswer     = "no_answer"
	codeNotShareable = "not_shareable"
	codeWrongThread  = "invalid_thread"

	descSaveRunAnswer = "Save the answer of a run as an observation: a plain question asked in chat or in a comment is answered in the chat only, and this files that answer as an observation, with its text and evidence, in the folder of the dashboard it was about, else in your default folder. Only the one who asked may. An answer whose run read data only Admins can query, such as the audit trail, is saved in the folder of the dashboard or observation it was about when only Admins can read that folder, else in the built-in Audit folder, and is refused with restricted_answer when neither is one only Admins can read. Returns the observation id and url; a repeat call returns the same observation with replayed true."

	descShareAnswer = "Share the answer of a private question with the people who can read what it was about. A question asked with start_run kind ask is read only by you and Admins; this posts the question and the agent's answer as a new comment thread on that dashboard or observation, at the same panel or block, and on an observation also writes the answer onto the page. With thread_id it adds both to that thread instead, which must be on the same dashboard or observation. Only the one who asked may, once the run has an answer; you need Comment there, and an answer whose evidence reads a datasource only Admins can query, or whose run read one such as the audit trail, can only be shared where only Admins can read (restricted_answer otherwise). A repeat call returns the same thread with replayed true."

	schemaShareAnswer = `{"type":"object","properties":{"run_id":{"type":"string","format":"uuid","description":"The run whose answer to share: the id of the request, from get_run or the chat."},"thread_id":{"type":"string","format":"uuid","description":"A comment thread on the same dashboard or observation to add the question and answer to; omit to start a new thread."}},"required":["run_id"],"additionalProperties":false}`

	schemaSaveRunAnswer = `{"type":"object","properties":{"run_id":{"type":"string","format":"uuid","description":"The run whose answer to save: the id of the request, from get_run or the chat url."}},"required":["run_id"],"additionalProperties":false}`
)

type answerSharedOut struct {
	ThreadID  string `json:"thread_id"`
	MessageID string `json:"message_id"`
	Replayed  bool   `json:"replayed"`
}

type runAnswerSavedOut struct {
	ObservationID string `json:"observation_id"`
	URL           string `json:"url"`
	Replayed      bool   `json:"replayed"`
}

func init() { registerEntries((*Registry).runAnswerEntries) }

func (r *Registry) runAnswerEntries() []entry {
	return []entry{
		{Tool{Name: nameSaveRunAnswer, Description: descSaveRunAnswer, InputSchema: json.RawMessage(schemaSaveRunAnswer), MCPOnly: true, MinRole: auth.RoleViewer}, r.saveRunAnswer},
		{Tool{Name: nameShareAnswer, Description: descShareAnswer, InputSchema: json.RawMessage(schemaShareAnswer), MCPOnly: true, MinRole: auth.RoleViewer}, r.shareAnswer},
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
	}
	if res, ok := restrictedRefusal(err); ok {
		return res, nil
	}
	if err != nil {
		return Result{}, err
	}
	res := success("Saved the answer of run "+in.RunID+" as observation "+f.ObservationID, runAnswerSavedOut{ObservationID: f.ObservationID, URL: links.Observation(r.deps.PublicURL, f.ObservationID), Replayed: replayed})
	res.Created = []string{f.ObservationID}
	return res, nil
}

func (r *Registry) shareAnswer(ctx context.Context, input json.RawMessage) (Result, error) {
	if r.deps.Threads == nil {
		return Result{}, errCommentsNotConfigured
	}
	var in struct {
		RunID    string  `json:"run_id"`
		ThreadID *string `json:"thread_id"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	share := threads.ShareInput{}
	var err error
	if share.RunID, err = db.ParseUUID(in.RunID); err != nil {
		return invalid("run_id must be a UUID"), nil
	}
	if in.ThreadID != nil {
		if share.ThreadID, err = db.ParseUUID(*in.ThreadID); err != nil {
			return invalid("thread_id must be a UUID"), nil
		}
	}
	principal, _ := auth.FromContext(ctx)
	shared, err := r.deps.Threads.ShareRun(ctx, principal, share)
	if res, ok := shareRefusal(err); ok {
		return res, nil
	}
	if err != nil {
		return Result{}, err
	}
	out := answerSharedOut{ThreadID: shared.Thread.ID, MessageID: shared.Message.ID, Replayed: shared.Replayed}
	return success("Shared the answer of run "+in.RunID+" in thread "+out.ThreadID, out), nil
}

func shareRefusal(err error) (Result, bool) {
	var wrong *threads.InvalidInputError
	switch {
	case err == nil:
		return Result{}, false
	case errors.Is(err, threads.ErrRunNotFound):
		return toolError(codeNotFound, "run not found"), true
	case errors.Is(err, threads.ErrNoAnswer):
		return toolError(codeNoAnswer, err.Error()), true
	case errors.Is(err, threads.ErrNotShareable):
		return toolError(codeNotShareable, err.Error()), true
	case errors.Is(err, threads.ErrNotFound), errors.As(err, &wrong) && wrong.Code == threads.CodeWrongThread:
		return toolError(codeWrongThread, "thread_id must be a thread you can comment in, on the dashboard or observation the question was about"), true
	case errors.Is(err, threads.ErrTargetNotFound):
		return toolError(codeNotFound, "the dashboard or observation the question was about no longer exists"), true
	case errors.Is(err, folders.ErrForbidden), errors.Is(err, folders.ErrNoAccess):
		return toolError(codeForbiddenRole, roleDeniedMessage), true
	}
	return restrictedRefusal(err)
}
