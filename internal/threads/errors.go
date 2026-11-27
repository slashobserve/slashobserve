package threads

import (
	"errors"

	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrNotFound       = errors.New("comment not found")
	ErrTargetNotFound = errors.New("target not found")
	ErrNotAuthor      = errors.New("only a person can edit their own comment; an agent's reply cannot be edited")
	ErrNotStarter     = errors.New("a commenter can resolve only a thread they started; ask someone who can edit, or the person who started it")
	ErrCannotDelete   = errors.New("only the author of a comment, or someone who manages its folder, can delete it; an agent's reply can be deleted only by someone who manages the folder")
	ErrUnknownPanel   = errors.New("panel does not exist in that version")
	ErrUnknownVersion = errors.New("version does not exist on this dashboard")
)

var errInvalidMention = &InvalidInputError{Code: CodeInvalidMention, Message: "you can mention only your own connected agents and service accounts that take runs"}

var errInvalidPerson = &InvalidInputError{Code: CodeInvalidMention, Message: "a mentioned person does not exist or cannot see this; remove the mention or pick someone from the mentionable people"}

var (
	errInvalidBlock   = &InvalidInputError{Code: CodeInvalidBlock, Message: "block_id must be a block of this observation"}
	errInvalidReplyTo = &InvalidInputError{Code: CodeInvalidReplyTo, Message: "reply_to_id must be a message in this thread"}
)

type ObservationThreadExistsError struct {
	ThreadID pgtype.UUID
}

func (e *ObservationThreadExistsError) Error() string {
	return "this observation already has a discussion"
}

type InvalidInputError struct {
	Code    string
	Message string
	Cause   error
}

func (e *InvalidInputError) Unwrap() error { return e.Cause }

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
	CodeNotStarter      = "not_starter"
	CodeUnknownPanel    = "unknown_panel"
	CodeUnknownVersion  = "unknown_version"
	CodeInvalidMention  = "invalid_mention"
	CodeTooManyAgents   = "too_many_agents"
	CodeInvalidBlock    = "invalid_block"
	CodeInvalidReplyTo  = "invalid_reply_to"
	CodeInvalidAnchor   = "invalid_anchor"
	CodeClientIDReused  = "client_id_reused"
	CodeInvalidTo       = "invalid_to"
	CodeInvalidClientID = "invalid_client_id"
)
