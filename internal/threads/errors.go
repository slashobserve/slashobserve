package threads

import (
	"errors"

	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrNotFound       = errors.New("comment not found")
	ErrTargetNotFound = errors.New("target not found")
	ErrNotAuthor      = errors.New("only the author can edit a comment")
	ErrUnknownPanel   = errors.New("panel does not exist in that version")
	ErrUnknownVersion = errors.New("version does not exist on this dashboard")
)

var errInvalidMention = &InvalidInputError{Code: CodeInvalidMention, Message: "you can mention only your own connected agents and service accounts that take runs"}

type ObservationThreadExistsError struct {
	ThreadID pgtype.UUID
}

func (e *ObservationThreadExistsError) Error() string {
	return "this observation already has a discussion"
}

type InvalidInputError struct {
	Code    string
	Message string
}

func (e *InvalidInputError) Error() string {
	return e.Message
}

const (
	CodeInvalidTarget   = "invalid_target"
	CodeInvalidBody     = "invalid_body"
	CodeInvalidPanelID  = "invalid_panel_id"
	CodeInvalidVersion  = "invalid_version"
	CodeInvalidStatus   = "invalid_status"
	CodeTooManyMentions = "too_many_mentions"
	CodeNotAuthor       = "not_author"
	CodeUnknownPanel    = "unknown_panel"
	CodeUnknownVersion  = "unknown_version"
	CodeInvalidMention  = "invalid_mention"
	CodeTooManyAgents   = "too_many_agents"

	CodeObservationThreadExists = "observation_thread_exists"
)
