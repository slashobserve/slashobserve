package presence

import (
	"time"

	"github.com/slashobserve/slashobserve/internal/lifecycle"
)

type State string

const (
	Working   State = "working"
	Listening State = "listening"
	Offline   State = "offline"
)

const ListenWindow = 40 * time.Second

func Of(seenAt time.Time, seen, working bool, now time.Time) State {
	switch {
	case working:
		return Working
	case seen && now.Sub(seenAt) <= ListenWindow:
		return Listening
	}
	return Offline
}

var States = lifecycle.Derived{Name: "agent presence", States: []lifecycle.DerivedState{
	{Name: "working", When: "It holds the lease of a run it claimed."},
	{Name: "listening", When: "It asked for work in the last 40 seconds."},
	{Name: "offline", When: "It has not asked for work in 40 seconds and holds no run."},
}}
