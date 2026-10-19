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

// Retry works around two testcontainers-go v0.44.0 reaper start failures that appear when several test binaries of one `go test ./...` run share a busy Docker host:
// "reaper: ... wait for reaper ...: unexpected container status" when a binary finds the shared reaper container before its creator has started it,
// and "reaper: new reaper: run container: ...: unexpected container status" or "container exited with code" when the reaper (ryuk) container dies while it starts.
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
	if !strings.Contains(msg, "reaper:") {
		return false
	}
	if strings.Contains(msg, "wait for reaper") && strings.Contains(msg, "unexpected container status") {
		return true
	}
	return strings.Contains(msg, "new reaper: run container:") &&
		(strings.Contains(msg, "unexpected container status") || strings.Contains(msg, "container exited with code"))
}
