package prng

import (
	"hash/fnv"
	"math/rand/v2"
	"sync"
	"time"
)

type Source struct {
	mu sync.Mutex
	r  *rand.Rand
}

func ForPod(pod string) *Source {
	h := fnv.New64a()
	_, _ = h.Write([]byte(pod))
	seed := h.Sum64()
	return &Source{r: rand.New(rand.NewPCG(seed, seed))} //nolint:gosec // deterministic seeded demo traffic, not security sensitive
}

func (s *Source) Between(lo, hi time.Duration) time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return lo + time.Duration(s.r.Int64N(int64(hi-lo)+1))
}

func (s *Source) Chance(probability float64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.r.Float64() < probability
}

func (s *Source) Percent(pct int) bool {
	if pct <= 0 {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.r.IntN(100) < pct
}
