package grafana

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const promRef = `{"uid":"prom","type":"prometheus"}`

func templating(variables ...string) string {
	return fmt.Sprintf(`"templating":{"list":[%s]}`, strings.Join(variables, ","))
}

func convertVariables(t *testing.T, variables ...string) *Result {
	t.Helper()
	return convertDoc(t, dashboardJSON(templating(variables...), panelJSON(1, 0, 0, 12, 4, "")))
}

func variableNamed(t *testing.T, res *Result, name string) spec.Variable {
	t.Helper()
	for _, v := range res.Spec.Variables {
		if v.Name == name {
			return v
		}
	}
	t.Fatalf("no variable %q in %+v", name, res.Spec.Variables)
	return spec.Variable{}
}

func TestAdhocSavedFiltersAreReported(t *testing.T) {
	res := convertVariables(t,
		`{"name":"env","type":"adhoc","datasource":`+promRef+`,"filters":[{"key":"a","operator":"=","value":"b"}],"baseFilters":[{"key":"c","operator":"=","value":"d"}]}`,
		`{"name":"plain","type":"adhoc","datasource":`+promRef+`,"filters":[]}`,
	)
	if got := variableNamed(t, res, "env").Type; got != spec.AdhocVariable {
		t.Fatalf("env type = %q", got)
	}
	notes := notesFor(res, "variable env")
	if len(notes) != 1 || notes[0].Reason != "Saved filters are not kept." ||
		!hasPath(notes, "templating.list[0].filters") || !hasPath(notes, "templating.list[0].baseFilters") {
		t.Fatalf("env notes = %+v", notes)
	}
	if notes := notesFor(res, "variable plain"); len(notes) != 0 {
		t.Fatalf("plain notes = %+v, want none for an empty filter list", notes)
	}
}

func TestReorderedVariablesAreReported(t *testing.T) {
	later := `{"name":"b","type":"query","datasource":` + promRef + `,"query":"label_values(up{job=\"$a\"}, instance)"}`
	first := `{"name":"a","type":"query","datasource":` + promRef + `,"query":"label_values(up, job)"}`

	res := convertVariables(t, later, first)
	if names := variableNames(res); !slices.Equal(names, []string{"a", "b"}) {
		t.Fatalf("variable order = %v, want a then b", names)
	}
	notes := notesFor(res, "variables")
	if len(notes) != 1 || !hasPath(notes, "templating.list[1]") {
		t.Fatalf("variables notes = %+v, want one naming the pulled-forward variable", notes)
	}

	res = convertVariables(t, first, later)
	if notes := notesFor(res, "variables"); len(notes) != 0 {
		t.Fatalf("variables notes = %+v, want none when the order held", notes)
	}
}

func TestIndirectlyReorderedVariablesAreReported(t *testing.T) {
	queryUsing := func(name, uses string) string {
		query := "label_values(up, job)"
		if uses != "" {
			query = `label_values(up{job=\"$` + uses + `\"}, instance)`
		}
		return `{"name":"` + name + `","type":"query","datasource":` + promRef + `,"query":"` + query + `"}`
	}
	res := convertVariables(t, queryUsing("a", "c"), queryUsing("b", ""), queryUsing("c", "b"), queryUsing("d", ""))
	if names := variableNames(res); !slices.Equal(names, []string{"b", "c", "a", "d"}) {
		t.Fatalf("variable order = %v, want b, c, a, d", names)
	}
	notes := notesFor(res, "variables")
	if len(notes) != 1 || !slices.Equal(notes[0].Paths, []string{"templating.list[0]", "templating.list[1]", "templating.list[2]"}) {
		t.Fatalf("variables notes = %+v, want a, b and c, whose relative order changed, and not d", notes)
	}
}

func variableNames(res *Result) []string {
	var names []string
	for _, v := range res.Spec.Variables {
		names = append(names, v.Name)
	}
	return names
}

func TestCustomPairsSplitAtTheLastSeparator(t *testing.T) {
	res := convertVariables(t, `{"name":"c","type":"custom","query":"a : b : c, d : e,plain"}`)
	if got := variableNamed(t, res, "c").Options; !slices.Equal(got, []string{"c", "e", "plain"}) {
		t.Fatalf("options = %q", got)
	}
}

func TestDatasourceVariableAllValueIsReported(t *testing.T) {
	res := convertVariables(t, `{"name":"ds","type":"datasource","query":"prometheus","allValue":".*"}`)
	if notes := notesFor(res, "variable ds"); len(notes) != 1 || !hasPath(notes, "templating.list[0].allValue") {
		t.Fatalf("notes = %+v", notes)
	}
}

func TestIntervalDefault(t *testing.T) {
	cases := []struct {
		name    string
		current string
		auto    bool
		want    *spec.Selection
	}{
		{"auto selected", `"$__auto_interval_iv"`, true, nil},
		{"plain auto selected", `"$__auto"`, true, nil},
		{"first option with auto on is written", `"1m"`, true, &spec.Selection{Values: []string{"1m"}}},
		{"later option", `"5m"`, true, &spec.Selection{Values: []string{"5m"}}},
		{"later option without auto", `"5m"`, false, &spec.Selection{Values: []string{"5m"}}},
		{"unknown value", `"7m"`, true, nil},
		{"no current", ``, true, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			current := ""
			if tc.current != "" {
				current = `,"current":{"value":` + tc.current + `}`
			}
			res := convertVariables(t, fmt.Sprintf(`{"name":"iv","type":"interval","query":"1m,5m","auto":%v%s}`, tc.auto, current))
			v := variableNamed(t, res, "iv")
			if v.Auto != tc.auto {
				t.Fatalf("auto = %v", v.Auto)
			}
			if (v.Default == nil) != (tc.want == nil) || (tc.want != nil && !slices.Equal(v.Default.Values, tc.want.Values)) {
				t.Fatalf("default = %+v, want %+v", v.Default, tc.want)
			}
		})
	}
}
