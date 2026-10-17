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
)

const DefaultPct = 30

type State struct {
	errorsPct  atomic.Int32
	latencyPct atomic.Int32
}

func ParseKind(s string) (Kind, error) {
	switch Kind(s) {
	case Errors:
		return Errors, nil
	case Latency:
		return Latency, nil
	}
	return "", fmt.Errorf("type must be %q or %q", Errors, Latency)
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
	s.errorsPct.Store(0)
	s.latencyPct.Store(0)
}

func (s *State) slot(kind Kind) *atomic.Int32 {
	if kind == Latency {
		return &s.latencyPct
	}
	return &s.errorsPct
}
