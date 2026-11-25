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
		"add_block":                PhaseWrite,
		"pin_dashboard":            PhaseWrite,
		"claim_run":                PhaseWrite,
		"list_runs":                PhaseDiscover,
		"note_hypotheses":          PhaseLedger,
		"no_such_tool":             PhaseDiscover,
		"":                         PhaseDiscover,
	} {
		if got := Phase(name); got != want {
			t.Errorf("Phase(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestPhaseFollowsTheRegistryFlags(t *testing.T) {
	for _, tool := range New(Deps{}).List() {
		phase := Phase(tool.Name)
		switch {
		case tool.ReadOnly && (phase == PhaseWrite || phase == PhaseSave):
			t.Errorf("%s is read only and is %s", tool.Name, phase)
		case !tool.ReadOnly && phase != PhaseWrite && phase != PhaseSave:
			t.Errorf("%s writes and is %s", tool.Name, phase)
		case tool.Answers && phase != PhaseSave:
			t.Errorf("%s answers and is %s", tool.Name, phase)
		}
	}
}

func TestWriteToolsAreOfferedInEveryRunButSuggest(t *testing.T) {
	for _, tool := range New(Deps{}).List() {
		if Phase(tool.Name) != PhaseWrite || tool.MCPOnly {
			continue
		}
		for _, mode := range []Mode{ModeCreate, ModeRevise, ModeInvestigate} {
			if !tool.OfferedIn(mode) {
				t.Errorf("%s is not offered in %s runs", tool.Name, mode)
			}
		}
		if tool.OfferedIn(ModeSuggest) {
			t.Errorf("%s is offered in suggest runs", tool.Name)
		}
	}
}
