package health

import (
	"testing"

	"github.com/slashobserve/slashobserve/internal/revisions"
)

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
	for _, name := range []string{OutcomeQueued, OutcomeRunning, OutcomeRepaired, OutcomeStillFailing, OutcomeProposed, OutcomeFailed} {
		if !RepairOutcomes.Has(name) {
			t.Errorf("RepairOutcomes lacks %s", name)
		}
	}
}

func TestRepairOutcomeReadsTheRunStateAndTheCheckedVersion(t *testing.T) {
	two, three, four := 2, 3, 4
	failing := []Problem{{Kind: KindQueryFailed}}
	empty := []Problem{{Kind: KindNoData}}
	badVariable := []Problem{{Kind: KindVariableFailed}}
	tests := []struct {
		name    string
		run     string
		kind    revisions.ResultKind
		version *int
		checked *int
		found   []Problem
		want    string
	}{
		{"queued", "queued", revisions.ResultNone, nil, nil, nil, "queued"},
		{"running", "running", revisions.ResultNone, nil, &two, nil, "running"},
		{"canceling", "canceling", revisions.ResultNone, nil, nil, nil, "running"},
		{"saved version not checked yet", "succeeded", revisions.ResultVersion, &three, &two, failing, "repaired"},
		{"saved version and nothing checked", "succeeded", revisions.ResultVersion, &three, nil, nil, "repaired"},
		{"saved version checked and fixed", "succeeded", revisions.ResultVersion, &three, &three, nil, "repaired"},
		{"saved version checked with only other problems", "succeeded", revisions.ResultVersion, &three, &three, []Problem{{Kind: KindDuplicateTitle}}, "repaired"},
		{"saved version checked and still failing", "succeeded", revisions.ResultVersion, &three, &three, failing, "still_failing"},
		{"saved version checked and still without data", "succeeded", revisions.ResultVersion, &three, &three, empty, "still_failing"},
		{"saved version checked with a failing variable", "succeeded", revisions.ResultVersion, &three, &three, badVariable, "still_failing"},
		{"a later version was checked and fails", "succeeded", revisions.ResultVersion, &three, &four, failing, "still_failing"},
		{"a later version was checked and is fixed", "succeeded", revisions.ResultVersion, &three, &four, nil, "repaired"},
		{"succeeded with a proposal", "succeeded", revisions.ResultProposal, nil, &two, failing, "proposed"},
		{"succeeded without a saved version", "succeeded", revisions.ResultNone, nil, &two, nil, "still_failing"},
		{"failed", "failed", revisions.ResultNone, nil, nil, nil, "failed"},
		{"failed with a proposal kind left", "failed", revisions.ResultProposal, nil, nil, nil, "failed"},
		{"canceled", "canceled", revisions.ResultNone, nil, nil, nil, "failed"},
		{"unknown state", "paused", revisions.ResultNone, nil, nil, nil, "failed"},
		{"empty state", "", revisions.ResultNone, nil, nil, nil, "failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := repairOutcome(tt.run, tt.kind, tt.version, tt.checked, tt.found); got != tt.want {
				t.Errorf("repairOutcome = %s, want %s", got, tt.want)
			}
		})
	}
}
