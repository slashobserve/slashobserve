package queryproxy

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync/atomic"
	"testing"
	"time"
)

const slotWait = 30 * time.Millisecond

func mustAcquire(t *testing.T, s *Slots, uid string) func() {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	release, err := s.Acquire(ctx, uid)
	if err != nil {
		t.Fatalf("Acquire(%q): %v", uid, err)
	}
	return release
}

func blocks(s *Slots, uid string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), slotWait)
	defer cancel()
	release, err := s.Acquire(ctx, uid)
	if err == nil {
		release()
		return false
	}
	return errors.Is(err, context.DeadlineExceeded)
}

func acquireInBackground(s *Slots, uid string) <-chan func() {
	got := make(chan func(), 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if release, err := s.Acquire(ctx, uid); err == nil {
			got <- release
		}
	}()
	return got
}

func awaitRelease(t *testing.T, got <-chan func()) func() {
	t.Helper()
	select {
	case release := <-got:
		return release
	case <-time.After(5 * time.Second):
		t.Fatal("the waiter never got a slot")
		return nil
	}
}

func TestSlotsCapEachDatasourceAtItsTotal(t *testing.T) {
	s := NewSlots(map[string]int{"a": 2, "b": 2}, nil)
	first, second := mustAcquire(t, s, "a"), mustAcquire(t, s, "a")
	if !blocks(s, "a") {
		t.Fatal("a third acquire on a datasource with a total of 2 did not wait")
	}
	other := mustAcquire(t, s, "b")
	other()

	waiter := acquireInBackground(s, "a")
	first()
	awaitRelease(t, waiter)()
	second()
}

func TestSlotsDefaultTheTotalWhenMissingOrNotPositive(t *testing.T) {
	s := NewSlots(map[string]int{"zero": 0, "negative": -4}, nil)
	for _, u := range s.Usage() {
		if u.Limit != DefaultMaxConcurrentQueries {
			t.Errorf("limit of %q = %d, want %d", u.Datasource, u.Limit, DefaultMaxConcurrentQueries)
		}
	}
	releases := make([]func(), DefaultMaxConcurrentQueries)
	for i := range releases {
		releases[i] = mustAcquire(t, s, "unlisted")
	}
	if !blocks(s, "unlisted") {
		t.Errorf("acquire number %d on an unlisted datasource did not wait", DefaultMaxConcurrentQueries+1)
	}
	for _, release := range releases {
		release()
	}
}

func TestSlotsShareDividesTheTotalAndNeverGoesBelowOne(t *testing.T) {
	var live atomic.Int64
	live.Store(2)
	share := func(total int) int { return max(1, (total+int(live.Load())-1)/int(live.Load())) }
	s := NewSlots(map[string]int{"a": 4}, share)

	first, second := mustAcquire(t, s, "a"), mustAcquire(t, s, "a")
	if !blocks(s, "a") {
		t.Fatal("a share of 2 allowed a third acquire")
	}

	live.Store(100)
	first()
	second()
	third := mustAcquire(t, s, "a")
	if !blocks(s, "a") {
		t.Fatal("a share that rounds to 1 allowed two holders")
	}
	third()
}

func TestSlotsLetAWaiterInWhenTheShareGrows(t *testing.T) {
	var share atomic.Int64
	share.Store(1)
	s := NewSlots(map[string]int{"a": 4}, func(int) int { return int(share.Load()) })
	held := mustAcquire(t, s, "a")
	other := mustAcquire(t, s, "b")

	waiter := acquireInBackground(s, "a")
	time.Sleep(slotWait)
	select {
	case <-waiter:
		t.Fatal("the waiter got a slot beyond the share")
	default:
	}

	share.Store(2)
	other()
	awaitRelease(t, waiter)()
	held()
}

func TestSlotsLeaveUsageUnchangedWhenTheContextEnds(t *testing.T) {
	s := NewSlots(map[string]int{"a": 1}, nil)
	held := mustAcquire(t, s, "a")
	defer held()
	before := s.Usage()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.Acquire(ctx, "a"); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if after := s.Usage(); !slices.Equal(before, after) {
		t.Errorf("usage changed from %v to %v", before, after)
	}
}

func TestSlotsCapAllDatasourcesTogether(t *testing.T) {
	s := NewSlots(nil, nil)
	releases := make([]func(), maxUpstreamCalls)
	for i := range releases {
		releases[i] = mustAcquire(t, s, fmt.Sprintf("ds%02d", i%40))
	}
	if !blocks(s, "ds39") {
		t.Fatal("the 33rd call across datasources did not wait")
	}
	if got := inUseTotal(s); got != maxUpstreamCalls {
		t.Errorf("in use = %d after a refused acquire, want %d", got, maxUpstreamCalls)
	}
	waiter := acquireInBackground(s, "ds39")
	releases[0]()
	awaitRelease(t, waiter)()
	for _, release := range releases[1:] {
		release()
	}
	if got := inUseTotal(s); got != 0 {
		t.Errorf("in use = %d after every release", got)
	}
}

func inUseTotal(s *Slots) int {
	total := 0
	for _, u := range s.Usage() {
		total += u.InUse
	}
	return total
}

func TestSlotsUsageListsEveryDatasourceSortedByUID(t *testing.T) {
	s := NewSlots(map[string]int{"prom": 8, "loki": 0, "ch": 64}, func(total int) int { return total / 2 })
	release := mustAcquire(t, s, "prom")
	defer release()

	want := []SlotUsage{
		{Datasource: "ch", Limit: 32},
		{Datasource: "loki", Limit: 16},
		{Datasource: "prom", InUse: 1, Limit: 4},
	}
	if got := s.Usage(); !slices.Equal(got, want) {
		t.Errorf("Usage() = %v, want %v", got, want)
	}
}
