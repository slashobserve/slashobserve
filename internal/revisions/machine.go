package revisions

import (
	"slices"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/lifecycle"
)

type State string

const (
	StateQueued    State = "queued"
	StateRunning   State = "running"
	StateCanceling State = "canceling"
	StateSucceeded State = "succeeded"
	StateFailed    State = "failed"
	StateCanceled  State = "canceled"
)

type ResultKind string

const (
	ResultNone           ResultKind = "none"
	ResultVersion        ResultKind = "version"
	ResultPartialVersion ResultKind = "partial_version"
	ResultProposal       ResultKind = "proposal"
	ResultSuggestion     ResultKind = "suggestion"
	ResultObservation    ResultKind = "observation"
)

var ResultKinds = []ResultKind{ResultNone, ResultVersion, ResultPartialVersion, ResultProposal, ResultSuggestion, ResultObservation}

type Action string

const (
	ActionEnqueue   Action = "enqueue"
	ActionClaim     Action = "claim"
	ActionRelease   Action = "release"
	ActionCancel    Action = "cancel"
	ActionSucceed   Action = "succeed"
	ActionFail      Action = "fail"
	ActionSettle    Action = "settle"
	ActionExhaust   Action = "exhaust"
	ActionExpire    Action = "expire"
	ActionKeepBuild Action = "keep_build"
	ActionContinue  Action = "continue"
	ActionRetry     Action = "retry"
	ActionPin       Action = "pin"
	ActionArchive   Action = "archive"
)

var allStates = []State{StateQueued, StateRunning, StateCanceling, StateSucceeded, StateFailed, StateCanceled}

var RunMachine = lifecycle.Machine[State, Action]{
	Name:     "run",
	States:   allStates,
	Terminal: []State{StateSucceeded, StateFailed, StateCanceled},
	Meaning: map[State]string{
		StateQueued:    "Waiting for a worker.",
		StateRunning:   "A worker holds its lease and runs the agent.",
		StateCanceling: "The requester canceled it and its worker is stopping.",
		StateSucceeded: "It saved its result: a version, a suggestion or a finding saved into an observation.",
		StateFailed:    "It stopped without a result; a build it left can be kept or continued.",
		StateCanceled:  "The requester canceled it; a build it left can be kept or continued.",
	},
	Arrive: map[State]string{
		StateQueued:    activity.TypeRunQueued,
		StateRunning:   activity.TypeRunStarted,
		StateCanceling: activity.TypeRunCanceling,
		StateSucceeded: activity.TypeRunSucceeded,
		StateFailed:    activity.TypeRunFailed,
		StateCanceled:  activity.TypeRunCanceled,
	},
	Moves: []lifecycle.Move[State, Action]{
		{Action: ActionEnqueue, To: []State{StateQueued}, Who: []lifecycle.Who{lifecycle.Requester}},
		{Action: ActionClaim, From: []State{StateQueued, StateRunning}, To: []State{StateRunning}, Who: []lifecycle.Who{lifecycle.System}},
		{Action: ActionRelease, From: []State{StateRunning}, To: []State{StateQueued}, Event: activity.TypeRunReleased, Who: []lifecycle.Who{lifecycle.System}},
		{Action: ActionCancel, From: []State{StateQueued, StateRunning, StateCanceling}, To: []State{StateCanceling, StateCanceled, StateSucceeded}, Who: []lifecycle.Who{lifecycle.Requester}},
		{Action: ActionSucceed, From: []State{StateRunning, StateCanceling}, To: []State{StateSucceeded}, Who: []lifecycle.Who{lifecycle.System}},
		{Action: ActionFail, From: []State{StateRunning}, To: []State{StateFailed}, Who: []lifecycle.Who{lifecycle.System}},
		{Action: ActionSettle, From: []State{StateQueued, StateCanceling}, To: []State{StateCanceled, StateSucceeded}, Who: []lifecycle.Who{lifecycle.System}},
		{Action: ActionExhaust, From: []State{StateRunning}, To: []State{StateFailed, StateSucceeded}, Who: []lifecycle.Who{lifecycle.System}},
		{Action: ActionExpire, From: []State{StateQueued, StateRunning}, To: []State{StateFailed, StateSucceeded}, Who: []lifecycle.Who{lifecycle.System}},
		{Action: ActionKeepBuild, From: []State{StateFailed, StateCanceled}, Event: activity.TypeRunKept, Who: []lifecycle.Who{lifecycle.Requester}},
		{Action: ActionContinue, From: []State{StateFailed, StateCanceled}, Who: []lifecycle.Who{lifecycle.Requester}},
		{Action: ActionRetry, From: []State{StateSucceeded, StateFailed, StateCanceled}, Who: []lifecycle.Who{lifecycle.Starter}},
		{Action: ActionPin, From: allStates, Who: []lifecycle.Who{lifecycle.Requester}},
		{Action: ActionArchive, From: allStates, Who: []lifecycle.Who{lifecycle.Requester}},
	},
}

func (s State) Terminal() bool {
	return slices.Contains(RunMachine.Terminal, s)
}

type runFacts struct {
	state       State
	source      Source
	onDashboard bool
	requester   bool
	mayStart    bool
	mayEdit     bool
	buildPanels int
	buildSaved  bool
	continued   bool
}

func (f runFacts) retryable() bool {
	return !f.continued && (f.source == SourceObserve || f.source == SourceInvestigate || (f.source == SourcePrompt && f.onDashboard))
}

func (f runFacts) refuses(a Action) bool {
	hasBuild := f.buildPanels >= 1 && !f.buildSaved
	switch a {
	case ActionCancel:
		return f.source == SourceSuggest
	case ActionKeepBuild:
		return !hasBuild || !f.mayEdit
	case ActionContinue:
		return f.source == SourceInvestigate || !hasBuild || f.continued
	case ActionRetry:
		return !f.retryable()
	}
	return false
}

func allowedActions(f runFacts) []Action {
	can := func(w lifecycle.Who) bool {
		return (w == lifecycle.Requester && f.requester) || (w == lifecycle.Starter && f.mayStart)
	}
	out := []Action{}
	for _, a := range RunMachine.Allowed(f.state, can) {
		if !f.refuses(a) {
			out = append(out, a)
		}
	}
	return out
}

func actionStrings(actions []Action) []string {
	out := make([]string, len(actions))
	for i, a := range actions {
		out[i] = string(a)
	}
	return out
}
