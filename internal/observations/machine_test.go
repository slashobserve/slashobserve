package observations

import (
	"errors"
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/lifecycle"
)

var (
	viewing    = folders.AbilitiesFor(folders.LevelView)
	commenting = folders.AbilitiesFor(folders.LevelComment)
	editing    = folders.AbilitiesFor(folders.LevelEdit)
)

func TestTheMachinesAreConsistent(t *testing.T) {
	if err := Machine.Check(); err != nil {
		t.Errorf("observation: %v", err)
	}
	if err := BlockMachine.Check(); err != nil {
		t.Errorf("block: %v", err)
	}
	if Machine.Name != "observation" || BlockMachine.Name != "observation_block" {
		t.Errorf("names %q and %q", Machine.Name, BlockMachine.Name)
	}
}

func TestEveryEventOfTheMachinesIsAKnownActivityType(t *testing.T) {
	for _, mv := range Machine.Moves {
		if mv.Event != "" && !activity.Known(mv.Event) {
			t.Errorf("observation %s: unknown event %q", mv.Action, mv.Event)
		}
	}
	for _, mv := range BlockMachine.Moves {
		if mv.Event != "" && !activity.Known(mv.Event) {
			t.Errorf("block %s: unknown event %q", mv.Action, mv.Event)
		}
	}
	for state, event := range Machine.Arrive {
		if !activity.Known(event) {
			t.Errorf("observation arrive %s: unknown event %q", state, event)
		}
	}
	for state, event := range BlockMachine.Arrive {
		if !activity.Known(event) {
			t.Errorf("block arrive %s: unknown event %q", state, event)
		}
	}
}

func TestTheEventOfEachMoveIsTheOneThePlanNames(t *testing.T) {
	events := map[Action]string{
		ActionUpdate: activity.TypeObservationUpdated, ActionMove: activity.TypeObservationMoved,
		ActionReopen: activity.TypeObservationReopened, ActionPublish: activity.TypeObservationPublished,
	}
	for action, want := range events {
		mv, found := Machine.Find(StateOpen, action)
		if action == ActionReopen {
			mv, found = Machine.Find(StateResolved, action)
		}
		if !found || mv.Event != want {
			t.Errorf("%s: event %q, want %q", action, mv.Event, want)
		}
	}
	if got := Machine.EventFor(ActionCreate, StateOpen); got != activity.TypeObservationCreated {
		t.Errorf("create event %q", got)
	}
	if got := Machine.EventFor(ActionResolve, StateResolved); got != activity.TypeObservationResolved {
		t.Errorf("resolve event %q", got)
	}
	if got := BlockMachine.EventFor(BlockAdd, BlockRequested); got != activity.TypeObservationBlockAdded {
		t.Errorf("add event %q", got)
	}
}

func TestWhatEachAbilityMayDoToAnObservation(t *testing.T) {
	cases := []struct {
		name      string
		state     State
		abilities folders.Abilities
		want      []Action
	}{
		{"viewer of an open one", StateOpen, viewing, []Action{}},
		{"commenter of an open one", StateOpen, commenting, []Action{}},
		{"editor of an open one", StateOpen, editing, []Action{ActionUpdate, ActionMove, ActionResolve, ActionPublish}},
		{"editor of a resolved one", StateResolved, editing, []Action{ActionUpdate, ActionMove, ActionReopen, ActionPublish}},
		{"viewer of a resolved one", StateResolved, viewing, []Action{}},
	}
	for _, c := range cases {
		if got := allowedActions(c.state, c.abilities); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestWhatEachAbilityMayDoToABlock(t *testing.T) {
	cases := []struct {
		name      string
		state     BlockState
		abilities folders.Abilities
		author    bool
		want      []BlockAction
	}{
		{"viewer of a live block", BlockLive, viewing, false, []BlockAction{}},
		{"editor of a live block", BlockLive, editing, false, []BlockAction{BlockUpdate, BlockRemove}},
		{"editor of a requested block", BlockRequested, editing, false, []BlockAction{BlockUpdate, BlockKeep, BlockDiscard}},
		{"commenter who asked for it", BlockRequested, commenting, true, []BlockAction{BlockDiscard}},
		{"commenter who did not ask for it", BlockRequested, commenting, false, []BlockAction{}},
		{"viewer who asked for it", BlockRequested, viewing, true, []BlockAction{BlockDiscard}},
		{"commenter author of a live block", BlockLive, commenting, true, []BlockAction{}},
	}
	for _, c := range cases {
		if got := allowedBlockActions(c.state, c.abilities, c.author); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestTheStateOfANewBlockFollowsTheAbility(t *testing.T) {
	cases := []struct {
		abilities folders.Abilities
		want      BlockState
		ok        bool
	}{
		{viewing, "", false},
		{commenting, BlockRequested, true},
		{editing, BlockLive, true},
	}
	for _, c := range cases {
		got, ok := addedState(c.abilities)
		if got != c.want || ok != c.ok {
			t.Errorf("%+v: %q %v, want %q %v", c.abilities, got, ok, c.want, c.ok)
		}
	}
}

func TestAnActionNobodyMayTakeInThatStateIsForbiddenAndOtherwiseRefused(t *testing.T) {
	if err := permit(Machine, StateOpen, ActionResolve, can(viewing, false)); !errors.Is(err, folders.ErrForbidden) {
		t.Errorf("viewer resolving: %v", err)
	}
	if err := permit(Machine, StateOpen, ActionResolve, can(editing, false)); err != nil {
		t.Errorf("editor resolving: %v", err)
	}
	err := permit(Machine, StateResolved, ActionResolve, can(editing, false))
	var refusal *lifecycle.Refusal
	if !errors.As(err, &refusal) || refusal.State != "resolved" || !slices.Equal(refusal.Allowed, []string{"update", "move", "reopen", "publish"}) {
		t.Errorf("editor resolving twice: %v", err)
	}
	err = permit(BlockMachine, BlockRequested, BlockKeep, can(commenting, true))
	if !errors.As(err, &refusal) || !slices.Equal(refusal.Allowed, []string{"discard"}) {
		t.Errorf("author keeping their own request: %v", err)
	}
	if err := permit(BlockMachine, BlockRequested, BlockKeep, can(commenting, false)); !errors.Is(err, folders.ErrForbidden) {
		t.Errorf("another commenter keeping it: %v", err)
	}
}

func TestOnlyTheActionsAPersonMayTakeAreOffered(t *testing.T) {
	if got, want := Machine.Offered(), []Action{ActionUpdate, ActionMove, ActionResolve, ActionReopen, ActionPublish}; !slices.Equal(got, want) {
		t.Errorf("observation offers %v, want %v", got, want)
	}
	if got, want := BlockMachine.Offered(), []BlockAction{BlockUpdate, BlockKeep, BlockDiscard, BlockRemove}; !slices.Equal(got, want) {
		t.Errorf("block offers %v, want %v", got, want)
	}
}
