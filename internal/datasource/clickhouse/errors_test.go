package clickhouse

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	ch "github.com/ClickHouse/clickhouse-go/v2"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

type staleContext struct {
	context.Context
	deadline time.Time
}

func (c staleContext) Deadline() (time.Time, bool) { return c.deadline, true }
func (c staleContext) Err() error                  { return nil }

func TestErrorMappingByContext(t *testing.T) {
	expired, cancelExpired := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancelExpired()
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	live, cancelLive := context.WithTimeout(context.Background(), time.Minute)
	defer cancelLive()

	socketTimeout := &net.OpError{Op: "read", Net: "tcp", Err: os.ErrDeadlineExceeded}
	refused := &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
	exception := &ch.Exception{Code: 62, Message: "Syntax error: failed at position 8"}

	tests := []struct {
		name        string
		ctx         context.Context
		err         error
		wantKind    error
		wantMessage string
	}{
		{name: "socket timeout after the deadline", ctx: expired, err: socketTimeout, wantKind: datasource.ErrTimeout, wantMessage: "after 1.5s"},
		{name: "socket timeout before the context timer fired", ctx: staleContext{Context: context.Background(), deadline: time.Now().Add(-time.Millisecond)}, err: socketTimeout, wantKind: datasource.ErrTimeout, wantMessage: "after 1.5s"},
		{name: "closed connection after the deadline", ctx: expired, err: io.EOF, wantKind: datasource.ErrTimeout, wantMessage: "after 1.5s"},
		{name: "closed connection after cancellation", ctx: cancelled, err: io.EOF, wantKind: context.Canceled},
		{name: "socket error after cancellation", ctx: cancelled, err: socketTimeout, wantKind: context.Canceled},
		{name: "connection refused with a live context", ctx: live, err: refused, wantKind: datasource.ErrUnavailable, wantMessage: "connection failed"},
		{name: "end of file with a live context", ctx: live, err: io.EOF, wantKind: datasource.ErrUnavailable, wantMessage: "connection failed"},
		{name: "exception with a passed deadline", ctx: expired, err: exception, wantKind: datasource.ErrInvalidQuery, wantMessage: "Syntax error"},
		{name: "exception after cancellation", ctx: cancelled, err: exception, wantKind: datasource.ErrInvalidQuery, wantMessage: "Syntax error"},
		{name: "context deadline error", ctx: live, err: context.DeadlineExceeded, wantKind: datasource.ErrTimeout, wantMessage: "after 1.5s"},
		{name: "context canceled error", ctx: live, err: context.Canceled, wantKind: context.Canceled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapError(tt.ctx, tt.err, 1500*time.Millisecond)
			if !errors.Is(got, tt.wantKind) {
				t.Fatalf("mapError = %v, want kind %v", got, tt.wantKind)
			}
			if !errors.Is(tt.wantKind, context.Canceled) && !strings.Contains(got.Error(), tt.wantMessage) {
				t.Fatalf("mapError = %q, want message containing %q", got, tt.wantMessage)
			}
		})
	}
}

func TestErrorMappingIsIdempotent(t *testing.T) {
	first := mapError(context.Background(), io.EOF, time.Second)
	if second := mapError(context.Background(), first, time.Second); !errors.Is(second, first) {
		t.Fatalf("second mapping = %v, want %v unchanged", second, first)
	}
}
