package db

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

type waitResult struct {
	payload string
	err     error
}

type fakeNotificationConn struct {
	mu        sync.Mutex
	results   chan waitResult
	pings     int
	pingErr   error
	waitCalls int
}

func newFakeNotificationConn() *fakeNotificationConn {
	return &fakeNotificationConn{results: make(chan waitResult, 8)}
}

func (f *fakeNotificationConn) WaitForNotification(ctx context.Context) (*pgconn.Notification, error) {
	f.mu.Lock()
	f.waitCalls++
	f.mu.Unlock()
	select {
	case r := <-f.results:
		if r.err != nil {
			return nil, r.err
		}
		return &pgconn.Notification{Payload: r.payload}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (f *fakeNotificationConn) Ping(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pings++
	return f.pingErr
}

func (f *fakeNotificationConn) stats() (waits, pings int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.waitCalls, f.pings
}

func runWait(ctx context.Context, conn notificationConn, period time.Duration, handle func(string)) <-chan error {
	done := make(chan error, 1)
	go func() { done <- waitForNotifications(ctx, conn, "chan", period, handle) }()
	return done
}

func expectError(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("waitForNotifications did not return")
		return nil
	}
}

func TestWaitForNotificationsPingsIdleConnectionAndKeepsWaiting(t *testing.T) {
	conn := newFakeNotificationConn()
	got := make(chan string, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := runWait(ctx, conn, 5*time.Millisecond, func(p string) { got <- p })

	deadline := time.After(5 * time.Second)
	for {
		if _, pings := conn.stats(); pings >= 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("idle connection was not pinged")
		case <-time.After(time.Millisecond):
		}
	}
	conn.results <- waitResult{payload: "hello"}
	select {
	case p := <-got:
		if p != "hello" {
			t.Fatalf("payload %q", p)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("notification was not handled after pings")
	}
	cancel()
	if err := expectError(t, done); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

func TestWaitForNotificationsReturnsPingFailure(t *testing.T) {
	conn := newFakeNotificationConn()
	conn.pingErr = errors.New("connection reset")

	err := expectError(t, runWait(context.Background(), conn, time.Millisecond, func(string) {}))

	if !errors.Is(err, conn.pingErr) {
		t.Fatalf("got %v", err)
	}
}

func TestWaitForNotificationsReturnsWaitFailureWithoutPing(t *testing.T) {
	conn := newFakeNotificationConn()
	failure := errors.New("server closed the connection")
	conn.results <- waitResult{err: failure}

	err := expectError(t, runWait(context.Background(), conn, time.Hour, func(string) {}))

	if !errors.Is(err, failure) {
		t.Fatalf("got %v", err)
	}
	if _, pings := conn.stats(); pings != 0 {
		t.Fatalf("pinged %d times", pings)
	}
}

func TestWaitForNotificationsReturnsPromptlyOnParentCancel(t *testing.T) {
	conn := newFakeNotificationConn()
	ctx, cancel := context.WithCancel(context.Background())
	done := runWait(ctx, conn, time.Hour, func(string) {})

	cancel()

	if err := expectError(t, done); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	if _, pings := conn.stats(); pings != 0 {
		t.Fatalf("pinged %d times", pings)
	}
}
