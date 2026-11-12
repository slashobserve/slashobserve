package proposals

import (
	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/lifecycle"
)

type State string

const (
	StateOpen      State = "open"
	StateMerged    State = "merged"
	StateDiscarded State = "discarded"
)

type Action string

const (
	ActionPropose Action = "propose"
	ActionUpdate  Action = "update"
	ActionMerge   Action = "merge"
	ActionDiscard Action = "discard"
)

var ProposalMachine = lifecycle.Machine[State, Action]{
	Name:     "proposal",
	States:   []State{StateOpen, StateMerged, StateDiscarded},
	Terminal: []State{StateMerged, StateDiscarded},
	Meaning: map[State]string{
		StateOpen:      "Waiting for someone who can edit to merge or discard it.",
		StateMerged:    "Its change was saved as a new version of the dashboard.",
		StateDiscarded: "Closed without saving its change.",
	},
	Arrive: map[State]string{
		StateOpen:      activity.TypeProposalOpened,
		StateMerged:    activity.TypeProposalMerged,
		StateDiscarded: activity.TypeProposalDiscarded,
	},
	Moves: []lifecycle.Move[State, Action]{
		{Action: ActionPropose, To: []State{StateOpen}, Who: []lifecycle.Who{lifecycle.Commenter}},
		{Action: ActionUpdate, From: []State{StateOpen}, Event: activity.TypeProposalUpdated, Who: []lifecycle.Who{lifecycle.Requester}},
		{Action: ActionMerge, From: []State{StateOpen}, To: []State{StateMerged}, Who: []lifecycle.Who{lifecycle.Editor}},
		{Action: ActionDiscard, From: []State{StateOpen}, To: []State{StateDiscarded}, Who: []lifecycle.Who{lifecycle.Requester, lifecycle.Editor}},
	},
}

type facts struct {
	state   State
	author  bool
	edit    bool
	propose bool
}

func (f facts) can(w lifecycle.Who) bool {
	switch w {
	case lifecycle.Requester:
		return f.author
	case lifecycle.Editor:
		return f.edit && !f.propose
	}
	return false
}

func allowedActions(f facts) []Action {
	return ProposalMachine.Allowed(f.state, f.can)
}
