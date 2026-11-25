package revisions

import "testing"

func TestEveryRunPhaseIsAValidStepPhase(t *testing.T) {
	for _, phase := range []string{"discover", "lint", "query", "save", "write", "repair"} {
		if !validPhase(phase) {
			t.Errorf("%q is not a valid step phase", phase)
		}
	}
	if validPhase("") || validPhase("other") {
		t.Fatal("an unknown phase was accepted")
	}
}
