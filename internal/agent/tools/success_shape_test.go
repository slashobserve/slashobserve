package tools

import (
	"encoding/json"
	"testing"
)

func TestSuccessResultsCarryNoCode(t *testing.T) {
	for name, out := range map[string]any{
		"observation write": observationWrittenOut{},
		"rca publish":       rcaPublishedOut{},
		"finding":           findingSavedOut{},
		"run answer":        runAnswerSavedOut{},
	} {
		body, err := json.Marshal(out)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(body, &fields); err != nil {
			t.Fatal(err)
		}
		if _, has := fields["code"]; has {
			t.Errorf("%s success carries a code field: %s", name, body)
		}
	}
}
