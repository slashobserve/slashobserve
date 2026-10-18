package ttlcache

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

const testKey = "k"

func waiters(g *Group[string], key string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	if c, ok := g.calls[key]; ok {
		return c.waiters
	}
	return 0
}

func waitForWaiters(t *testing.T, g *Group[string], key string, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for waiters(g, key) != n {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d waiters, have %d", n, waiters(g, key))
		}
		time.Sleep(time.Millisecond)
	}
}

type outcome struct {
	value string
	err   error
}

func doAsync(ctx context.Context, g *Group[string], key string, fn func(context.Context) (string, error)) <-chan outcome {
	out := make(chan outcome, 1)
	go func() {
		v, err := g.Do(ctx, key, fn)
		out <- outcome{v, err}
	}()
	return out
}

func receive(t *testing.T, ch <-chan outcome) outcome {
	t.Helper()
	select {
	case o := <-ch:
		return o
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a caller to return")
		return outcome{}
	}
}

func TestGroupConcurrentCallersRunOnce(t *testing.T) {
	const callers = 20
	g := NewGroup[string](time.Minute)
	release := make(chan struct{})
	var mu sync.Mutex
	runs := 0
	fn := func(context.Context) (string, error) {
		mu.Lock()
		runs++
		mu.Unlock()
		<-release
		return "shared", nil
	}

	results := make([]<-chan outcome, callers)
	for i := range results {
		results[i] = doAsync(context.Background(), g, testKey, fn)
	}
	waitForWaiters(t, g, testKey, callers)
	close(release)

	for i, ch := range results {
		if o := receive(t, ch); o.err != nil || o.value != "shared" {
			t.Fatalf("caller %d = %+v", i, o)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if runs != 1 {
		t.Fatalf("fn ran %d times, want 1", runs)
	}
}

func TestGroupLeaderCancelDoesNotFailOthers(t *testing.T) {
	g := NewGroup[string](time.Minute)
	release := make(chan struct{})
	sharedErr := make(chan error, 1)
	fn := func(ctx context.Context) (string, error) {
		select {
		case <-release:
			return "value", nil
		case <-ctx.Done():
			sharedErr <- ctx.Err()
			return "", ctx.Err()
		}
	}

	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	leader := doAsync(leaderCtx, g, testKey, fn)
	waitForWaiters(t, g, testKey, 1)
	follower := doAsync(context.Background(), g, testKey, fn)
	waitForWaiters(t, g, testKey, 2)

	cancelLeader()
	if o := receive(t, leader); !errors.Is(o.err, context.Canceled) {
		t.Fatalf("leader err = %v, want context.Canceled", o.err)
	}
	waitForWaiters(t, g, testKey, 1)
	select {
	case err := <-sharedErr:
		t.Fatalf("the shared call was canceled by the leader leaving: %v", err)
	default:
	}

	close(release)
	if o := receive(t, follower); o.err != nil || o.value != "value" {
		t.Fatalf("follower = %+v", o)
	}
}

func TestGroupCallerCancelReturnsItsContextError(t *testing.T) {
	g := NewGroup[string](time.Minute)
	release := make(chan struct{})
	defer close(release)
	fn := func(context.Context) (string, error) { <-release; return "late", nil }

	ctx, cancel := context.WithCancel(context.Background())
	caller := doAsync(ctx, g, testKey, fn)
	waitForWaiters(t, g, testKey, 1)
	cancel()

	o := receive(t, caller)
	if !errors.Is(o.err, context.Canceled) || o.value != "" {
		t.Fatalf("caller = %+v, want context.Canceled and no value", o)
	}
}

func TestGroupLastCallerLeavingCancelsSharedContext(t *testing.T) {
	g := NewGroup[string](time.Minute)
	started := make(chan struct{})
	canceled := make(chan struct{})
	fn := func(ctx context.Context) (string, error) {
		close(started)
		<-ctx.Done()
		close(canceled)
		return "", ctx.Err()
	}

	first, cancelFirst := context.WithCancel(context.Background())
	second, cancelSecond := context.WithCancel(context.Background())
	a := doAsync(first, g, testKey, fn)
	<-started
	b := doAsync(second, g, testKey, fn)
	waitForWaiters(t, g, testKey, 2)

	cancelFirst()
	receive(t, a)
	select {
	case <-canceled:
		t.Fatal("the shared context was canceled while a caller was still waiting")
	case <-time.After(50 * time.Millisecond):
	}

	cancelSecond()
	receive(t, b)
	select {
	case <-canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("the shared context was not canceled after the last caller left")
	}
}

func TestGroupNewCallerAfterAbandonedCallStartsFreshCall(t *testing.T) {
	g := NewGroup[string](time.Minute)
	oldCanceled := make(chan struct{})
	releaseOld := make(chan struct{})
	old := func(ctx context.Context) (string, error) {
		<-ctx.Done()
		close(oldCanceled)
		<-releaseOld
		return "", ctx.Err()
	}

	ctx, cancel := context.WithCancel(context.Background())
	first := doAsync(ctx, g, testKey, old)
	waitForWaiters(t, g, testKey, 1)
	cancel()
	receive(t, first)
	<-oldCanceled

	v, err := g.Do(context.Background(), testKey, func(context.Context) (string, error) { return "fresh", nil })
	if err != nil || v != "fresh" {
		t.Fatalf("new caller = %q, %v; it must not join the canceled call", v, err)
	}
	close(releaseOld)
}

func TestGroupTimeoutCancelsTheSharedCall(t *testing.T) {
	g := NewGroup[string](20 * time.Millisecond)
	_, err := g.Do(context.Background(), testKey, func(ctx context.Context) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
}

func TestGroupPanicBecomesAnError(t *testing.T) {
	g := NewGroup[string](time.Minute)
	release := make(chan struct{})
	fn := func(context.Context) (string, error) {
		<-release
		panic("boom")
	}

	a := doAsync(context.Background(), g, testKey, fn)
	waitForWaiters(t, g, testKey, 1)
	b := doAsync(context.Background(), g, testKey, fn)
	waitForWaiters(t, g, testKey, 2)
	close(release)

	for _, ch := range []<-chan outcome{a, b} {
		if o := receive(t, ch); !errors.Is(o.err, ErrPanic) || o.value != "" {
			t.Fatalf("caller = %+v, want ErrPanic", o)
		}
	}
	v, err := g.Do(context.Background(), testKey, func(context.Context) (string, error) { return "after", nil })
	if err != nil || v != "after" {
		t.Fatalf("group unusable after a panic: %q, %v", v, err)
	}
}

func TestGroupCleansUpFinishedCalls(t *testing.T) {
	g := NewGroup[string](time.Minute)
	if _, err := g.Do(context.Background(), testKey, func(context.Context) (string, error) { return "v", nil }); err != nil {
		t.Fatal(err)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.calls) != 0 {
		t.Fatalf("%d calls left behind", len(g.calls))
	}
}
