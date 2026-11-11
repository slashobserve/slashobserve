package queryproxy

import (
	"context"
	"maps"
	"slices"
	"sync"

	"golang.org/x/sync/semaphore"
)

const (
	DefaultMaxConcurrentQueries = 32
	maxUpstreamCalls            = 32
)

var defaultSlots = NewSlots(nil, nil)

type SlotUsage struct {
	Datasource string
	InUse      int
	Limit      int
}

type Slots struct {
	totals  map[string]int
	share   func(total int) int
	process *semaphore.Weighted

	mu    sync.Mutex
	inUse map[string]int
	freed chan struct{}
}

func NewSlots(totals map[string]int, share func(total int) int) *Slots {
	if share == nil {
		share = func(total int) int { return total }
	}
	return &Slots{
		totals:  maps.Clone(totals),
		share:   share,
		process: semaphore.NewWeighted(maxUpstreamCalls),
		inUse:   map[string]int{},
		freed:   make(chan struct{}),
	}
}

func WithSlots(s *Slots) RunnerOption {
	return func(r *Runner) { r.slots = s }
}

func (s *Slots) limit(datasourceUID string) int {
	total := s.totals[datasourceUID]
	if total < 1 {
		total = DefaultMaxConcurrentQueries
	}
	return max(1, s.share(total))
}

func (s *Slots) Acquire(ctx context.Context, datasourceUID string) (release func(), err error) {
	if err := s.takeDatasource(ctx, datasourceUID); err != nil {
		return nil, err
	}
	if err := s.process.Acquire(ctx, 1); err != nil {
		s.releaseDatasource(datasourceUID)
		return nil, err
	}
	return func() {
		s.process.Release(1)
		s.releaseDatasource(datasourceUID)
	}, nil
}

func (s *Slots) takeDatasource(ctx context.Context, datasourceUID string) error {
	for {
		s.mu.Lock()
		if s.inUse[datasourceUID] < s.limit(datasourceUID) {
			s.inUse[datasourceUID]++
			s.mu.Unlock()
			return nil
		}
		freed := s.freed
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-freed:
		}
	}
}

func (s *Slots) releaseDatasource(datasourceUID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inUse[datasourceUID]--; s.inUse[datasourceUID] <= 0 {
		delete(s.inUse, datasourceUID)
	}
	close(s.freed)
	s.freed = make(chan struct{})
}

func (s *Slots) Usage() []SlotUsage {
	s.mu.Lock()
	defer s.mu.Unlock()
	uids := map[string]struct{}{}
	for uid := range s.totals {
		uids[uid] = struct{}{}
	}
	for uid := range s.inUse {
		uids[uid] = struct{}{}
	}
	usage := make([]SlotUsage, 0, len(uids))
	for _, uid := range slices.Sorted(maps.Keys(uids)) {
		usage = append(usage, SlotUsage{Datasource: uid, InUse: s.inUse[uid], Limit: s.limit(uid)})
	}
	return usage
}
