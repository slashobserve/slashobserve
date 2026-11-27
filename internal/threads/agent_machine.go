package threads

import (
	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/lifecycle"
)

type AgentState string

const (
	AgentNone      AgentState = "none"
	AgentFollowing AgentState = "following"
	AgentOff       AgentState = "off"
)

type AgentAction string

const (
	AgentOpen    AgentAction = "open"
	AgentSet     AgentAction = "set_agent"
	AgentTurnOff AgentAction = "turn_agent_off"
	AgentClear   AgentAction = "clear_agent"
)

var AgentMachine = lifecycle.Machine[AgentState, AgentAction]{
	Name:   "thread_agent",
	States: []AgentState{AgentNone, AgentFollowing, AgentOff},
	Meaning: map[AgentState]string{
		AgentNone:      "No message has asked an agent yet, or the thread's agent can no longer take runs; a plain reply asks nobody.",
		AgentFollowing: "The thread has an agent and a plain reply asks it.",
		AgentOff:       "The thread keeps its agent, but a message was sent to everyone, so a plain reply asks nobody until the agent is set again.",
	},
	Arrive: map[AgentState]string{
		AgentNone:      activity.TypeThreadOpened,
		AgentFollowing: string(actionThreadAgentSet),
		AgentOff:       string(actionThreadAgentSet),
	},
	Moves: []lifecycle.Move[AgentState, AgentAction]{
		{Action: AgentOpen, To: []AgentState{AgentNone}, Who: []lifecycle.Who{lifecycle.Commenter}},
		{Action: AgentSet, From: []AgentState{AgentNone, AgentFollowing, AgentOff}, To: []AgentState{AgentFollowing}, Who: []lifecycle.Who{lifecycle.Commenter}},
		{Action: AgentTurnOff, From: []AgentState{AgentFollowing}, To: []AgentState{AgentOff}, Who: []lifecycle.Who{lifecycle.Commenter}},
		{Action: AgentClear, From: []AgentState{AgentFollowing, AgentOff}, To: []AgentState{AgentNone}, Event: string(actionThreadAgentSet), Who: []lifecycle.Who{lifecycle.System}},
	},
}

func (a threadAgent) state() AgentState {
	switch {
	case a.Agent == nil:
		return AgentNone
	case a.Off:
		return AgentOff
	}
	return AgentFollowing
}

func (a threadAgent) allows(action AgentAction) bool {
	_, found := AgentMachine.Find(a.state(), action)
	return found
}
