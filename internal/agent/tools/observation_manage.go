package tools

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/observations"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	nameUpdateObservation = "update_observation"
	nameMoveObservation   = "move_observation"
	nameReopenObservation = "reopen_observation"

	messageNoObservationChange = "no change given: send title, from and to, or clear_range"

	descUpdateObservation = "Change an observation's title or its time range (from and to together), or clear the range. A new title also renames the chat with no dashboard that the observation was the one observation made from; chat_rename in the result is renamed, or kept when you did not start that chat."
	descMoveObservation   = "Move an observation and its blocks to another folder from list_folders. Needs edit access to both folders."
	descReopenObservation = "Reopen a resolved observation so blocks can be added again."

	schemaUpdateObservation = `{"type":"object","properties":{` + propObservationID + `,` +
		`"title":{"type":"string","minLength":1,"maxLength":200,"description":"The new title."},` +
		`"from":{"type":"string","format":"date-time","description":"Start of the time range as an RFC 3339 time; send with to."},` +
		`"to":{"type":"string","format":"date-time","description":"End of the time range as an RFC 3339 time; send with from."},` +
		`"clear_range":{"type":"boolean","description":"Remove the time range. Not with from or to."}},"required":["observation_id"],"additionalProperties":false}`
	schemaMoveObservation = `{"type":"object","properties":{` + propObservationID + `,` +
		`"folder_id":{"type":"string","format":"uuid","description":"The target folder, from list_folders."}},"required":["observation_id","folder_id"],"additionalProperties":false}`
	schemaReopenObservation = `{"type":"object","properties":{` + propObservationID + `},"required":["observation_id"],"additionalProperties":false}`
)

func init() { registerEntries((*Registry).observationManageEntries) }

func (r *Registry) observationManageEntries() []entry {
	return []entry{
		{Tool{Name: nameUpdateObservation, Description: descUpdateObservation, InputSchema: json.RawMessage(schemaUpdateObservation), MinRole: auth.RoleViewer}, r.updateObservation},
		{Tool{Name: nameMoveObservation, Description: descMoveObservation, InputSchema: json.RawMessage(schemaMoveObservation), MinRole: auth.RoleViewer}, r.moveObservation},
		{Tool{Name: nameReopenObservation, Description: descReopenObservation, InputSchema: json.RawMessage(schemaReopenObservation), MinRole: auth.RoleViewer}, r.reopenObservation},
	}
}

type observationManagedOut struct {
	ObservationID string `json:"observation_id"`
	Title         string `json:"title"`
	State         string `json:"state"`
	FolderID      string `json:"folder_id"`
	URL           string `json:"url"`

	ChatRename revisions.SharedTitle `json:"chat_rename,omitempty"`
}

func (r *Registry) observationManaged(o observations.Observation) observationManagedOut {
	return observationManagedOut{ObservationID: o.ID, Title: o.Title, State: string(o.State), FolderID: o.FolderID, URL: r.deps.Observations.URL(o.ID), ChatRename: o.ChatRename}
}

func (r *Registry) observationManageFailure(ctx context.Context, step string, err error) (Result, error) {
	switch {
	case errors.Is(err, observations.ErrNothingToUpdate):
		return invalid("%s", messageNoObservationChange), nil
	case errors.Is(err, folders.ErrMoveIntoPersonal):
		return toolError(codePersonalFolder, err.Error()), nil
	}
	return r.observationFailure(ctx, step, err)
}

func (r *Registry) updateObservation(ctx context.Context, input json.RawMessage) (Result, error) {
	if r.deps.Observations == nil {
		return Result{}, errObservationsNotConfigured
	}
	var in struct {
		ObservationID string  `json:"observation_id"`
		Title         *string `json:"title"`
		From          string  `json:"from"`
		To            string  `json:"to"`
		ClearRange    bool    `json:"clear_range"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	id, res, ok := parseObservationID(in.ObservationID)
	if !ok {
		return res, nil
	}
	change := observations.UpdateInput{Title: in.Title, ClearRange: in.ClearRange}
	if change.From, res, ok = annotationTime("from", in.From); !ok {
		return res, nil
	}
	if change.To, res, ok = annotationTime("to", in.To); !ok {
		return res, nil
	}
	switch {
	case in.ClearRange && (change.From != nil || change.To != nil):
		return invalid("send from and to, or clear_range, not both"), nil
	case change.Title == nil && change.From == nil && change.To == nil && !in.ClearRange:
		return invalid("%s", messageNoObservationChange), nil
	}
	principal, _ := auth.FromContext(ctx)
	o, err := r.deps.Observations.Update(ctx, principal, id, change)
	if err != nil {
		return r.observationManageFailure(ctx, "Updated an observation", err)
	}
	return success("Updated observation "+o.ID+chatRenameNote[o.ChatRename], r.observationManaged(o)), nil
}

var chatRenameNote = map[revisions.SharedTitle]string{
	revisions.TitleRenamed: " and renamed the chat it was made from",
	revisions.TitleKept:    "; the chat it was made from keeps its title because only the person who started it may rename it",
}

func (r *Registry) moveObservation(ctx context.Context, input json.RawMessage) (Result, error) {
	if r.deps.Observations == nil {
		return Result{}, errObservationsNotConfigured
	}
	var in struct {
		ObservationID string `json:"observation_id"`
		FolderID      string `json:"folder_id"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	id, res, ok := parseObservationID(in.ObservationID)
	if !ok {
		return res, nil
	}
	folderID, res, ok := requiredFolderID(in.FolderID)
	if !ok {
		return res, nil
	}
	principal, _ := auth.FromContext(ctx)
	o, err := r.deps.Observations.Move(ctx, principal, id, folderID)
	if err != nil {
		return r.observationManageFailure(ctx, "Moved an observation", err)
	}
	return success("Moved observation "+o.ID, r.observationManaged(o)), nil
}

func (r *Registry) reopenObservation(ctx context.Context, input json.RawMessage) (Result, error) {
	if r.deps.Observations == nil {
		return Result{}, errObservationsNotConfigured
	}
	var in struct {
		ObservationID string `json:"observation_id"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	id, res, ok := parseObservationID(in.ObservationID)
	if !ok {
		return res, nil
	}
	principal, _ := auth.FromContext(ctx)
	o, err := r.deps.Observations.Reopen(ctx, principal, id)
	if err != nil {
		return r.observationManageFailure(ctx, "Reopened an observation", err)
	}
	return success("Reopened observation "+o.ID, r.observationManaged(o)), nil
}
