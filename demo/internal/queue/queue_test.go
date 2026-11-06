package queue

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func startQueue(t *testing.T, workers, capacity int, pct *atomic.Int32, process func(Job[int])) *Queue[int] {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	q := New(workers, capacity, func() int { return int(pct.Load()) }, process)
	stopped := make(chan struct{})
	go func() {
		q.Run(ctx)
		close(stopped)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-stopped:
		case <-time.After(5 * time.Second):
			t.Error("Run did not return after its context was canceled")
		}
	})
	return q
}

func eventually(t *testing.T, what string, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !check() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestQueueDepthRisesWhilePausedAndDrainsAfterReset(t *testing.T) {
	var pct atomic.Int32
	pct.Store(100)
	var processed atomic.Int32
	q := startQueue(t, 4, 100, &pct, func(Job[int]) { processed.Add(1) })

	for i := range 6 {
		if !q.Push(i) {
			t.Fatalf("Push(%d) refused", i)
		}
	}
	time.Sleep(5 * pausePoll)
	if got := q.Depth(); got != 6 {
		t.Fatalf("depth while every worker is paused = %d, want 6", got)
	}
	if got := processed.Load(); got != 0 {
		t.Fatalf("processed while every worker is paused = %d, want 0", got)
	}
	if age := q.OldestAge(); age < 5*pausePoll {
		t.Fatalf("oldest age = %s, want at least %s", age, 5*pausePoll)
	}

	pct.Store(0)
	eventually(t, "the queue to drain", func() bool { return q.Depth() == 0 && processed.Load() == 6 })
	if age := q.OldestAge(); age != 0 {
		t.Fatalf("oldest age of an empty queue = %s, want 0", age)
	}
}

func TestQueuePausesWorkersRoundedUp(t *testing.T) {
	cases := []struct {
		pct, paused int
	}{
		{0, 0},
		{1, 1},
		{25, 1},
		{30, 2},
		{50, 2},
		{75, 3},
		{76, 4},
		{100, 4},
	}
	for _, tc := range cases {
		q := New(4, 10, func() int { return tc.pct }, func(Job[int]) {})
		if got := q.PausedWorkers(); got != tc.paused {
			t.Errorf("pct %d: paused workers = %d, want %d", tc.pct, got, tc.paused)
		}
	}
}

func TestQueuePartialPauseLeavesTheOtherWorkersProcessing(t *testing.T) {
	var pct atomic.Int32
	pct.Store(50)
	release := make(chan struct{})
	var busy atomic.Int32
	q := startQueue(t, 4, 100, &pct, func(Job[int]) {
		busy.Add(1)
		<-release
	})
	t.Cleanup(func() { close(release) })

	for i := range 10 {
		q.Push(i)
	}
	eventually(t, "two workers to take an order", func() bool { return busy.Load() == 2 })
	time.Sleep(5 * pausePoll)
	if got := busy.Load(); got != 2 {
		t.Fatalf("busy workers with half of 4 paused = %d, want 2", got)
	}
	if got := q.Depth(); got != 8 {
		t.Fatalf("depth = %d, want 8", got)
	}
}

func TestQueueRefusesPushWhenFull(t *testing.T) {
	var pct atomic.Int32
	pct.Store(100)
	q := startQueue(t, 1, 2, &pct, func(Job[int]) {})

	got := []bool{q.Push(1), q.Push(2), q.Push(3)}
	want := []bool{true, true, false}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Push results = %v, want %v", got, want)
		}
	}
	if depth := q.Depth(); depth != 2 {
		t.Fatalf("depth = %d, want 2", depth)
	}
}
