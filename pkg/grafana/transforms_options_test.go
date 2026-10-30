package grafana

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestTransformOptionsConvert(t *testing.T) {
	cases := []struct {
		name, transformation, want string
	}{
		{
			"organize include",
			`{"id":"organize","options":{"includeByName":{"Value":true,"Time":false,"Other":true}}}`,
			`[{"type":"organize","include":["Other","Value"]}]`,
		},
		{
			"rename",
			`{"id":"rename","options":{"renameByName":{"Value":"Latency"}}}`,
			`[{"type":"organize","rename":{"Value":"Latency"}}]`,
		},
		{
			"order",
			`{"id":"order","options":{"indexByName":{"Time":1,"Value":0}}}`,
			`[{"type":"organize","order":["Value","Time"]}]`,
		},
		{
			"filterFields names and pattern",
			`{"id":"filterFields","options":{"include":{"id":"byNames","options":{"names":["Value"]}},"exclude":{"id":"byRegexp","options":"/^Ti/"}}}`,
			`[{"type":"filterFieldsByName","include":["Value"],"excludeRegex":"/^Ti/"}]`,
		},
		{
			"filterFields single name",
			`{"id":"filterFields","options":{"exclude":{"id":"byName","options":"Time"}}}`,
			`[{"type":"filterFieldsByName","exclude":["Time"]}]`,
		},
		{
			"reduce time, labels and fields",
			`{"id":"reduce","options":{"reducers":["max"],"includeTimeField":true,"labelsToFields":true,"fields":{"id":"byNames","options":{"names":["a","b"]}}}}`,
			`[{"type":"reduce","reducers":["max"],"fields":["a","b"],"includeTime":true,"labelsToFields":true}]`,
		},
		{
			"reduce fields by regexp",
			`{"id":"reduce","options":{"reducers":["max"],"fields":{"id":"byRegexp","options":"/^cpu/"}}}`,
			`[{"type":"reduce","reducers":["max"],"fieldsRegex":"/^cpu/"}]`,
		},
		{
			"outer tabular join",
			`{"id":"joinByField","options":{"mode":"outerTabular","byField":"host"}}`,
			`[{"type":"joinByField","mode":"outerTabular","field":"host"}]`,
		},
		{
			"filter by name excludes",
			`{"id":"filterFieldsByName","options":{"include":{"names":["Value"]},"exclude":{"names":["Time"],"pattern":"/^t/i"}}}`,
			`[{"type":"filterFieldsByName","exclude":["Time"],"include":["Value"],"excludeRegex":"/^t/i"}]`,
		},
		{
			"filter by name from a variable",
			`{"id":"filterFieldsByName","options":{"byVariable":true,"include":{"variable":"$fields"}}}`,
			`[{"type":"filterFieldsByName","variable":"fields"}]`,
		},
		{
			"filter by name from a braced variable",
			`{"id":"filterFieldsByName","options":{"byVariable":true,"include":{"variable":"${fields}"}}}`,
			`[{"type":"filterFieldsByName","variable":"fields"}]`,
		},
		{
			"unary",
			`{"id":"calculateField","options":{"mode":"unary","unary":{"operator":"ln","fieldName":"Value"}}}`,
			`[{"type":"calculateField","alias":"ln(Value)","unary":{"fn":"ln","field":"Value"}}]`,
		},
		{
			"unary defaults",
			`{"id":"calculateField","options":{"mode":"unary"}}`,
			`[{"type":"calculateField","alias":"abs","unary":{}}]`,
		},
		{
			"index as percentile",
			`{"id":"calculateField","options":{"mode":"index","index":{"asPercentile":true},"alias":"Place"}}`,
			`[{"type":"calculateField","alias":"Place","index":{"percentile":true}}]`,
		},
		{
			"cumulative mean",
			`{"id":"calculateField","options":{"mode":"cumulativeFunctions","cumulative":{"reducer":"mean","field":"Value"}}}`,
			`[{"type":"calculateField","alias":"Cumulative mean","cumulative":{"fn":"mean","field":"Value"}}]`,
		},
		{
			"cumulative defaults",
			`{"id":"calculateField","options":{"mode":"cumulativeFunctions"}}`,
			`[{"type":"calculateField","alias":"Cumulative total","cumulative":{}}]`,
		},
		{
			"fixed centered window",
			`{"id":"calculateField","options":{"mode":"windowFunctions","window":{"reducer":"stdDev","windowSize":5,"windowSizeMode":"fixed","windowAlignment":"centered"}}}`,
			`[{"type":"calculateField","alias":"Centered stdDev","window":{"fn":"stdDev","size":5,"align":"centered"}}]`,
		},
		{
			"window share",
			`{"id":"calculateField","options":{"mode":"windowFunctions","window":{"windowSize":0.25,"windowSizeMode":"percentage"}}}`,
			`[{"type":"calculateField","alias":"Trailing mean","window":{"share":0.25}}]`,
		},
		{
			"window share equal to the default",
			`{"id":"calculateField","options":{"mode":"windowFunctions","window":{"windowSize":0.1}}}`,
			`[{"type":"calculateField","alias":"Trailing mean","window":{}}]`,
		},
		{
			"window size without a mode",
			`{"id":"calculateField","options":{"mode":"windowFunctions","window":{"windowSize":10}}}`,
			`[{"type":"calculateField","alias":"Trailing mean","window":{"size":10}}]`,
		},
		{
			"binary modulo and power",
			`{"id":"calculateField","options":{"mode":"binary","binary":{"left":"a","operator":"%","right":"b"}}}`,
			`[{"type":"calculateField","alias":"a % b","expr":"a % b"}]`,
		},
		{
			"replace fields",
			`{"id":"calculateField","options":{"mode":"reduceRow","reduce":{"reducer":"max"},"replaceFields":true}}`,
			`[{"type":"calculateField","alias":"Max","reduceRow":"max","replace":true}]`,
		},
		{
			"sort by several fields",
			`{"id":"sortBy","options":{"sort":[{"field":"Value","desc":true},{"field":"Time"},{"field":"host","desc":true}]}}`,
			`[{"type":"sortBy","field":"Value","desc":true,"then":[{"field":"Time"},{"field":"host","desc":true}]}]`,
		},
		{
			"limit from a variable",
			`{"id":"limit","options":{"limitField":"$limit"}}`,
			`[{"type":"limit","count":"${limit}"}]`,
		},
		{
			"limit from a braced variable",
			`{"id":"limit","options":{"limitField":"${limit}"}}`,
			`[{"type":"limit","count":"${limit}"}]`,
		},
		{
			"limit of a number in a string",
			`{"id":"limit","options":{"limitField":"7"}}`,
			`[{"type":"limit","count":7}]`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := transformPanel(t, "", tc.transformation)
			if line.Status != statusImported {
				t.Fatalf("status %s, paths %v", line.Status, line.Paths)
			}
			wantJSON(t, panel.Transform, tc.want)
		})
	}
}

func TestTransformOptionsReport(t *testing.T) {
	cases := []struct {
		name, transformation, path string
		kept                       int
	}{
		{"percent unary", `{"id":"calculateField","options":{"mode":"unary","unary":{"operator":"percent"}}}`, "transformations[0].options.unary.operator", 0},
		{"window max", `{"id":"calculateField","options":{"mode":"windowFunctions","window":{"reducer":"max"}}}`, "transformations[0].options.window.reducer", 0},
		{"fixed window without a size", `{"id":"calculateField","options":{"mode":"windowFunctions","window":{"windowSizeMode":"fixed"}}}`, "transformations[0].options.window.windowSizeMode", 0},
		{"fractional fixed window", `{"id":"calculateField","options":{"mode":"windowFunctions","window":{"windowSizeMode":"fixed","windowSize":2.5}}}`, "transformations[0].options.window.windowSize", 0},
		{"percentage above one", `{"id":"calculateField","options":{"mode":"windowFunctions","window":{"windowSizeMode":"percentage","windowSize":4}}}`, "transformations[0].options.window.windowSize", 0},
		{"cumulative max", `{"id":"calculateField","options":{"mode":"cumulativeFunctions","cumulative":{"reducer":"max"}}}`, "transformations[0].options.cumulative.reducer", 0},
		{"other window key", `{"id":"calculateField","options":{"mode":"windowFunctions","window":{"extra":1}}}`, "transformations[0].options.window.extra", 1},
		{"reduce fields as plain text", `{"id":"reduce","options":{"reducers":["max"],"fields":"Value"}}`, "transformations[0].options.fields", 1},
		{"reduce fields by an unknown matcher", `{"id":"reduce","options":{"reducers":["max"],"fields":{"id":"byType","options":"number"}}}`, "transformations[0].options.fields", 1},
		{"variable without byVariable", `{"id":"filterFieldsByName","options":{"include":{"names":["a"],"variable":"$fields"}}}`, "transformations[0].options.include.variable", 1},
		{"byVariable without a variable", `{"id":"filterFieldsByName","options":{"byVariable":true}}`, "transformations[0].options.byVariable", 0},
		{"global as a variable", `{"id":"filterFieldsByName","options":{"byVariable":true,"include":{"variable":"$__interval"}}}`, "transformations[0].options.byVariable", 0},
		{"exclude variable", `{"id":"filterFieldsByName","options":{"include":{"names":["a"]},"exclude":{"names":["b"],"variable":"x"}}}`, "transformations[0].options.exclude.variable", 1},
		{"filterFields matcher of another kind", `{"id":"filterFields","options":{"include":{"id":"byType","options":"number"}}}`, "transformations[0].options.include", 0},
		{"limit from a global", `{"id":"limit","options":{"limitField":"$__interval"}}`, "transformations[0].options.limitField", 0},
		{"second sort field without a name", `{"id":"sortBy","options":{"sort":[{"field":"Value"},{"desc":true}]}}`, "transformations[0].options.sort[1]", 1},
		{"orderBy", `{"id":"order","options":{"indexByName":{"a":0},"orderBy":[{"type":"name","name":"a"}]}}`, "transformations[0].options.orderBy", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := transformPanel(t, "", tc.transformation)
			if line.Status != statusPartial || !slices.Contains(line.Paths, tc.path) {
				t.Fatalf("status %s, paths %v, want %s reported", line.Status, line.Paths, tc.path)
			}
			if len(panel.Transform) != tc.kept {
				t.Fatalf("kept %d transformations, want %d: %+v", len(panel.Transform), tc.kept, panel.Transform)
			}
		})
	}
}

func TestSortKeepsEveryKeyBeforeAnInvalidOne(t *testing.T) {
	panel, line := transformPanel(t, "", `{"id":"sortBy","options":{"sort":[{"field":"Value"},{"desc":true},{"field":"Time"}]}}`)
	if !slices.Contains(line.Paths, "transformations[0].options.sort[1]") {
		t.Fatalf("paths %v", line.Paths)
	}
	wantJSON(t, panel.Transform, `[{"type":"sortBy","field":"Value"}]`)
}

func filterPanel(t *testing.T, filter string) (string, PanelLine) {
	t.Helper()
	doc := `{"title":"T","schemaVersion":39,"panels":[{` + panelFrame + `,"type":"table",` +
		`"targets":[{"refId":"A","expr":"up"},{"refId":"B","expr":"down"},{"refId":"C","expr":"sideways"}],` +
		`"transformations":[{"id":"merge","options":{},"filter":` + filter + `}]}]}`
	panel, line := convertedPanel(t, doc)
	if len(panel.Transform) != 1 {
		t.Fatalf("kept %d transformations", len(panel.Transform))
	}
	raw, err := json.Marshal(panel.Transform[0].Queries)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw), line
}

func TestTransformsScopedToQueries(t *testing.T) {
	cases := []struct{ name, filter, want string }{
		{"one id", `{"id":"byRefId","options":"B"}`, `["B"]`},
		{"pipe list", `{"id":"byRefId","options":"A|C"}`, `["A","C"]`},
		{"regular expression", `{"id":"byRefId","options":"/^(A|B)$/"}`, `["A","B"]`},
		{"regular expression with a flag", `{"id":"byRefId","options":"/b/i"}`, `["B"]`},
		{"array", `{"id":"byRefId","options":["C","A"]}`, `["C","A"]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			queries, line := filterPanel(t, tc.filter)
			if line.Status != statusImported {
				t.Fatalf("status %s, paths %v", line.Status, line.Paths)
			}
			if queries != tc.want {
				t.Fatalf("queries %s, want %s", queries, tc.want)
			}
		})
	}
}

func TestTransformScopeIsReportedWhenItCannotBeKept(t *testing.T) {
	cases := []struct{ name, filter, want string }{
		{"frame filter", `{"id":"byFrameRefID","options":"A"}`, `null`},
		{"unknown id", `{"id":"byRefId","options":"Z"}`, `null`},
		{"partly unknown", `{"id":"byRefId","options":"A|Z"}`, `["A"]`},
		{"regular expression matching nothing", `{"id":"byRefId","options":"/^Z/"}`, `null`},
		{"invalid regular expression", `{"id":"byRefId","options":"/(/"}`, `null`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			queries, line := filterPanel(t, tc.filter)
			if line.Status != statusPartial || !slices.Contains(line.Paths, "transformations[0].filter") {
				t.Fatalf("status %s, paths %v", line.Status, line.Paths)
			}
			if queries != tc.want {
				t.Fatalf("queries %s, want %s", queries, tc.want)
			}
		})
	}
}
