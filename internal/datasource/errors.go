package datasource

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidQuery   = errors.New("invalid query")
	ErrForbiddenQuery = errors.New("query not allowed")
	ErrLimitExceeded  = errors.New("query limit exceeded")
	ErrTimeout        = errors.New("query timed out")
	ErrUnavailable    = errors.New("datasource unavailable")
	ErrNotFound       = errors.New("not found")
)

type UpstreamError struct {
	Kind    error
	Message string
}

func (e *UpstreamError) Error() string { return e.Kind.Error() + ": " + e.Message }

func (e *UpstreamError) Unwrap() error { return e.Kind }

func Errorf(kind error, format string, args ...any) error {
	return &UpstreamError{Kind: kind, Message: fmt.Sprintf(format, args...)}
}
