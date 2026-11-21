package audit

import (
	"context"
	"crypto/sha256"
	"sync"
	"time"
)

const (
	RepeatWindow      = 10 * time.Minute
	repeatsRemembered = 10000
)

type repeatKey struct {
	actor   Actor
	subject [sha256.Size]byte
}

type Repeats struct {
	mu   sync.Mutex
	seen map[repeatKey]time.Time
}

func NewRepeats() *Repeats {
	return &Repeats{seen: map[repeatKey]time.Time{}}
}

func (m *Repeats) SeenRecently(ctx context.Context, subject string, now time.Time) bool {
	scope, ok := ScopeOf(ctx)
	if !ok {
		return false
	}
	key := repeatKey{actor: Actor{Kind: scope.Actor.Kind, ID: scope.Actor.ID}, subject: sha256.Sum256([]byte(subject))}
	m.mu.Lock()
	defer m.mu.Unlock()
	if at, found := m.seen[key]; found && now.Sub(at) < RepeatWindow {
		return true
	}
	if len(m.seen) >= repeatsRemembered {
		m.forgetExpired(now)
	}
	if len(m.seen) < repeatsRemembered {
		m.seen[key] = now
	}
	return false
}

func (m *Repeats) forgetExpired(now time.Time) {
	for key, at := range m.seen {
		if now.Sub(at) >= RepeatWindow {
			delete(m.seen, key)
		}
	}
}
