package llm

import (
	"context"
	"errors"
	"testing"
	"time"
)

type recordedSleeps struct{ waits []time.Duration }

func stubSleep(t *testing.T) *recordedSleeps {
	t.Helper()
	rec := &recordedSleeps{}
	original := sleep
	sleep = func(_ context.Context, d time.Duration) error {
		rec.waits = append(rec.waits, d)
		return nil
	}
	t.Cleanup(func() { sleep = original })
	return rec
}

func failing(status int, failure CallFailure) *CallError {
	return NewCallError("anthropic", failure, status)
}

func countingAttempt(errs ...error) (func(context.Context) (string, error), *int) {
	calls := 0
	return func(context.Context) (string, error) {
		calls++
		if calls <= len(errs) {
			return "", errs[calls-1]
		}
		return "ok", nil
	}, &calls
}

func TestRetryRetriesOnlyRetryableFailures(t *testing.T) {
	idle := IdleTimeoutError("anthropic")
	tests := []struct {
		name      string
		err       error
		wantCalls int
	}{
		{"rate limited", failing(429, RateLimited), 4},
		{"idle timeout", idle, 4},
		{"request timeout status", failing(408, ProviderError), 4},
		{"internal error", failing(500, ProviderError), 4},
		{"bad gateway", failing(502, ProviderError), 4},
		{"unavailable", failing(503, ProviderError), 4},
		{"gateway timeout", failing(504, ProviderError), 4},
		{"overloaded", failing(529, ProviderError), 4},
		{"connection error", failing(0, ProviderError), 4},
		{"bad request", failing(400, ProviderError), 1},
		{"unauthorized", failing(401, ProviderError), 1},
		{"forbidden", failing(403, ProviderError), 1},
		{"not found", failing(404, ProviderError), 1},
		{"our own call timeout", failing(0, CallTimeout), 1},
		{"not a call error", errors.New("boom"), 1},
		{"a caller cancel", context.Canceled, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := stubSleep(t)
			attempt, calls := countingAttempt(tt.err, tt.err, tt.err, tt.err, tt.err)
			_, err := Retry(context.Background(), attempt)
			if !errors.Is(err, tt.err) {
				t.Fatalf("err = %v, want %v", err, tt.err)
			}
			if *calls != tt.wantCalls || len(rec.waits) != tt.wantCalls-1 {
				t.Fatalf("%d attempts and %d waits, want %d attempts", *calls, len(rec.waits), tt.wantCalls)
			}
		})
	}
}

func TestRetrySucceedsOnALaterAttemptAndBacksOffWithJitter(t *testing.T) {
	rec := stubSleep(t)
	attempt, calls := countingAttempt(failing(503, ProviderError), failing(503, ProviderError), failing(503, ProviderError))
	got, err := Retry(context.Background(), attempt)
	if err != nil || got != "ok" || *calls != 4 {
		t.Fatalf("got %q, err %v after %d attempts", got, err, *calls)
	}
	bounds := [][2]time.Duration{{500 * time.Millisecond, time.Second}, {time.Second, 2 * time.Second}, {2 * time.Second, 4 * time.Second}}
	if len(rec.waits) != len(bounds) {
		t.Fatalf("waits = %v", rec.waits)
	}
	for i, wait := range rec.waits {
		if wait < bounds[i][0] || wait > bounds[i][1] {
			t.Fatalf("wait %d = %s, want within %s and %s", i, wait, bounds[i][0], bounds[i][1])
		}
	}
}

func TestRetryHonorsRetryAfterUpToItsCap(t *testing.T) {
	tests := []struct {
		name       string
		retryAfter time.Duration
		want       time.Duration
	}{
		{"larger than the backoff", 20 * time.Second, 20 * time.Second},
		{"capped", 5 * time.Minute, maxRetryAfter},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := stubSleep(t)
			limited := failing(429, RateLimited)
			limited.RetryAfter = tt.retryAfter
			attempt, _ := countingAttempt(limited)
			if _, err := Retry(context.Background(), attempt); err != nil {
				t.Fatalf("Retry: %v", err)
			}
			if len(rec.waits) != 1 || rec.waits[0] != tt.want {
				t.Fatalf("waits = %v, want [%s]", rec.waits, tt.want)
			}
		})
	}

	t.Run("smaller than the backoff is ignored", func(t *testing.T) {
		rec := stubSleep(t)
		limited := failing(429, RateLimited)
		limited.RetryAfter = time.Nanosecond
		attempt, _ := countingAttempt(limited)
		if _, err := Retry(context.Background(), attempt); err != nil {
			t.Fatalf("Retry: %v", err)
		}
		if len(rec.waits) != 1 || rec.waits[0] < baseBackoff/2 {
			t.Fatalf("waits = %v, want at least %s", rec.waits, baseBackoff/2)
		}
	})
}

func TestRetryStopsBeforeTheDeadline(t *testing.T) {
	rec := stubSleep(t)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	attempt, calls := countingAttempt(failing(503, ProviderError), failing(503, ProviderError))
	_, err := Retry(ctx, attempt)
	var callErr *CallError
	if !errors.As(err, &callErr) || callErr.Status != 503 {
		t.Fatalf("err = %v, want the last attempt's error", err)
	}
	if *calls != 1 || len(rec.waits) != 0 {
		t.Fatalf("%d attempts and %d waits, want one attempt and no wait that passes the deadline", *calls, len(rec.waits))
	}
}

func TestRetryNeverRetriesACancel(t *testing.T) {
	rec := stubSleep(t)
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	_, err := Retry(ctx, func(context.Context) (string, error) {
		calls++
		cancel()
		return "", failing(503, ProviderError)
	})
	if err == nil || calls != 1 || len(rec.waits) != 0 {
		t.Fatalf("err %v after %d attempts and %d waits, want one attempt", err, calls, len(rec.waits))
	}
}

func TestRetryStopsWhenTheWaitIsInterrupted(t *testing.T) {
	original := sleep
	t.Cleanup(func() { sleep = original })
	sleep = func(context.Context, time.Duration) error { return context.Canceled }
	attempt, calls := countingAttempt(failing(503, ProviderError), failing(503, ProviderError))
	if _, err := Retry(context.Background(), attempt); err == nil || *calls != 1 {
		t.Fatalf("err %v after %d attempts, want the first error after one attempt", err, *calls)
	}
}

func TestParseRetryAfter(t *testing.T) {
	tests := []struct {
		header string
		want   time.Duration
	}{
		{"7", 7 * time.Second},
		{"0", 0},
		{"-3", 0},
		{"", 0},
		{"Wed, 21 Oct 2026 07:28:00 GMT", 0},
		{"99999999999999999999", 0},
		{"86400", maxRetryAfter},
	}
	for _, tt := range tests {
		if got := ParseRetryAfter(tt.header); got != tt.want {
			t.Errorf("ParseRetryAfter(%q) = %s, want %s", tt.header, got, tt.want)
		}
	}
}
