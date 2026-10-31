package grafana

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func groupByJSON(extra string) string {
	return `{"name":"by","type":"groupby","datasource":` + promRef + extra + `}`
}

func TestGroupByVariableImportsItsSavedLabelsAsTheDefault(t *testing.T) {
	cases := []struct {
		name  string
		extra string
		want  []string
	}{
		{"no selection", ``, nil},
		{"a single current value", `,"current":{"text":"job","value":"job"}`, []string{"job"}},
		{"a current list", `,"current":{"text":["job","pod"],"value":["job","pod"]}`, []string{"job", "pod"}},
		{"only a default value", `,"defaultValue":{"text":"job","value":"job"}`, []string{"job"}},
		{"duplicates and empty values go", `,"current":{"value":["job","","job","pod"]}`, []string{"job", "pod"}},
		{"an empty current falls back to the default value", `,"current":{"value":[]},"defaultValue":{"value":["pod"]}`, []string{"pod"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := convertVariables(t, groupByJSON(tc.extra))
			v := variableNamed(t, res, "by")
			if v.Type != spec.GroupByVariable || v.Datasource != "prom" {
				t.Fatalf("variable = %+v, want a group by variable on prom", v)
			}
			switch {
			case tc.want == nil && v.Default != nil:
				t.Fatalf("default = %+v, want none", v.Default)
			case tc.want != nil && (v.Default == nil || !slices.Equal(v.Default.Values, tc.want) || !v.Default.List):
				t.Fatalf("default = %+v, want the list %v", v.Default, tc.want)
			}
			if notes := notesFor(res, "variable by"); len(notes) != 0 {
				t.Fatalf("notes = %+v, want none", notes)
			}
		})
	}
}

func TestGroupByVariableReportsWhatItDoesNotKeep(t *testing.T) {
	cases := []struct {
		name     string
		extra    string
		wantPath string
		want     []string
	}{
		{"different current and default values", `,"current":{"value":"job"},"defaultValue":{"value":"pod"}`, "templating.list[0].defaultValue", []string{"job"}},
		{"a label that is not a label name", `,"current":{"value":["job","a-b"]}`, "templating.list[0].current", []string{"job"}},
		{"a saved option that is not a label name", `,"options":[{"value":"job"},{"value":"a-b"}]`, "templating.list[0].options[1]", nil},
		{"a single-select variable", `,"multi":false`, "templating.list[0].multi", nil},
		{"an include all option", `,"includeAll":true`, "templating.list[0].includeAll", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := convertVariables(t, groupByJSON(tc.extra))
			if !hasPath(notesFor(res, "variable by"), tc.wantPath) {
				t.Fatalf("notes = %+v, want one naming %s", notesFor(res, "variable by"), tc.wantPath)
			}
			if got := variableNamed(t, res, "by").Default; tc.want != nil && (got == nil || !slices.Equal(got.Values, tc.want)) {
				t.Fatalf("default = %+v, want %v", got, tc.want)
			}
		})
	}
}

func TestGroupByVariableCutsSavedLabelsAtTwenty(t *testing.T) {
	keys := make([]string, 22)
	for i := range keys {
		keys[i] = fmt.Sprintf("%q", fmt.Sprintf("k%d", i))
	}
	res := convertVariables(t, groupByJSON(`,"current":{"value":[`+strings.Join(keys, ",")+`]}`))
	if got := variableNamed(t, res, "by").Default; got == nil || len(got.Values) != spec.MaxGroupByKeys || got.Values[19] != "k19" {
		t.Fatalf("default = %+v, want the first 20 labels", got)
	}
	if !hasPath(notesFor(res, "variable by"), "templating.list[0].current") {
		t.Fatalf("notes = %+v, want one naming the current selection", notesFor(res, "variable by"))
	}
}

func TestGroupByVariableImportsItsSavedOptionsAsKeys(t *testing.T) {
	cases := []struct {
		name    string
		options string
		want    []string
		reason  string
		paths   []string
	}{
		{"objects", `[{"text":"job","value":"job"},{"text":"instance","value":"instance"}]`, []string{"job", "instance"}, "", nil},
		{"plain strings", `["job","pod"]`, []string{"job", "pod"}, "", nil},
		{"text stands in for a missing value", `[{"text":"job"}]`, []string{"job"}, "", nil},
		{"duplicates and empty entries go", `[{"value":"job"},{"value":"job"},{"value":""},{"text":""}]`, []string{"job"}, "", nil},
		{"an empty list writes nothing", `[]`, nil, "", nil},
		{"a text that differs from the value", `[{"text":"Job","value":"job"},{"text":"pod","value":"pod"},{"text":"Zone","value":"zone"}]`, []string{"job", "pod", "zone"}, "Key labels are shown as the key itself.", []string{"templating.list[0].options[0]", "templating.list[0].options[2]"}},
		{"an invalid label name", `[{"value":"job"},{"value":"a-b"},{"value":"9x"}]`, []string{"job"}, "A saved label is not a valid label name, so it is not kept.", []string{"templating.list[0].options[1]", "templating.list[0].options[2]"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := convertVariables(t, groupByJSON(`,"options":`+tc.options))
			if got := variableNamed(t, res, "by").Keys; !slices.Equal(got, tc.want) {
				t.Fatalf("keys = %v, want %v", got, tc.want)
			}
			notes := notesFor(res, "variable by")
			if tc.reason == "" {
				if len(notes) != 0 {
					t.Fatalf("notes = %+v, want none", notes)
				}
				return
			}
			if len(notes) != 1 || notes[0].Reason != tc.reason || !slices.Equal(notes[0].Paths, tc.paths) {
				t.Fatalf("notes = %+v, want one %q at %v", notes, tc.reason, tc.paths)
			}
		})
	}
}

func TestGroupByVariableCutsSavedOptionsAtOneHundred(t *testing.T) {
	options := make([]string, 102)
	for i := range options {
		options[i] = fmt.Sprintf(`{"value":"k%d"}`, i)
	}
	res := convertVariables(t, groupByJSON(`,"options":[`+strings.Join(options, ",")+`]`))
	keys := variableNamed(t, res, "by").Keys
	if len(keys) != 100 || keys[99] != "k99" {
		t.Fatalf("keys = %d long, want the first 100", len(keys))
	}
	if !hasPath(notesFor(res, "variable by"), "templating.list[0].options") {
		t.Fatalf("notes = %+v, want one naming the options", notesFor(res, "variable by"))
	}
}

func TestGroupByVariableFollowsADatasourceVariable(t *testing.T) {
	res := convertVariables(t,
		`{"name":"ds","type":"datasource","query":"prometheus"}`,
		`{"name":"by","type":"groupby","datasource":{"type":"prometheus","uid":"${ds}"},"current":{"value":"job"}}`,
	)
	if got := variableNamed(t, res, "by"); got.Type != spec.GroupByVariable || got.Datasource != "${ds}" {
		t.Fatalf("variable = %+v, want a group by variable on ${ds}", got)
	}
}

func TestGroupByVariableOnAnotherDatasourceIsDroppedAndReported(t *testing.T) {
	const chRef = `{"uid":"ch","type":"grafana-clickhouse-datasource"}`
	cases := []struct {
		name      string
		variables []string
		reason    string
	}{
		{"clickhouse", []string{`{"name":"by","type":"groupby","datasource":` + chRef + `}`}, decisionGroupByClickHouse.reason},
		{"a clickhouse datasource variable", []string{`{"name":"ds","type":"datasource","query":"grafana-clickhouse-datasource"}`, `{"name":"by","type":"groupby","datasource":{"uid":"${ds}"}}`}, "Group-by variables on this datasource are not supported."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := convertVariables(t, tc.variables...)
			if slices.ContainsFunc(res.Spec.Variables, func(v spec.Variable) bool { return v.Name == "by" }) {
				t.Fatalf("variables = %+v, want the group by variable dropped", res.Spec.Variables)
			}
			notes := notesFor(res, "variable by")
			if len(notes) != 1 || notes[0].Reason != tc.reason || !hasPath(notes, "templating.list["+fmt.Sprint(len(tc.variables)-1)+"].datasource") {
				t.Fatalf("notes = %+v, want the reason %q at the datasource", notes, tc.reason)
			}
		})
	}
}

func TestAdhocVariableWithGroupByImportsAGroupByVariableBesideIt(t *testing.T) {
	res := convertVariables(t, `{"name":"filters","type":"adhoc","datasource":`+promRef+`,"enableGroupBy":true,"hide":2}`)
	if got, want := variableNames(res), []string{"filters", "filters_groupby"}; !slices.Equal(got, want) {
		t.Fatalf("variables = %v, want %v", got, want)
	}
	adhoc, groupBy := variableNamed(t, res, "filters"), variableNamed(t, res, "filters_groupby")
	if adhoc.Type != spec.AdhocVariable || groupBy.Type != spec.GroupByVariable || groupBy.Datasource != adhoc.Datasource || groupBy.Hide != "variable" {
		t.Fatalf("variables = %+v and %+v, want an adhoc variable and a group by variable on its datasource with its hide", adhoc, groupBy)
	}
	if notes := notesFor(res, "variable filters"); len(notes) != 0 {
		t.Fatalf("notes = %+v, want none", notes)
	}
}

func TestAdhocGroupByVariableKeepsSkipUrlSyncAndKeys(t *testing.T) {
	res := convertVariables(t, `{"name":"filters","type":"adhoc","datasource":`+promRef+`,"enableGroupBy":true,"skipUrlSync":true,"defaultKeys":[{"text":"job","value":"job"},{"text":"pod","value":"pod"}]}`)
	adhoc, groupBy := variableNamed(t, res, "filters"), variableNamed(t, res, "filters_groupby")
	if !groupBy.SkipURLSync || !slices.Equal(groupBy.Keys, []string{"job", "pod"}) {
		t.Fatalf("companion = %+v, want skipUrlSync and the keys job and pod", groupBy)
	}
	if !adhoc.SkipURLSync || !slices.Equal(adhoc.Keys, []string{"job", "pod"}) {
		t.Fatalf("adhoc = %+v, want skipUrlSync and the keys job and pod", adhoc)
	}
}

func TestAdhocGroupByVariableWithoutSkipUrlSyncOrKeysWritesNeither(t *testing.T) {
	res := convertVariables(t, `{"name":"filters","type":"adhoc","datasource":`+promRef+`,"enableGroupBy":true}`)
	if groupBy := variableNamed(t, res, "filters_groupby"); groupBy.SkipURLSync || groupBy.Keys != nil {
		t.Fatalf("companion = %+v, want neither skipUrlSync nor keys", groupBy)
	}
}

func TestAdhocVariableWithoutGroupByImportsAlone(t *testing.T) {
	res := convertVariables(t, `{"name":"filters","type":"adhoc","datasource":`+promRef+`,"enableGroupBy":false}`)
	if got := variableNames(res); !slices.Equal(got, []string{"filters"}) {
		t.Fatalf("variables = %v, want only the adhoc variable", got)
	}
	if notes := notesFor(res, "variable filters"); len(notes) != 0 {
		t.Fatalf("notes = %+v, want none", notes)
	}
}

func TestAdhocGroupByVariableNameAvoidsDeclaredNames(t *testing.T) {
	long := strings.Repeat("a", 40)
	res := convertVariables(t,
		`{"name":"filters","type":"adhoc","datasource":`+promRef+`,"enableGroupBy":true}`,
		`{"name":"filters_groupby","type":"custom","query":"a,b"}`,
		`{"name":"`+long+`","type":"adhoc","datasource":`+promRef+`,"enableGroupBy":true}`,
	)
	got := variableNames(res)
	if !slices.Contains(got, "filters_groupby2") || !slices.Contains(got, strings.Repeat("a", 32)+"_groupby") {
		t.Fatalf("variables = %v, want filters_groupby2 and a name cut to 40 characters", got)
	}
	if len(got) != 5 {
		t.Fatalf("variables = %v, want five distinct variables", got)
	}
	encoded, err := json.Marshal(res.Spec)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := spec.Parse(encoded); err != nil {
		t.Fatalf("the Spec does not parse: %v", err)
	}
}

func TestAdhocGroupByOnAnotherDatasourceIsReported(t *testing.T) {
	const chRef = `{"uid":"ch","type":"grafana-clickhouse-datasource"}`
	res := convertVariables(t, `{"name":"filters","type":"adhoc","datasource":`+chRef+`,"enableGroupBy":true}`)
	if got := variableNames(res); !slices.Equal(got, []string{"filters"}) {
		t.Fatalf("variables = %v, want only the adhoc variable", got)
	}
	notes := notesFor(res, "variable filters")
	if len(notes) != 1 || notes[0].Reason != "Group-by on this datasource is not supported." || !hasPath(notes, "templating.list[0].enableGroupBy") {
		t.Fatalf("notes = %+v, want the reason at enableGroupBy", notes)
	}
}

func TestAdhocGroupByPastTheVariableLimitIsReported(t *testing.T) {
	variables := make([]string, 0, 20)
	for i := range 19 {
		variables = append(variables, fmt.Sprintf(`{"name":"c%d","type":"constant","query":"x"}`, i))
	}
	variables = append(variables, `{"name":"filters","type":"adhoc","datasource":`+promRef+`,"enableGroupBy":true}`)
	res := convertVariables(t, variables...)
	if len(res.Spec.Variables) != 20 {
		t.Fatalf("%d variables, want 20", len(res.Spec.Variables))
	}
	if notes := notesFor(res, "variable filters"); len(notes) != 1 || !hasPath(notes, "templating.list[19].enableGroupBy") {
		t.Fatalf("notes = %+v, want one at enableGroupBy", notes)
	}
}
