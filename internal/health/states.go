package health

import (
	"slices"

	"github.com/slashobserve/slashobserve/internal/lifecycle"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	StateUnchecked = "unchecked"
	StateOutdated  = "outdated"
	StateSkipped   = "skipped"
	StateFailing   = "failing"
	StateHealthy   = "healthy"

	OutcomeQueued       = "queued"
	OutcomeRunning      = "running"
	OutcomeRepaired     = "repaired"
	OutcomeStillFailing = "still_failing"
	OutcomeFailed       = "failed"
)

var ItemStates = lifecycle.Derived{Name: "dashboard health", States: []lifecycle.DerivedState{
	{Name: StateUnchecked, When: "The health check has not checked this dashboard yet."},
	{Name: StateOutdated, When: "The last check ran on an older version than the latest one."},
	{Name: StateSkipped, When: "The last check skipped the dashboard because its queries read the viewer's identity, so no one result applies."},
	{Name: StateFailing, When: "The last check found a panel query or a variable that fails."},
	{Name: StateHealthy, When: "None of the above. No data, a duplicate title and not being opened stay problems in the list but do not make a dashboard failing."},
}}

var RepairOutcomes = lifecycle.Derived{Name: "health repair", States: []lifecycle.DerivedState{
	{Name: OutcomeQueued, When: "The repair run waits for a worker."},
	{Name: OutcomeRunning, When: "The repair run is running or stopping."},
	{Name: OutcomeRepaired, When: "The run saved a version that the health check has not checked yet."},
	{Name: OutcomeStillFailing, When: "The run saved a version and the check that ran on it still lists the dashboard."},
	{Name: OutcomeFailed, When: "The run failed or was canceled."},
}}

func itemState(checkedVersion *int, latestVersion int, skipped string, checked []Problem) string {
	switch {
	case checkedVersion == nil:
		return StateUnchecked
	case *checkedVersion < latestVersion:
		return StateOutdated
	case skipped != "":
		return StateSkipped
	case slices.ContainsFunc(checked, func(p Problem) bool { return p.Kind == KindQueryFailed || p.Kind == KindVariableFailed }):
		return StateFailing
	}
	return StateHealthy
}

func repairOutcome(runState string, version, checkedVersion *int) string {
	switch revisions.State(runState) {
	case revisions.StateQueued:
		return OutcomeQueued
	case revisions.StateRunning, revisions.StateCanceling:
		return OutcomeRunning
	case revisions.StateSucceeded:
		if version != nil && (checkedVersion == nil || *checkedVersion < *version) {
			return OutcomeRepaired
		}
		return OutcomeStillFailing
	}
	return OutcomeFailed
}
