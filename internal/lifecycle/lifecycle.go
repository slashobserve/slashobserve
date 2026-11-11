package lifecycle

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

type Who int

const (
	Requester Who = iota + 1
	Starter
	Commenter
	Editor
	Admin
	System
)

func (w Who) String() string {
	switch w {
	case Requester:
		return "requester"
	case Starter:
		return "anyone who may start one"
	case Commenter:
		return "commenter"
	case Editor:
		return "editor"
	case Admin:
		return "admin"
	case System:
		return "server"
	}
	return fmt.Sprintf("who(%d)", int(w))
}

type Move[S ~string, A ~string] struct {
	Action A
	From   []S
	To     []S
	Event  string
	Who    []Who
}

type Machine[S ~string, A ~string] struct {
	Name     string
	States   []S
	Terminal []S
	Meaning  map[S]string
	Arrive   map[S]string
	Moves    []Move[S, A]
}

var ErrInvalidTransition = errors.New("invalid transition")

type Refusal struct {
	Machine string
	State   string
	Action  string
	Allowed []string
}

func (r *Refusal) Error() string {
	return fmt.Sprintf("%s is not allowed while the %s is %s", r.Action, r.Machine, r.State)
}

func (r *Refusal) Is(target error) bool {
	return target == ErrInvalidTransition
}

func (m Machine[S, A]) Find(from S, a A) (Move[S, A], bool) {
	for _, mv := range m.Moves {
		if mv.Action == a && slices.Contains(mv.From, from) {
			return mv, true
		}
	}
	return Move[S, A]{}, false
}

func (m Machine[S, A]) Allowed(from S, can func(Who) bool) []A {
	out := []A{}
	for _, mv := range m.Moves {
		if !slices.Contains(mv.From, from) || slices.Contains(out, mv.Action) {
			continue
		}
		for _, w := range mv.Who {
			if w != System && can(w) {
				out = append(out, mv.Action)
				break
			}
		}
	}
	return out
}

func (m Machine[S, A]) From(a A) []string {
	var found bool
	union := map[S]bool{}
	for _, mv := range m.Moves {
		if mv.Action != a {
			continue
		}
		found = true
		for _, s := range mv.From {
			union[s] = true
		}
	}
	if !found {
		panic(fmt.Sprintf("lifecycle: machine %q has no move %q", m.Name, a))
	}
	out := []string{}
	for _, s := range m.States {
		if union[s] {
			out = append(out, string(s))
		}
	}
	return out
}

func (m Machine[S, A]) EventFor(a A, to S) string {
	for _, mv := range m.Moves {
		if mv.Action != a {
			continue
		}
		if len(mv.To) == 0 && slices.Contains(mv.From, to) {
			return mv.Event
		}
		if slices.Contains(mv.To, to) && mv.Event != "" {
			return mv.Event
		}
	}
	return m.Arrive[to]
}

func (m Machine[S, A]) Offered() []A {
	out := []A{}
	for _, mv := range m.Moves {
		if len(mv.From) == 0 || slices.Contains(out, mv.Action) {
			continue
		}
		for _, w := range mv.Who {
			if w != System {
				out = append(out, mv.Action)
				break
			}
		}
	}
	return out
}

func (m Machine[S, A]) Refuse(from S, a A, can func(Who) bool) *Refusal {
	allowed := []string{}
	for _, x := range m.Allowed(from, can) {
		allowed = append(allowed, string(x))
	}
	return &Refusal{Machine: m.Name, State: string(from), Action: string(a), Allowed: allowed}
}

func (m Machine[S, A]) Check() error {
	for _, check := range []func() error{m.checkStates, m.checkReachable, m.checkTerminals, m.checkEvents, m.checkWho, m.checkMeanings} {
		if err := check(); err != nil {
			return err
		}
	}
	return nil
}

func (m Machine[S, A]) checkStates() error {
	known := map[S]bool{}
	for _, s := range m.States {
		if known[s] {
			return fmt.Errorf("%s: duplicate state %q", m.Name, s)
		}
		known[s] = true
	}
	for _, mv := range m.Moves {
		for _, s := range slices.Concat(mv.From, mv.To) {
			if !known[s] {
				return fmt.Errorf("%s: move %q names unknown state %q", m.Name, mv.Action, s)
			}
		}
	}
	return nil
}

func (m Machine[S, A]) checkReachable() error {
	reached := map[S]bool{}
	for grew := true; grew; {
		grew = false
		for _, mv := range m.Moves {
			if len(mv.From) > 0 && !slices.ContainsFunc(mv.From, func(s S) bool { return reached[s] }) {
				continue
			}
			for _, s := range mv.To {
				if !reached[s] {
					reached[s] = true
					grew = true
				}
			}
		}
	}
	for _, s := range m.States {
		if !reached[s] {
			return fmt.Errorf("%s: state %q is not reachable", m.Name, s)
		}
	}
	return nil
}

func (m Machine[S, A]) checkTerminals() error {
	for _, mv := range m.Moves {
		if len(mv.To) > 0 && slices.ContainsFunc(mv.From, func(s S) bool { return slices.Contains(m.Terminal, s) }) {
			return fmt.Errorf("%s: move %q leaves a terminal state", m.Name, mv.Action)
		}
	}
	return nil
}

func (m Machine[S, A]) checkEvents() error {
	for _, mv := range m.Moves {
		if mv.Event != "" {
			continue
		}
		for _, s := range mv.To {
			if m.Arrive[s] == "" {
				return fmt.Errorf("%s: move %q to %q has no event", m.Name, mv.Action, s)
			}
		}
	}
	return nil
}

func (m Machine[S, A]) checkWho() error {
	for _, mv := range m.Moves {
		if len(mv.Who) == 0 {
			return fmt.Errorf("%s: move %q has nobody who may take it", m.Name, mv.Action)
		}
	}
	return nil
}

func (m Machine[S, A]) checkMeanings() error {
	for _, s := range m.States {
		if strings.TrimSpace(m.Meaning[s]) == "" {
			return fmt.Errorf("%s: state %q has no meaning", m.Name, s)
		}
	}
	return nil
}
