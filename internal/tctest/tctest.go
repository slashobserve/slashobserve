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

// Retry works around testcontainers-go v0.44.0 failing a start in its reaper (ryuk) handling when several test binaries of one `go test ./...` run share a busy Docker host:
// a binary can find the shared reaper container before it has started or after it has stopped, and the reaper can die while it starts.
// A failed attempt leaves a created container behind, so each one is terminated before the next attempt.
// The errors are plain wrapped strings with no type to match, so the classification anchors on the reaper start prefixes of the chain; a failure of the container under test never carries them.
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
		if err == nil || attempt == limit || !isTransientReaperFailure(err) {
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

func isTransientReaperFailure(err error) bool {
	msg := err.Error()
	if !strings.Contains(msg, "reaper:") {
		return false
	}
	reuse := strings.Contains(msg, "wait for reaper")
	create := strings.Contains(msg, "new reaper: run container:")
	return (reuse || create) && reaperNotRunning(msg)
}

func reaperNotRunning(msg string) bool {
	return strings.Contains(msg, "unexpected container status") || strings.Contains(msg, "container exited with code 0")
}
