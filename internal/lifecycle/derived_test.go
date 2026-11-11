package lifecycle_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/lifecycle"
)

var sample = lifecycle.Derived{Name: "light", States: []lifecycle.DerivedState{
	{Name: "off", When: "The switch is down."},
	{Name: "on", When: "None of the above."},
}}

func TestDerivedNamesKeepTheRuleOrder(t *testing.T) {
	if got := sample.Names(); !slices.Equal(got, []string{"off", "on"}) {
		t.Errorf("Names = %v", got)
	}
}

func TestDerivedHasOnlyItsOwnStates(t *testing.T) {
	for name, want := range map[string]bool{"off": true, "on": true, "dim": false, "": false} {
		if got := sample.Has(name); got != want {
			t.Errorf("Has(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestRenderWritesADerivedSectionWithoutActions(t *testing.T) {
	got := lifecycle.Render(sample.Doc("Light", "`GET /lights` returns it."))
	want := strings.Join([]string{
		"## Light",
		"",
		"`GET /lights` returns it.",
		"This state is computed from stored fields on every read; nothing stores it.",
		"",
		"| State | When |",
		"| --- | --- |",
		"| `off` | The switch is down. |",
		"| `on` | None of the above. |",
		"",
	}, "\n")
	if !strings.HasSuffix(got, "\n"+want) {
		t.Errorf("derived section mismatch\n--- got ---\n%s\n--- want suffix ---\n%s", got, want)
	}
	if strings.Contains(got, "| Action |") || strings.Contains(got, "Actions |") {
		t.Errorf("derived section has an actions or moves table:\n%s", got)
	}
}
