package queryproxy

import (
	"context"
	"errors"
	"net/http"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

var (
	ErrPanelNotFound     = errors.New("panel not found")
	ErrUnknownDatasource = errors.New("unknown datasource")
	ErrUnknownVariable   = errors.New("unknown variable")
	ErrVariableEmpty     = errors.New("variable has no values")
	ErrInvalidRequest    = errors.New("invalid request")
	ErrUpstreamBusy      = errors.New("datasource calls are busy")

	errInternal = errors.New(internalMessage)
)

const (
	CodeQueryInvalid       spec.IssueCode = "query_invalid"
	CodeQueryForbidden     spec.IssueCode = "query_forbidden"
	CodeLimitExceeded      spec.IssueCode = "limit_exceeded"
	CodeQueryTimeout       spec.IssueCode = "query_timeout"
	CodeDatasourceDown     spec.IssueCode = "datasource_unavailable"
	CodeNotFound           spec.IssueCode = "not_found"
	CodeCanceled           spec.IssueCode = "canceled"
	CodeInternal           spec.IssueCode = "internal"
	CodePanelNotFound      spec.IssueCode = "panel_not_found"
	CodeInvalidRequest     spec.IssueCode = "invalid_request"
	CodeQueryTooLong       spec.IssueCode = "query_too_long"
	CodeUnknownDatasource  spec.IssueCode = "unknown_datasource"
	CodeQueryEmpty         spec.IssueCode = "query_empty"
	CodeVariableFailed     spec.IssueCode = "variable_failed"
	CodeVariableEmpty      spec.IssueCode = "variable_empty"
	CodeVariableUnresolved spec.IssueCode = "variable_unresolved"
	CodeTimeRangeInvalid   spec.IssueCode = "time_range_invalid"
	CodeTooManyRequests    spec.IssueCode = "too_many_requests"
)

const (
	internalMessage     = "internal error"
	queryTimeoutMessage = "query timed out"
	canceledMessage     = "request canceled"
	upstreamBusyMessage = "the server is making too many datasource calls right now; try again shortly"
	statusClientGone    = 0
)

func Classify(err error) (status int, code, message string) {
	if err == nil {
		return http.StatusOK, "", ""
	}
	switch {
	case errors.Is(err, context.Canceled):
		return statusClientGone, string(CodeCanceled), canceledMessage
	case errors.Is(err, ErrInvalidRequest):
		return http.StatusBadRequest, string(CodeInvalidRequest), err.Error()
	case errors.Is(err, datasource.ErrInvalidQuery):
		return http.StatusUnprocessableEntity, string(CodeQueryInvalid), err.Error()
	case errors.Is(err, datasource.ErrForbiddenQuery):
		return http.StatusUnprocessableEntity, string(CodeQueryForbidden), err.Error()
	case errors.Is(err, datasource.ErrLimitExceeded):
		return http.StatusUnprocessableEntity, string(CodeLimitExceeded), err.Error()
	case errors.Is(err, datasource.ErrTimeout):
		return http.StatusGatewayTimeout, string(CodeQueryTimeout), err.Error()
	case errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout, string(CodeQueryTimeout), queryTimeoutMessage
	case errors.Is(err, datasource.ErrUnavailable):
		return http.StatusBadGateway, string(CodeDatasourceDown), err.Error()
	case errors.Is(err, datasource.ErrNotFound):
		return http.StatusNotFound, string(CodeNotFound), err.Error()
	default:
		return classifyRunner(err)
	}
}

func classifyRunner(err error) (status int, code, message string) {
	switch {
	case errors.Is(err, ErrPanelNotFound):
		return http.StatusNotFound, string(CodePanelNotFound), err.Error()
	case errors.Is(err, ErrUnknownDatasource):
		return http.StatusUnprocessableEntity, string(CodeUnknownDatasource), err.Error()
	case errors.Is(err, ErrUnknownVariable):
		return http.StatusBadRequest, string(CodeInvalidRequest), err.Error()
	case errors.Is(err, ErrUpstreamBusy):
		return http.StatusTooManyRequests, string(CodeTooManyRequests), upstreamBusyMessage
	case errors.Is(err, ErrVariableEmpty):
		return http.StatusUnprocessableEntity, string(CodeVariableEmpty), err.Error()
	default:
		return http.StatusInternalServerError, string(CodeInternal), internalMessage
	}
}
