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
	repeatsSweepEvery = time.Minute
)

type repeatKey struct {
	actor      Actor
	initiator  Initiator
	agent      uuid.NullUUID
	run        uuid.NullUUID
	credential string
	subject    [sha256.Size]byte
}

type sighting struct {
	at      time.Time
	request uuid.UUID
}

type Repeats struct {
	mu         sync.Mutex
	seen       map[repeatKey]sighting
	sweepAfter time.Time
	sweeps     int
}

func NewRepeats() *Repeats {
	return &Repeats{seen: map[repeatKey]sighting{}}
}

func (m *Repeats) SeenRecently(ctx context.Context, subject string, now time.Time) bool {
	scope, ok := ScopeOf(ctx)
	if !ok {
		return false
	}
	key := repeatKey{
		actor:      Actor{Kind: scope.Actor.Kind, ID: scope.Actor.ID},
		initiator:  scope.Initiator,
		agent:      scope.Agent.ID,
		run:        scope.RunID,
		credential: scope.Auth.CredentialID,
		subject:    sha256.Sum256([]byte(subject)),
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if last, found := m.seen[key]; found && now.Sub(last.at) < RepeatWindow {
		return last.request != scope.RequestID
	}
	if len(m.seen) >= repeatsRemembered && !now.Before(m.sweepAfter) {
		m.forgetExpired(now)
		m.sweepAfter = now.Add(repeatsSweepEvery)
	}
	if len(m.seen) < repeatsRemembered {
		m.seen[key] = sighting{at: now, request: scope.RequestID}
	}
	return false
}

func (m *Repeats) forgetExpired(now time.Time) {
	m.sweeps++
	for key, last := range m.seen {
		if now.Sub(last.at) >= RepeatWindow {
			delete(m.seen, key)
		}
	}
}
