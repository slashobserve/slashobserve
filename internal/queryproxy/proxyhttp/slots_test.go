package proxyhttp

import "testing"

func newTestSlots() *slots { return &slots{held: map[string]int{}} }

func TestSlotsRefuseTheNinthConcurrentRequest(t *testing.T) {
	s := newTestSlots()
	var releases []func()
	for range maxInFlightPerPrincipal {
		release, ok := s.acquire("alice")
		if !ok {
			t.Fatal("refused before the limit")
		}
		releases = append(releases, release)
	}
	if _, ok := s.acquire("alice"); ok {
		t.Fatal("the ninth concurrent request was admitted")
	}
	if _, ok := s.acquire("bob"); !ok {
		t.Fatal("another principal was refused")
	}
	releases[0]()
	if _, ok := s.acquire("alice"); !ok {
		t.Fatal("a freed slot was not reusable")
	}
}

func TestSlotsDoubleReleaseFreesOnce(t *testing.T) {
	s := newTestSlots()
	var releases []func()
	for range maxInFlightPerPrincipal {
		release, _ := s.acquire("alice")
		releases = append(releases, release)
	}
	releases[0]()
	releases[0]()

	if _, ok := s.acquire("alice"); !ok {
		t.Fatal("the released slot was not freed")
	}
	if _, ok := s.acquire("alice"); ok {
		t.Fatal("a double release freed two slots")
	}
}

func TestSlotsCleanUpWhenIdle(t *testing.T) {
	s := newTestSlots()
	releaseA, _ := s.acquire("alice")
	releaseB, _ := s.acquire("bob")
	releaseA()
	releaseB()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.held) != 0 {
		t.Fatalf("%d principals left in the map: %v", len(s.held), s.held)
	}
}
