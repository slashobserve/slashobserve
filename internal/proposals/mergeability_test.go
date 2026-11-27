package proposals

import (
	"encoding/json"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestProposalJSONSaysWhetherAMergeWouldGoThrough(t *testing.T) {
	clash := spec.Conflict{Kind: spec.UnitPanel, ID: "p1", Reason: spec.ReasonBothChanged}
	removed := spec.Conflict{Kind: spec.UnitPanel, ID: "p2", Reason: spec.ReasonChangedAndRemoved}
	broken := spec.Conflict{Kind: spec.UnitVariable, ID: "env", Reason: spec.ReasonInvalid}
	cases := []struct {
		name     string
		proposal Proposal
		want     string
	}{
		{"an open proposal on the latest version", Proposal{Item: Item{State: StateOpen}}, `"current"`},
		{"an open proposal behind with nothing clashing", Proposal{Item: Item{State: StateOpen, Behind: true}}, `"behind_clean"`},
		{"an open proposal behind with a unit both sides changed", Proposal{Item: Item{State: StateOpen, Behind: true}, Conflicts: []spec.Conflict{clash, removed}}, `"behind_conflicting"`},
		{"an open proposal whose merge would be invalid", Proposal{Item: Item{State: StateOpen, Behind: true}, Conflicts: []spec.Conflict{clash, broken}}, `"blocked"`},
		{"a merged proposal", Proposal{Item: Item{State: StateMerged, Behind: true}}, `null`},
		{"a discarded proposal", Proposal{Item: Item{State: StateDiscarded, Behind: true}}, `null`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw, err := json.Marshal(NewProposalJSON(c.proposal))
			if err != nil {
				t.Fatal(err)
			}
			var out map[string]json.RawMessage
			if err := json.Unmarshal(raw, &out); err != nil {
				t.Fatal(err)
			}
			if got := string(out["mergeability"]); got != c.want {
				t.Fatalf("mergeability = %q, want %s", got, c.want)
			}
		})
	}
}
