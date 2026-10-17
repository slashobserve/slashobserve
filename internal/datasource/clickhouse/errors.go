package clickhouse

import (
	"context"
	"errors"
	"time"
	"unicode/utf8"

	ch "github.com/ClickHouse/clickhouse-go/v2"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const maxExceptionMessage = 512

func mapError(err error, timeout time.Duration) error {
	if err == nil {
		return nil
	}
	var ue *datasource.UpstreamError
	if errors.As(err, &ue) {
		return err
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return datasource.Errorf(datasource.ErrTimeout, "query timed out after %s", timeout)
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	var ex *ch.Exception
	if !errors.As(err, &ex) {
		return datasource.Errorf(datasource.ErrUnavailable, "connection failed")
	}
	switch ex.Code {
	case 159:
		return datasource.Errorf(datasource.ErrTimeout, "%s", truncate(ex.Message))
	case 158, 307, 396:
		return datasource.Errorf(datasource.ErrLimitExceeded, "%s", truncate(ex.Message))
	case 164, 497:
		return datasource.Errorf(datasource.ErrForbiddenQuery, "not allowed for the read-only user")
	case 516, 192:
		return datasource.Errorf(datasource.ErrUnavailable, "authentication failed")
	}
	return datasource.Errorf(datasource.ErrInvalidQuery, "%s", truncate(ex.Message))
}

func truncate(s string) string {
	if len(s) <= maxExceptionMessage {
		return s
	}
	s = s[:maxExceptionMessage]
	for !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

func exceptionCode(err error) int32 {
	var ex *ch.Exception
	if errors.As(err, &ex) {
		return ex.Code
	}
	return 0
}

func messageOf(err error) string {
	var ue *datasource.UpstreamError
	if errors.As(err, &ue) {
		return ue.Message
	}
	return err.Error()
}
