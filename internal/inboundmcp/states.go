package inboundmcp

import "github.com/slashobserve/slashobserve/internal/lifecycle"

type ToolState string

type ToolAction string

const (
	ToolOffered  ToolState = "offered"
	ToolApproved ToolState = "approved"
	ToolChanged  ToolState = "changed"
	ToolGone     ToolState = "gone"
	ToolRefused  ToolState = "refused"

	ActionList    ToolAction = "list"
	ActionReject  ToolAction = "reject"
	ActionOffer   ToolAction = "offer"
	ActionRefuse  ToolAction = "refuse"
	ActionApprove ToolAction = "approve"
	ActionRevoke  ToolAction = "revoke"
	ActionChange  ToolAction = "change"
	ActionDrop    ToolAction = "drop"
	ActionRestore ToolAction = "restore"

	eventDiscover = "mcp_source.discover"
	eventApprove  = "mcp_source.approve"
	eventRevoke   = "mcp_source.revoke"
)

var ToolMachine = lifecycle.Machine[ToolState, ToolAction]{
	Name:     "MCP source tool",
	States:   []ToolState{ToolOffered, ToolApproved, ToolChanged, ToolGone, ToolRefused},
	Terminal: nil,
	Meaning: map[ToolState]string{
		ToolOffered:  "An Admin has not approved the tool; the agent cannot call it. read_only says whether the server marks it as only reading.",
		ToolApproved: "An Admin approved the tool as pinned (its name, description and input schema, by hash); the agent may call it.",
		ToolChanged:  "The server now describes an approved tool differently from what was pinned; it is not served until an Admin approves the new description.",
		ToolGone:     "The server no longer lists an approved tool; it is not served, and it returns to approved if the server lists it again unchanged.",
		ToolRefused:  "The server marks the tool destructive, or its description or schema is too large; refusal says why and nobody can approve it.",
	},
	Arrive: map[ToolState]string{
		ToolOffered:  eventDiscover,
		ToolApproved: eventApprove,
		ToolChanged:  eventDiscover,
		ToolGone:     eventDiscover,
		ToolRefused:  eventDiscover,
	},
	Moves: []lifecycle.Move[ToolState, ToolAction]{
		{Action: ActionList, To: []ToolState{ToolOffered}, Event: eventDiscover, Who: []lifecycle.Who{lifecycle.System}},
		{Action: ActionReject, To: []ToolState{ToolRefused}, Event: eventDiscover, Who: []lifecycle.Who{lifecycle.System}},
		{Action: ActionOffer, From: []ToolState{ToolRefused}, To: []ToolState{ToolOffered}, Event: eventDiscover, Who: []lifecycle.Who{lifecycle.System}},
		{Action: ActionRefuse, From: []ToolState{ToolOffered, ToolApproved, ToolChanged, ToolGone}, To: []ToolState{ToolRefused}, Event: eventDiscover, Who: []lifecycle.Who{lifecycle.System}},
		{Action: ActionApprove, From: []ToolState{ToolOffered, ToolChanged}, To: []ToolState{ToolApproved}, Event: eventApprove, Who: []lifecycle.Who{lifecycle.Admin}},
		{Action: ActionRevoke, From: []ToolState{ToolApproved, ToolChanged}, To: []ToolState{ToolOffered}, Event: eventRevoke, Who: []lifecycle.Who{lifecycle.Admin}},
		{Action: ActionChange, From: []ToolState{ToolApproved, ToolGone}, To: []ToolState{ToolChanged}, Event: eventDiscover, Who: []lifecycle.Who{lifecycle.System}},
		{Action: ActionDrop, From: []ToolState{ToolApproved, ToolChanged}, To: []ToolState{ToolGone}, Event: eventDiscover, Who: []lifecycle.Who{lifecycle.System}},
		{Action: ActionRestore, From: []ToolState{ToolChanged, ToolGone}, To: []ToolState{ToolApproved}, Event: eventDiscover, Who: []lifecycle.Who{lifecycle.System}},
	},
}

const (
	StateUnchecked   = "unchecked"
	StateReady       = "ready"
	StateUnreachable = "unreachable"
)

var SourceStates = lifecycle.Derived{Name: "MCP source", States: []lifecycle.DerivedState{
	{Name: StateUnchecked, When: "No discovery has run since the source was added or its address changed."},
	{Name: StateUnreachable, When: "The last discovery failed; failure says why, such as the domain not being on the outbound allowlist, the secret being refused or the server not answering."},
	{Name: StateReady, When: "None of the above: the last discovery listed the server's tools."},
}}

func sourceState(checked bool, failure string) string {
	switch {
	case !checked:
		return StateUnchecked
	case failure != "":
		return StateUnreachable
	}
	return StateReady
}
