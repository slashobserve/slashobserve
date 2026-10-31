package grafana

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func tableOptionsPanel(t *testing.T, options string) (spec.Panel, PanelLine) {
	t.Helper()
	return convertedPanel(t, panelDoc("table", upTarget+`,"options":`+options))
}

func propsJSON(t *testing.T, props spec.FieldProps) string {
	t.Helper()
	data, err := json.Marshal(props)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestTableColumnFieldPropsImport(t *testing.T) {
	cases := []struct{ name, custom, want string }{
		{"width", `{"width":120}`, `{"width":120}`},
		{"minimum width", `{"minWidth":80}`, `{"minWidth":80}`},
		{"fractional width rounds", `{"width":120.6}`, `{"width":121}`},
		{"align left", `{"align":"left"}`, `{"align":"left"}`},
		{"align center", `{"align":"center"}`, `{"align":"center"}`},
		{"align right", `{"align":"right"}`, `{"align":"right"}`},
		{"auto align is the default", `{"align":"auto"}`, `{}`},
		{"filterable", `{"filterable":true}`, `{"filterable":true}`},
		{"not filterable is the default", `{"filterable":false}`, `{}`},
		{"inspect", `{"inspect":true}`, `{"inspect":true}`},
		{"hidden header", `{"hideHeader":true}`, `{"hideHeader":true}`},
		{"wrapped header", `{"wrapHeaderText":true}`, `{"wrapHeader":true}`},
		{"unwrapped header is the default", `{"wrapHeaderText":false}`, `{}`},
		{"tooltip from another field", `{"tooltip":{"field":"Status"}}`, `{"tooltipField":"Status"}`},
		{"tooltip placement", `{"tooltip":{"field":"Status","placement":"left"}}`, `{"tooltipField":"Status","tooltipPlacement":"left"}`},
		{"automatic tooltip placement is the default", `{"tooltip":{"field":"Status","placement":"auto"}}`, `{"tooltipField":"Status"}`},
		{"a placement without a field shows no tooltip", `{"tooltip":{"placement":"top"}}`, `{}`},
		{"zero width means unset", `{"width":0}`, `{}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := tableCellPanel(t, tc.custom, "")
			if got := propsJSON(t, panel.FieldProps); got != tc.want {
				t.Fatalf("props = %s, want %s", got, tc.want)
			}
			requireImported(t, line)
		})
	}
}

func TestTableColumnWidthIsKeptWithinTheSchemaRange(t *testing.T) {
	cases := []struct {
		name, custom, want string
	}{
		{"too narrow", `{"width":5}`, `{"width":20}`},
		{"too wide", `{"minWidth":9000}`, `{"minWidth":2000}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := tableCellPanel(t, tc.custom, "")
			if got := propsJSON(t, panel.FieldProps); got != tc.want {
				t.Fatalf("props = %s, want %s", got, tc.want)
			}
			if line.Status != statusPartial {
				t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
			}
		})
	}
}

func TestTableColumnOverridesWriteTheirValueWhole(t *testing.T) {
	overrides := `{"matcher":{"id":"byName","options":"Value"},"properties":[
		{"id":"custom.width","value":90},
		{"id":"custom.align","value":"auto"},
		{"id":"custom.filterable","value":false},
		{"id":"custom.inspect","value":true}]}`
	panel, line := tableCellPanel(t, `{"align":"right","filterable":true}`, overrides)
	if got := propsJSON(t, panel.FieldProps); got != `{"align":"right","filterable":true}` {
		t.Fatalf("panel props = %s", got)
	}
	if len(panel.Overrides) != 1 {
		t.Fatalf("overrides = %+v", panel.Overrides)
	}
	if got := propsJSON(t, panel.Overrides[0].FieldProps); got != `{"width":90,"align":"auto","filterable":false,"inspect":true}` {
		t.Fatalf("override props = %s", got)
	}
	requireImported(t, line)
}

func TestTableColumnTooltipAndHeaderInAnOverrideAreWrittenWhole(t *testing.T) {
	overrides := `{"matcher":{"id":"byName","options":"Value"},"properties":[
		{"id":"custom.wrapHeaderText","value":false},
		{"id":"custom.tooltip","value":{"field":"Status","placement":"auto"}},
		{"id":"description","value":"Requests handled"}]}`
	panel, line := tableCellPanel(t, `{"wrapHeaderText":true,"tooltip":{"field":"Other","placement":"top"}}`, overrides)
	if got := propsJSON(t, panel.FieldProps); got != `{"wrapHeader":true,"tooltipField":"Other","tooltipPlacement":"top"}` {
		t.Fatalf("panel props = %s", got)
	}
	if len(panel.Overrides) != 1 {
		t.Fatalf("overrides = %+v", panel.Overrides)
	}
	want := `{"wrapHeader":false,"fieldDescription":"Requests handled","tooltipField":"Status","tooltipPlacement":"auto"}`
	if got := propsJSON(t, panel.Overrides[0].FieldProps); got != want {
		t.Fatalf("override props = %s, want %s", got, want)
	}
	requireImported(t, line)
}

func TestTableColumnTooltipReportsWhatItCannotKeep(t *testing.T) {
	cases := []struct{ name, custom, want, path string }{
		{"unknown placement", `{"tooltip":{"field":"Status","placement":"diagonal"}}`, `{}`, "fieldConfig.defaults.custom.tooltip"},
		{"field name too long", `{"tooltip":{"field":"` + strings.Repeat("a", 201) + `"}}`, `{}`, "fieldConfig.defaults.custom.tooltip.field"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := tableCellPanel(t, tc.custom, "")
			if got := propsJSON(t, panel.FieldProps); got != tc.want {
				t.Fatalf("props = %s, want %s", got, tc.want)
			}
			if line.Status != statusPartial || !slices.Contains(line.Paths, tc.path) {
				t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
			}
		})
	}
}

func fieldDescriptionPanel(t *testing.T, typ, description string) (spec.Panel, PanelLine) {
	t.Helper()
	quoted, err := json.Marshal(description)
	if err != nil {
		t.Fatal(err)
	}
	return convertedPanel(t, panelDoc(typ, upTarget+`,"fieldConfig":{"defaults":{"description":`+string(quoted)+`},"overrides":[]}`))
}

func TestFieldDescriptionImportsOnTables(t *testing.T) {
	panel, line := fieldDescriptionPanel(t, "table", "Requests handled")
	if panel.FieldDescription != "Requests handled" || panel.Description != "" {
		t.Fatalf("field description = %q, panel description = %q", panel.FieldDescription, panel.Description)
	}
	requireImported(t, line)
}

func TestFieldDescriptionOverTheLimitIsCutAndReported(t *testing.T) {
	panel, line := fieldDescriptionPanel(t, "table", strings.Repeat("é", 2001))
	if got := len([]rune(panel.FieldDescription)); got != 2000 {
		t.Fatalf("kept %d characters, want 2000", got)
	}
	if line.Status != statusPartial || !slices.Contains(line.Reasons, "The field description was cut to 2000 characters.") {
		t.Fatalf("status = %s, reasons %v", line.Status, line.Reasons)
	}
	_, exact := fieldDescriptionPanel(t, "table", strings.Repeat("é", 2000))
	requireImported(t, exact)
}

func TestFieldDescriptionStaysReportedOutsideTables(t *testing.T) {
	panel, line := fieldDescriptionPanel(t, "timeseries", "Requests handled")
	if panel.FieldDescription != "" {
		t.Fatalf("field description = %q", panel.FieldDescription)
	}
	if line.Status != statusPartial || !slices.Contains(line.Paths, "fieldConfig.defaults.description") {
		t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
	}
}

func TestTableColumnPropsStayReportedOutsideTables(t *testing.T) {
	for _, custom := range []string{`{"width":120}`, `{"align":"left"}`, `{"filterable":true}`, `{"wrapHeaderText":true}`, `{"tooltip":{"field":"Status"}}`} {
		_, line := customPanelOf(t, "stat", custom, "")
		if line.Status != statusPartial {
			t.Fatalf("%s: status = %s, paths %v", custom, line.Status, line.Paths)
		}
	}
}

func TestTableDisplayOptionsImport(t *testing.T) {
	cases := []struct{ name, options, want string }{
		{"medium rows", `{"cellHeight":"md"}`, `{"cellHeight":"md"}`},
		{"large rows", `{"cellHeight":"lg"}`, `{"cellHeight":"lg"}`},
		{"rows that fit their content", `{"cellHeight":"auto"}`, `{"cellHeight":"auto"}`},
		{"small rows are the default", `{"cellHeight":"sm"}`, `null`},
		{"a tallest row", `{"cellHeight":"auto","maxRowHeight":100}`, `{"cellHeight":"auto","maxRowHeight":100}`},
		{"the smallest tallest row", `{"maxRowHeight":20}`, `{"maxRowHeight":20}`},
		{"the largest tallest row", `{"maxRowHeight":1000}`, `{"maxRowHeight":1000}`},
		{"the second frame", `{"frameIndex":1}`, `{"frameIndex":1}`},
		{"the first frame", `{"frameIndex":0}`, `{"frameIndex":0}`},
		{"the last frame index", `{"frameIndex":99}`, `{"frameIndex":99}`},
		{"type icons", `{"showTypeIcons":true}`, `{"typeIcons":true}`},
		{"pagination", `{"enablePagination":true}`, `{"pagination":true}`},
		{"pagination beside the footer", `{"footer":{"enablePagination":true}}`, `{"pagination":true}`},
		{"frozen columns", `{"frozenColumns":{"left":2}}`, `{"frozenColumns":2}`},
		{"no frozen columns", `{"frozenColumns":{"left":0}}`, `null`},
		{"sort ascending", `{"sortBy":[{"displayName":"Time"}]}`, `{"sortBy":"Time"}`},
		{"sort descending", `{"sortBy":[{"displayName":"Value","desc":true}]}`, `{"sortBy":"Value","sortDesc":true}`},
		{"no sort", `{"sortBy":[]}`, `null`},
		{"hidden header", `{"showHeader":false}`, `{"header":false}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := tableOptionsPanel(t, tc.options)
			if got := displayJSON(t, panel); got != tc.want {
				t.Fatalf("display = %s, want %s", got, tc.want)
			}
			requireImported(t, line)
		})
	}
}

func TestTableFooterOptionsImport(t *testing.T) {
	cases := []struct{ name, footer, want string }{
		{"hidden footer ignores the rest", `{"show":false,"reducer":["mean"],"countRows":true,"fields":"Value"}`, `null`},
		{"shown with the default reducer", `{"show":true}`, `{"footer":["sum"]}`},
		{"reducers", `{"show":true,"reducer":["mean","max"]}`, `{"footer":["mean","max"]}`},
		{"a single legacy reducer", `{"show":true,"reducer":"mean"}`, `{"footer":["mean"]}`},
		{"no reducer at all", `{"show":true,"reducer":[]}`, `null`},
		{"row count", `{"show":true,"reducer":[],"countRows":true}`, `{"footerCountRows":true}`},
		{"one footer column", `{"show":true,"fields":"Value"}`, `{"footer":["sum"],"footerFields":["Value"]}`},
		{"several footer columns", `{"show":true,"fields":["A"," B ","A",""]}`, `{"footer":["sum"],"footerFields":["A","B"]}`},
		{"every column", `{"show":true,"fields":""}`, `{"footer":["sum"]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := tableOptionsPanel(t, `{"footer":`+tc.footer+`}`)
			if got := displayJSON(t, panel); got != tc.want {
				t.Fatalf("display = %s, want %s", got, tc.want)
			}
			requireImported(t, line)
		})
	}
}

func TestTableFooterFromFieldDefaultsImports(t *testing.T) {
	cases := []struct {
		name, options, reducers, want string
		wantPaths                     []string
	}{
		{"reducers alone", ``, `["sum","countAll"]`, `{"footer":["sum","countAll"]}`, nil},
		{"a single reducer", ``, `["p95"]`, `{"footer":["p95"]}`, nil},
		{"shown footer with the same reducers", `"footer":{"show":true,"reducer":["mean"]},`, `["mean"]`, `{"footer":["mean"]}`, nil},
		{"shown footer without a reducer", `"footer":{"show":true},`, `["max"]`, `{"footer":["max"]}`, nil},
		{"hidden footer ignores them", `"footer":{"show":false},`, `["sum"]`, `null`, nil},
		{"the newer setting wins over a different older one", `"footer":{"show":true,"reducer":["mean"]},`, `["max"]`, `{"footer":["max"]}`, []string{"options.footer.reducer"}},
		{"a reducer that is not a number", ``, `["allIsNull"]`, `null`, []string{"fieldConfig.defaults.custom.footer.reducers"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := panelDoc("table", upTarget+`,"options":{`+strings.TrimSuffix(tc.options, ",")+`},"fieldConfig":{"defaults":{"custom":{"footer":{"reducers":`+tc.reducers+`}}},"overrides":[]}`)
			panel, line := convertedPanel(t, doc)
			if got := displayJSON(t, panel); got != tc.want {
				t.Fatalf("display = %s, want %s", got, tc.want)
			}
			if len(tc.wantPaths) == 0 {
				requireImported(t, line)
				return
			}
			if line.Status != statusPartial || !slices.Equal(line.Paths, tc.wantPaths) {
				t.Fatalf("status = %s, paths %v, want %v", line.Status, line.Paths, tc.wantPaths)
			}
		})
	}
}

func TestTableFooterFromFieldDefaultsStaysReportedOutsideTables(t *testing.T) {
	_, line := customPanelOf(t, "stat", `{"footer":{"reducers":["sum"]}}`, "")
	if line.Status != statusPartial || !slices.Contains(line.Paths, "fieldConfig.defaults.custom.footer.reducers") {
		t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
	}
}

func TestTableFooterInAColumnOverrideStaysReported(t *testing.T) {
	overrides := `{"matcher":{"id":"byName","options":"Value"},"properties":[{"id":"custom.footer","value":{"reducers":["sum"]}}]}`
	panel, line := tableCellPanel(t, `{}`, overrides)
	if len(panel.Overrides) != 0 {
		t.Fatalf("overrides = %+v", panel.Overrides)
	}
	if line.Status != statusPartial || !slices.Contains(line.Paths, "fieldConfig.overrides[0].properties[0]") {
		t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
	}
}

func TestTableDisplayOptionsReportWhatTheyCannotKeep(t *testing.T) {
	cases := []struct{ name, options, want, path string }{
		{"an unknown row height", `{"cellHeight":"huge"}`, `null`, "options.cellHeight"},
		{"a tallest row under 20", `{"maxRowHeight":19}`, `null`, "options.maxRowHeight"},
		{"a tallest row over 1000", `{"maxRowHeight":1001}`, `null`, "options.maxRowHeight"},
		{"a fractional tallest row", `{"maxRowHeight":50.5}`, `null`, "options.maxRowHeight"},
		{"a negative frame index", `{"frameIndex":-1}`, `null`, "options.frameIndex"},
		{"a frame index over 99", `{"frameIndex":100}`, `null`, "options.frameIndex"},
		{"more than ten frozen columns", `{"frozenColumns":{"left":12}}`, `{"frozenColumns":10}`, "options.frozenColumns.left"},
		{"a second sort column", `{"sortBy":[{"displayName":"A"},{"displayName":"B","desc":true}]}`, `{"sortBy":"A"}`, "options.sortBy"},
		{"a nameless sort column", `{"sortBy":[{"desc":true}]}`, `null`, "options.sortBy"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := tableOptionsPanel(t, tc.options)
			if got := displayJSON(t, panel); got != tc.want {
				t.Fatalf("display = %s, want %s", got, tc.want)
			}
			if line.Status != statusPartial || !slices.Contains(line.Paths, tc.path) {
				t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
			}
		})
	}
}

func legacyTablePanel(t *testing.T, members string) (spec.Panel, PanelLine) {
	t.Helper()
	return convertedPanel(t, panelDoc("table-old", upTarget+","+members))
}

func TestLegacyTableStyleAlignImports(t *testing.T) {
	cases := []struct{ name, styles, wantPanel, wantOverride string }{
		{"every column", `[{"pattern":"/.*/","align":"center"}]`, `{"align":"center"}`, ""},
		{"one column", `[{"pattern":"Value","align":"right"}]`, `{}`, `{"align":"right"}`},
		{"auto", `[{"pattern":"/.*/","align":"auto"}]`, `{}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := legacyTablePanel(t, `"styles":`+tc.styles)
			if got := propsJSON(t, panel.FieldProps); got != tc.wantPanel {
				t.Fatalf("panel props = %s, want %s", got, tc.wantPanel)
			}
			if tc.wantOverride != "" {
				if len(panel.Overrides) != 1 || propsJSON(t, panel.Overrides[0].FieldProps) != tc.wantOverride {
					t.Fatalf("overrides = %+v", panel.Overrides)
				}
			}
			requireImported(t, line)
		})
	}
}

func TestLegacyTableStyleKeysGrafanaDoesNotDrawAreNotReported(t *testing.T) {
	cases := []struct {
		name, style, reported string
	}{
		{"a date format on a number column", `{"pattern":"Value","type":"number","dateFormat":"YYYY-MM-DD"}`, ""},
		{"a date format on a hidden column", `{"pattern":"Time","type":"hidden","dateFormat":"YYYY-MM-DD"}`, ""},
		{"link options with the link off", `{"pattern":"Value","type":"number","link":false,"linkUrl":"http://x","linkTooltip":"Drill","linkTargetBlank":true}`, ""},
		{"link options with no link flag", `{"pattern":"Value","type":"number","linkUrl":"http://x","linkTooltip":"Drill","linkTargetBlank":true}`, ""},
		{"a date format on a date column", `{"pattern":"Time","type":"date","dateFormat":"YYYY-MM-DD"}`, "styles[0].dateFormat"},
		{"a link", `{"pattern":"Value","type":"number","link":true,"linkUrl":"http://x"}`, "styles[0].link"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, line := legacyTablePanel(t, `"styles":[`+tc.style+`]`)
			if tc.reported == "" {
				requireImported(t, line)
				return
			}
			if line.Status != statusPartial || !slices.Contains(line.Paths, tc.reported) {
				t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
			}
		})
	}
}

func TestTableWithStylesAndNoFieldConfigRunsAsAnAngularTable(t *testing.T) {
	const graphLeftovers = `"bars":false,"lines":true,"fill":1,"linewidth":1,"nullPointMode":"null as zero","legend":{"avg":false,"show":true},"tooltip":{"shared":false,"sort":0,"value_type":"individual"},"xaxis":{"mode":"time","show":true},"yaxes":[{"format":"short","logBase":1,"min":0,"show":true}]`
	panel, line := convertedPanel(t, panelDoc("table", upTarget+`,"transform":"table","styles":[{"pattern":"Value","type":"number","unit":"s","decimals":2},{"pattern":"/.*/","type":"string","unit":"short"}],`+graphLeftovers))
	requireImported(t, line)
	if line.SourceType != "table-old" || line.Type != "table" {
		t.Fatalf("type = %s, source type = %s", line.Type, line.SourceType)
	}
	if panel.Unit != "short" || len(panel.Overrides) != 1 || panel.Overrides[0].Match.Field != "Value" || panel.Overrides[0].Unit != "s" {
		t.Fatalf("panel = %+v", panel)
	}
}

func TestMigratedTableKeepsItsAngularKeysWithoutANote(t *testing.T) {
	const fieldConfig = `"fieldConfig":{"defaults":{"custom":{"align":null}},"overrides":[]},"options":{"showHeader":true}`
	const angular = `"transform":"table","showHeader":true,"fontSize":"100%","columns":[{"text":"Avg","value":"avg"}],"sort":{"col":1,"desc":false},"styles":[{"pattern":"Time","type":"date","dateFormat":"YYYY-MM-DD","alias":"Time","align":"auto"},{"pattern":"/.*/","type":"number","unit":"short","decimals":2,"colors":["red","green","blue"],"thresholds":[1,2],"colorMode":"cell","link":true,"linkUrl":"http://x","linkTooltip":"Drill","linkTargetBlank":true}],"yaxes":[{"format":"short","logBase":1,"min":0,"show":true}],"lines":true,"bars":false,"legend":{"show":true,"values":false},"tooltip":{"shared":true}`
	plain, _ := convertedPanel(t, panelDoc("table", upTarget+","+fieldConfig))
	left, line := convertedPanel(t, panelDoc("table", upTarget+","+fieldConfig+","+angular))
	requireImported(t, line)
	if line.SourceType != "table" || !reflect.DeepEqual(plain, left) {
		t.Fatalf("source type = %s, panel = %+v, want %+v", line.SourceType, left, plain)
	}
}

func TestLegacyTableSort(t *testing.T) {
	cases := []struct {
		name, sort string
		partial    bool
	}{
		{"Grafana's own default", `{"col":0,"desc":true}`, false},
		{"no column", `{"col":null,"desc":false}`, false},
		{"another column", `{"col":2,"desc":false}`, true},
		{"the first column ascending", `{"col":0,"desc":false}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, line := legacyTablePanel(t, `"sort":`+tc.sort)
			if tc.partial != (line.Status == statusPartial) || (tc.partial && !slices.Contains(line.Paths, "sort")) {
				t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
			}
		})
	}
}
