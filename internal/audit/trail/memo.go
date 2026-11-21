package trail

import (
	"context"
	"crypto/sha256"
	"sync"
	"time"

	"github.com/slashobserve/slashobserve/internal/audit"
)

const (
	searchMemoWindow = 10 * time.Minute
	searchMemoLimit  = 10000
)

type searchKey struct {
	actor audit.Actor
	query [sha256.Size]byte
}

type searchMemo struct {
	mu   sync.Mutex
	seen map[searchKey]time.Time
}

func newSearchMemo() *searchMemo {
	return &searchMemo{seen: map[searchKey]time.Time{}}
}

func (m *searchMemo) seenRecently(ctx context.Context, query string, now time.Time) bool {
	scope, ok := audit.ScopeOf(ctx)
	if !ok {
		return false
	}
	key := searchKey{actor: audit.Actor{Kind: scope.Actor.Kind, ID: scope.Actor.ID}, query: sha256.Sum256([]byte(query))}
	m.mu.Lock()
	defer m.mu.Unlock()
	if at, found := m.seen[key]; found && now.Sub(at) < searchMemoWindow {
		return true
	}
	if len(m.seen) >= searchMemoLimit {
		m.forgetExpired(now)
	}
	if len(m.seen) < searchMemoLimit {
		m.seen[key] = now
	}
	return false
}

func (m *searchMemo) forgetExpired(now time.Time) {
	for key, at := range m.seen {
		if now.Sub(at) >= searchMemoWindow {
			delete(m.seen, key)
		}
	}
}
