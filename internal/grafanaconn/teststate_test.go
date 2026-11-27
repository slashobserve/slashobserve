package grafanaconn

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/egress"
)

func TestAConnectionsTestStateIsDerivedFromItsLastTest(t *testing.T) {
	at := time.Now()
	tests := []struct {
		name  string
		last  *TestResult
		state string
		kind  FailureKind
	}{
		{"never tested", nil, TestUntested, ""},
		{"the last test passed", &TestResult{At: at, OK: true, Message: "Connected."}, TestOK, ""},
		{"the last test failed with a known cause", &TestResult{At: at, Message: "Grafana refused the token (status 401).", FailureKind: FailureCredentials}, TestFailing, FailureCredentials},
		{"a failure stored before causes were kept", &TestResult{At: at, Message: "Grafana could not be reached."}, TestFailing, FailureUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, kind := testStateOf(tt.last)
			if state != tt.state || kind != tt.kind {
				t.Fatalf("state = %q, kind = %q, want %q, %q", state, kind, tt.state, tt.kind)
			}
			if !TestStates.Has(state) {
				t.Fatalf("%q is not a declared state", state)
			}
		})
	}
}

func TestATestFailureIsSortedByWhoCanFixIt(t *testing.T) {
	tests := []struct {
		err  error
		want FailureKind
	}{
		{&egress.StatusError{Status: http.StatusUnauthorized}, FailureCredentials},
		{&egress.StatusError{Status: http.StatusForbidden}, FailureCredentials},
		{&egress.StatusError{Status: http.StatusNotFound}, FailureConfiguration},
		{&egress.StatusError{Status: http.StatusTooManyRequests}, FailureTransient},
		{&egress.StatusError{Status: http.StatusBadGateway}, FailureTransient},
		{&egress.StatusError{Status: http.StatusTeapot}, FailureUnknown},
		{&egress.HostError{Host: "grafana.example.com", Err: egress.ErrNotAllowed}, FailureConfiguration},
		{egress.ErrInvalidURL, FailureConfiguration},
		{egress.ErrRedirected, FailureConfiguration},
		{egress.ErrBlockedAddress, FailureConfiguration},
		{egress.ErrContentType, FailureConfiguration},
		{errBadResponse, FailureConfiguration},
		{egress.ErrTooLarge, FailureUnsupported},
		{fmt.Errorf("get: %w", egress.ErrUnavailable), FailureTransient},
		{context.DeadlineExceeded, FailureTransient},
		{errors.New("boom"), FailureUnknown},
	}
	for _, tt := range tests {
		if got := failureKindOf(tt.err); got != tt.want {
			t.Errorf("%v: kind = %q, want %q", tt.err, got, tt.want)
		}
	}
}
