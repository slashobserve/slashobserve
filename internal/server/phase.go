package server

import (
	"log/slog"
	"slices"
	"sync"

	"github.com/slashobserve/slashobserve/internal/lifecycle"
)

type Phase string

const (
	PhaseBooting      Phase = "booting"
	PhaseMigrating    Phase = "migrating"
	PhaseProvisioning Phase = "provisioning"
	PhaseReady        Phase = "ready"
	PhaseStopping     Phase = "stopping"
	PhaseStopped      Phase = "stopped"
)

type PhaseAction string

const (
	phaseBoot      PhaseAction = "boot"
	phaseMigrate   PhaseAction = "migrate"
	phaseProvision PhaseAction = "provision"
	phaseServe     PhaseAction = "serve"
	phaseStop      PhaseAction = "stop"
	phaseExit      PhaseAction = "exit"
)

var PhaseMachine = lifecycle.Machine[Phase, PhaseAction]{
	Name:     "Server process",
	States:   []Phase{PhaseBooting, PhaseMigrating, PhaseProvisioning, PhaseReady, PhaseStopping, PhaseStopped},
	Terminal: []Phase{PhaseStopped},
	Meaning: map[Phase]string{
		PhaseBooting:      "It reads its settings and connects to Postgres.",
		PhaseMigrating:    "It applies the pending database migrations.",
		PhaseProvisioning: "It reconciles roles, applies the service-account and MCP source files and builds its services; it does not listen yet.",
		PhaseReady:        "It listens and serves requests, and `/readyz` answers 200 while its checks pass.",
		PhaseStopping:     "It got the shutdown signal: `/readyz` answers 503 while it keeps serving for 5 seconds, then it stops accepting requests and finishes the open ones. The 503 code `shutting_down` of the event streams is this phase.",
		PhaseStopped:      "It serves nothing more and exits.",
	},
	Arrive: map[Phase]string{
		PhaseBooting:      "starting slashobserve-server",
		PhaseMigrating:    "migrating database",
		PhaseProvisioning: "provisioning",
		PhaseReady:        "listening",
		PhaseStopping:     "shutting down",
		PhaseStopped:      "server stopped",
	},
	Moves: []lifecycle.Move[Phase, PhaseAction]{
		{Action: phaseBoot, To: []Phase{PhaseBooting}, Who: []lifecycle.Who{lifecycle.System}},
		{Action: phaseMigrate, From: []Phase{PhaseBooting}, To: []Phase{PhaseMigrating}, Who: []lifecycle.Who{lifecycle.System}},
		{Action: phaseProvision, From: []Phase{PhaseMigrating}, To: []Phase{PhaseProvisioning}, Who: []lifecycle.Who{lifecycle.System}},
		{Action: phaseServe, From: []Phase{PhaseProvisioning}, To: []Phase{PhaseReady}, Who: []lifecycle.Who{lifecycle.System}},
		{Action: phaseStop, From: []Phase{PhaseReady}, To: []Phase{PhaseStopping}, Who: []lifecycle.Who{lifecycle.System}},
		{Action: phaseExit, From: []Phase{PhaseBooting, PhaseMigrating, PhaseProvisioning, PhaseReady, PhaseStopping}, To: []Phase{PhaseStopped}, Who: []lifecycle.Who{lifecycle.System}},
	},
}

func PhaseNames() []string {
	names := make([]string, len(PhaseMachine.States))
	for i, p := range PhaseMachine.States {
		names[i] = string(p)
	}
	return names
}

type Phases struct {
	logger *slog.Logger

	mu      sync.Mutex
	current Phase
}

func StartPhases(logger *slog.Logger, attrs ...any) *Phases {
	p := &Phases{logger: logger}
	p.Enter(PhaseBooting, attrs...)
	return p
}

func (p *Phases) Current() Phase {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.current
}

func (p *Phases) Enter(next Phase, attrs ...any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !phaseFollows(p.current, next) {
		p.logger.Warn("server phase move refused", slog.String("phase", string(p.current)), slog.String("refused_phase", string(next)))
		return
	}
	p.current = next
	p.logger.Info(PhaseMachine.Arrive[next], append(attrs, slog.String("phase", string(next)))...)
}

func phaseFollows(from, to Phase) bool {
	return slices.ContainsFunc(PhaseMachine.Moves, func(mv lifecycle.Move[Phase, PhaseAction]) bool {
		starts := len(mv.From) == 0 && from == ""
		return slices.Contains(mv.To, to) && (starts || slices.Contains(mv.From, from))
	})
}
