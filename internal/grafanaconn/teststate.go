package grafanaconn

import (
	"context"
	"errors"
	"net/http"

	"github.com/slashobserve/slashobserve/internal/egress"
	"github.com/slashobserve/slashobserve/internal/lifecycle"
)

type FailureKind string

const (
	FailureTransient     FailureKind = "transient"
	FailureCredentials   FailureKind = "credentials"
	FailureConfiguration FailureKind = "configuration"
	FailureUnsupported   FailureKind = "unsupported"
	FailureUnknown       FailureKind = "unknown"
)

const (
	TestUntested = "untested"
	TestFailing  = "failing"
	TestOK       = "ok"
)

var TestStates = lifecycle.Derived{Name: "Grafana connection test", States: []lifecycle.DerivedState{
	{Name: TestUntested, When: "The connection has not been tested since it was added or its URL or token changed; last_test is null."},
	{Name: TestFailing, When: "The last test could not read Grafana; failure_kind says who can fix it: transient (Grafana did not answer, try again), credentials (the token was refused), configuration (the URL, the outbound allowlist or what answers at the address), unsupported (an answer this server cannot take) or unknown."},
	{Name: TestOK, When: "None of the above: the last test read Grafana's health, folders and dashboards."},
}}

func testStateOf(last *TestResult) (string, FailureKind) {
	switch {
	case last == nil:
		return TestUntested, ""
	case last.OK:
		return TestOK, ""
	case last.FailureKind == "":
		return TestFailing, FailureUnknown
	}
	return TestFailing, last.FailureKind
}

func failureKindOf(err error) FailureKind {
	var status *egress.StatusError
	switch {
	case errors.As(err, &status):
		return statusFailureKind(status.Status)
	case errors.Is(err, egress.ErrNotAllowed), errors.Is(err, egress.ErrInvalidURL), errors.Is(err, egress.ErrRedirected),
		errors.Is(err, egress.ErrBlockedAddress), errors.Is(err, egress.ErrContentType), errors.Is(err, errBadResponse):
		return FailureConfiguration
	case errors.Is(err, egress.ErrTooLarge):
		return FailureUnsupported
	case errors.Is(err, egress.ErrUnavailable), errors.Is(err, context.DeadlineExceeded):
		return FailureTransient
	}
	return FailureUnknown
}

func statusFailureKind(status int) FailureKind {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return FailureCredentials
	case status == http.StatusNotFound:
		return FailureConfiguration
	case status == http.StatusTooManyRequests || status >= http.StatusInternalServerError:
		return FailureTransient
	}
	return FailureUnknown
}
