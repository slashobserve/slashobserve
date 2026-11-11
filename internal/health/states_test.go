package health

import "testing"

func TestItemStateFollowsTheRulesInOrder(t *testing.T) {
	three, five := 3, 5
	failing := []Problem{{Kind: KindQueryFailed}}
	noData := []Problem{{Kind: KindNoData}, {Kind: KindDuplicateTitle}, {Kind: KindUnopened}}
	tests := []struct {
		name    string
		checked *int
		latest  int
		skipped string
		found   []Problem
		want    string
	}{
		{"never checked", nil, 5, "", nil, "unchecked"},
		{"unchecked beats skipped", nil, 5, SkippedDependsOnViewer, failing, "unchecked"},
		{"older version checked", &three, 5, "", nil, "outdated"},
		{"outdated beats failing", &three, 5, "", failing, "outdated"},
		{"outdated beats skipped", &three, 5, SkippedDependsOnViewer, nil, "outdated"},
		{"skipped on the latest version", &five, 5, SkippedDependsOnViewer, nil, "skipped"},
		{"skipped beats failing", &five, 5, SkippedDependsOnViewer, failing, "skipped"},
		{"failing query", &five, 5, "", failing, "failing"},
		{"failing variable", &five, 5, "", []Problem{{Kind: KindNoData}, {Kind: KindVariableFailed}}, "failing"},
		{"no data, duplicate title and unopened alone are healthy", &five, 5, "", noData, "healthy"},
		{"nothing found", &five, 5, "", nil, "healthy"},
		{"checked version ahead of the latest", &five, 3, "", nil, "healthy"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := itemState(tt.checked, tt.latest, tt.skipped, tt.found); got != tt.want {
				t.Errorf("itemState = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestItemStatesListEveryReturnedState(t *testing.T) {
	for _, name := range []string{StateUnchecked, StateOutdated, StateSkipped, StateFailing, StateHealthy} {
		if !ItemStates.Has(name) {
			t.Errorf("ItemStates lacks %s", name)
		}
	}
	for _, name := range []string{OutcomeQueued, OutcomeRunning, OutcomeRepaired, OutcomeStillFailing, OutcomeFailed} {
		if !RepairOutcomes.Has(name) {
			t.Errorf("RepairOutcomes lacks %s", name)
		}
	}
}

func TestRepairOutcomeReadsTheRunStateAndTheCheckedVersion(t *testing.T) {
	two, three, four := 2, 3, 4
	tests := []struct {
		name    string
		run     string
		version *int
		checked *int
		want    string
	}{
		{"queued", "queued", nil, nil, "queued"},
		{"running", "running", nil, &two, "running"},
		{"canceling", "canceling", nil, nil, "running"},
		{"saved version not checked yet", "succeeded", &three, &two, "repaired"},
		{"saved version and nothing checked", "succeeded", &three, nil, "repaired"},
		{"saved version checked and still listed", "succeeded", &three, &three, "still_failing"},
		{"a later version was checked", "succeeded", &three, &four, "still_failing"},
		{"succeeded without a saved version", "succeeded", nil, &two, "still_failing"},
		{"failed", "failed", nil, nil, "failed"},
		{"canceled", "canceled", nil, nil, "failed"},
		{"unknown state", "paused", nil, nil, "failed"},
		{"empty state", "", nil, nil, "failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := repairOutcome(tt.run, tt.version, tt.checked); got != tt.want {
				t.Errorf("repairOutcome = %s, want %s", got, tt.want)
			}
		})
	}
}
