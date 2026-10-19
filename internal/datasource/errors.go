package datasource

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const timeoutRounding = 100 * time.Millisecond

var (
	ErrInvalidQuery   = errors.New("invalid query")
	ErrForbiddenQuery = errors.New("query not allowed")
	ErrLimitExceeded  = errors.New("query limit exceeded")
	ErrTimeout        = errors.New("query timed out")
	ErrUnavailable    = errors.New("datasource unavailable")
	ErrNotFound       = errors.New("not found")

	ErrNotReadOnly = refine(ErrForbiddenQuery)
	ErrUnsafeGrant = refine(ErrForbiddenQuery)
	ErrAuthFailed  = refine(ErrUnavailable)
)

// refinedKind reads and matches as its parent, so it adds a distinguishable kind without changing any error text.
type refinedKind struct{ parent error }

func refine(parent error) error { return &refinedKind{parent: parent} }

func (k *refinedKind) Error() string { return k.parent.Error() }

func (k *refinedKind) Unwrap() error { return k.parent }

type UpstreamError struct {
	Kind    error
	Message string
}

func (e *UpstreamError) Error() string {
	if e.Kind == nil {
		return e.Message
	}
	return e.Kind.Error() + ": " + e.Message
}

func (e *UpstreamError) Unwrap() error { return e.Kind }

// ConfigError is a configuration failure whose text the server wrote without echoing any setting value,
// so it is safe to log.
type ConfigError struct{ Message string }

func (e *ConfigError) Error() string { return e.Message }

func ConfigErrorf(format string, args ...any) error {
	return &ConfigError{Message: fmt.Sprintf(format, args...)}
}

func IsTransient(err error) bool {
	return errors.Is(err, ErrUnavailable) || errors.Is(err, ErrTimeout) || errors.Is(err, context.Canceled)
}

func Errorf(kind error, format string, args ...any) error {
	if kind == nil {
		kind = ErrUnavailable
	}
	return &UpstreamError{Kind: kind, Message: fmt.Sprintf(format, args...)}
}

// Drivers arm socket deadlines from the context, so an expired or cancelled call can surface as a plain
// network error before the context's own state is visible; the context is therefore checked directly.
func ContextError(ctx context.Context, err error, timeout time.Duration) error {
	switch {
	case deadlinePassed(ctx) || errors.Is(err, context.DeadlineExceeded):
		if timeout <= 0 {
			return Errorf(ErrTimeout, "query timed out")
		}
		if timeout >= timeoutRounding {
			timeout = timeout.Round(timeoutRounding)
		}
		return Errorf(ErrTimeout, "query timed out after %s", timeout)
	case errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled):
		return context.Canceled
	}
	return nil
}

func deadlinePassed(ctx context.Context) bool {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return true
	}
	deadline, ok := ctx.Deadline()
	return ok && !time.Now().Before(deadline)
}
