package stream

import (
	"net/http"
	"sync"
)

type slots struct {
	perPrincipal int
	perProcess   int

	mu          sync.Mutex
	total       int
	byPrincipal map[string]int
}

func newSlots(perPrincipal, perProcess int) *slots {
	return &slots{perPrincipal: perPrincipal, perProcess: perProcess, byPrincipal: map[string]int{}}
}

func (s *slots) acquire(key string) (int, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.byPrincipal[key] >= s.perPrincipal {
		return http.StatusTooManyRequests, "too_many_streams"
	}
	if s.total >= s.perProcess {
		return http.StatusServiceUnavailable, "too_many_streams"
	}
	s.byPrincipal[key]++
	s.total++
	return 0, ""
}

func (s *slots) release(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.total--
	if s.byPrincipal[key]--; s.byPrincipal[key] <= 0 {
		delete(s.byPrincipal, key)
	}
}
