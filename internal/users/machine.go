package users

import (
	"slices"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/lifecycle"
)

type State string

const (
	StateInvited     State = "invited"
	StateActive      State = "active"
	StateDeactivated State = "deactivated"
	StateRemoved     State = "removed"
)

type Action string

const (
	ActionInvite     Action = "invite"
	ActionWithdraw   Action = "withdraw"
	ActionJoin       Action = "join"
	ActionSetRole    Action = "set_role"
	ActionSignOut    Action = "sign_out"
	ActionDeactivate Action = "deactivate"
	ActionReactivate Action = "reactivate"
	ActionRemove     Action = "remove"
)

const eventSignedIn = "auth.login"

var PersonMachine = lifecycle.Machine[State, Action]{
	Name:     "person",
	States:   []State{StateInvited, StateActive, StateDeactivated, StateRemoved},
	Terminal: []State{StateRemoved},
	Meaning: map[State]string{
		StateInvited:     "An Admin invited the email and nobody has signed in with it; listed by `GET /invites`, not by `GET /users`.",
		StateActive:      "Can sign in and work.",
		StateDeactivated: "Cannot sign in; their sessions and connected agents are gone and their work is kept.",
		StateRemoved:     "The invite was withdrawn, or the deactivated person and their My Dashboards were deleted.",
	},
	Arrive: map[State]string{
		StateInvited:     activity.TypeUserInvited,
		StateActive:      eventSignedIn,
		StateDeactivated: activity.TypeUserDeactivated,
		StateRemoved:     activity.TypeUserRemoved,
	},
	Moves: []lifecycle.Move[State, Action]{
		{Action: ActionInvite, To: []State{StateInvited}, Who: []lifecycle.Who{lifecycle.Admin}},
		{Action: ActionInvite, From: []State{StateInvited}, Event: activity.TypeUserInviteUpdated, Who: []lifecycle.Who{lifecycle.Admin}},
		{Action: ActionWithdraw, From: []State{StateInvited}, To: []State{StateRemoved}, Event: activity.TypeUserInviteWithdrawn, Who: []lifecycle.Who{lifecycle.Admin}},
		{Action: ActionJoin, To: []State{StateActive}, Who: []lifecycle.Who{lifecycle.System}},
		{Action: ActionJoin, From: []State{StateInvited}, To: []State{StateActive}, Who: []lifecycle.Who{lifecycle.System}},
		{Action: ActionSetRole, From: []State{StateActive, StateDeactivated}, Event: activity.TypeUserRoleChanged, Who: []lifecycle.Who{lifecycle.Admin}},
		{Action: ActionSignOut, From: []State{StateActive}, Event: activity.TypeUserSignedOutEverywhere, Who: []lifecycle.Who{lifecycle.Admin}},
		{Action: ActionDeactivate, From: []State{StateActive}, To: []State{StateDeactivated}, Who: []lifecycle.Who{lifecycle.Admin}},
		{Action: ActionReactivate, From: []State{StateDeactivated}, To: []State{StateActive}, Event: activity.TypeUserReactivated, Who: []lifecycle.Who{lifecycle.Admin}},
		{Action: ActionRemove, From: []State{StateDeactivated}, To: []State{StateRemoved}, Who: []lifecycle.Who{lifecycle.Admin}},
	},
}

type facts struct {
	state      State
	roleLocked bool
	self       bool
	caller     auth.Principal
}

func stateOf(deactivatedAt pgtype.Timestamptz) State {
	if deactivatedAt.Valid {
		return StateDeactivated
	}
	return StateActive
}

func factsOf(caller auth.Principal, id pgtype.UUID, source auth.RoleSource, deactivatedAt pgtype.Timestamptz) facts {
	return facts{state: stateOf(deactivatedAt), roleLocked: source.Locked(), self: caller.ID == id.String(), caller: caller}
}

func (f facts) can(w lifecycle.Who) bool {
	return w == lifecycle.Admin && f.caller.Kind == auth.KindUser && f.caller.Role == auth.RoleAdmin
}

func (f facts) blocked(a Action) bool {
	switch a {
	case ActionSetRole:
		return f.roleLocked
	case ActionDeactivate:
		return f.self
	}
	return false
}

func allowedActions(f facts) []Action {
	return slices.DeleteFunc(PersonMachine.Allowed(f.state, f.can), f.blocked)
}
