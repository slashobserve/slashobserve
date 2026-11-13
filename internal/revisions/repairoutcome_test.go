package revisions

import (
	"encoding/json"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func dashboardOf(t *testing.T, raw string) *spec.Dashboard {
	t.Helper()
	var d spec.Dashboard
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		t.Fatal(err)
	}
	return &d
}

func TestRepairedPanelsCountsNamedPanelsTheSavedVersionChangedAndVerified(t *testing.T) {
	parent := dashboardOf(t, `{"title":"Node","panels":[
		{"id":"fixed","type":"timeseries","queries":[{"query":"bad"}]},
		{"id":"untouched","type":"timeseries","queries":[{"query":"bad"}]},
		{"id":"still_failing","type":"timeseries","queries":[{"query":"bad"}]},
		{"id":"deleted","type":"timeseries","queries":[{"query":"bad"}]},
		{"id":"moved","type":"timeseries","gridPos":{"x":0,"y":0,"w":6,"h":4},"queries":[{"query":"bad"}]},
		{"id":"other","type":"timeseries","queries":[{"query":"ok"}]}]}`)
	saved := dashboardOf(t, `{"title":"Node","panels":[
		{"id":"fixed","type":"timeseries","queries":[{"query":"good"}]},
		{"id":"untouched","type":"timeseries","queries":[{"query":"bad"}]},
		{"id":"still_failing","type":"timeseries","queries":[{"query":"worse"}]},
		{"id":"moved","type":"timeseries","gridPos":{"x":6,"y":0,"w":6,"h":4},"queries":[{"query":"bad"}]},
		{"id":"other","type":"timeseries","queries":[{"query":"changed"}]}]}`)
	named := []string{"fixed", "untouched", "still_failing", "deleted", "moved", "missing"}

	if got := repairedPanels(named, parent, saved, []string{"still_failing"}); got != 1 {
		t.Errorf("repaired = %d, want 1: only fixed changed its queries, kept verified and was named", got)
	}
}

func TestRepairedPanelsCountsEveryVerifiedNamedPanelWithoutAParent(t *testing.T) {
	saved := dashboardOf(t, `{"panels":[{"id":"a","type":"stat","queries":[{"query":"x"}]},{"id":"b","type":"stat","queries":[{"query":"y"}]}]}`)

	if got := repairedPanels([]string{"a", "b", "c"}, nil, saved, nil); got != 2 {
		t.Errorf("repaired = %d, want 2", got)
	}
	if got := repairedPanels(nil, saved, saved, nil); got != 0 {
		t.Errorf("repaired = %d with nothing named, want 0", got)
	}
}
