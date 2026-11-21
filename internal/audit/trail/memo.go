package trail

import (
	"context"
	"crypto/sha256"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/slashobserve/slashobserve/internal/audit"
)

const (
	repeatWindow = 10 * time.Minute
	repeatLimit  = 10000
)

type repeatKey struct {
	actor audit.Actor
	query [sha256.Size]byte
}

type sighting struct {
	at      time.Time
	request uuid.UUID
}

type Repeats struct {
	mu   sync.Mutex
	now  func() time.Time
	seen map[repeatKey]sighting
}

func NewRepeats(now func() time.Time) *Repeats {
	return &Repeats{now: now, seen: map[repeatKey]sighting{}}
}

func (m *Repeats) Repeated(ctx context.Context, query string) bool {
	scope, ok := audit.ScopeOf(ctx)
	if !ok {
		return false
	}
	now := m.now()
	key := repeatKey{actor: audit.Actor{Kind: scope.Actor.Kind, ID: scope.Actor.ID}, query: sha256.Sum256([]byte(query))}
	m.mu.Lock()
	defer m.mu.Unlock()
	if last, found := m.seen[key]; found && now.Sub(last.at) < repeatWindow {
		return last.request != scope.RequestID
	}
	if len(m.seen) >= repeatLimit {
		m.forgetExpired(now)
	}
	if len(m.seen) < repeatLimit {
		m.seen[key] = sighting{at: now, request: scope.RequestID}
	}
	return false
}

func (m *Repeats) forgetExpired(now time.Time) {
	for key, last := range m.seen {
		if now.Sub(last.at) >= repeatWindow {
			delete(m.seen, key)
		}
	}
}
