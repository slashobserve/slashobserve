package harness

import (
	"log/slog"

	"github.com/slashobserve/slashobserve/internal/lifecycle"
)

type State string

const (
	StateStarting State = "starting"
	StateRunning  State = "running"
	StateBackoff  State = "backoff"
	StateStopping State = "stopping"
)

type Action string

const (
	ActionStart   Action = "start"
	ActionRun     Action = "run"
	ActionBackOff Action = "back_off"
	ActionRestart Action = "restart"
	ActionStop    Action = "stop"
)

const stateLogKey = "agent_state"

var SupervisorMachine = lifecycle.Machine[State, Action]{
	Name:     "built-in agent supervisor",
	States:   []State{StateStarting, StateRunning, StateBackoff, StateStopping},
	Terminal: []State{StateStopping},
	Meaning: map[State]string{
		StateStarting: "The server is starting the `slashobserve-harness` process.",
		StateRunning:  "The process runs and can claim runs.",
		StateBackoff:  "The process exited or could not be started, and the server waits 1 to 30 seconds before it starts it again.",
		StateStopping: "The server is shutting down and stops the process.",
	},
	Arrive: map[State]string{
		StateStarting: "harness starting",
		StateRunning:  "harness running",
		StateBackoff:  "harness waiting to restart",
		StateStopping: "harness supervisor stopping",
	},
	Moves: []lifecycle.Move[State, Action]{
		{Action: ActionStart, To: []State{StateStarting}, Who: []lifecycle.Who{lifecycle.System}},
		{Action: ActionRun, From: []State{StateStarting}, To: []State{StateRunning}, Who: []lifecycle.Who{lifecycle.System}},
		{Action: ActionBackOff, From: []State{StateStarting, StateRunning}, To: []State{StateBackoff}, Who: []lifecycle.Who{lifecycle.System}},
		{Action: ActionRestart, From: []State{StateBackoff}, To: []State{StateStarting}, Who: []lifecycle.Who{lifecycle.System}},
		{Action: ActionStop, From: []State{StateStarting, StateRunning, StateBackoff}, To: []State{StateStopping}, Who: []lifecycle.Who{lifecycle.System}},
	},
}

func (s *Supervisor) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

func (s *Supervisor) move(a Action, to State) {
	s.mu.Lock()
	_, declared := SupervisorMachine.Find(s.state, a)
	allowed := declared || (s.state == "" && a == ActionStart)
	if allowed {
		s.state = to
	}
	s.mu.Unlock()
	if allowed {
		s.cfg.Logger.Info(SupervisorMachine.EventFor(a, to), slog.String(stateLogKey, string(to)))
	}
}
