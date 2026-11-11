package main

import "testing"

func TestBuildOutcomeFailureCode(t *testing.T) {
	if got := (buildOutcome{failure: "provider_error"}).failureCode(); got != "provider_error" {
		t.Errorf("failureCode = %q, want provider_error", got)
	}
	if got := (buildOutcome{}).failureCode(); got != "unknown" {
		t.Errorf("a failure with no code must read unknown, got %q", got)
	}
}

func TestFailureDetailNamesTheFirstFailure(t *testing.T) {
	outcomes := []buildOutcome{{ok: true}, {failure: "provider_error"}}
	if got, want := failureDetail(outcomes, 1, 2), "1 of 2 succeeded; first failure: provider_error"; got != want {
		t.Errorf("failureDetail = %q, want %q", got, want)
	}
}
