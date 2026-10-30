package grafana

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func transformPanel(t *testing.T, dashboardMembers, transformation string) (spec.Panel, PanelLine) {
	t.Helper()
	doc := `{"title":"T","schemaVersion":39` + dashboardMembers + `,"panels":[{` + panelFrame +
		`,"type":"table",` + upTarget + `,"transformations":[` + transformation + `]}]}`
	return convertedPanel(t, doc)
}

func wantJSON(t *testing.T, got any, want string) {
	t.Helper()
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != want {
		t.Fatalf("got  %s\nwant %s", raw, want)
	}
}

func TestFieldTransformsConvert(t *testing.T) {
	cases := []struct {
		name, transformation, want string
	}{
		{
			"convertFieldType",
			`{"id":"convertFieldType","options":{"fields":{},"conversions":[
				{"targetField":"Time","destinationType":"time","dateFormat":"YYYY-MM-DD HH:mm"},
				{"targetField":"value","destinationType":"number"},
				{"targetField":"tags","destinationType":"string","joinWith":" | "},
				{"targetField":"up","destinationType":"boolean"}]}}`,
			`[{"type":"convertFieldType","conversions":[{"field":"Time","to":"time","dateFormat":"YYYY-MM-DD HH:mm"},{"field":"value","to":"number"},{"field":"tags","to":"string","joinWith":" | "},{"field":"up","to":"boolean"}]}]`,
		},
		{
			"formatString upper",
			`{"id":"formatString","options":{"stringField":"service","outputFormat":"Upper Case"}}`,
			`[{"type":"formatString","field":"service","format":"upper"}]`,
		},
		{
			"formatString substring",
			`{"id":"formatString","options":{"stringField":"service","outputFormat":"Substring","substringStart":2,"substringEnd":6}}`,
			`[{"type":"formatString","field":"service","format":"substring","start":2,"end":6}]`,
		},
		{
			"formatString substring from zero",
			`{"id":"formatString","options":{"stringField":"service","outputFormat":"Substring","substringStart":0,"substringEnd":4}}`,
			`[{"type":"formatString","field":"service","format":"substring","end":4}]`,
		},
		{
			"formatTime",
			`{"id":"formatTime","options":{"timeField":"Time","outputFormat":"YYYY-MM-DD HH:mm:ss","useTimezone":true}}`,
			`[{"type":"formatTime","field":"Time","format":"YYYY-MM-DD HH:mm:ss"}]`,
		},
		{
			"formatTime without the dashboard time zone",
			`{"id":"formatTime","options":{"timeField":"Time","outputFormat":"HH:mm","useTimezone":false}}`,
			`[{"type":"formatTime","field":"Time","format":"HH:mm","utc":true}]`,
		},
		{
			"renameByRegex",
			`{"id":"renameByRegex","options":{"regex":"(.*) requests","renamePattern":"$1"}}`,
			`[{"type":"renameByRegex","regex":"(.*) requests","renameTo":"$1"}]`,
		},
		{
			"filterByValue defaults are dropped",
			`{"id":"filterByValue","options":{"type":"include","match":"any","filters":[
				{"fieldName":"load","config":{"id":"greaterOrEqual","options":{"value":5}}},
				{"fieldName":"host","config":{"id":"notEqual","options":{"value":"a1"}}},
				{"fieldName":"host","config":{"id":"isNotNull","options":{}}}]}}`,
			`[{"type":"filterByValue","filters":[{"field":"load","op":"gte","value":5},{"field":"host","op":"neq","value":"a1"},{"field":"host","op":"notNull"}]}]`,
		},
		{
			"filterByValue exclude all with range and regex",
			`{"id":"filterByValue","options":{"type":"exclude","match":"all","filters":[
				{"fieldName":"load","config":{"id":"range","options":{"from":1,"to":9}}},
				{"fieldName":"host","config":{"id":"regex","options":{"value":"a.*"}}},
				{"fieldName":"load","config":{"id":"lower","options":{"value":"3"}}},
				{"fieldName":"load","config":{"id":"equal","options":{"value":0}}}]}}`,
			`[{"type":"filterByValue","keep":"exclude","match":"all","filters":[{"field":"load","op":"range","from":1,"to":9},{"field":"host","op":"regex","value":"a.*"},{"field":"load","op":"lt","value":3},{"field":"load","op":"eq","value":0}]}]`,
		},
		{
			"filterByRefId",
			`{"id":"filterByRefId","options":{"include":"A | A"}}`,
			`[{"type":"filterByRefId","include":["A"]}]`,
		},
		{
			"extractFields json with paths",
			`{"id":"extractFields","options":{"source":"line","format":"json","replace":true,"keepTime":true,"jsonPaths":[{"path":"a.b","alias":"ab"},{"path":"c[0]"}]}}`,
			`[{"type":"extractFields","field":"line","format":"json","replace":true,"keepTime":true,"paths":[{"path":"a.b","alias":"ab"},{"path":"c[0]"}]}]`,
		},
		{
			"extractFields auto is the default",
			`{"id":"extractFields","options":{"source":"line","format":"auto","replace":false,"keepTime":false}}`,
			`[{"type":"extractFields","field":"line"}]`,
		},
		{
			"extractFields delimiter and regexp",
			`{"id":"extractFields","options":{"source":"line","format":"regexp","regExp":"(?<m>\\w+) (?<s>\\d+)","delimiter":";"}}`,
			`[{"type":"extractFields","field":"line","format":"regexp","regex":"(?\u003cm\u003e\\w+) (?\u003cs\u003e\\d+)","delimiter":";"}]`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := transformPanel(t, "", tc.transformation)
			if line.Status != statusImported || len(line.Paths) != 0 {
				t.Fatalf("status %s, paths %v", line.Status, line.Paths)
			}
			wantJSON(t, panel.Transform, tc.want)
		})
	}
}

func TestFormatTimeFollowsTheDashboardTimeZone(t *testing.T) {
	const transformation = `{"id":"formatTime","options":{"timeField":"Time","outputFormat":"HH:mm","useTimezone":true}}`
	panel, _ := transformPanel(t, `,"timezone":"utc"`, transformation)
	wantJSON(t, panel.Transform, `[{"type":"formatTime","field":"Time","format":"HH:mm","utc":true}]`)
	panel, _ = transformPanel(t, `,"timezone":"browser"`, transformation)
	wantJSON(t, panel.Transform, `[{"type":"formatTime","field":"Time","format":"HH:mm"}]`)
}

func TestFieldTransformsReport(t *testing.T) {
	cases := []struct {
		name, transformation, path string
		kept                       int
	}{
		{"unknown output format", `{"id":"formatString","options":{"stringField":"s","outputFormat":"Reverse"}}`, "transformations[0].options.outputFormat", 0},
		{"unknown filter id", `{"id":"filterByValue","options":{"filters":[{"fieldName":"load","config":{"id":"between","options":{}}}]}}`, "transformations[0].options.filters[0].config.id", 0},
		{"one unknown filter id among others", `{"id":"filterByValue","options":{"filters":[{"fieldName":"load","config":{"id":"between"}},{"fieldName":"load","config":{"id":"isNull"}}]}}`, "transformations[0].options.filters[0].config.id", 1},
		{"filter without a number", `{"id":"filterByValue","options":{"filters":[{"fieldName":"load","config":{"id":"greater","options":{"value":"many"}}}]}}`, "transformations[0].options.filters[0].config.options", 0},
		{"range without bounds", `{"id":"filterByValue","options":{"filters":[{"fieldName":"load","config":{"id":"range","options":{}}}]}}`, "transformations[0].options.filters[0].config.options", 0},
		{"refId the panel does not have", `{"id":"filterByRefId","options":{"include":"Z"}}`, "transformations[0].options.include", 0},
		{"time format with moment words", `{"id":"formatTime","options":{"timeField":"Time","outputFormat":"dddd MMM Do"}}`, "transformations[0].options.outputFormat", 0},
		{"conversion date format with moment words", `{"id":"convertFieldType","options":{"conversions":[{"targetField":"Time","destinationType":"time","dateFormat":"MMM D"}]}}`, "transformations[0].options.conversions[0].dateFormat", 0},
		{"conversion without a destination", `{"id":"convertFieldType","options":{"conversions":[{"targetField":"Time"}]}}`, "transformations[0].options.conversions[0]", 0},
		{"enum settings", `{"id":"convertFieldType","options":{"conversions":[{"targetField":"s","destinationType":"enum","enumConfig":{"text":["a"]}}]}}`, "transformations[0].options.conversions[0].enumConfig", 1},
		{"fields set", `{"id":"convertFieldType","options":{"fields":{"a":1},"conversions":[{"targetField":"s","destinationType":"number"}]}}`, "transformations[0].options.fields", 1},
		{"empty rename pattern", `{"id":"renameByRegex","options":{"regex":"a","renamePattern":""}}`, "transformations[0].options.renamePattern", 0},
		{"unknown extract format", `{"id":"extractFields","options":{"source":"line","format":"xml"}}`, "transformations[0].options.format", 0},
		{"path with a missing path", `{"id":"extractFields","options":{"source":"line","jsonPaths":[{"alias":"x"}]}}`, "transformations[0].options.jsonPaths[0]", 1},
		{"substring beyond the limit", `{"id":"formatString","options":{"stringField":"s","outputFormat":"Substring","substringEnd":20000}}`, "transformations[0].options.substringEnd", 1},
		{"other option", `{"id":"formatString","options":{"stringField":"s","outputFormat":"Trim","applyTo":"all"}}`, "transformations[0].options.applyTo", 1},
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

func TestSeveralFieldTransformsImportTogether(t *testing.T) {
	_, line := transformPanel(t, "", `{"id":"filterByValue","options":{"filters":[{"fieldName":"load","config":{"id":"greater","options":{"value":1}}}]}},
		{"id":"formatString","options":{"stringField":"s","outputFormat":"Title Case"}}`)
	if line.Status != statusImported {
		t.Fatalf("status %s, paths %v", line.Status, line.Paths)
	}
}
