package grafana

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func tableCellPanel(t *testing.T, defaults, overrides string) (spec.Panel, PanelLine) {
	t.Helper()
	return customPanelOf(t, "table", defaults, overrides)
}

func cellJSON(t *testing.T, cell *spec.Cell) string {
	t.Helper()
	if cell == nil {
		return "null"
	}
	data, err := json.Marshal(cell)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestTableCellTypesImport(t *testing.T) {
	cases := []struct{ name, custom, want string }{
		{"auto writes nothing", `{"cellOptions":{"type":"auto"}}`, "null"},
		{"color text", `{"cellOptions":{"type":"color-text"}}`, `{"type":"text"}`},
		{"color background", `{"cellOptions":{"type":"color-background","mode":"basic"}}`, `{"type":"background"}`},
		{"gradient background is flat", `{"cellOptions":{"type":"color-background","mode":"gradient"}}`, `{"type":"background"}`},
		{"background for the whole row", `{"cellOptions":{"type":"color-background","applyToRow":true}}`, `{"type":"background","wholeRow":true}`},
		{"basic gauge", `{"cellOptions":{"type":"gauge","mode":"basic"}}`, `{"type":"gauge"}`},
		{"gradient gauge is flat", `{"cellOptions":{"type":"gauge","mode":"gradient"}}`, `{"type":"gauge"}`},
		{"lcd gauge", `{"cellOptions":{"type":"gauge","mode":"lcd","valueDisplayMode":"hidden"}}`, `{"type":"gauge","gaugeStyle":"lcd","valueMode":"hidden"}`},
		{"gauge value as text", `{"cellOptions":{"type":"gauge","valueDisplayMode":"text"}}`, `{"type":"gauge","valueMode":"text"}`},
		{"sparkline", `{"cellOptions":{"type":"sparkline"}}`, `{"type":"sparkline"}`},
		{"sparkline without its value", `{"cellOptions":{"type":"sparkline","hideValue":true}}`, `{"type":"sparkline","valueMode":"hidden"}`},
		{"json", `{"cellOptions":{"type":"json-view"}}`, `{"type":"json"}`},
		{"image", `{"cellOptions":{"type":"image","alt":"Logo","title":"Company logo"}}`, `{"type":"image","imageAlt":"Logo","imageTitle":"Company logo"}`},
		{"data links", `{"cellOptions":{"type":"data-links"}}`, `{"type":"links"}`},
		{"pill", `{"cellOptions":{"type":"pill"}}`, `{"type":"pill"}`},
		{"wrapped text inside the cell options", `{"cellOptions":{"type":"color-text","wrapText":true}}`, `{"type":"text","wrap":true}`},
		{"wrapped text beside the cell options", `{"wrapText":true}`, `{"type":"auto","wrap":true}`},
		{"wrapped text beside a type", `{"cellOptions":{"type":"pill"},"wrapText":true}`, `{"type":"pill","wrap":true}`},
		{"options of another type are inert", `{"cellOptions":{"type":"color-text","applyToRow":true,"alt":"x","mode":"gradient"}}`, `{"type":"text"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := tableCellPanel(t, tc.custom, "")
			if got := cellJSON(t, panel.Cell); got != tc.want {
				t.Fatalf("cell = %s, want %s", got, tc.want)
			}
			if line.Status != statusImported {
				t.Fatalf("status = %s, paths %v, reasons %v", line.Status, line.Paths, line.Reasons)
			}
		})
	}
}

func TestTableLegacyDisplayModesImport(t *testing.T) {
	cases := []struct{ mode, want string }{
		{"auto", "null"},
		{"color-text", `{"type":"text"}`},
		{"color-background", `{"type":"background"}`},
		{"color-background-solid", `{"type":"background"}`},
		{"gradient-gauge", `{"type":"gauge"}`},
		{"lcd-gauge", `{"type":"gauge","gaugeStyle":"lcd"}`},
		{"basic", `{"type":"gauge"}`},
		{"gauge", `{"type":"gauge"}`},
		{"sparkline", `{"type":"sparkline"}`},
		{"json-view", `{"type":"json"}`},
		{"image", `{"type":"image"}`},
		{"data-links", `{"type":"links"}`},
	}
	for _, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			panel, line := tableCellPanel(t, `{"displayMode":"`+tc.mode+`"}`, "")
			if got := cellJSON(t, panel.Cell); got != tc.want {
				t.Fatalf("cell = %s, want %s", got, tc.want)
			}
			if line.Status != statusImported {
				t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
			}
		})
	}
}

func TestTableCellOptionsWinOverTheLegacyDisplayMode(t *testing.T) {
	panel, line := tableCellPanel(t, `{"cellOptions":{"type":"pill"},"displayMode":"color-text"}`, "")
	if got := cellJSON(t, panel.Cell); got != `{"type":"pill"}` {
		t.Fatalf("cell = %s", got)
	}
	if line.Status != statusImported {
		t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
	}
}

func TestTableSparklineOptionsImportAsFieldProps(t *testing.T) {
	custom := `{"cellOptions":{"type":"sparkline","drawStyle":"bars","lineInterpolation":"smooth","lineWidth":3,"fillOpacity":30,"gradientMode":"opacity","showPoints":"always","pointSize":8,"spanNulls":true,"axisSoftMin":0,"axisSoftMax":100}}`
	panel, line := tableCellPanel(t, custom, "")
	if panel.Style != "bars" || panel.LineInterpolation != "smooth" || panel.Points != "always" || panel.SpanNulls != true {
		t.Fatalf("props = %+v", panel.FieldProps)
	}
	if panel.LineWidth == nil || *panel.LineWidth != 3 || panel.Fill == nil || *panel.Fill != 30 || panel.PointSize == nil || *panel.PointSize != 8 {
		t.Fatalf("sizes = %+v", panel.FieldProps)
	}
	if panel.AxisSoftMin == nil || *panel.AxisSoftMin != 0 || panel.AxisSoftMax == nil || *panel.AxisSoftMax != 100 {
		t.Fatalf("soft limits = %+v", panel.FieldProps)
	}
	if line.Status != statusImported {
		t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
	}
}

func TestTableSparklineGrafanaLookImportsAsTheCanvasLook(t *testing.T) {
	panel, line := tableCellPanel(t, `{"cellOptions":{"type":"sparkline","lineWidth":1,"pointSize":5,"lineInterpolation":"linear","showPoints":"auto","drawStyle":"line"}}`, "")
	if panel.LineWidth != nil || panel.PointSize != nil || panel.LineInterpolation != "" || panel.Points != "" {
		t.Fatalf("props = %+v", panel.FieldProps)
	}
	if line.Status != statusImported {
		t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
	}
}

func TestTableCellOverridesConvertToo(t *testing.T) {
	override := `{"matcher":{"id":"byName","options":"Value"},"properties":[{"id":"custom.cellOptions","value":{"type":"gauge","mode":"lcd"}}]}`
	panel, line := tableCellPanel(t, `{}`, override)
	if len(panel.Overrides) != 1 {
		t.Fatalf("overrides = %+v", panel.Overrides)
	}
	if got := cellJSON(t, panel.Overrides[0].Cell); got != `{"type":"gauge","gaugeStyle":"lcd"}` {
		t.Fatalf("cell = %s", got)
	}
	if line.Status != statusImported {
		t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
	}
}

func TestTableCellOverrideCanResetToAutoAndKeepWholeRowOff(t *testing.T) {
	override := `{"matcher":{"id":"byName","options":"Value"},"properties":[{"id":"custom.cellOptions","value":{"type":"auto"}},{"id":"custom.wrapText","value":false}]}`
	panel, _ := tableCellPanel(t, `{"cellOptions":{"type":"color-background","applyToRow":true}}`, override)
	if got := cellJSON(t, panel.Overrides[0].Cell); got != `{"type":"auto","wrap":false}` {
		t.Fatalf("cell = %s", got)
	}
}

func TestTableCellOverrideLegacyModeAndWrap(t *testing.T) {
	override := `{"matcher":{"id":"byName","options":"Value"},"properties":[{"id":"custom.wrapText","value":true},{"id":"custom.displayMode","value":"color-text"}]}`
	panel, line := tableCellPanel(t, `{}`, override)
	if got := cellJSON(t, panel.Overrides[0].Cell); got != `{"type":"text","wrap":true}` {
		t.Fatalf("cell = %s", got)
	}
	if line.Status != statusImported {
		t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
	}
}

func TestTableCellTypesTheRendererLacksAreReported(t *testing.T) {
	cases := []struct{ name, custom, path, reason string }{
		{"actions", `{"cellOptions":{"type":"actions"}}`, "fieldConfig.defaults.custom.cellOptions", "Cell actions are not supported yet."},
		{"markdown", `{"cellOptions":{"type":"markdown"}}`, "fieldConfig.defaults.custom.cellOptions", "The cell type markdown is not supported yet."},
		{"unknown legacy mode", `{"displayMode":"mystery"}`, "fieldConfig.defaults.custom.displayMode", "The cell type mystery is not supported yet."},
		{"long image alt", `{"cellOptions":{"type":"image","alt":"` + strings.Repeat("a", 201) + `"}}`, "fieldConfig.defaults.custom.cellOptions", "The image alt was cut to 200 characters."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, line := tableCellPanel(t, tc.custom, "")
			if line.Status != statusPartial || !slices.Contains(line.Paths, tc.path) || !slices.Contains(line.Reasons, tc.reason) {
				t.Fatalf("status = %s, paths %v, reasons %v", line.Status, line.Paths, line.Reasons)
			}
		})
	}
}

func TestTableCellOverrideActionsAreReportedAtTheProperty(t *testing.T) {
	override := `{"matcher":{"id":"byName","options":"Value"},"properties":[{"id":"custom.cellOptions","value":{"type":"actions"}}]}`
	panel, line := tableCellPanel(t, `{}`, override)
	if len(panel.Overrides) != 0 {
		t.Fatalf("overrides = %+v", panel.Overrides)
	}
	if !slices.Contains(line.Paths, "fieldConfig.overrides[0].properties[0]") || !slices.Contains(line.Reasons, "Cell actions are not supported yet.") {
		t.Fatalf("paths %v, reasons %v", line.Paths, line.Reasons)
	}
}

func TestTableCellOptionsStayReportedOutsideTables(t *testing.T) {
	panel, line := customPanelOf(t, "timeseries", `{"cellOptions":{"type":"color-text"},"wrapText":true}`, "")
	if panel.Cell != nil {
		t.Fatalf("cell = %+v", panel.Cell)
	}
	if line.Status != statusPartial {
		t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
	}
}

func TestTableThresholdsStayForColoringCells(t *testing.T) {
	thresholds := `"thresholds":{"mode":"absolute","steps":[{"color":"green","value":null},{"color":"red","value":80}]}`
	for _, custom := range []string{
		`{"cellOptions":{"type":"gauge"}}`,
		`{"cellOptions":{"type":"color-background"}}`,
		`{"displayMode":"lcd-gauge"}`,
	} {
		t.Run(custom, func(t *testing.T) {
			panel, _ := convertedPanel(t, panelDoc("table", upTarget+`,"fieldConfig":{"defaults":{"custom":`+custom+","+thresholds+`}}`))
			if len(panel.Thresholds) != 2 {
				t.Fatalf("thresholds = %+v", panel.Thresholds)
			}
		})
	}
}
