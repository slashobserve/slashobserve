package observations

import (
	"errors"
	"fmt"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	CodeInvalidTitle     = "invalid_title"
	CodeInvalidSummary   = "invalid_summary"
	CodeInvalidRange     = "invalid_range"
	CodeInvalidBlock     = "invalid_block"
	CodeInvalidPanel     = "invalid_panel"
	CodeVariableMissing  = "variable_missing"
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
	ErrCursorOrder      = errors.New("a cursor needs the default order, most recently changed first; page with an offset to sort another way")
	ErrTooManyBlocks    = &InvalidInputError{Code: CodeTooManyBlocks, Message: "an observation holds at most 100 blocks"}
	ErrTooManyPanels    = &InvalidInputError{Code: CodeTooManyPanels, Message: "an observation holds at most 24 panel and snapshot blocks together"}
	ErrSnapshotTooLarge = &InvalidInputError{Code: CodeSnapshotTooLarge, Message: "the frozen data is larger than 2 MiB; narrow the time range or the query"}
)

type InvalidInputError struct {
	Code    string
	Message string
	Issues  []spec.Issue
	Cause   error
}

func (e *InvalidInputError) Unwrap() error { return e.Cause }

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

func invalidText(code string, cause error) *InvalidInputError {
	return &InvalidInputError{Code: code, Message: cause.Error(), Cause: cause}
}

type BlockError struct {
	Index int
	Err   error
}

func (e *BlockError) Error() string { return fmt.Sprintf("block %d: %v", e.Index, e.Err) }

func (e *BlockError) Unwrap() error { return e.Err }
