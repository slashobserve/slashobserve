package lifecycle_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/lifecycle"
)

type state string
type action string

const (
	open   state = "open"
	closed state = "closed"
	gone   state = "gone"
)

func tiny() lifecycle.Machine[state, action] {
	return lifecycle.Machine[state, action]{
		Name:     "ticket",
		States:   []state{open, closed, gone},
		Terminal: []state{gone},
		Meaning:  map[state]string{open: "Open.", closed: "Closed.", gone: "Gone."},
		Arrive:   map[state]string{open: "ticket.opened", closed: "ticket.closed", gone: "ticket.deleted"},
		Moves: []lifecycle.Move[state, action]{
			{Action: "create", To: []state{open}, Who: []lifecycle.Who{lifecycle.Requester}},
			{Action: "close", From: []state{open}, To: []state{closed}, Who: []lifecycle.Who{lifecycle.Editor, lifecycle.Admin}},
			{Action: "reopen", From: []state{closed}, To: []state{open}, Event: "ticket.reopened", Who: []lifecycle.Who{lifecycle.Requester}},
			{Action: "purge", From: []state{open, closed}, To: []state{gone}, Who: []lifecycle.Who{lifecycle.System}},
			{Action: "pin", From: []state{open, closed, gone}, Who: []lifecycle.Who{lifecycle.Requester, lifecycle.Editor}},
			{Action: "close", From: []state{gone}, Who: []lifecycle.Who{lifecycle.Admin}},
		},
	}
}

func only(ws ...lifecycle.Who) func(lifecycle.Who) bool {
	return func(w lifecycle.Who) bool { return slices.Contains(ws, w) }
}

func TestFindReturnsTheFirstMoveOfTheAction(t *testing.T) {
	m := tiny()
	cases := []struct {
		from state
		a    action
		ok   bool
		to   []state
	}{
		{open, "close", true, []state{closed}},
		{gone, "close", true, nil},
		{closed, "close", false, nil},
		{open, "nothing", false, nil},
	}
	for _, c := range cases {
		mv, ok := m.Find(c.from, c.a)
		if ok != c.ok || !slices.Equal(mv.To, c.to) {
			t.Errorf("Find(%s, %s) = %v, %v; want %v, %v", c.from, c.a, mv.To, ok, c.to, c.ok)
		}
	}
}

func TestAllowedFiltersByWhoKeepsOrderAndNeverOffersSystem(t *testing.T) {
	m := tiny()
	cases := []struct {
		name string
		from state
		can  func(lifecycle.Who) bool
		want []action
	}{
		{"requester sees its moves", open, only(lifecycle.Requester), []action{"pin"}},
		{"editor sees close and pin in move order", open, only(lifecycle.Editor), []action{"close", "pin"}},
		{"system is never offered", open, func(lifecycle.Who) bool { return true }, []action{"close", "pin"}},
		{"a second move of an action is not repeated", gone, func(lifecycle.Who) bool { return true }, []action{"pin", "close"}},
		{"nobody", open, only(), []action{}},
		{"reopen only from closed", closed, only(lifecycle.Requester), []action{"reopen", "pin"}},
	}
	for _, c := range cases {
		got := m.Allowed(c.from, c.can)
		if got == nil || !slices.Equal(got, c.want) {
			t.Errorf("%s: Allowed = %#v; want %#v", c.name, got, c.want)
		}
	}
}

func TestFromIsTheUnionInStateOrder(t *testing.T) {
	m := tiny()
	if got := m.From("purge"); !slices.Equal(got, []string{"open", "closed"}) {
		t.Errorf("From(purge) = %v", got)
	}
	if got := m.From("close"); !slices.Equal(got, []string{"open", "gone"}) {
		t.Errorf("From(close) = %v", got)
	}
	defer func() {
		if recover() == nil {
			t.Error("From of an unknown action did not panic")
		}
	}()
	m.From("nothing")
}

func TestEventForPrefersTheMoveOverTheArrival(t *testing.T) {
	m := tiny()
	cases := []struct {
		a    action
		to   state
		want string
	}{
		{"reopen", open, "ticket.reopened"},
		{"create", open, "ticket.opened"},
		{"close", closed, "ticket.closed"},
		{"unknown", gone, "ticket.deleted"},
		{"pin", open, ""},
		{"pin", gone, ""},
	}
	for _, c := range cases {
		if got := m.EventFor(c.a, c.to); got != c.want {
			t.Errorf("EventFor(%s, %s) = %q; want %q", c.a, c.to, got, c.want)
		}
	}
}

func TestEventForAMoveThatLeavesTheStateIsItsOwnEvent(t *testing.T) {
	m := tiny()
	m.Moves = append(m.Moves, lifecycle.Move[state, action]{Action: "star", From: []state{open}, Event: "ticket.starred", Who: []lifecycle.Who{lifecycle.Editor}})
	if got := m.EventFor("star", open); got != "ticket.starred" {
		t.Errorf("EventFor(star, open) = %q", got)
	}
}

func TestOfferedListsEveryActionSomeoneMayTakeFromAStateOnce(t *testing.T) {
	got := tiny().Offered()
	want := []action{"close", "reopen", "pin"}
	if !slices.Equal(got, want) {
		t.Errorf("Offered = %v; want %v", got, want)
	}
}

func TestRefuseNamesWhatIsAllowedAndMatchesErrInvalidTransition(t *testing.T) {
	m := tiny()
	r := m.Refuse(open, "reopen", only(lifecycle.Editor))
	if r.Error() != "reopen is not allowed while the ticket is open" {
		t.Errorf("Error = %q", r.Error())
	}
	if !slices.Equal(r.Allowed, []string{"close", "pin"}) {
		t.Errorf("Allowed = %v", r.Allowed)
	}
	var err error = r
	if !errors.Is(err, lifecycle.ErrInvalidTransition) {
		t.Error("a refusal is not ErrInvalidTransition")
	}
	if errors.Is(err, errors.New("other")) {
		t.Error("a refusal matched an unrelated error")
	}
	empty := m.Refuse(open, "reopen", only())
	if empty.Allowed == nil || len(empty.Allowed) != 0 {
		t.Errorf("Allowed = %#v; want an empty non-nil slice", empty.Allowed)
	}
}

func TestWhoStrings(t *testing.T) {
	cases := map[lifecycle.Who]string{
		lifecycle.Requester: "requester",
		lifecycle.Starter:   "anyone who may start one",
		lifecycle.Commenter: "commenter",
		lifecycle.Editor:    "editor",
		lifecycle.Admin:     "admin",
		lifecycle.System:    "server",
	}
	for w, want := range cases {
		if w.String() != want {
			t.Errorf("Who(%d) = %q; want %q", int(w), w.String(), want)
		}
	}
}

func TestCheckReportsEachDefect(t *testing.T) {
	if err := tiny().Check(); err != nil {
		t.Fatalf("a sound machine failed Check: %v", err)
	}
	cases := []struct {
		name   string
		mutate func(m *lifecycle.Machine[state, action])
		want   string
	}{
		{"duplicate state", func(m *lifecycle.Machine[state, action]) { m.States = append(m.States, open) }, "duplicate state"},
		{"unknown state in a move", func(m *lifecycle.Machine[state, action]) {
			m.Moves = append(m.Moves, lifecycle.Move[state, action]{Action: "x", From: []state{"ghost"}, Who: []lifecycle.Who{lifecycle.Admin}})
		}, "unknown state"},
		{"unreachable state", func(m *lifecycle.Machine[state, action]) {
			m.States = append(m.States, "island")
			m.Meaning["island"] = "Alone."
		}, "not reachable"},
		{"move out of a terminal state", func(m *lifecycle.Machine[state, action]) {
			m.Moves = append(m.Moves, lifecycle.Move[state, action]{Action: "revive", From: []state{gone}, To: []state{open}, Who: []lifecycle.Who{lifecycle.Admin}})
		}, "leaves a terminal state"},
		{"target without an event", func(m *lifecycle.Machine[state, action]) { delete(m.Arrive, closed) }, "no event"},
		{"move without a who", func(m *lifecycle.Machine[state, action]) { m.Moves[1].Who = nil }, "nobody"},
		{"state without a meaning", func(m *lifecycle.Machine[state, action]) { delete(m.Meaning, closed) }, "no meaning"},
	}
	for _, c := range cases {
		m := tiny()
		m.Meaning = map[state]string{open: "Open.", closed: "Closed.", gone: "Gone."}
		m.Arrive = map[state]string{open: "ticket.opened", closed: "ticket.closed", gone: "ticket.deleted"}
		c.mutate(&m)
		err := m.Check()
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: Check = %v; want an error containing %q", c.name, err, c.want)
		}
	}
}
