package audit

import (
	"context"
	"strconv"
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

func TestARepeatIsNotSharedAcrossRunsAgentsOrCredentialsOfOneActor(t *testing.T) {
	user := uuid.NullUUID{UUID: uuid.MustParse("00000000-0000-4000-8000-0000000000a1"), Valid: true}
	scope := func(change func(*Scope)) context.Context {
		s := Scope{RequestID: uuid.New(), Actor: Actor{Kind: ActorUser, ID: user}, Initiator: InitiatorPerson}
		change(&s)
		return Open(t.Context(), s)
	}
	runA, runB := uuid.NullUUID{UUID: uuid.New(), Valid: true}, uuid.NullUUID{UUID: uuid.New(), Valid: true}
	agent := Agent{ID: uuid.NullUUID{UUID: uuid.New(), Valid: true}, Name: "diagnose"}
	at := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	m := NewRepeats()
	page := scope(func(*Scope) {})
	if m.SeenRecently(page, "q", at) {
		t.Fatal("a first read was seen before")
	}
	for name, ctx := range map[string]context.Context{
		"a run of the agent":       scope(func(s *Scope) { s.Initiator, s.Agent, s.RunID = InitiatorAgent, agent, runA }),
		"another run of the agent": scope(func(s *Scope) { s.Initiator, s.Agent, s.RunID = InitiatorAgent, agent, runB }),
		"another credential":       scope(func(s *Scope) { s.Auth.CredentialID = "key-2" }),
	} {
		if m.SeenRecently(ctx, "q", at.Add(time.Second)) {
			t.Errorf("%s folded into an earlier read of another caller", name)
		}
	}
	if !m.SeenRecently(scope(func(*Scope) {}), "q", at.Add(time.Second)) {
		t.Error("the same person on the same page did not fold")
	}
}

func TestAFullRepeatsTableIsSweptOncePerIntervalNotOncePerInsert(t *testing.T) {
	as := func() context.Context {
		return Open(t.Context(), Scope{RequestID: uuid.New(), Actor: Actor{Kind: ActorUser, ID: uuid.NullUUID{UUID: uuid.New(), Valid: true}}})
	}
	at := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	m := NewRepeats()
	for i := range repeatsRemembered {
		m.SeenRecently(as(), "q"+strconv.Itoa(i), at)
	}
	for i := range 50 {
		m.SeenRecently(as(), "late"+strconv.Itoa(i), at.Add(time.Second))
	}
	if m.sweeps != 1 {
		t.Fatalf("sweeps = %d after 50 inserts into a full table, want 1", m.sweeps)
	}
	m.SeenRecently(as(), "after the window", at.Add(RepeatWindow+time.Minute))
	if m.sweeps != 2 || len(m.seen) != 1 {
		t.Fatalf("sweeps = %d, remembered = %d, want a second sweep that dropped every expired key", m.sweeps, len(m.seen))
	}
}
