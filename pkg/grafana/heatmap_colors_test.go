package grafana

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
	"github.com/slashobserve/slashobserve/spec/schema"
)

func heatmapOptions(t *testing.T, options string) (spec.Display, PanelLine) {
	t.Helper()
	panel, line := convertedPanel(t, panelDoc("heatmap", upTarget+`,"options":`+options))
	return displayOf(t, panel), line
}

func TestHeatmapWithoutOptionsImportsGrafanasDefaults(t *testing.T) {
	d, line := heatmapOptions(t, `{}`)
	requireImported(t, line)
	if d.ColorMode != "scheme" || d.ColorScheme != "oranges" || d.ColorSteps != 64 || d.CellGap != 1 || d.ShowValues != "auto" {
		t.Fatalf("display = %+v", d)
	}
	if d.ColorScale != "" || d.ColorFill != "" || d.Legend != "" || d.HideBelow != nil || d.HideAbove != nil || d.CellRadius != 0 {
		t.Fatalf("display = %+v", d)
	}
}

func TestHeatmapColorMode(t *testing.T) {
	cases := []struct {
		name, color string
		check       func(spec.Display) bool
	}{
		{"scheme mode", `{"mode":"scheme","scheme":"Spectral"}`, func(d spec.Display) bool {
			return d.ColorMode == "scheme" && d.ColorScheme == "redYellowGreen" && d.ColorFill == ""
		}},
		{"a scheme equal to the Spec default is not written", `{"mode":"scheme","scheme":"Blues"}`, func(d spec.Display) bool {
			return d.ColorMode == "scheme" && d.ColorScheme == ""
		}},
		{"opacity mode is the Spec default and takes Grafana's fill and scale", `{"mode":"opacity"}`, func(d spec.Display) bool {
			return d.ColorMode == "" && d.ColorScheme == "" && d.ColorFill == "warning" && d.ColorScale == "exponential"
		}},
		{"opacity mode takes the written fill", `{"mode":"opacity","fill":"red"}`, func(d spec.Display) bool {
			return d.ColorFill == "critical"
		}},
		{"opacity mode keeps a literal hex fill", `{"mode":"opacity","fill":"#112233"}`, func(d spec.Display) bool {
			return d.ColorFill == "#112233"
		}},
		{"the default exponent is not written", `{"mode":"opacity","scale":"exponential","exponent":0.5}`, func(d spec.Display) bool {
			return d.ColorScale == "exponential" && d.ColorExponent == nil
		}},
		{"another exponent is written", `{"mode":"opacity","scale":"exponential","exponent":0.8}`, func(d spec.Display) bool {
			return d.ColorExponent != nil && *d.ColorExponent == 0.8
		}},
		{"a linear scale is the Spec default and drops the exponent", `{"mode":"opacity","scale":"linear","exponent":0.8}`, func(d spec.Display) bool {
			return d.ColorScale == "" && d.ColorExponent == nil
		}},
		{"scheme mode ignores fill, scale and exponent as Grafana does", `{"mode":"scheme","fill":"red","scale":"linear","exponent":0.8}`, func(d spec.Display) bool {
			return d.ColorFill == "" && d.ColorScale == "" && d.ColorExponent == nil
		}},
		{"opacity mode ignores the scheme", `{"mode":"opacity","scheme":"Spectral"}`, func(d spec.Display) bool {
			return d.ColorScheme == ""
		}},
		{"steps", `{"steps":32}`, func(d spec.Display) bool { return d.ColorSteps == 32 }},
		{"min, max and reverse", `{"min":0,"max":100,"reverse":true}`, func(d spec.Display) bool {
			return d.ColorMin != nil && *d.ColorMin == 0 && d.ColorMax != nil && *d.ColorMax == 100 && d.ColorReverse != nil && *d.ColorReverse
		}},
		{"reverse false is the default", `{"reverse":false}`, func(d spec.Display) bool { return d.ColorReverse == nil }},
		{"min and max may be strings", `{"min":"2","max":"9.5"}`, func(d spec.Display) bool {
			return d.ColorMin != nil && *d.ColorMin == 2 && d.ColorMax != nil && *d.ColorMax == 9.5
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, line := heatmapOptions(t, `{"color":`+tc.color+`}`)
			requireImported(t, line)
			if !tc.check(d) {
				t.Fatalf("display = %+v", d)
			}
		})
	}
}

func TestHeatmapColorsTheRendererCannotDrawAreReported(t *testing.T) {
	cases := []struct {
		name, color, path string
		check             func(spec.Display) bool
	}{
		{"a scheme with no token ramp", `{"scheme":"Warm"}`, "options.color.scheme", func(d spec.Display) bool {
			return d.ColorMode == "scheme" && d.ColorScheme == ""
		}},
		{"a fill with no token hue", `{"mode":"opacity","fill":"purple"}`, "options.color.fill", func(d spec.Display) bool {
			return d.ColorFill == ""
		}},
		{"too few steps", `{"steps":1}`, "options.color.steps", func(d spec.Display) bool { return d.ColorSteps == 0 }},
		{"too many steps", `{"steps":129}`, "options.color.steps", func(d spec.Display) bool { return d.ColorSteps == 0 }},
		{"a fractional step count", `{"steps":2.5}`, "options.color.steps", func(d spec.Display) bool { return d.ColorSteps == 0 }},
		{"an exponent out of range", `{"mode":"opacity","exponent":3}`, "options.color.exponent", func(d spec.Display) bool {
			return d.ColorExponent == nil
		}},
		{"a min that is not a number", `{"min":"low"}`, "options.color.min", func(d spec.Display) bool { return d.ColorMin == nil }},
		{"a mode Grafana does not have", `{"mode":"hue"}`, "options.color.mode", func(d spec.Display) bool {
			return d.ColorMode == "scheme"
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, line := heatmapOptions(t, `{"color":`+tc.color+`}`)
			if line.Status != statusPartial || !slices.Contains(line.Paths, tc.path) {
				t.Fatalf("status %s, paths %v", line.Status, line.Paths)
			}
			if !tc.check(d) {
				t.Fatalf("display = %+v", d)
			}
		})
	}
}

func TestHeatmapCells(t *testing.T) {
	cases := []struct {
		name, options string
		check         func(spec.Display) bool
	}{
		{"a gap", `{"cellGap":3}`, func(d spec.Display) bool { return d.CellGap == 3 }},
		{"no gap is the Spec default", `{"cellGap":0}`, func(d spec.Display) bool { return d.CellGap == 0 }},
		{"no radius is the default", `{"cellRadius":0}`, func(d spec.Display) bool { return d.CellRadius == 0 }},
		{"a radius", `{"cellRadius":4}`, func(d spec.Display) bool { return d.CellRadius == 4 }},
		{"a cell value unit", `{"cellValues":{"unit":"ms"}}`, func(d spec.Display) bool { return d.CellUnit == "ms" }},
		{"an unknown cell value unit becomes a suffix", `{"cellValues":{"unit":"reqs"}}`, func(d spec.Display) bool {
			return d.CellUnit == "suffix:reqs"
		}},
		{"cell value decimals", `{"cellValues":{"decimals":2}}`, func(d spec.Display) bool {
			return d.CellDecimals != nil && *d.CellDecimals == 2
		}},
		{"zero decimals are kept", `{"cellValues":{"decimals":0}}`, func(d spec.Display) bool {
			return d.CellDecimals != nil && *d.CellDecimals == 0
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, line := heatmapOptions(t, tc.options)
			requireImported(t, line)
			if !tc.check(d) {
				t.Fatalf("display = %+v", d)
			}
		})
	}
}

func TestHeatmapCellSizesOutOfRangeAreReported(t *testing.T) {
	cases := []struct{ name, options, path string }{
		{"gap", `{"cellGap":26}`, "options.cellGap"},
		{"fractional gap", `{"cellGap":1.5}`, "options.cellGap"},
		{"radius", `{"cellRadius":11}`, "options.cellRadius"},
		{"decimals", `{"cellValues":{"decimals":11}}`, "options.cellValues.decimals"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, line := heatmapOptions(t, tc.options)
			if line.Status != statusPartial || !slices.Contains(line.Paths, tc.path) {
				t.Fatalf("status %s, paths %v", line.Status, line.Paths)
			}
			if d.CellRadius != 0 || d.CellDecimals != nil || d.CellGap > 1 {
				t.Fatalf("display = %+v", d)
			}
		})
	}
}

func TestHeatmapValueFilters(t *testing.T) {
	cases := []struct {
		name, filter string
		below, above *float64
	}{
		{"Grafana's own low filter is the Spec default", `{"le":1e-9}`, nil, nil},
		{"a zero low filter is the Spec default", `{"le":0}`, nil, nil},
		{"a low filter", `{"le":0.1}`, ptr(0.1), nil},
		{"a high filter", `{"ge":0.2}`, nil, ptr(0.2)},
		{"both", `{"le":5,"ge":50}`, ptr(5.0), ptr(50.0)},
		{"a negative low filter draws zeros", `{"le":-1}`, ptr(-1.0), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, line := heatmapOptions(t, `{"filterValues":`+tc.filter+`}`)
			requireImported(t, line)
			if !sameNumber(d.HideBelow, tc.below) || !sameNumber(d.HideAbove, tc.above) {
				t.Fatalf("hideBelow %v, hideAbove %v", d.HideBelow, d.HideAbove)
			}
		})
	}
}

func TestHeatmapLegendAndCellValues(t *testing.T) {
	cases := []struct {
		name, options, legend, show string
	}{
		{"the legend is shown by default", `{"legend":{"show":true}}`, "", "auto"},
		{"a hidden legend", `{"legend":{"show":false}}`, "hidden", "auto"},
		{"values always", `{"showValue":"always"}`, "", "always"},
		{"values never is the Spec default", `{"showValue":"never"}`, "", ""},
		{"values auto is written because the Spec default is never", `{"showValue":"auto"}`, "", "auto"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, line := heatmapOptions(t, tc.options)
			requireImported(t, line)
			if d.Legend != tc.legend || d.ShowValues != tc.show {
				t.Fatalf("legend %q, showValues %q", d.Legend, d.ShowValues)
			}
		})
	}
}

func TestLegacyHeatmapKeepsItsOwnOptions(t *testing.T) {
	doc := strings.Replace(panelDoc("heatmap", upTarget+`,"dataFormat":"tsbuckets"`), `"schemaVersion":39`, `"schemaVersion":30`, 1)
	panel, line := convertedPanel(t, doc)
	if line.Status != statusPartial || !slices.Contains(line.Paths, "dataFormat") {
		t.Fatalf("status %s, paths %v", line.Status, line.Paths)
	}
	if panel.Display != nil {
		t.Fatalf("display = %+v", *panel.Display)
	}
}

func sameNumber(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func TestHeatmapSchemesAreTheFieldSchemes(t *testing.T) {
	var doc struct {
		Defs struct {
			DataPanel struct {
				AllOf []struct {
					If struct {
						Properties struct {
							Type struct {
								Const string `json:"const"`
							} `json:"type"`
						} `json:"properties"`
					} `json:"if"`
					Then struct {
						Properties struct {
							Display struct {
								Properties struct {
									ColorScheme struct {
										Enum []string `json:"enum"`
									} `json:"colorScheme"`
								} `json:"properties"`
							} `json:"display"`
						} `json:"properties"`
					} `json:"then"`
				} `json:"allOf"`
			} `json:"dataPanel"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(schema.ObserveSpecV0, &doc); err != nil {
		t.Fatal(err)
	}
	var heatmap []string
	for _, branch := range doc.Defs.DataPanel.AllOf {
		if branch.If.Properties.Type.Const == "heatmap" {
			heatmap = branch.Then.Properties.Display.Properties.ColorScheme.Enum
		}
	}
	if len(heatmap) == 0 || !slices.Equal(heatmap, schemeNames(t)) {
		t.Fatalf("heatmap schemes %v, field schemes %v", heatmap, schemeNames(t))
	}
}
