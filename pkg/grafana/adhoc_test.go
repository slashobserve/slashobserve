package grafana

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const chRef = `{"uid":"ch","type":"grafana-clickhouse-datasource"}`

func adhocWith(ref, filters string) string {
	return `{"name":"v","type":"adhoc","datasource":` + ref + `,"filters":[` + filters + `]}`
}

func TestAdhocSavedFilterVariantsConvert(t *testing.T) {
	tests := []struct {
		name    string
		ref     string
		filters string
		want    []spec.AdhocFilter
	}{
		{"less than on ClickHouse", chRef, `{"key":"n","operator":"<","value":"5"}`, []spec.AdhocFilter{{Key: "n", Operator: "<", Value: "5"}}},
		{"greater than on ClickHouse", chRef, `{"key":"n","operator":">","value":"5"}`, []spec.AdhocFilter{{Key: "n", Operator: ">", Value: "5"}}},
		{"OR on ClickHouse", chRef, `{"key":"a","operator":"=","value":"1"},{"key":"b","operator":"=","value":"2","condition":"OR"}`, []spec.AdhocFilter{
			{Key: "a", Operator: "=", Value: "1"},
			{Key: "b", Operator: "=", Value: "2", Condition: "or"},
		}},
		{"or in any case on ClickHouse", chRef, `{"key":"a","operator":"=","value":"1","condition":"or"},{"key":"b","operator":"=","value":"2","condition":"Or"}`, []spec.AdhocFilter{
			{Key: "a", Operator: "=", Value: "1", Condition: "or"},
			{Key: "b", Operator: "=", Value: "2", Condition: "or"},
		}},
		{"AND writes nothing", chRef, `{"key":"a","operator":"=","value":"1","condition":"AND"},{"key":"b","operator":"=","value":"2","condition":"and"}`, []spec.AdhocFilter{
			{Key: "a", Operator: "=", Value: "1"},
			{Key: "b", Operator: "=", Value: "2"},
		}},
		{"a key label and the first value label", promRef, `{"key":"job","operator":"=","value":"api","keyLabel":"Job","valueLabels":["API"]}`, []spec.AdhocFilter{{Key: "job", Operator: "=", Value: "api", KeyLabel: "Job", ValueLabel: "API"}}},
		{"a values list equal to the regular expression", promRef, `{"key":"job","operator":"=~","value":"api|web","values":["api","web"]}`, []spec.AdhocFilter{{Key: "job", Operator: "=~", Value: "api|web"}}},
		{"a values list equal to the negated regular expression", promRef, `{"key":"job","operator":"!~","value":"api|web","values":["api","web"]}`, []spec.AdhocFilter{{Key: "job", Operator: "!~", Value: "api|web"}}},
		{"a single value in the list", promRef, `{"key":"job","operator":"=~","value":"api","values":["api"]}`, []spec.AdhocFilter{{Key: "job", Operator: "=~", Value: "api"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := convertVariables(t, adhocWith(tt.ref, tt.filters))
			got := variableNamed(t, res, "v").Default
			if got == nil || !slices.Equal(got.Filters, tt.want) {
				t.Fatalf("default = %+v, want %+v", got, tt.want)
			}
			if notes := notesFor(res, "variable v"); len(notes) != 0 {
				t.Fatalf("notes = %+v, want none", notes)
			}
		})
	}
}

func TestAdhocSavedFilterVariantsThatCannotBeKeptAreReported(t *testing.T) {
	tests := []struct {
		name     string
		ref      string
		filters  string
		want     []spec.AdhocFilter
		reason   string
		wantPath string
	}{
		{
			"less than on Prometheus is dropped", promRef, `{"key":"n","operator":"<","value":"5"},{"key":"ok","operator":"=","value":"1"}`,
			[]spec.AdhocFilter{{Key: "ok", Operator: "=", Value: "1"}},
			"SlashObserve supports the filter operators =, !=, =~ and !~; saved filters that use < are not kept.", "templating.list[0].filters[0]",
		},
		{
			"greater than on Prometheus is dropped", promRef, `{"key":"n","operator":">","value":"5"}`,
			nil,
			"SlashObserve supports the filter operators =, !=, =~ and !~; saved filters that use > are not kept.", "templating.list[0].filters[0]",
		},
		{
			"OR on Prometheus is reported and the filter is kept as AND", promRef, `{"key":"a","operator":"=","value":"1"},{"key":"b","operator":"=","value":"2","condition":"OR"}`,
			[]spec.AdhocFilter{{Key: "a", Operator: "=", Value: "1"}, {Key: "b", Operator: "=", Value: "2"}},
			`SlashObserve combines saved filters with AND; "OR" is not kept.`, "templating.list[0].filters[1].condition",
		},
		{
			"another condition on ClickHouse", chRef, `{"key":"a","operator":"=","value":"1","condition":"XOR"}`,
			[]spec.AdhocFilter{{Key: "a", Operator: "=", Value: "1"}},
			`SlashObserve combines saved filters with AND; "XOR" is not kept.`, "templating.list[0].filters[0].condition",
		},
		{
			"an unknown operator on ClickHouse names the six it supports", chRef, `{"key":"a","operator":"=|","value":"1"}`,
			nil,
			"SlashObserve supports the filter operators =, !=, =~, !~, < and > on ClickHouse; saved filters that use =| are not kept.", "templating.list[0].filters[0]",
		},
		{
			"values that differ from the value", promRef, `{"key":"job","operator":"=~","value":"api|db","values":["api","web"]}`,
			[]spec.AdhocFilter{{Key: "job", Operator: "=~", Value: "api|db"}},
			"A saved filter's list of values that is not its regular expression's alternatives joined with | is not kept.", "templating.list[0].filters[0].values",
		},
		{
			"values on an operator that is not a regular expression", promRef, `{"key":"job","operator":"=","value":"api|web","values":["api","web"]}`,
			[]spec.AdhocFilter{{Key: "job", Operator: "=", Value: "api|web"}},
			"A saved filter's list of values that is not its regular expression's alternatives joined with | is not kept.", "templating.list[0].filters[0].values",
		},
		{
			"values that are not text", promRef, `{"key":"job","operator":"=~","value":"1","values":[1]}`,
			[]spec.AdhocFilter{{Key: "job", Operator: "=~", Value: "1"}},
			"A saved filter's list of values that is not its regular expression's alternatives joined with | is not kept.", "templating.list[0].filters[0].values",
		},
		{
			"a second value label", promRef, `{"key":"job","operator":"=~","value":"api|web","values":["api","web"],"valueLabels":["API","Web"]}`,
			[]spec.AdhocFilter{{Key: "job", Operator: "=~", Value: "api|web", ValueLabel: "API"}},
			"Only the first value label of a saved filter is kept.", "templating.list[0].filters[0].valueLabels",
		},
		{
			"a key label over 200 characters", promRef, `{"key":"job","operator":"=","value":"api","keyLabel":"` + strings.Repeat("k", 201) + `"}`,
			[]spec.AdhocFilter{{Key: "job", Operator: "=", Value: "api"}},
			"A saved filter's key label that is not text of 1 to 200 characters is not kept.", "templating.list[0].filters[0].keyLabel",
		},
		{
			"a value label over 500 characters", promRef, `{"key":"job","operator":"=","value":"api","valueLabels":["` + strings.Repeat("v", 501) + `"]}`,
			[]spec.AdhocFilter{{Key: "job", Operator: "=", Value: "api"}},
			"A saved filter's value label that is not text of 1 to 500 characters is not kept.", "templating.list[0].filters[0].valueLabels",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := convertVariables(t, adhocWith(tt.ref, tt.filters))
			var got []spec.AdhocFilter
			if d := variableNamed(t, res, "v").Default; d != nil {
				got = d.Filters
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("default = %+v, want %+v", got, tt.want)
			}
			notes := notesFor(res, "variable v")
			if len(notes) != 1 || notes[0].Reason != tt.reason || !slices.Equal(notes[0].Paths, []string{tt.wantPath}) {
				t.Fatalf("notes = %+v, want one %q at %s", notes, tt.reason, tt.wantPath)
			}
		})
	}
}

func TestAdhocBaseFiltersConvertLikeSavedFilters(t *testing.T) {
	tests := []struct {
		name     string
		ref      string
		base     string
		want     []spec.AdhocFilter
		reason   string
		wantPath string
	}{
		{"equals", promRef, `{"key":"env","operator":"=","value":"prod"}`, []spec.AdhocFilter{{Key: "env", Operator: "=", Value: "prod"}}, "", ""},
		{"labels and OR on ClickHouse", chRef, `{"key":"a","operator":"=","value":"1","keyLabel":"A"},{"key":"b","operator":">","value":"2","condition":"OR","valueLabels":["Two"]}`, []spec.AdhocFilter{
			{Key: "a", Operator: "=", Value: "1", KeyLabel: "A"},
			{Key: "b", Operator: ">", Value: "2", Condition: "or", ValueLabel: "Two"},
		}, "", ""},
		{"an operator Prometheus cannot keep", promRef, `{"key":"env","operator":"<","value":"5"}`, nil,
			"SlashObserve supports the filter operators =, !=, =~ and !~; saved filters that use < are not kept.", "templating.list[0].baseFilters[0]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := convertVariables(t, `{"name":"v","type":"adhoc","datasource":`+tt.ref+`,"baseFilters":[`+tt.base+`]}`)
			v := variableNamed(t, res, "v")
			if !slices.Equal(v.BaseFilters, tt.want) {
				t.Fatalf("baseFilters = %+v, want %+v", v.BaseFilters, tt.want)
			}
			if v.Default != nil {
				t.Fatalf("default = %+v, want none", v.Default)
			}
			notes := notesFor(res, "variable v")
			if tt.reason == "" {
				if len(notes) != 0 {
					t.Fatalf("notes = %+v, want none", notes)
				}
				return
			}
			if len(notes) != 1 || notes[0].Reason != tt.reason || !slices.Equal(notes[0].Paths, []string{tt.wantPath}) {
				t.Fatalf("notes = %+v, want one %q at %s", notes, tt.reason, tt.wantPath)
			}
		})
	}
}

func TestAdhocDefaultKeysAndAllowCustomValueConvert(t *testing.T) {
	tests := []struct {
		name       string
		ref        string
		extra      string
		wantKeys   []string
		wantListed bool
		reason     string
		wantPaths  []string
	}{
		{"objects", promRef, `,"defaultKeys":[{"text":"job","value":"job"},{"text":"pod","value":"pod"}]`, []string{"job", "pod"}, false, "", nil},
		{"strings", promRef, `,"defaultKeys":["job","pod"]`, []string{"job", "pod"}, false, "", nil},
		{"duplicates go", promRef, `,"defaultKeys":["job","job"]`, []string{"job"}, false, "", nil},
		{"an empty list writes nothing", promRef, `,"defaultKeys":[]`, nil, false, "", nil},
		{"a text that differs from the value", promRef, `,"defaultKeys":[{"text":"Job","value":"job"}]`, []string{"job"}, false,
			"Key labels are shown as the key itself.", []string{"templating.list[0].defaultKeys[0]"}},
		{"a key that is not a label name", promRef, `,"defaultKeys":[{"value":"job"},{"value":"a-b"}]`, []string{"job"}, false,
			"A saved key that is not a label or column name of its datasource is not kept.", []string{"templating.list[0].defaultKeys[1]"}},
		{"keys on ClickHouse", chRef, `,"defaultKeys":[{"value":"ServiceName"}]`, []string{"ServiceName"}, false, "", nil},
		{"no custom values on Prometheus", promRef, `,"allowCustomValue":false`, nil, true, "", nil},
		{"custom values allowed", promRef, `,"allowCustomValue":true`, nil, false, "", nil},
		{"no custom values on ClickHouse", chRef, `,"allowCustomValue":false`, nil, false,
			"Only Prometheus ad hoc variables can limit values to the listed ones.", []string{"templating.list[0].allowCustomValue"}},
		{"custom values allowed on ClickHouse", chRef, `,"allowCustomValue":true`, nil, false, "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := convertVariables(t, `{"name":"v","type":"adhoc","datasource":`+tt.ref+tt.extra+`}`)
			v := variableNamed(t, res, "v")
			if !slices.Equal(v.Keys, tt.wantKeys) || v.ListedValuesOnly != tt.wantListed {
				t.Fatalf("keys = %v, listedValuesOnly = %v, want %v and %v", v.Keys, v.ListedValuesOnly, tt.wantKeys, tt.wantListed)
			}
			notes := notesFor(res, "variable v")
			if tt.reason == "" {
				if len(notes) != 0 {
					t.Fatalf("notes = %+v, want none", notes)
				}
				return
			}
			if len(notes) != 1 || notes[0].Reason != tt.reason || !slices.Equal(notes[0].Paths, tt.wantPaths) {
				t.Fatalf("notes = %+v, want one %q at %v", notes, tt.reason, tt.wantPaths)
			}
		})
	}
}

func TestAdhocDefaultKeysAreCutAtOneHundred(t *testing.T) {
	keys := make([]string, 101)
	for i := range keys {
		keys[i] = fmt.Sprintf(`"k%d"`, i)
	}
	res := convertVariables(t, `{"name":"v","type":"adhoc","datasource":`+promRef+`,"defaultKeys":[`+strings.Join(keys, ",")+`]}`)
	if got := variableNamed(t, res, "v").Keys; len(got) != 100 || got[99] != "k99" {
		t.Fatalf("keys = %d long, want the first 100", len(got))
	}
	if !hasPath(notesFor(res, "variable v"), "templating.list[0].defaultKeys") {
		t.Fatalf("notes = %+v, want one naming the keys", notesFor(res, "variable v"))
	}
}
