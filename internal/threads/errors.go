package threads

import "errors"

var (
	ErrNotFound       = errors.New("comment not found")
	ErrTargetNotFound = errors.New("target not found")
	ErrNotAuthor      = errors.New("only the author can edit a comment")
	ErrUnknownPanel   = errors.New("panel does not exist in that version")
	ErrUnknownVersion = errors.New("version does not exist on this dashboard")
)

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
)
