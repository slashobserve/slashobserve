package queryproxy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantStatus  int
		wantCode    string
		wantMessage string
	}{
		{"invalid query", datasource.Errorf(datasource.ErrInvalidQuery, "bad promql"), http.StatusUnprocessableEntity, "query_invalid", "invalid query: bad promql"},
		{"forbidden query", datasource.Errorf(datasource.ErrForbiddenQuery, "INSERT"), http.StatusUnprocessableEntity, "query_forbidden", "query not allowed: INSERT"},
		{"limit exceeded", datasource.Errorf(datasource.ErrLimitExceeded, "too many series"), http.StatusUnprocessableEntity, "limit_exceeded", "query limit exceeded: too many series"},
		{"upstream timeout", datasource.Errorf(datasource.ErrTimeout, "after 30s"), http.StatusGatewayTimeout, "query_timeout", "query timed out: after 30s"},
		{"deadline exceeded", context.DeadlineExceeded, http.StatusGatewayTimeout, "query_timeout", "query timed out"},
		{"wrapped deadline exceeded", fmt.Errorf("get: %w", context.DeadlineExceeded), http.StatusGatewayTimeout, "query_timeout", "query timed out"},
		{"unavailable", datasource.Errorf(datasource.ErrUnavailable, "dial tcp"), http.StatusBadGateway, "datasource_unavailable", "datasource unavailable: dial tcp"},
		{"nil kind defaults to unavailable", datasource.Errorf(nil, "x"), http.StatusBadGateway, "datasource_unavailable", "datasource unavailable: x"},
		{"not found", datasource.Errorf(datasource.ErrNotFound, "no such metric"), http.StatusNotFound, "not_found", "not found: no such metric"},
		{"panel not found", fmt.Errorf("%w: %q", ErrPanelNotFound, "p9"), http.StatusNotFound, "panel_not_found", `panel not found: "p9"`},
		{"unknown datasource", fmt.Errorf("%w: %q", ErrUnknownDatasource, "x"), http.StatusUnprocessableEntity, "unknown_datasource", `unknown datasource: "x"`},
		{"type mismatch", fmt.Errorf("%w: boom", ErrDatasourceTypeMismatch), http.StatusUnprocessableEntity, "datasource_type_mismatch", "datasource type mismatch: boom"},
		{"unknown variable", fmt.Errorf("%w: %q", ErrUnknownVariable, "v"), http.StatusBadRequest, "invalid_request", `unknown variable: "v"`},
		{"variable empty", fmt.Errorf("%w: %q", ErrVariableEmpty, "v"), http.StatusUnprocessableEntity, "variable_empty", `variable has no values: "v"`},
		{"canceled", context.Canceled, statusClientGone, "canceled", "request canceled"},
		{"wrapped canceled", fmt.Errorf("get: %w", context.Canceled), statusClientGone, "canceled", "request canceled"},
		{"canceled wins over upstream kind", errors.Join(datasource.ErrUnavailable, context.Canceled), statusClientGone, "canceled", "request canceled"},
		{"internal sentinel", errInternal, http.StatusInternalServerError, "internal", "internal error"},
		{"unknown error hides detail", errors.New("dial tcp 10.0.0.5:9090: password=hunter2"), http.StatusInternalServerError, "internal", "internal error"},
		{"nil error", nil, http.StatusOK, "", ""},
		{"invalid request", fmt.Errorf("%w: variable %q does not allow All", ErrInvalidRequest, "v"), http.StatusBadRequest, "invalid_request", `invalid request: variable "v" does not allow All`},
		{"invalid request built from an upstream message", invalidRangeError(datasource.Errorf(datasource.ErrInvalidQuery, "time range requires from and to")), http.StatusBadRequest, "invalid_request", "invalid request: time range requires from and to"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, code, message := Classify(tt.err)
			if status != tt.wantStatus || code != tt.wantCode || message != tt.wantMessage {
				t.Fatalf("Classify = (%d, %q, %q), want (%d, %q, %q)", status, code, message, tt.wantStatus, tt.wantCode, tt.wantMessage)
			}
			if strings.Contains(message, "hunter2") {
				t.Fatalf("message leaked detail: %q", message)
			}
		})
	}
}
