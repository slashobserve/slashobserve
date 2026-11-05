package fault

import (
	"errors"
	"fmt"
	"sync/atomic"
)

type Kind string

const (
	Errors  Kind = "errors"
	Latency Kind = "latency"
	Queue   Kind = "queue"
	SlowDB  Kind = "slow_db"
)

var Kinds = [...]Kind{Errors, Latency, Queue, SlowDB}

const DefaultPct = 30

type State struct {
	pct [len(Kinds)]atomic.Int32
}

func ParseKind(s string) (Kind, error) {
	for _, kind := range Kinds {
		if Kind(s) == kind {
			return kind, nil
		}
	}
	return "", fmt.Errorf("type must be %q, %q, %q or %q", Errors, Latency, Queue, SlowDB)
}

func (s *State) Set(kind Kind, pct int) error {
	if pct < 0 || pct > 100 {
		return errors.New("pct must be an integer between 0 and 100")
	}
	s.slot(kind).Store(int32(pct))
	return nil
}

func (s *State) Pct(kind Kind) int {
	return int(s.slot(kind).Load())
}

func (s *State) Reset() {
	for i := range s.pct {
		s.pct[i].Store(0)
	}
}

func (s *State) slot(kind Kind) *atomic.Int32 {
	for i, known := range Kinds {
		if kind == known {
			return &s.pct[i]
		}
	}
	panic(fmt.Sprintf("fault: unknown kind %q", kind))
}
