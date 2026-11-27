package revisions

import (
	"encoding/json"
	"testing"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func runFields(t *testing.T, out agentRequestJSON) map[string]json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	return fields
}

func TestOnlyTheHTTPRunReadCarriesTheSettledResult(t *testing.T) {
	if _, present := runFields(t, agentRequestJSON{})["settled"]; present {
		t.Error("a run rendered for a tool result carries settled, and tool results must not change")
	}
	saved := "saved"
	for want, progress := range map[string]*runProgressJSON{"null": {}, `"saved"`: {Settled: &saved}} {
		if got := runFields(t, agentRequestJSON{runProgressJSON: progress})["settled"]; string(got) != want {
			t.Errorf("settled = %s, want %s", got, want)
		}
	}
	if !NewObserveHandler(nil, ObserveOptions{}).runs.httpOnlyFields || NewRunDesk(nil, ObserveOptions{}).httpOnlyFields {
		t.Error("the HTTP handler's desk adds the HTTP-only fields and the tools' desk does not")
	}
}

func TestOnlyARunAWorkerHoldsHasASessionToRead(t *testing.T) {
	for _, state := range RunMachine.States {
		if got, want := holdsSession(state), state == StateRunning || state == StateCanceling; got != want {
			t.Errorf("holdsSession(%s) = %t, want %t", state, got, want)
		}
	}
}

func TestTheHTTPRunReadCountsAdmissionCheckFailuresAgainstTheirCeiling(t *testing.T) {
	if _, present := runFields(t, agentRequestJSON{})["check_failures"]; present {
		t.Error("a run rendered for a tool result carries check_failures, and tool results must not change")
	}
	progress := progressOf(dbsqlc.AgentRequest{State: string(StateQueued), CheckFailures: 2}, "")
	fields := runFields(t, agentRequestJSON{runProgressJSON: &progress})
	if string(fields["check_failures"]) != "2" || string(fields["check_failures_max"]) != "3" || string(fields["settled"]) != "null" {
		t.Errorf("check_failures %s of %s, settled %s; want 2 of 3 and null", fields["check_failures"], fields["check_failures_max"], fields["settled"])
	}
	if !checksExhausted(dbsqlc.AgentRequest{CheckFailures: int32(progress.CheckFailuresMax - 1)}) || checksExhausted(dbsqlc.AgentRequest{CheckFailures: int32(progress.CheckFailuresMax - 2)}) {
		t.Error("check_failures_max is not the count at which a run fails with check_failed")
	}
}

func TestAFailedAdmissionCheckIsNamedInTheFailure(t *testing.T) {
	seen := map[string]admissionCheck{}
	for _, check := range []admissionCheck{checkSavedResult, checkRequester, checkRequesterAccess} {
		out := checkFailedOutcome(check, nil)
		if out.failure != FailureCheckFailed || out.message != string(check) || out.message == "" {
			t.Errorf("%q fails as %q with %q", check, out.failure, out.message)
		}
		if other, taken := seen[out.message]; taken {
			t.Errorf("%q and %q fail with the same message", check, other)
		}
		seen[out.message] = check
	}
	if checkSavedResult != "could not check the request's saved version" {
		t.Errorf("the saved version check changed its message to %q", checkSavedResult)
	}
}
