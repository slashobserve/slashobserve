package proposals

import (
	"slices"

	"github.com/slashobserve/slashobserve/internal/lifecycle"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

type Mergeability string

const (
	MergeabilityCurrent           Mergeability = "current"
	MergeabilityBehindClean       Mergeability = "behind_clean"
	MergeabilityBehindConflicting Mergeability = "behind_conflicting"
	MergeabilityBlocked           Mergeability = "blocked"
)

var MergeabilityStates = lifecycle.Derived{Name: "proposal mergeability", States: []lifecycle.DerivedState{
	{Name: string(MergeabilityCurrent), When: "It builds on the dashboard's latest version; a merge saves it as it is."},
	{Name: string(MergeabilityBehindClean), When: "The dashboard has newer versions that changed other panels, variables and settings; a merge combines them without a choice."},
	{Name: string(MergeabilityBehindConflicting), When: "The dashboard has newer versions that changed the same panel, variable or setting; a merge needs a choice for each conflict."},
	{Name: string(MergeabilityBlocked), When: "Combined with the latest version it is not a valid dashboard (a conflict whose reason is `invalid`); no choice fixes that, so every merge is refused until its author saves a new revision."},
}}

const notMergeable Mergeability = ""

func (p Proposal) Mergeability() Mergeability {
	switch {
	case p.State != StateOpen:
		return notMergeable
	case !p.Behind:
		return MergeabilityCurrent
	case slices.ContainsFunc(p.Conflicts, func(c spec.Conflict) bool { return c.Reason == spec.ReasonInvalid }):
		return MergeabilityBlocked
	case len(p.Conflicts) > 0:
		return MergeabilityBehindConflicting
	}
	return MergeabilityBehindClean
}
