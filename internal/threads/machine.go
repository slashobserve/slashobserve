package threads

import (
	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/lifecycle"
)

type State string

type Action string

const (
	ActionOpen    Action = "open"
	ActionReply   Action = "reply"
	ActionResolve Action = "resolve"
	ActionReopen  Action = "reopen"
)

var ThreadMachine = lifecycle.Machine[State, Action]{
	Name:   "thread",
	States: []State{StatusOpen, StatusResolved},
	Meaning: map[State]string{
		StatusOpen:     "Waiting for an answer or a change.",
		StatusResolved: "Someone, or an agent run that saved the change it asked for, marked it done.",
	},
	Arrive: map[State]string{
		StatusOpen:     activity.TypeThreadOpened,
		StatusResolved: activity.TypeThreadResolved,
	},
	Moves: []lifecycle.Move[State, Action]{
		{Action: ActionOpen, To: []State{StatusOpen}, Who: []lifecycle.Who{lifecycle.Commenter}},
		{Action: ActionReply, From: []State{StatusOpen, StatusResolved}, Event: activity.TypeThreadReplied, Who: []lifecycle.Who{lifecycle.Commenter}},
		{Action: ActionResolve, From: []State{StatusOpen}, To: []State{StatusResolved}, Who: []lifecycle.Who{lifecycle.Commenter, lifecycle.System}},
		{Action: ActionReopen, From: []State{StatusResolved}, To: []State{StatusOpen}, Event: activity.TypeThreadReopened, Who: []lifecycle.Who{lifecycle.Commenter}},
	},
}

func allowedActions(status string, canComment bool) []string {
	allowed := ThreadMachine.Allowed(State(status), func(w lifecycle.Who) bool { return w == lifecycle.Commenter && canComment })
	out := make([]string, len(allowed))
	for i, a := range allowed {
		out[i] = string(a)
	}
	return out
}
