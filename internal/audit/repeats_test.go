package audit

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestARepeatIsSeenOncePerActorAndSubjectWithinTheWindow(t *testing.T) {
	as := func(id string) context.Context {
		return Open(t.Context(), Scope{RequestID: uuid.New(), Actor: Actor{Kind: ActorUser, ID: uuid.NullUUID{UUID: uuid.MustParse(id), Valid: true}}})
	}
	const adaID = "00000000-0000-4000-8000-0000000000a1"
	ada, eve, adaAgain := as(adaID), as("00000000-0000-4000-8000-0000000000e1"), as(adaID)
	start := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	m := NewRepeats()
	for _, c := range []struct {
		name    string
		ctx     context.Context
		subject string
		at      time.Duration
		want    bool
	}{
		{"a first read", ada, "run.view:1", 0, false},
		{"the same read again in the same request", ada, "run.view:1", time.Second, false},
		{"the same read in another request a second later", adaAgain, "run.view:1", time.Second, true},
		{"another subject", ada, "run.view:2", time.Second, false},
		{"another actor", eve, "run.view:1", time.Second, false},
		{"the same read just inside the window", adaAgain, "run.view:1", RepeatWindow - time.Second, true},
		{"the same read once the window passed", ada, "run.view:1", RepeatWindow, false},
		{"a read with no scope", t.Context(), "run.view:1", RepeatWindow, false},
	} {
		if got := m.SeenRecently(c.ctx, c.subject, start.Add(c.at)); got != c.want {
			t.Errorf("%s: seen = %v, want %v", c.name, got, c.want)
		}
	}
}
