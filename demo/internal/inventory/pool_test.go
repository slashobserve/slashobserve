package inventory

import (
	"context"
	"sync"
	"testing"
	"time"
)

func waitFor(t *testing.T, what string, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !check() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestPoolCountsHeldSlotsAndBlocksWhenFull(t *testing.T) {
	pool := NewPool(2)
	if pool.Size() != 2 || pool.InUse() != 0 {
		t.Fatalf("new pool: size %d, in use %d; want 2, 0", pool.Size(), pool.InUse())
	}
	release, cancel := context.WithCancel(context.Background())
	var holders sync.WaitGroup
	for range 2 {
		holders.Go(func() { pool.Hold(release, time.Hour) })
	}
	waitFor(t, "both slots to be held", func() bool { return pool.InUse() == 2 })

	waiting, stopWaiting := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer stopWaiting()
	started := time.Now()
	if pool.Hold(waiting, time.Millisecond) {
		t.Fatal("Hold on a full pool succeeded")
	}
	if waited := time.Since(started); waited < 50*time.Millisecond {
		t.Fatalf("Hold on a full pool gave up after %s, want it to wait for its context", waited)
	}

	cancel()
	holders.Wait()
	if got := pool.InUse(); got != 0 {
		t.Fatalf("in use after every holder returned = %d, want 0", got)
	}
}

func TestPoolHoldReleasesTheSlotWhenDone(t *testing.T) {
	pool := NewPool(1)
	for range 3 {
		if !pool.Hold(context.Background(), time.Millisecond) {
			t.Fatal("Hold on a free pool failed")
		}
	}
	if got := pool.InUse(); got != 0 {
		t.Fatalf("in use after the holds finished = %d, want 0", got)
	}
}
