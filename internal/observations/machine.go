package observations

import (
	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/lifecycle"
)

type State string

const (
	StateOpen     State = "open"
	StateResolved State = "resolved"
)

type Action string

const (
	ActionCreate  Action = "create"
	ActionUpdate  Action = "update"
	ActionMove    Action = "move"
	ActionResolve Action = "resolve"
	ActionReopen  Action = "reopen"
	ActionPublish Action = "publish"
)

type BlockState string

const (
	BlockLive      BlockState = "live"
	BlockRequested BlockState = "requested"
	BlockDeleted   BlockState = "deleted"
)

type BlockAction string

const (
	BlockAdd     BlockAction = "add"
	BlockUpdate  BlockAction = "update"
	BlockKeep    BlockAction = "keep"
	BlockDiscard BlockAction = "discard"
	BlockRemove  BlockAction = "remove"
)

type Kind string

const (
	KindText     Kind = "text"
	KindPanel    Kind = "panel"
	KindSnapshot Kind = "snapshot"
	KindRun      Kind = "run"
)

type Width string

const (
	WidthFull Width = "full"
	WidthHalf Width = "half"
)

var editor = []lifecycle.Who{lifecycle.Editor}

var Machine = lifecycle.Machine[State, Action]{
	Name:   "observation",
	States: []State{StateOpen, StateResolved},
	Meaning: map[State]string{
		StateOpen:     "Still being worked on: blocks are added and the discussion goes on.",
		StateResolved: "Closed with a one-line summary of what was found; it can be reopened.",
	},
	Arrive: map[State]string{
		StateOpen:     activity.TypeObservationCreated,
		StateResolved: activity.TypeObservationResolved,
	},
	Moves: []lifecycle.Move[State, Action]{
		{Action: ActionCreate, To: []State{StateOpen}, Who: editor},
		{Action: ActionUpdate, From: []State{StateOpen, StateResolved}, Event: activity.TypeObservationUpdated, Who: editor},
		{Action: ActionMove, From: []State{StateOpen, StateResolved}, Event: activity.TypeObservationMoved, Who: editor},
		{Action: ActionResolve, From: []State{StateOpen}, To: []State{StateResolved}, Who: editor},
		{Action: ActionReopen, From: []State{StateResolved}, To: []State{StateOpen}, Event: activity.TypeObservationReopened, Who: editor},
		{Action: ActionPublish, From: []State{StateOpen, StateResolved}, Event: activity.TypeObservationPublished, Who: editor},
	},
}

var BlockMachine = lifecycle.Machine[BlockState, BlockAction]{
	Name:     "observation_block",
	States:   []BlockState{BlockLive, BlockRequested, BlockDeleted},
	Terminal: []BlockState{BlockDeleted},
	Meaning: map[BlockState]string{
		BlockLive:      "On the page for everyone who can see the observation.",
		BlockRequested: "Asked for by someone who can comment but not edit; an editor keeps it or discards it.",
		BlockDeleted:   "Gone: the row is deleted, so this state is never stored.",
	},
	Arrive: map[BlockState]string{
		BlockLive:      activity.TypeObservationBlockAdded,
		BlockRequested: activity.TypeObservationBlockAdded,
		BlockDeleted:   activity.TypeObservationBlockRemoved,
	},
	Moves: []lifecycle.Move[BlockState, BlockAction]{
		{Action: BlockAdd, To: []BlockState{BlockLive}, Who: editor},
		{Action: BlockAdd, To: []BlockState{BlockRequested}, Who: []lifecycle.Who{lifecycle.Commenter}},
		{Action: BlockUpdate, From: []BlockState{BlockLive, BlockRequested}, Event: activity.TypeObservationBlockUpdated, Who: editor},
		{Action: BlockKeep, From: []BlockState{BlockRequested}, To: []BlockState{BlockLive}, Event: activity.TypeObservationBlockKept, Who: editor},
		{Action: BlockDiscard, From: []BlockState{BlockRequested}, To: []BlockState{BlockDeleted}, Event: activity.TypeObservationBlockDiscarded, Who: []lifecycle.Who{lifecycle.Editor, lifecycle.Requester}},
		{Action: BlockRemove, From: []BlockState{BlockLive}, To: []BlockState{BlockDeleted}, Event: activity.TypeObservationBlockRemoved, Who: editor},
	},
}

func can(abilities folders.Abilities, author bool) func(lifecycle.Who) bool {
	return func(w lifecycle.Who) bool {
		switch w {
		case lifecycle.Editor:
			return abilities.Edit
		case lifecycle.Commenter:
			return abilities.Comment
		case lifecycle.Requester:
			return author
		}
		return false
	}
}

func allowedActions(state State, abilities folders.Abilities) []Action {
	return Machine.Allowed(state, can(abilities, false))
}

func allowedBlockActions(state BlockState, abilities folders.Abilities, author bool) []BlockAction {
	return BlockMachine.Allowed(state, can(abilities, author))
}

func addedState(abilities folders.Abilities) (BlockState, bool) {
	allowed := can(abilities, false)
	for _, mv := range BlockMachine.Moves {
		if mv.Action != BlockAdd {
			continue
		}
		for _, w := range mv.Who {
			if allowed(w) {
				return mv.To[0], true
			}
		}
	}
	return "", false
}

func permit[S, A ~string](m lifecycle.Machine[S, A], from S, a A, allowed func(lifecycle.Who) bool) error {
	if mv, found := m.Find(from, a); found {
		for _, w := range mv.Who {
			if w != lifecycle.System && allowed(w) {
				return nil
			}
		}
	}
	refusal := m.Refuse(from, a, allowed)
	if len(refusal.Allowed) == 0 {
		return folders.ErrForbidden
	}
	return refusal
}
