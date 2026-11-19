package tools

import (
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func variableDashboard(queries ...string) *spec.Dashboard {
	d := &spec.Dashboard{Variables: []spec.Variable{
		{Name: "service", Type: spec.QueryVariable},
		{Name: "search", Type: spec.TextboxVariable},
		{Name: "ds", Type: spec.DatasourceVariable},
	}}
	d.Panels = []spec.Panel{queryPanel("errors", spec.Timeseries, queries...)}
	return d
}

func TestADroppedFilterLeavesANoteNamingItsVariable(t *testing.T) {
	before := variableDashboard(`{service="$service"} |~ "$search"`)
	after := variableDashboard(`{service="$service"}`)
	var a advice
	a.noteDroppedVariables(before, after)
	want := []string{"variable search is no longer used by any panel or other variable after this save; remove it or use it again"}
	if !slices.Equal(a.Notes, want) {
		t.Fatalf("notes = %q", a.Notes)
	}
}

func TestVariablesStillUsedOrNeverUsedGetNoNote(t *testing.T) {
	cases := []struct {
		name          string
		before, after *spec.Dashboard
	}{
		{"no earlier version", nil, variableDashboard(`up`)},
		{"still used", variableDashboard(`up{s="$search"}`), variableDashboard(`up{s="$search", s2="$service"}`)},
		{"used in braces with a format", variableDashboard(`up{s="${search:regex}"}`), variableDashboard(`up{s=~"${search:regex}"}`)},
		{"never used before", variableDashboard(`up`), variableDashboard(`up{x="1"}`)},
	}
	for _, c := range cases {
		var a advice
		a.noteDroppedVariables(c.before, c.after)
		if len(a.Notes) != 0 {
			t.Errorf("%s: notes = %q", c.name, a.Notes)
		}
	}
}

func TestAVariableWithTheSamePrefixDoesNotKeepAnother(t *testing.T) {
	var a advice
	a.noteDroppedVariables(variableDashboard(`up{s="$search"}`), variableDashboard(`up{s="$searchterm"}`))
	if len(a.Notes) != 1 {
		t.Fatalf("notes = %q", a.Notes)
	}
}

func TestARepeatedPanelKeepsItsVariableInUse(t *testing.T) {
	after := variableDashboard(`up`)
	after.Panels[0].Repeat = "service"
	if !referencesVariable(after, "service") {
		t.Fatal("repeat does not count as a use")
	}
	if referencesVariable(after, "search") {
		t.Fatal("search is not used")
	}
}

func TestADatasourceVariableIsNeverReportedUnused(t *testing.T) {
	before := variableDashboard(`up`)
	before.Panels[0].Datasource = "${ds}"
	after := variableDashboard(`up`)
	var a advice
	a.noteDroppedVariables(before, after)
	if len(a.Notes) != 0 {
		t.Fatalf("notes = %q", a.Notes)
	}
}

func TestARepeatOnAVariableNameWithAQuoteStillCountsAsAUse(t *testing.T) {
	after := variableDashboard(`up`)
	after.Variables = append(after.Variables, spec.Variable{Name: `a"b`, Type: spec.QueryVariable})
	after.Panels[0].Repeat = `a"b`
	if !referencesVariable(after, `a"b`) {
		t.Fatal("repeat on a quoted name does not count as a use")
	}
}
