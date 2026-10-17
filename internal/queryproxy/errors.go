package queryproxy

import (
	"context"
	"errors"
	"net/http"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

var (
	ErrPanelNotFound          = errors.New("panel not found")
	ErrUnknownDatasource      = errors.New("unknown datasource")
	ErrDatasourceTypeMismatch = errors.New("datasource type mismatch")
	ErrUnknownVariable        = errors.New("unknown variable")
	ErrVariableEmpty          = errors.New("variable has no values")
	ErrInvalidRequest         = errors.New("invalid request")

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
	CodeUnknownDatasource  spec.IssueCode = "unknown_datasource"
	CodeDatasourceMismatch spec.IssueCode = "datasource_type_mismatch"
	CodeQueryEmpty         spec.IssueCode = "query_empty"
	CodeVariableFailed     spec.IssueCode = "variable_failed"
	CodeVariableEmpty      spec.IssueCode = "variable_empty"
	CodeVariableUnresolved spec.IssueCode = "variable_unresolved"
	CodeTimeRangeInvalid   spec.IssueCode = "time_range_invalid"
)

const (
	internalMessage     = "internal error"
	queryTimeoutMessage = "query timed out"
	canceledMessage     = "request canceled"
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
	case errors.Is(err, ErrDatasourceTypeMismatch):
		return http.StatusUnprocessableEntity, string(CodeDatasourceMismatch), err.Error()
	case errors.Is(err, ErrUnknownVariable):
		return http.StatusBadRequest, string(CodeInvalidRequest), err.Error()
	case errors.Is(err, ErrVariableEmpty):
		return http.StatusUnprocessableEntity, string(CodeVariableEmpty), err.Error()
	default:
		return http.StatusInternalServerError, string(CodeInternal), internalMessage
	}
}
