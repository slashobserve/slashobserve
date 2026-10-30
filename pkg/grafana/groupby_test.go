package grafana

import (
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
		{"saved label options", `,"options":[{"text":"job","value":"job"}]`, "templating.list[0].options", nil},
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
		{"clickhouse", []string{`{"name":"by","type":"groupby","datasource":` + chRef + `}`}, "Group-by variables on this datasource are not supported."},
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
