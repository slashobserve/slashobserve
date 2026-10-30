package grafana

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
	"github.com/slashobserve/slashobserve/spec/schema"
)

func colorPanel(t *testing.T, typ, color, extra string) (spec.Panel, PanelLine) {
	t.Helper()
	if extra != "" {
		extra = "," + extra
	}
	return convertedPanel(t, panelDoc(typ, upTarget+`,"fieldConfig":{"defaults":{"color":`+color+extra+`},"overrides":[]}`))
}

func colorOverridePanel(t *testing.T, defaults, override string) (spec.Panel, PanelLine) {
	t.Helper()
	overrides := `{"matcher":{"id":"byName","options":"up"},"properties":[{"id":"color","value":` + override + `}]}`
	return convertedPanel(t, panelDoc("timeseries", upTarget+`,"fieldConfig":{"defaults":{"color":`+defaults+`},"overrides":[`+overrides+`]}`))
}

func TestColorModesOnSeriesPanels(t *testing.T) {
	cases := []struct {
		name, typ, color string
		want             spec.FieldProps
	}{
		{"classic palette stays absent", "timeseries", `{"mode":"palette-classic"}`, spec.FieldProps{}},
		{"classic palette by name", "timeseries", `{"mode":"palette-classic-by-name"}`, spec.FieldProps{ColorMode: "paletteByName"}},
		{"classic palette by name on a trend", "trend", `{"mode":"palette-classic-by-name"}`, spec.FieldProps{ColorMode: "paletteByName"}},
		{"thresholds color the series", "timeseries", `{"mode":"thresholds"}`, spec.FieldProps{ColorMode: "thresholds"}},
		{"thresholds by the lowest value", "timeseries", `{"mode":"thresholds","seriesBy":"min"}`, spec.FieldProps{ColorMode: "thresholds", ColorBy: "min"}},
		{"fixed color needs no mode", "timeseries", `{"mode":"fixed","fixedColor":"blue"}`, spec.FieldProps{Color: "#5794F2"}},
		{"shades of a fixed color", "timeseries", `{"mode":"shades","fixedColor":"blue"}`, spec.FieldProps{Color: "#5794F2", ColorMode: "shades"}},
		{"scheme with the last value is the default", "timeseries", `{"mode":"continuous-GrYlRd","seriesBy":"last"}`, spec.FieldProps{ColorMode: "scheme", ColorScheme: "greenYellowRed"}},
		{"scheme by the highest value", "timeseries", `{"mode":"continuous-BlYlRd","seriesBy":"max"}`, spec.FieldProps{ColorMode: "scheme", ColorScheme: "blueYellowRed", ColorBy: "max"}},
		{"scheme by the lowest value on a trend", "trend", `{"mode":"continuous-viridis","seriesBy":"min"}`, spec.FieldProps{ColorMode: "scheme", ColorScheme: "viridis", ColorBy: "min"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := colorPanel(t, tc.typ, tc.color, "")
			requireImported(t, line)
			if panel.Color != tc.want.Color || panel.ColorMode != tc.want.ColorMode || panel.ColorScheme != tc.want.ColorScheme || panel.ColorBy != tc.want.ColorBy {
				t.Fatalf("color props = %q %q %q %q, want %+v", panel.Color, panel.ColorMode, panel.ColorScheme, panel.ColorBy, tc.want)
			}
		})
	}
}

func TestEveryContinuousModeNamesASchemeTheSchemaKnows(t *testing.T) {
	known := schemeNames(t)
	for mode, scheme := range continuousSchemes {
		if !strings.HasPrefix(mode, "continuous-") || !slices.Contains(known, scheme) {
			t.Errorf("%s maps to %q, which the schema does not list", mode, scheme)
		}
		panel, line := colorPanel(t, "timeseries", `{"mode":"`+mode+`"}`, "")
		requireImported(t, line)
		if panel.ColorMode != "scheme" || panel.ColorScheme != scheme {
			t.Errorf("%s imported as %q %q", mode, panel.ColorMode, panel.ColorScheme)
		}
	}
	for name, scheme := range heatmapSchemes {
		if !slices.Contains(known, scheme) {
			t.Errorf("heatmap scheme %s maps to %q, which the schema does not list", name, scheme)
		}
	}
}

func schemeNames(t *testing.T) []string {
	t.Helper()
	var doc struct {
		Defs struct {
			FieldProps struct {
				Properties struct {
					ColorScheme struct {
						Enum []string `json:"enum"`
					} `json:"colorScheme"`
				} `json:"properties"`
			} `json:"fieldProps"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(schema.ObserveSpecV0, &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Defs.FieldProps.Properties.ColorScheme.Enum
}

func TestColorSchemesOnValuePanels(t *testing.T) {
	for _, typ := range []string{"stat", "gauge", "bargauge", "barchart", "xychart"} {
		t.Run(typ, func(t *testing.T) {
			panel, line := colorPanel(t, typ, `{"mode":"continuous-RdYlGr"}`, "")
			requireImported(t, line)
			if panel.ColorMode != "scheme" || panel.ColorScheme != "redYellowGreen" || panel.ColorBy != "" {
				t.Fatalf("color props = %q %q %q", panel.ColorMode, panel.ColorScheme, panel.ColorBy)
			}
		})
	}
}

func TestTableSchemeNeedsColoredCells(t *testing.T) {
	plain, line := colorPanel(t, "table", `{"mode":"continuous-GrYlRd"}`, "")
	requireImported(t, line)
	if plain.ColorMode != "" || plain.ColorScheme != "" {
		t.Fatalf("a table without colored cells kept %q %q", plain.ColorMode, plain.ColorScheme)
	}
	colored, line := convertedPanel(t, panelDoc("table", upTarget+`,"fieldConfig":{"defaults":{"color":{"mode":"continuous-GrYlRd"},"custom":{"cellOptions":{"type":"color-background"}}},"overrides":[]}`))
	if line.Status == statusPlaceholder {
		t.Fatalf("status = %s", line.Status)
	}
	if colored.ColorMode != "scheme" || colored.ColorScheme != "greenYellowRed" {
		t.Fatalf("color props = %q %q", colored.ColorMode, colored.ColorScheme)
	}
}

func TestThresholdColorModeIsSilentWhereValuesAreAlreadyColoredByThem(t *testing.T) {
	for _, typ := range []string{"stat", "gauge", "bargauge", "piechart", "barchart", "xychart", "state-timeline"} {
		t.Run(typ, func(t *testing.T) {
			panel, line := colorPanel(t, typ, `{"mode":"thresholds"}`, "")
			requireImported(t, line)
			if panel.ColorMode != "" {
				t.Fatalf("color mode = %q", panel.ColorMode)
			}
		})
	}
}

func TestColorModesTheRendererDoesNotDrawAreReported(t *testing.T) {
	cases := []struct {
		name, typ, color, reason string
	}{
		{"thresholds on a histogram", "histogram", `{"mode":"thresholds"}`, "The color mode thresholds is not drawn on histogram panels."},
		{"scheme on a state timeline", "state-timeline", `{"mode":"continuous-GrYlRd"}`, "The color mode continuous-GrYlRd is not drawn on state-timeline panels."},
		{"palette by name on a stat", "stat", `{"mode":"palette-classic-by-name"}`, "The color mode palette-classic-by-name is not drawn on stat panels."},
		{"unknown mode", "timeseries", `{"mode":"rainbow"}`, "The color mode rainbow is not supported yet."},
		{"fixed without a color", "timeseries", `{"mode":"fixed"}`, "This fixed color is not supported yet."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, line := colorPanel(t, tc.typ, tc.color, "")
			requireReported(t, line, "fieldConfig.defaults.color", tc.reason)
		})
	}
}

func TestColorOverridesResetOnlyWhatThePanelSet(t *testing.T) {
	cases := []struct {
		name, defaults, override string
		wantMode                 string
		wantColor                string
	}{
		{"classic palette over a scheme", `{"mode":"continuous-GrYlRd"}`, `{"mode":"palette-classic"}`, "palette", ""},
		{"fixed color over a scheme", `{"mode":"continuous-GrYlRd"}`, `{"mode":"fixed","fixedColor":"red"}`, "fixed", "#F2495C"},
		{"fixed color over a fixed color", `{"mode":"fixed","fixedColor":"blue"}`, `{"mode":"fixed","fixedColor":"red"}`, "fixed", "#F2495C"},
		{"fixed color over the classic palette", `{"mode":"palette-classic"}`, `{"mode":"fixed","fixedColor":"red"}`, "", "#F2495C"},
		{"classic palette over the classic palette", `{"mode":"palette-classic"}`, `{"mode":"palette-classic"}`, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := colorOverridePanel(t, tc.defaults, tc.override)
			requireImported(t, line)
			if len(panel.Overrides) != 1 {
				if tc.wantMode == "" && tc.wantColor == "" {
					return
				}
				t.Fatalf("overrides = %+v", panel.Overrides)
			}
			got := panel.Overrides[0]
			if got.ColorMode != tc.wantMode || got.Color != tc.wantColor {
				t.Fatalf("override color props = %q %q, want %q %q", got.ColorMode, got.Color, tc.wantMode, tc.wantColor)
			}
		})
	}
}

func TestAnOverrideResetsColorHideAndStackGroupTogether(t *testing.T) {
	defaults := `{"color":{"mode":"continuous-GrYlRd","seriesBy":"max"},"custom":{"hideFrom":{"legend":true,"tooltip":true,"viz":false},"stacking":{"mode":"normal","group":"B"}}}`
	properties := []string{
		property("color", `{"mode":"palette-classic"}`),
		property("custom.hideFrom", `{"legend":false,"tooltip":false,"viz":false}`),
		property("custom.stacking", `{"mode":"normal","group":"A"}`),
	}
	doc := panelDoc("timeseries", upTarget+`,"fieldConfig":{"defaults":`+defaults+`,"overrides":[{"matcher":`+byUp+`,"properties":[`+strings.Join(properties, ",")+`]}]}`)
	panel, line := convertedPanel(t, doc)
	requireImported(t, line)
	if panel.ColorMode != "scheme" || panel.ColorBy != "max" || panel.StackGroup != "B" {
		t.Fatalf("panel = %q %q %q, want scheme max B", panel.ColorMode, panel.ColorBy, panel.StackGroup)
	}
	got := onlyOverride(t, panel)
	if got.ColorMode != "palette" || got.StackGroup != "A" {
		t.Fatalf("override color mode %q stack group %q, want palette A", got.ColorMode, got.StackGroup)
	}
	if got.HideInLegend == nil || *got.HideInLegend || got.HideInTooltip == nil || *got.HideInTooltip {
		t.Fatalf("override = %+v, want both hide resets written as false", got.FieldProps)
	}
}
