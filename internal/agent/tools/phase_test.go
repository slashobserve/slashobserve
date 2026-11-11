package tools

import "testing"

func TestPhaseOfEachTool(t *testing.T) {
	for name, want := range map[string]string{
		"lint_query":               PhaseLint,
		"query_range":              PhaseQuery,
		"query_instant":            PhaseQuery,
		"create_dashboard":         PhaseSave,
		"update_dashboard":         PhaseSave,
		"update_panels":            PhaseSave,
		"import_grafana_dashboard": PhaseSave,
		"save_finding":             PhaseSave,
		"get_panels":               PhaseDiscover,
		"list_datasources":         PhaseDiscover,
		"no_such_tool":             PhaseDiscover,
		"":                         PhaseDiscover,
	} {
		if got := Phase(name); got != want {
			t.Errorf("Phase(%q) = %q, want %q", name, got, want)
		}
	}
}
