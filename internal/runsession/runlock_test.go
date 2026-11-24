package runsession

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunLockAdmitsOneHolderPerKey(t *testing.T) {
	var locks runLocks
	key := runKey{id: [16]byte{1}, attempt: 1}
	var inside, overlaps atomic.Int32
	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, err := locks.acquire(context.Background(), key)
			if err != nil {
				t.Error(err)
				return
			}
			if inside.Add(1) > 1 {
				overlaps.Add(1)
			}
			time.Sleep(time.Millisecond)
			inside.Add(-1)
			release()
		}()
	}
	wg.Wait()
	if overlaps.Load() != 0 {
		t.Fatalf("%d holders overlapped", overlaps.Load())
	}
	if locks.size() != 0 {
		t.Fatalf("%d entries left after every holder left", locks.size())
	}
}

func TestRunLockCountsHolderAndWaiters(t *testing.T) {
	var locks runLocks
	key := runKey{id: [16]byte{1}, attempt: 1}
	holder, err := locks.acquire(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	var waiter sync.WaitGroup
	waiter.Go(func() { _, _ = locks.acquire(ctx, key) })
	deadline := time.Now().Add(time.Second)
	for locks.waiters(key) != 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := locks.waiters(key); got != 2 {
		t.Fatalf("waiters = %d, want the holder and one waiter", got)
	}
	cancel()
	waiter.Wait()
	holder()
	holder()
	if locks.size() != 0 {
		t.Fatalf("%d entries left", locks.size())
	}
}

func TestRunLockKeysDoNotBlockEachOther(t *testing.T) {
	var locks runLocks
	first, err := locks.acquire(context.Background(), runKey{id: [16]byte{1}, attempt: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer first()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for _, key := range []runKey{{id: [16]byte{1}, attempt: 2}, {id: [16]byte{2}, attempt: 1}} {
		release, err := locks.acquire(ctx, key)
		if err != nil {
			t.Fatalf("key %+v waited behind another key: %v", key, err)
		}
		release()
	}
}

func TestRunLockWaiterThatGivesUpLeavesNoEntryAndKeepsTheHolder(t *testing.T) {
	var locks runLocks
	key := runKey{id: [16]byte{1}, attempt: 1}
	holder, err := locks.acquire(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := locks.acquire(ctx, key); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiter error = %v, want deadline exceeded", err)
	}
	if locks.size() != 1 {
		t.Fatalf("%d entries while the holder holds, want 1", locks.size())
	}
	holder()
	if locks.size() != 0 {
		t.Fatalf("%d entries after the holder left, want 0", locks.size())
	}
}

func TestRunLockAcquireWithACanceledContextLeavesNoEntry(t *testing.T) {
	var locks runLocks
	key := runKey{id: [16]byte{1}, attempt: 1}
	holder, err := locks.acquire(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := locks.acquire(ctx, key); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want canceled", err)
	}
	holder()
	if locks.size() != 0 {
		t.Fatalf("%d entries left", locks.size())
	}
}
