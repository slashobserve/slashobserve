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

func TestClickHouseAdhocNoteListsTheQueriesItWillNotFilter(t *testing.T) {
	const chRef = `{"type":"grafana-clickhouse-datasource","uid":"ch"}`
	adhoc := func(datasource string) string {
		return `{"name":"filters","type":"adhoc","datasource":` + datasource + `}`
	}
	chPanel := func(id int, sql string) string {
		return panelJSON(id, 0, id*4, 12, 4, `"type":"table","datasource":`+chRef+`,"targets":[{"refId":"A","rawSql":"`+sql+`","format":1}]`)
	}
	promPanel := panelJSON(3, 0, 12, 12, 4, `"type":"stat","datasource":`+promRef+`,"targets":[{"refId":"A","expr":"up"}]`)
	reasonFor := func(unfiltered string) string {
		return "Grafana applies these filters to every query on this datasource; SlashObserve applies them only in queries that use $filters, so " + unfiltered + " not filtered."
	}

	cases := []struct {
		name      string
		variable  string
		panels    []string
		wantPaths []string
		wantCount string
	}{
		{"one of two queries uses the variable", adhoc(chRef), []string{chPanel(1, "SELECT 1 WHERE $filters"), chPanel(2, "SELECT 2")}, []string{"panels[1].targets[0]"}, "1 query is"},
		{"two of three queries use no variable", adhoc(chRef), []string{chPanel(1, "SELECT 1 WHERE $filters"), chPanel(2, "SELECT 2"), chPanel(3, "SELECT 3")}, []string{"panels[1].targets[0]", "panels[2].targets[0]"}, "2 queries are"},
		{"every query uses the variable", adhoc(chRef), []string{chPanel(1, "SELECT 1 WHERE $filters"), chPanel(2, "SELECT 2 WHERE $filters")}, nil, ""},
		{"queries on other datasources are not counted", adhoc(chRef), []string{chPanel(1, "SELECT 1 WHERE $filters"), promPanel}, nil, ""},
		{"prometheus adhoc variables filter every query", adhoc(promRef), []string{promPanel, chPanel(1, "SELECT 1")}, nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := convertDoc(t, dashboardJSON(templating(tc.variable), tc.panels...))
			var got []Note
			for _, note := range notesFor(res, "variable filters") {
				if note.Reason != "Saved filters are not kept." {
					got = append(got, note)
				}
			}
			if tc.wantPaths == nil {
				if len(got) != 0 {
					t.Fatalf("notes = %+v, want none", got)
				}
				return
			}
			wantReason := reasonFor(tc.wantCount)
			if len(got) != 1 || got[0].Reason != wantReason || !slices.Equal(got[0].Paths, tc.wantPaths) {
				t.Fatalf("notes = %+v, want one %q with paths %v", got, wantReason, tc.wantPaths)
			}
		})
	}
}

const (
	resolutionCustom   = `{"name":"resolution","type":"custom","query":"1s,15s,30s,1m","current":{"value":"30s"}}`
	intervalDeclared   = `{"name":"iv","type":"interval","query":"1m,5m","current":{"value":"5m"}}`
	promotedNoteReason = "Imported as an interval variable because queries use it as their min step."
)

func minStepDoc(variables, panelMembers, targetMembers string) string {
	if panelMembers != "" {
		panelMembers += ","
	}
	return fmt.Sprintf(`{"title":"T","schemaVersion":39,%s,"panels":[{%s,"type":"timeseries",%s"targets":[{"refId":"A","expr":"up"%s}]}]}`,
		templating(variables), panelFrame, panelMembers, targetMembers)
}

func TestMinStepWrittenAsAnIntervalVariableImportsAsAReference(t *testing.T) {
	forms := []string{"$iv", "${iv}", "[[iv]]", "${iv:text}", "[[iv:raw]]", " ${iv} "}
	for _, form := range forms {
		t.Run(form, func(t *testing.T) {
			panel, line := convertedPanel(t, minStepDoc(intervalDeclared, "", `,"interval":"`+form+`"`))
			if got := panel.Queries[0].Interval; got != "${iv}" {
				t.Fatalf("query interval = %q, want ${iv}", got)
			}
			if line.Status != statusImported {
				t.Fatalf("status = %s, reasons %v, want imported", line.Status, line.Reasons)
			}
		})
	}
}

func TestPanelMinIntervalWrittenAsAnIntervalVariableImportsAsAReference(t *testing.T) {
	panel, line := convertedPanel(t, minStepDoc(intervalDeclared, `"interval":"$iv"`, ""))
	if panel.Interval != "${iv}" || line.Status != statusImported {
		t.Fatalf("panel interval = %q, status %s (%v), want ${iv} imported", panel.Interval, line.Status, line.Reasons)
	}
}

func TestBuiltInIntervalAsAMinStepImportsAsNoMinimum(t *testing.T) {
	for _, form := range []string{"$__interval", "${__interval}", "$__rate_interval", "${__rate_interval}"} {
		t.Run(form, func(t *testing.T) {
			panel, line := convertedPanel(t, minStepDoc("", `"interval":"`+form+`"`, `,"interval":"`+form+`"`))
			if panel.Interval != "" || panel.Queries[0].Interval != "" || line.Status != statusImported {
				t.Fatalf("panel interval %q, query interval %q, status %s (%v), want no minimum and imported", panel.Interval, panel.Queries[0].Interval, line.Status, line.Reasons)
			}
		})
	}
}

func TestMinStepNamingAnUnusableVariableKeepsTheOldReport(t *testing.T) {
	cases := []struct {
		name      string
		variables string
		interval  string
	}{
		{"undeclared", "", "$missing"},
		{"textbox variable", `{"name":"tb","type":"textbox","query":"30s"}`, "$tb"},
		{"not a name", "", "$iv + 1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := convertedPanel(t, minStepDoc(tc.variables, "", `,"interval":"`+tc.interval+`"`))
			if panel.Queries[0].Interval != "" || line.Status != statusPartial || !slices.Contains(line.Paths, "targets[0].interval") {
				t.Fatalf("interval %q, status %s, paths %v, want the min step reported", panel.Queries[0].Interval, line.Status, line.Paths)
			}
			want := fmt.Sprintf("The min step %s cannot be kept; SlashObserve picks the step.", tc.interval)
			if !slices.Contains(line.Reasons, want) {
				t.Fatalf("reasons = %v, want %q", line.Reasons, want)
			}
		})
	}
}

func TestCustomVariableUsedAsAMinStepImportsAsAnIntervalVariable(t *testing.T) {
	docs := map[string]string{
		"target":       minStepDoc(resolutionCustom, "", `,"interval":"$resolution"`),
		"panel":        minStepDoc(resolutionCustom, `"interval":"${resolution}"`, ""),
		"legacy style": minStepDoc(resolutionCustom, "", `,"interval":"[[resolution]]"`),
	}
	for name, doc := range docs {
		t.Run(name, func(t *testing.T) {
			res := convertDoc(t, doc)
			v := variableNamed(t, res, "resolution")
			if v.Type != spec.IntervalVariable || !slices.Equal(v.Options, []string{"1s", "15s", "30s", "1m"}) || v.Auto {
				t.Fatalf("variable = %+v, want an interval variable with the four options and no auto", v)
			}
			if v.Default == nil || !slices.Equal(v.Default.Values, []string{"30s"}) {
				t.Fatalf("default = %+v, want 30s", v.Default)
			}
			notes := notesFor(res, "variable resolution")
			if len(notes) != 1 || notes[0].Reason != promotedNoteReason {
				t.Fatalf("notes = %+v, want the promotion note once", notes)
			}
			if line := panelLine(t, res, "p1"); line.Status != statusImported {
				t.Fatalf("panel line = %+v, want imported", line)
			}
		})
	}
}

func TestCustomVariableNotUsedAsAMinStepStaysCustom(t *testing.T) {
	res := convertDoc(t, minStepDoc(resolutionCustom, "", `,"legendFormat":"{{pod}} $resolution"`))
	if v := variableNamed(t, res, "resolution"); v.Type != spec.CustomVariable {
		t.Fatalf("type = %q, want custom", v.Type)
	}
	if notes := notesFor(res, "variable resolution"); len(notes) != 0 {
		t.Fatalf("notes = %+v, want none", notes)
	}
}

func TestCustomVariableThatIsNotASingleListOfDurationsStaysCustom(t *testing.T) {
	cases := map[string]string{
		"multi-select":      `{"name":"resolution","type":"custom","query":"1s,15s","multi":true,"current":{"value":["1s"]}}`,
		"with All":          `{"name":"resolution","type":"custom","query":"1s,15s","includeAll":true,"current":{"value":"1s"}}`,
		"text : value form": `{"name":"resolution","type":"custom","query":"Fast : 1s,Slow : 15s","current":{"value":"1s"}}`,
		"not all durations": `{"name":"resolution","type":"custom","query":"1s,15s,auto","current":{"value":"1s"}}`,
	}
	for name, variable := range cases {
		t.Run(name, func(t *testing.T) {
			res := convertDoc(t, minStepDoc(variable, "", `,"interval":"$resolution"`))
			if v := variableNamed(t, res, "resolution"); v.Type != spec.CustomVariable {
				t.Fatalf("type = %q, want custom", v.Type)
			}
			if slices.ContainsFunc(res.Report.Notes, func(n Note) bool { return n.Reason == promotedNoteReason }) {
				t.Fatalf("notes = %+v, want no promotion note", res.Report.Notes)
			}
			line := panelLine(t, res, "p1")
			if line.Status != statusPartial || !slices.Contains(line.Paths, "targets[0].interval") {
				t.Fatalf("line = %+v, want a partial panel naming targets[0].interval", line)
			}
		})
	}
}

func TestCustomVariableUsedOnlyInsideACollapsedRowPromotes(t *testing.T) {
	child := `{"id":2,"type":"timeseries","title":"C","gridPos":{"x":0,"y":1,"w":12,"h":8},"datasource":{"type":"prometheus","uid":"prom"},"targets":[{"refId":"A","expr":"up","interval":"$resolution"}]}`
	doc := fmt.Sprintf(`{"title":"T","schemaVersion":39,%s,"panels":[%s]}`, templating(resolutionCustom), rowJSON(1, 0, true, `"panels":[`+child+`]`))
	res := convertDoc(t, doc)
	if v := variableNamed(t, res, "resolution"); v.Type != spec.IntervalVariable {
		t.Fatalf("type = %q, want interval", v.Type)
	}
}

func TestResolutionFactorImportsWhenItIsTwoToTen(t *testing.T) {
	cases := []struct {
		factor  string
		want    int
		partial bool
	}{
		{"1", 0, false},
		{"2", 2, false},
		{"5", 5, false},
		{"10", 10, false},
		{"11", 0, true},
		{"0", 0, true},
		{"2.5", 0, true},
		{"-2", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.factor, func(t *testing.T) {
			panel, line := convertedPanel(t, minStepDoc("", "", `,"intervalFactor":`+tc.factor))
			if got := panel.Queries[0].IntervalFactor; got != tc.want {
				t.Fatalf("intervalFactor = %d, want %d", got, tc.want)
			}
			reported := slices.Contains(line.Reasons, "The resolution factor of this query is not supported yet.")
			if reported != tc.partial || (line.Status == statusPartial) != tc.partial {
				t.Fatalf("status %s, reasons %v, want partial = %v", line.Status, line.Reasons, tc.partial)
			}
		})
	}
}
