package dashboards

import "github.com/slashobserve/slashobserve/internal/lifecycle"

const (
	OutcomeProposed = "proposed"
	OutcomeReplayed = "replayed"
	OutcomeMerged   = "merged"
	OutcomeCreated  = "created"
)

var SaveOutcomes = lifecycle.Derived{Name: "dashboard save", States: []lifecycle.DerivedState{
	{Name: OutcomeProposed, When: "The caller may not save here, so the change became a proposal, or a revision of the caller's open one; no version was saved."},
	{Name: OutcomeReplayed, When: "The save repeats a version that already exists, so that version is returned and no version was saved."},
	{Name: OutcomeMerged, When: "The save was stale and the server merged it onto the latest version; the merge is the new version and merged_from names both ends."},
	{Name: OutcomeCreated, When: "None of the above; the save is the new version."},
}}

func (r SaveResult) Outcome() string {
	switch {
	case r.Proposal != nil:
		return OutcomeProposed
	case r.Replayed:
		return OutcomeReplayed
	case r.MergedFrom != nil:
		return OutcomeMerged
	}
	return OutcomeCreated
}
