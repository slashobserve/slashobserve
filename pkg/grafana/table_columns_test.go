package grafana

import (
	"encoding/json"
	"slices"
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

func TestTableColumnPropsStayReportedOutsideTables(t *testing.T) {
	for _, custom := range []string{`{"width":120}`, `{"align":"left"}`, `{"filterable":true}`} {
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
		{"small rows are the default", `{"cellHeight":"sm"}`, `null`},
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

func TestTableDisplayOptionsReportWhatTheyCannotKeep(t *testing.T) {
	cases := []struct{ name, options, want, path string }{
		{"automatic row height", `{"cellHeight":"auto"}`, `null`, "options.cellHeight"},
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
