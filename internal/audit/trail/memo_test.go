package trail

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/slashobserve/slashobserve/internal/audit"
)

func requestBy(person uuid.UUID) context.Context {
	return audit.Open(context.Background(), audit.Scope{
		RequestID: uuid.New(),
		Actor:     audit.Actor{Kind: audit.ActorUser, ID: uuid.NullUUID{UUID: person, Valid: true}},
	})
}

func TestARepeatIsAnotherRequestOfTheSameCallerAndQueryWithinTenMinutes(t *testing.T) {
	clock := now
	m := NewRepeats(func() time.Time { return clock })
	ada, bo := uuid.New(), uuid.New()

	first := requestBy(ada)
	for range 2 {
		if m.Repeated(first, "outcome:denied") {
			t.Fatal("one request counted as its own repeat")
		}
	}
	if !m.Repeated(requestBy(ada), "outcome:denied") {
		t.Fatal("the same query from the same caller was not a repeat")
	}
	if m.Repeated(requestBy(bo), "outcome:denied") || m.Repeated(requestBy(ada), "origin:mcp") {
		t.Fatal("another caller or another query counted as a repeat")
	}
	if m.Repeated(context.Background(), "outcome:denied") {
		t.Fatal("a call outside a request counted as a repeat")
	}
	clock = clock.Add(repeatWindow)
	if m.Repeated(requestBy(ada), "outcome:denied") {
		t.Fatal("a query ten minutes later was still a repeat")
	}
}
