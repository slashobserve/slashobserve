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
	}{
		{"transport failure", transport},
		{"unavailable", datasource.Errorf(datasource.ErrUnavailable, "upstream %s unreachable", address)},
		{"timeout", datasource.Errorf(datasource.ErrTimeout, "%s timed out", address)},
		{"forbidden", datasource.Errorf(datasource.ErrForbiddenQuery, "user bob at %s", address)},
		{"limit", datasource.Errorf(datasource.ErrLimitExceeded, "%s sent too much", address)},
		{"invalid", datasource.Errorf(datasource.ErrInvalidQuery, "%s rejected", address)},
		{"deadline", fmt.Errorf("%s: %w", address, context.DeadlineExceeded)},
		{"wrapped", errors.Join(errors.New(address), datasource.Errorf(nil, "%s down", address))},
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
			if message == "" {
				t.Fatal("an unhealthy datasource should explain itself")
			}
			for _, leak := range []string{"10.1.2.3", "9090", "http://", "bob"} {
				if strings.Contains(message, leak) {
					t.Errorf("message %q contains %q", message, leak)
				}
			}
		})
	}
}
