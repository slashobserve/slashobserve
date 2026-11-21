package audit

import (
	"context"
	"crypto/sha256"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	RepeatWindow      = 10 * time.Minute
	repeatsRemembered = 10000
)

type repeatKey struct {
	actor   Actor
	subject [sha256.Size]byte
}

type sighting struct {
	at      time.Time
	request uuid.UUID
}

type Repeats struct {
	mu   sync.Mutex
	seen map[repeatKey]sighting
}

func NewRepeats() *Repeats {
	return &Repeats{seen: map[repeatKey]sighting{}}
}

func (m *Repeats) SeenRecently(ctx context.Context, subject string, now time.Time) bool {
	scope, ok := ScopeOf(ctx)
	if !ok {
		return false
	}
	key := repeatKey{actor: Actor{Kind: scope.Actor.Kind, ID: scope.Actor.ID}, subject: sha256.Sum256([]byte(subject))}
	m.mu.Lock()
	defer m.mu.Unlock()
	if last, found := m.seen[key]; found && now.Sub(last.at) < RepeatWindow {
		return last.request != scope.RequestID
	}
	if len(m.seen) >= repeatsRemembered {
		m.forgetExpired(now)
	}
	if len(m.seen) < repeatsRemembered {
		m.seen[key] = sighting{at: now, request: scope.RequestID}
	}
	return false
}

func (m *Repeats) forgetExpired(now time.Time) {
	for key, last := range m.seen {
		if now.Sub(last.at) >= RepeatWindow {
			delete(m.seen, key)
		}
	}
}
