package observations

import (
	"encoding/json"
	"testing"

	"github.com/slashobserve/slashobserve/internal/folders"
)

func TestAnObservationReadSaysWhatConfinesItOnlyWhenSomethingDoes(t *testing.T) {
	h := NewHandler(&Service{}, nil)
	for _, tc := range []struct {
		name        string
		confinement folders.Confinement
		want        string
	}{
		{"nothing confines it", folders.NotConfined, ""},
		{"it holds a kept answer", folders.ConfinedPermanent, "permanent"},
		{"a run about it is in flight", folders.ConfinedRunInFlight, "run_in_flight"},
	} {
		raw, err := json.Marshal(h.envelope(Observation{Confinement: tc.confinement}))
		if err != nil {
			t.Fatal(err)
		}
		var body struct {
			Confined *struct {
				Reason string `json:"reason"`
			} `json:"confined"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("decode %s: %v", raw, err)
		}
		switch {
		case tc.want == "" && body.Confined != nil:
			t.Errorf("%s: confined = %+v, want it left out", tc.name, body.Confined)
		case tc.want != "" && (body.Confined == nil || body.Confined.Reason != tc.want):
			t.Errorf("%s: confined = %+v, want reason %s", tc.name, body.Confined, tc.want)
		}
	}
}
