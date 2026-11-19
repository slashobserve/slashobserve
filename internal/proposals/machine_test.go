package proposals

import (
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/internal/activity"
)

func TestMachineIsConsistent(t *testing.T) {
	if err := ProposalMachine.Check(); err != nil {
		t.Fatal(err)
	}
}

func TestAllowedActionsFollowTheCallersPowers(t *testing.T) {
	cases := []struct {
		name string
		in   facts
		want []Action
	}{
		{"the author without edit", facts{state: StateOpen, author: true}, []Action{ActionUpdate, ActionDiscard}},
		{"an editor who is not the author", facts{state: StateOpen, edit: true}, []Action{ActionMerge, ActionDiscard}},
		{"the author who can edit", facts{state: StateOpen, author: true, edit: true}, []Action{ActionUpdate, ActionMerge, ActionDiscard}},
		{"the author who can edit in propose mode", facts{state: StateOpen, author: true, edit: true, propose: true}, []Action{ActionUpdate, ActionDiscard}},
		{"an editor in propose mode who is not the author", facts{state: StateOpen, edit: true, propose: true}, []Action{}},
		{"a commenter who is not the author", facts{state: StateOpen}, []Action{}},
		{"anyone on a merged proposal", facts{state: StateMerged, author: true, edit: true}, []Action{}},
		{"anyone on a discarded proposal", facts{state: StateDiscarded, author: true, edit: true}, []Action{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := allowedActions(c.in); !slices.Equal(got, c.want) {
				t.Errorf("allowedActions(%+v) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestTheLiveEventNamesTheStateAProposalMovedTo(t *testing.T) {
	cases := map[string]State{
		activity.TypeProposalOpened:    StateOpen,
		activity.TypeProposalUpdated:   StateOpen,
		activity.TypeProposalMerged:    StateMerged,
		activity.TypeProposalDiscarded: StateDiscarded,
	}
	for typ, want := range cases {
		if got := stateAfter(typ); got != want {
			t.Errorf("stateAfter(%s) = %s, want %s", typ, got, want)
		}
	}
}
