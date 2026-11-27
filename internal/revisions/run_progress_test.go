package revisions

import (
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
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

func TestTheRunReadCarriesTheSettledResultOrNull(t *testing.T) {
	saved := "saved"
	for want, settled := range map[string]*string{"null": nil, `"saved"`: &saved} {
		if got := runFields(t, agentRequestJSON{Settled: settled})["settled"]; string(got) != want {
			t.Errorf("settled = %s, want %s", got, want)
		}
	}
}

func TestOnlyARunAWorkerHoldsHasASessionToRead(t *testing.T) {
	for _, state := range RunMachine.States {
		if got, want := holdsSession(state), state == StateRunning || state == StateCanceling; got != want {
			t.Errorf("holdsSession(%s) = %t, want %t", state, got, want)
		}
	}
}

func TestTheRunReadCountsAdmissionCheckFailuresAgainstTheirCeiling(t *testing.T) {
	desk := NewRunDesk(&Service{logger: slog.New(slog.DiscardHandler)}, ObserveOptions{})
	out := desk.view(t.Context(), dbsqlc.AgentRequest{State: string(StateQueued), CheckFailures: 2}, auth.Principal{}, buildState{}, nil, nil)
	fields := runFields(t, out)
	if string(fields["check_failures"]) != "2" || string(fields["check_failures_max"]) != "3" {
		t.Errorf("check_failures %s of %s, want 2 of 3", fields["check_failures"], fields["check_failures_max"])
	}
	if out.CheckFailuresMax != maxCheckFailures || !checksExhausted(dbsqlc.AgentRequest{CheckFailures: maxCheckFailures - 1}) || checksExhausted(dbsqlc.AgentRequest{CheckFailures: maxCheckFailures - 2}) {
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
