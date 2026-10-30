package grafana

import (
	"encoding/json"
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

func TestAdhocSavedFiltersBecomeTheDefault(t *testing.T) {
	res := convertVariables(t,
		`{"name":"env","type":"adhoc","datasource":`+promRef+`,"filters":[{"key":"job","operator":"=","value":"api"},{"key":"pod","operator":"!~","value":"web-.*","condition":"AND"},{"key":"zone","operator":"!=","value":"","condition":""}]}`,
		`{"name":"plain","type":"adhoc","datasource":`+promRef+`,"filters":[]}`,
	)
	want := []spec.AdhocFilter{
		{Key: "job", Operator: "=", Value: "api"},
		{Key: "pod", Operator: "!~", Value: "web-.*"},
		{Key: "zone", Operator: "!=", Value: ""},
	}
	if got := variableNamed(t, res, "env").Default; got == nil || !slices.Equal(got.Filters, want) {
		t.Fatalf("env default = %+v, want %+v", got, want)
	}
	if got := variableNamed(t, res, "plain").Default; got != nil {
		t.Fatalf("plain default = %+v, want none", got)
	}
	for _, name := range []string{"env", "plain"} {
		if notes := notesFor(res, "variable "+name); len(notes) != 0 {
			t.Fatalf("%s notes = %+v, want none", name, notes)
		}
	}
}

func TestAdhocSavedFiltersThatCannotBeKeptAreReported(t *testing.T) {
	long := strings.Repeat("x", 501)
	filters := []string{
		`{"key":"a","operator":"<","value":"1"}`,
		`{"key":"b","operator":">","value":"1"}`,
		`{"key":"c","operator":"=|","value":"1"}`,
		`{"key":"Bad Key","operator":"=","value":"1"}`,
		`{"key":"d","operator":"=~","value":"a("}`,
		`{"key":"e","operator":"=","value":"` + long + `"}`,
		`{"key":"f","operator":"=","value":"1","condition":"OR"}`,
		`{"key":"g","operator":"=","value":"1","keyLabel":"G"}`,
		`{"key":"","operator":"=","value":"1"}`,
		`{"key":"h","operator":"="}`,
		`"text"`,
		`{"key":"ok","operator":"=","value":"fine"}`,
	}
	res := convertVariables(t, `{"name":"env","type":"adhoc","datasource":`+promRef+`,"filters":[`+strings.Join(filters, ",")+`],"baseFilters":[{"key":"c","operator":"=","value":"d"}]}`)
	var kept []string
	for _, f := range variableNamed(t, res, "env").Default.Filters {
		kept = append(kept, f.Key)
	}
	if want := []string{"f", "g", "ok"}; !slices.Equal(kept, want) {
		t.Fatalf("kept keys = %v, want %v", kept, want)
	}
	notes := notesFor(res, "variable env")
	reasonsWithPaths := map[string][]string{}
	for _, n := range notes {
		reasonsWithPaths[n.Reason] = n.Paths
	}
	for reason, paths := range map[string][]string{
		"SlashObserve supports the filter operators =, !=, =~ and !~; saved filters that use < are not kept.":                {"templating.list[0].filters[0]"},
		"SlashObserve supports the filter operators =, !=, =~ and !~; saved filters that use > are not kept.":                {"templating.list[0].filters[1]"},
		"SlashObserve supports the filter operators =, !=, =~ and !~; saved filters that use =| are not kept.":               {"templating.list[0].filters[2]"},
		"A saved filter whose key is not a label or column name of its datasource is not kept.":                              {"templating.list[0].filters[3]"},
		"A saved filter whose regular expression is not valid is not kept.":                                                  {"templating.list[0].filters[4]"},
		"A saved filter whose value is longer than 500 characters or 1024 bytes, or holds a control character, is not kept.": {"templating.list[0].filters[5]"},
		`SlashObserve combines saved filters with AND; "OR" is not kept.`:                                                    {"templating.list[0].filters[6].condition"},
		"Some options of the saved filters are not supported yet.":                                                           {"templating.list[0].filters[7].keyLabel"},
		"A saved filter without a key, an operator and a value is not kept.":                                                 {"templating.list[0].filters[8]", "templating.list[0].filters[9]"},
		"A saved filter is not an object and is not kept.":                                                                   {"templating.list[0].filters[10]"},
		"Base filters are not kept.": {"templating.list[0].baseFilters"},
	} {
		if got, ok := reasonsWithPaths[reason]; !ok || !slices.Equal(got, paths) {
			t.Errorf("note %q has paths %v (present %v), want %v", reason, got, ok, paths)
		}
	}
	if len(notes) != 11 {
		t.Errorf("env has %d notes, want 11: %+v", len(notes), notes)
	}
}

func TestAdhocSavedFiltersKeysFollowTheDatasource(t *testing.T) {
	const chRef = `{"uid":"ch","type":"grafana-clickhouse-datasource"}`
	res := convertVariables(t,
		`{"name":"prom","type":"adhoc","datasource":`+promRef+`,"filters":[{"key":"a.b","operator":"=","value":"1"}]}`,
		`{"name":"ch","type":"adhoc","datasource":`+chRef+`,"filters":[{"key":"a.b","operator":"=","value":"1"}]}`,
	)
	if got := variableNamed(t, res, "prom").Default; got != nil {
		t.Fatalf("prom default = %+v, want none for a key with a dot", got)
	}
	if got := variableNamed(t, res, "ch").Default; got == nil || len(got.Filters) != 1 {
		t.Fatalf("ch default = %+v, want the column key kept", got)
	}
}

func TestAdhocSavedFiltersAreCutAtTwenty(t *testing.T) {
	filters := make([]string, 22)
	for i := range filters {
		filters[i] = fmt.Sprintf(`{"key":"k%d","operator":"=","value":"v"}`, i)
	}
	res := convertVariables(t, `{"name":"env","type":"adhoc","datasource":`+promRef+`,"filters":[`+strings.Join(filters, ",")+`]}`)
	if got := variableNamed(t, res, "env").Default; got == nil || len(got.Filters) != 20 || got.Filters[19].Key != "k19" {
		t.Fatalf("env default = %+v, want the first 20 filters", got)
	}
	notes := notesFor(res, "variable env")
	if len(notes) != 1 || !hasPath(notes, "templating.list[0].filters[20]") || !hasPath(notes, "templating.list[0].filters[21]") {
		t.Fatalf("env notes = %+v, want one naming the filters past 20", notes)
	}
}

func TestVariableSortCodes(t *testing.T) {
	for code, want := range map[int]string{
		0: "", 1: "alpha", 2: "alphaDesc", 3: "num", 4: "numDesc",
		5: "alphaCaseInsensitive", 6: "alphaCaseInsensitiveDesc", 7: "natural", 8: "naturalDesc", 9: "",
	} {
		res := convertVariables(t, fmt.Sprintf(`{"name":"v","type":"query","datasource":%s,"query":"label_values(up, job)","sort":%d}`, promRef, code))
		if got := variableNamed(t, res, "v").Sort; got != want {
			t.Errorf("sort %d imports as %q, want %q", code, got, want)
		}
		if notes := notesFor(res, "variable v"); len(notes) != 0 {
			t.Errorf("sort %d notes = %+v, want none", code, notes)
		}
	}
}

func TestAllowCustomValueFollowsGrafanasDefault(t *testing.T) {
	for _, tc := range []struct {
		name, member string
		want         bool
	}{
		{"absent is allowed, as in Grafana", ``, true},
		{"true", `,"allowCustomValue":true`, true},
		{"false", `,"allowCustomValue":false`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := convertVariables(t,
				`{"name":"q","type":"query","datasource":`+promRef+`,"query":"label_values(up, job)"`+tc.member+`}`,
				`{"name":"c","type":"custom","query":"a,b"`+tc.member+`}`,
				`{"name":"f","type":"query","datasource":{"uid":"other","type":"loki"},"query":"x","options":[{"value":"a"}]`+tc.member+`}`,
			)
			for _, name := range []string{"q", "c", "f"} {
				if got := variableNamed(t, res, name).AllowCustomValue; got != tc.want {
					t.Errorf("%s allowCustomValue = %v, want %v", name, got, tc.want)
				}
				for _, n := range notesFor(res, "variable "+name) {
					if strings.Contains(strings.Join(n.Paths, ","), "allowCustomValue") {
						t.Errorf("%s reports allowCustomValue: %+v", name, n)
					}
				}
			}
		})
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
	if got := variableNamed(t, res, "c").Options; !slices.Equal(got, []spec.VariableOption{{Text: "a : b", Value: "c"}, {Text: "d", Value: "e"}, spec.PlainOption("plain")}) {
		t.Fatalf("options = %+v", got)
	}
	if notes := notesFor(res, "variable c"); len(notes) != 0 {
		t.Fatalf("notes = %+v, want none: the texts are kept", notes)
	}
}

func TestCustomPairsKeepTheirTextsAndSelectByValue(t *testing.T) {
	res := convertVariables(t, `{"name":"env","type":"custom","query":"Production : prod,Staging : stg,Other : prod,dev","current":{"value":"stg"}}`)
	v := variableNamed(t, res, "env")
	want := []spec.VariableOption{{Text: "Production", Value: "prod"}, {Text: "Staging", Value: "stg"}, spec.PlainOption("dev")}
	if !slices.Equal(v.Options, want) {
		t.Fatalf("options = %+v, want %+v (the first of two pairs with one value wins)", v.Options, want)
	}
	if v.Default == nil || !slices.Equal(v.Default.Values, []string{"stg"}) {
		t.Fatalf("default = %+v, want the value stg", v.Default)
	}
	data, err := json.Marshal(v)
	if err != nil || !strings.Contains(string(data), `{"text":"Production","value":"prod"}`) || !strings.Contains(string(data), `"dev"`) {
		t.Fatalf("variable JSON = %s (%v), want objects for pairs and a string for a plain option", data, err)
	}
}

func TestCustomPairsWithATextOverTwoHundredCharactersAreDropped(t *testing.T) {
	res := convertVariables(t, `{"name":"env","type":"custom","query":"`+strings.Repeat("x", 201)+` : a,Fine : b"}`)
	if got := variableNamed(t, res, "env").Options; !slices.Equal(got, []spec.VariableOption{{Text: "Fine", Value: "b"}}) {
		t.Fatalf("options = %+v", got)
	}
	if notes := notesFor(res, "variable env"); len(notes) != 1 {
		t.Fatalf("notes = %+v, want the dropped-values note", notes)
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

func TestClickHouseAdhocVariableAppliesToAllQueries(t *testing.T) {
	const (
		officialRef = `{"type":"grafana-clickhouse-datasource","uid":"ch"}`
		altinityRef = `{"type":"vertamedia-clickhouse-datasource","uid":"alt"}`
		altinityWhy = "Grafana's Altinity plugin applies these filters to the outermost query only; SlashObserve applies them on every table a query reads, subqueries included."
	)
	adhoc := func(datasource string) string {
		return `{"name":"filters","type":"adhoc","datasource":` + datasource + `}`
	}
	officialPanel := func(id int, sql string) string {
		return panelJSON(id, 0, id*4, 12, 4, `"type":"table","datasource":`+officialRef+`,"targets":[{"refId":"A","rawSql":"`+sql+`","format":1}]`)
	}
	altinityPanel := func(id int, sql string) string {
		return panelJSON(id, 0, id*4, 12, 4, `"type":"table","datasource":`+altinityRef+`,"targets":[{"refId":"A","query":"`+sql+`","format":"table"}]`)
	}
	promPanel := panelJSON(3, 0, 12, 12, 4, `"type":"stat","datasource":`+promRef+`,"targets":[{"refId":"A","expr":"up"}]`)

	cases := []struct {
		name      string
		variable  string
		panels    []string
		wantApply bool
		wantPaths []string
	}{
		{"official plugin, queries that do not name the variable", adhoc(officialRef), []string{officialPanel(1, "SELECT 1 WHERE $filters"), officialPanel(2, "SELECT 2")}, true, nil},
		{"official plugin, no query names the variable", adhoc(officialRef), []string{officialPanel(1, "SELECT 1")}, true, nil},
		{"altinity plugin lists the queries Grafana filters on the outermost level only", adhoc(altinityRef), []string{altinityPanel(1, "SELECT 1 WHERE $filters"), altinityPanel(2, "SELECT 2")}, true, []string{"panels[1].targets[0]"}},
		{"altinity plugin, every query names the variable", adhoc(altinityRef), []string{altinityPanel(1, "SELECT 1 WHERE $filters")}, true, nil},
		{"prometheus adhoc variables already filter every query", adhoc(promRef), []string{promPanel, officialPanel(1, "SELECT 1")}, false, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := convertDoc(t, dashboardJSON(templating(tc.variable), tc.panels...))
			if got := variableNamed(t, res, "filters").ApplyToAll; got != tc.wantApply {
				t.Fatalf("applyToAll = %v, want %v", got, tc.wantApply)
			}
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
			if len(got) != 1 || got[0].Reason != altinityWhy || !slices.Equal(got[0].Paths, tc.wantPaths) {
				t.Fatalf("notes = %+v, want one %q with paths %v", got, altinityWhy, tc.wantPaths)
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
			if v.Type != spec.IntervalVariable || !slices.Equal(v.Options, spec.PlainOptions("1s", "15s", "30s", "1m")) || v.Auto {
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

func TestVariableQueryNamingAnUndeclaredVariableSaysSo(t *testing.T) {
	account := `{"name":"account","type":"query","datasource":` + promRef + `,"query":"label_values(up, accountID)"}`
	project := `{"name":"project","type":"query","datasource":` + promRef + `,"query":"label_values(up{accountID=~\"$accountID\"}, projectID)"}`
	panel := `{"id":1,"type":"timeseries","title":"P","gridPos":{"x":0,"y":0,"w":12,"h":4},"datasource":` + promRef + `,"targets":[{"refId":"A","expr":"up{projectID=~\"$project\"}"}]}`
	res := convertDoc(t, dashboardJSON(templating(account, project), panel))

	if names := variableNames(res); !slices.Equal(names, []string{"account"}) {
		t.Fatalf("variables = %v, want only account", names)
	}
	want := "Its query uses $accountID, but this dashboard declares no variable with that name, and SlashObserve cannot keep a reference to an undeclared variable."
	notes := notesFor(res, "variable project")
	if len(notes) != 1 || notes[0].Reason != want || !hasPath(notes, "templating.list[1]") {
		t.Fatalf("notes = %+v, want one with %q", notes, want)
	}
	if line := panelLine(t, res, "p1"); line.Status != statusPlaceholder {
		t.Fatalf("panel using the variable = %+v, want a placeholder", line)
	}
}
