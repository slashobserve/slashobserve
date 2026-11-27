package observations

import (
	"errors"
	"slices"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/lifecycle"
)

type RCAState string

const (
	RCAPublished  RCAState = "published"
	RCASuperseded RCAState = "superseded"
	RCAShared     RCAState = "shared"
	RCAWithdrawn  RCAState = "withdrawn"
)

type RCAAction string

const (
	RCAPublish   RCAAction = "publish"
	RCASupersede RCAAction = "supersede"
	RCAShare     RCAAction = "share"
	RCAUnshare   RCAAction = "unshare"
	RCAWithdraw  RCAAction = "withdraw"
)

var manager = []lifecycle.Who{lifecycle.Admin}

var RCAMachine = lifecycle.Machine[RCAState, RCAAction]{
	Name:     "rca_revision",
	States:   []RCAState{RCAPublished, RCASuperseded, RCAShared, RCAWithdrawn},
	Terminal: []RCAState{RCAWithdrawn},
	Meaning: map[RCAState]string{
		RCAPublished:  "The newest revision of the observation's RCA, with no live public link.",
		RCASuperseded: "A newer revision was published after it, and it has no live public link.",
		RCAShared:     "At least one public link that has not expired shows it, so it cannot be withdrawn until they are revoked.",
		RCAWithdrawn:  "Gone: the revision and its frozen panels are deleted, so this state is never stored.",
	},
	Arrive: map[RCAState]string{
		RCAPublished:  activity.TypeObservationPublished,
		RCASuperseded: activity.TypeObservationPublished,
		RCAShared:     activity.TypePublicLinkCreated,
		RCAWithdrawn:  activity.TypeObservationRCAWithdrawn,
	},
	Moves: []lifecycle.Move[RCAState, RCAAction]{
		{Action: RCAPublish, To: []RCAState{RCAPublished}, Who: editor},
		{Action: RCASupersede, From: []RCAState{RCAPublished}, To: []RCAState{RCASuperseded}, Who: []lifecycle.Who{lifecycle.System}},
		{Action: RCAShare, From: []RCAState{RCAPublished, RCASuperseded, RCAShared}, To: []RCAState{RCAShared}, Who: manager},
		{Action: RCAUnshare, From: []RCAState{RCAShared}, To: []RCAState{RCAPublished, RCASuperseded}, Event: activity.TypePublicLinkRevoked, Who: manager},
		{Action: RCAWithdraw, From: []RCAState{RCAPublished, RCASuperseded}, To: []RCAState{RCAWithdrawn}, Who: editor},
	},
}

func rcaState(liveLinks, revision, newest int) RCAState {
	switch {
	case liveLinks > 0:
		return RCAShared
	case revision < newest:
		return RCASuperseded
	}
	return RCAPublished
}

func allowedRCAActions(state RCAState, abilities folders.Abilities) []RCAAction {
	return RCAMachine.Allowed(state, canObservation(abilities, false))
}

type RCARefusal struct {
	Code    string
	Cause   error
	State   string
	Allowed []string
}

func (e *RCARefusal) Error() string { return e.Cause.Error() }

func (e *RCARefusal) Unwrap() error { return e.Cause }

func named[A ~string](actions []A) []string {
	out := make([]string, len(actions))
	for i, a := range actions {
		out[i] = string(a)
	}
	return out
}

func withdrawable(state RCAState, abilities folders.Abilities) error {
	allowed := allowedRCAActions(state, abilities)
	if slices.Contains(allowed, RCAWithdraw) {
		return nil
	}
	if !slices.Contains(allowedRCAActions(RCAPublished, abilities), RCAWithdraw) {
		return folders.ErrForbidden
	}
	return &RCARefusal{Code: CodeRCAShared, Cause: ErrRCAShared, State: string(state), Allowed: named(allowed)}
}

func refusedPublish(err error, state State, allowed []Action) error {
	var unchanged *RCAUnchangedError
	refusal := RCARefusal{State: string(state), Allowed: named(allowed)}
	switch {
	case errors.As(err, &unchanged):
		refusal.Code, refusal.Cause = CodeRCAUnchanged, unchanged
	case errors.Is(err, ErrRCALimit):
		refusal.Code, refusal.Cause = CodeRCALimit, ErrRCALimit
	case errors.Is(err, ErrObservationChanged):
		refusal.Code, refusal.Cause = CodeObservationChanged, ErrObservationChanged
	default:
		return err
	}
	return &refusal
}
