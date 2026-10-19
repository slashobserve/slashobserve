package datasource_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"syscall"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

type failingHealth struct {
	datasource.Datasource
	err error
}

func (f failingHealth) Type() string                      { return "prometheus" }
func (f failingHealth) CheckHealth(context.Context) error { return f.err }
func (f failingHealth) Close() error                      { return nil }

func TestHealthMessageNeverCarriesTheDatasourceAddress(t *testing.T) {
	address := "http://10.1.2.3:9090"
	transport := &url.Error{Op: "Post", URL: address + "/api/v1/query", Err: fmt.Errorf("dial tcp 10.1.2.3:9090: %w", syscall.ECONNREFUSED)}
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"transport failure", transport, "The health check failed"},
		{"unavailable", datasource.Errorf(datasource.ErrUnavailable, "upstream %s unreachable", address), "The datasource is not answering"},
		{"timeout", datasource.Errorf(datasource.ErrTimeout, "%s timed out", address), "Timed out waiting for the datasource"},
		{"forbidden", datasource.Errorf(datasource.ErrForbiddenQuery, "user bob at %s", address), "The datasource refused the health check"},
		{"not read-only", datasource.Errorf(datasource.ErrNotReadOnly, "user bob at %s", address), "The ClickHouse user must be readonly = 2"},
		{"unsafe grant", datasource.Errorf(datasource.ErrUnsafeGrant, "user bob at %s holds FILE", address), "The ClickHouse user holds a grant that reaches outside systems; revoke it"},
		{"auth failed", datasource.Errorf(datasource.ErrAuthFailed, "user bob at %s refused", address), "The datasource refused the credentials"},
		{"host not allowed", datasource.Errorf(datasource.ErrHostNotAllowed, "user bob at %s", address), "The ClickHouse user may not connect from this server's address"},
		{"limit", datasource.Errorf(datasource.ErrLimitExceeded, "%s sent too much", address), "The health check hit a datasource limit"},
		{"invalid", datasource.Errorf(datasource.ErrInvalidQuery, "%s rejected", address), "The datasource rejected the health check"},
		{"deadline", fmt.Errorf("%s: %w", address, context.DeadlineExceeded), "Timed out waiting for the datasource"},
		{"wrapped", errors.Join(errors.New(address), datasource.Errorf(nil, "%s down", address)), "The datasource is not answering"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry, err := datasource.NewRegistry(nil, datasource.Instance{UID: "prom", Datasource: failingHealth{err: tt.err}})
			if err != nil {
				t.Fatal(err)
			}
			statuses := registry.Health(t.Context())
			if len(statuses) != 1 || statuses[0].Healthy {
				t.Fatalf("statuses = %+v", statuses)
			}
			message := statuses[0].Message
			if message != tt.want {
				t.Errorf("message = %q, want %q", message, tt.want)
			}
			for _, leak := range []string{"10.1.2.3", "9090", "http://", "bob"} {
				if strings.Contains(message, leak) {
					t.Errorf("message %q contains %q", message, leak)
				}
			}
		})
	}
}

type blockedHealth struct {
	datasource.Datasource
	release chan struct{}
}

func (b blockedHealth) Type() string { return "prometheus" }
func (b blockedHealth) CheckHealth(context.Context) error {
	<-b.release
	return nil
}
func (b blockedHealth) Close() error { return nil }

func TestHealthReportsACancelledRequestWithoutWaitingForTheCheck(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	registry, err := datasource.NewRegistry(nil, datasource.Instance{UID: "prom", Datasource: blockedHealth{release: release}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	statuses := registry.Health(ctx)
	if len(statuses) != 1 || statuses[0].Healthy || statuses[0].Message != "The health check was cancelled" {
		t.Fatalf("statuses = %+v", statuses)
	}
}

func TestRefinedKindsKeepTheirParentKind(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		parent error
	}{
		{"not read-only", datasource.Errorf(datasource.ErrNotReadOnly, "x"), datasource.ErrForbiddenQuery},
		{"unsafe grant", datasource.Errorf(datasource.ErrUnsafeGrant, "x"), datasource.ErrForbiddenQuery},
		{"auth failed", datasource.Errorf(datasource.ErrAuthFailed, "x"), datasource.ErrUnavailable},
		{"host not allowed", datasource.Errorf(datasource.ErrHostNotAllowed, "x"), datasource.ErrUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !errors.Is(tt.err, tt.parent) {
				t.Errorf("%v should match %v", tt.err, tt.parent)
			}
			if got, want := tt.err.Error(), tt.parent.Error()+": x"; got != want {
				t.Errorf("text = %q, want %q", got, want)
			}
		})
	}
	if errors.Is(datasource.Errorf(datasource.ErrNotReadOnly, "x"), datasource.ErrUnsafeGrant) {
		t.Error("refined kinds should stay distinct from each other")
	}
}
