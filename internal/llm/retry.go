package llm

import (
	"context"
	"errors"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"
)

const (
	maxAttempts   = 4
	baseBackoff   = time.Second
	maxBackoff    = 30 * time.Second
	maxRetryAfter = 60 * time.Second
)

var sleep = func(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func Retry[T any](ctx context.Context, attempt func(ctx context.Context) (T, error)) (T, error) {
	for n := 1; ; n++ {
		value, err := attempt(ctx)
		if err == nil || n >= maxAttempts || ctx.Err() != nil || !retryable(err) {
			return value, err
		}
		wait := retryWait(n-1, err)
		if deadline, ok := ctx.Deadline(); ok && time.Now().Add(wait).After(deadline) {
			return value, err
		}
		if sleep(ctx, wait) != nil {
			return value, err
		}
	}
}

func retryable(err error) bool {
	var callErr *CallError
	if !errors.As(err, &callErr) {
		return false
	}
	switch {
	case callErr.Failure == RateLimited || callErr.Idle:
		return true
	case callErr.Status == 0:
		return callErr.Failure == ProviderError
	}
	switch callErr.Status {
	case http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusBadGateway,
		http.StatusServiceUnavailable, http.StatusGatewayTimeout, 529:
		return true
	}
	return false
}

func retryWait(retry int, err error) time.Duration {
	ceiling := min(baseBackoff<<retry, maxBackoff)
	wait := ceiling/2 + rand.N(ceiling-ceiling/2+1) //nolint:gosec // jitter, not security
	var callErr *CallError
	if errors.As(err, &callErr) {
		wait = max(wait, min(callErr.RetryAfter, maxRetryAfter))
	}
	return wait
}

func ParseRetryAfter(header string) time.Duration {
	seconds, err := strconv.Atoi(header)
	if err != nil || seconds <= 0 {
		return 0
	}
	return time.Duration(min(seconds, int(maxRetryAfter/time.Second))) * time.Second
}
