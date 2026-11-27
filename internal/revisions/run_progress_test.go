package revisions

import (
	"encoding/json"
	"testing"
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
