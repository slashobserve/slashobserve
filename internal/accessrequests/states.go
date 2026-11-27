package accessrequests

import (
	"time"

	"github.com/slashobserve/slashobserve/internal/lifecycle"
)

type State string

const (
	StateOpen      State = "open"
	StateGranted   State = "granted"
	StateDismissed State = "dismissed"
	StateExpired   State = "expired"
)

const OpenFor = 30 * 24 * time.Hour

var States = lifecycle.Derived{Name: "access request", States: []lifecycle.DerivedState{
	{Name: string(StateOpen), When: "Asked in the last 30 days, not granted, and nobody who can grant it marked it done in their inbox."},
	{Name: string(StateGranted), When: "The requester got Comment access after asking."},
	{Name: string(StateDismissed), When: "Someone who can grant it marked it done in their inbox without granting it; the requester may ask again."},
	{Name: string(StateExpired), When: "Nobody granted or dismissed it within 30 days; the requester may ask again."},
}}

type standing struct {
	granted   bool
	dismissed bool
	age       time.Duration
}

func stateOf(s standing) State {
	switch {
	case s.granted:
		return StateGranted
	case s.dismissed:
		return StateDismissed
	case s.age > OpenFor:
		return StateExpired
	}
	return StateOpen
}
