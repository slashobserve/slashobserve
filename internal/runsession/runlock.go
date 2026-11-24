package runsession

import (
	"context"
	"sync"
)

type runKey struct {
	id      [16]byte
	attempt int
}

type runLocks struct {
	mu      sync.Mutex
	entries map[runKey]*runLock
}

type runLock struct {
	held    chan struct{}
	waiting int
}

func (l *runLocks) acquire(ctx context.Context, key runKey) (release func(), err error) {
	l.mu.Lock()
	if l.entries == nil {
		l.entries = make(map[runKey]*runLock)
	}
	entry := l.entries[key]
	if entry == nil {
		entry = &runLock{held: make(chan struct{}, 1)}
		l.entries[key] = entry
	}
	entry.waiting++
	l.mu.Unlock()

	select {
	case entry.held <- struct{}{}:
		return sync.OnceFunc(func() {
			<-entry.held
			l.leave(key, entry)
		}), nil
	case <-ctx.Done():
		l.leave(key, entry)
		return nil, ctx.Err()
	}
}

func (l *runLocks) leave(key runKey, entry *runLock) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry.waiting--
	if entry.waiting == 0 {
		delete(l.entries, key)
	}
}

func (l *runLocks) waiters(key runKey) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	if entry := l.entries[key]; entry != nil {
		return entry.waiting
	}
	return 0
}

func (l *runLocks) size() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.entries)
}
