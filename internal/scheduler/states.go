package scheduler

import (
	"time"

	"github.com/slashobserve/slashobserve/internal/lifecycle"
)

type State string

const (
	StateWaiting     State = "waiting"
	StateDue         State = "due"
	StateRunning     State = "running"
	StateInterrupted State = "interrupted"
)

var States = lifecycle.Derived{Name: "scheduled job", States: []lifecycle.DerivedState{
	{Name: string(StateRunning), When: "A server holds its lease and is running it."},
	{Name: string(StateInterrupted), When: "Its last run did not finish: the server crashed and the lease ran out, or the server shut down and gave the lease back. The next server that finds it due runs it again."},
	{Name: string(StateDue), When: "Its next run time has passed and no server has claimed it yet."},
	{Name: string(StateWaiting), When: "Its last run finished, or it never ran, and the next run time is ahead."},
}}

type Facts struct {
	LeaseUntil     time.Time
	Leased         bool
	LastStartedAt  time.Time
	Started        bool
	LastFinishedAt time.Time
	Finished       bool
	NextRunAt      time.Time
	Now            time.Time
}

func StateOf(f Facts) State {
	switch {
	case f.Leased && f.LeaseUntil.After(f.Now):
		return StateRunning
	case f.Started && (!f.Finished || f.LastFinishedAt.Before(f.LastStartedAt)):
		return StateInterrupted
	case !f.NextRunAt.After(f.Now):
		return StateDue
	}
	return StateWaiting
}
