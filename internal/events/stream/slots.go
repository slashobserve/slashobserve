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

func (s *slots) acquire(key string) (status int, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.byPrincipal[key] >= s.perPrincipal {
		return http.StatusTooManyRequests, perCaller
	}
	if s.total >= s.perProcess {
		return http.StatusServiceUnavailable, serverCapacity
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
