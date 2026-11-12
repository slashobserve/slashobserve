package observations

import (
	"errors"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	CodeInvalidTitle     = "invalid_title"
	CodeInvalidSummary   = "invalid_summary"
	CodeInvalidRange     = "invalid_range"
	CodeInvalidBlock     = "invalid_block"
	CodeInvalidPanel     = "invalid_panel"
	CodeKindNotAddable   = "kind_not_addable"
	CodeBodyNotEditable  = "body_not_editable"
	CodeTooManyBlocks    = "too_many_blocks"
	CodeTooManyPanels    = "too_many_panels"
	CodeSnapshotTooLarge = "snapshot_too_large"
)

var (
	ErrNotFound         = errors.New("observation not found")
	ErrFolderNotFound   = errors.New("folder not found")
	ErrInvalidCursor    = errors.New("invalid cursor")
	ErrNothingToUpdate  = errors.New("nothing to update")
	ErrTooManyBlocks    = &InvalidInputError{Code: CodeTooManyBlocks, Message: "an observation holds at most 100 blocks"}
	ErrTooManyPanels    = &InvalidInputError{Code: CodeTooManyPanels, Message: "an observation holds at most 24 panel and snapshot blocks together"}
	ErrSnapshotTooLarge = &InvalidInputError{Code: CodeSnapshotTooLarge, Message: "the frozen data is larger than 2 MiB; narrow the time range or the query"}
)

type InvalidInputError struct {
	Code    string
	Message string
	Issues  []spec.Issue
}

func (e *InvalidInputError) Error() string {
	return e.Message
}

func (e *InvalidInputError) Is(target error) bool {
	other, ok := target.(*InvalidInputError)
	return ok && other.Code == e.Code
}

type ConflictError struct {
	Current Block
}

func (e *ConflictError) Error() string {
	return "the block changed since you read it"
}

func invalid(code, message string) *InvalidInputError {
	return &InvalidInputError{Code: code, Message: message}
}
