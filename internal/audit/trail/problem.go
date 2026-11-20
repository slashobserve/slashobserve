package trail

import (
	"errors"
	"net/http"

	"github.com/slashobserve/slashobserve/internal/audit/filter"
)

const (
	CodeInvalidRequest = "invalid_request"
	CodeQueryInvalid   = "query_invalid"
	CodeQueryTimeout   = "query_timeout"
	CodeNotFound       = "not_found"
)

type Problem struct {
	Status  int    `json:"-"`
	Message string `json:"error"`
	Code    string `json:"code"`
	Start   *int   `json:"start,omitempty"`
	End     *int   `json:"end,omitempty"`
}

func ProblemOf(err error) (Problem, bool) {
	var parse *filter.Error
	var invalid *InvalidRequest
	var refused *QueryRefused
	switch {
	case errors.As(err, &parse):
		start, end := parse.Pos, max(parse.End, parse.Pos)
		return Problem{Status: http.StatusUnprocessableEntity, Code: CodeQueryInvalid, Message: parse.Msg, Start: &start, End: &end}, true
	case errors.As(err, &invalid):
		return Problem{Status: http.StatusBadRequest, Code: CodeInvalidRequest, Message: invalid.Msg}, true
	case errors.As(err, &refused):
		return Problem{Status: http.StatusUnprocessableEntity, Code: CodeQueryInvalid, Message: refused.Msg}, true
	case errors.Is(err, ErrTimeout):
		return Problem{Status: http.StatusUnprocessableEntity, Code: CodeQueryTimeout, Message: ErrTimeout.Error()}, true
	case errors.Is(err, ErrNotFound):
		return Problem{Status: http.StatusNotFound, Code: CodeNotFound, Message: "no audit event has this id"}, true
	}
	return Problem{}, false
}
