package tools

import (
	"encoding/json"
	"testing"

	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestASaveToolResultCarriesTheSaveOutcome(t *testing.T) {
	saved := dashboards.Version{Spec: &spec.Dashboard{}}
	merged := &dashboards.MergedFrom{BaseVersion: 1, OntoVersion: 2}
	cases := []dashboards.SaveResult{
		{Version: saved},
		{Version: saved, Replayed: true},
		{Version: saved, MergedFrom: merged},
		{Proposal: &dashboards.ProposalRef{ID: "p", Revision: 1, Opened: true}},
	}
	for _, c := range cases {
		res := (&Registry{}).saveSucceeded(t.Context(), c, nil)
		raw, err := json.Marshal(res.Content)
		if err != nil {
			t.Fatal(err)
		}
		var body struct {
			Outcome  string `json:"outcome"`
			Replayed *bool  `json:"replayed"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		if body.Outcome != c.Outcome() {
			t.Errorf("outcome = %q, want %q in %s", body.Outcome, c.Outcome(), raw)
		}
		if c.Proposal == nil && (body.Replayed == nil || *body.Replayed != c.Replayed) {
			t.Errorf("replayed must stay beside outcome %q in %s", c.Outcome(), raw)
		}
	}
}
