package tctest

import (
	"context"
	"strings"
	"time"

	"github.com/testcontainers/testcontainers-go"
)

const (
	attempts = 8
	backoff  = 500 * time.Millisecond
)

// Retry works around testcontainers-go v0.44.0 failing a start with "reaper: ... wait for reaper ...: unexpected container status"
// when several test binaries of one `go test ./...` run find the shared reaper container before its creator has started it.
// A failed attempt leaves a created container behind, so each one is terminated before the next attempt.
func Retry[T testcontainers.Container](ctx context.Context, start func() (T, error)) (T, error) {
	return retry(ctx, attempts, backoff, start)
}

func retry[T testcontainers.Container](ctx context.Context, limit int, pause time.Duration, start func() (T, error)) (T, error) {
	var zero T
	for attempt := 1; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		c, err := start()
		if err == nil || attempt == limit || !isReaperRace(err) {
			return c, err
		}
		_ = testcontainers.TerminateContainer(c)
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(time.Duration(attempt) * pause):
		}
	}
}

func isReaperRace(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "reaper:") && strings.Contains(msg, "wait for reaper") && strings.Contains(msg, "unexpected container status")
}
