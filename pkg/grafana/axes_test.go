package grafana

import (
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func flagOf(p *bool) string {
	switch {
	case p == nil:
		return "absent"
	case *p:
		return "true"
	}
	return "false"
}

func floatOf(p *float64) float64 {
	if p == nil {
		return -1
	}
	return *p
}

func requireReported(t *testing.T, line PanelLine, path, reason string) {
	t.Helper()
	if line.Status != statusPartial || len(line.Paths) != 1 || line.Paths[0] != path {
		t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
	}
	if !strings.Contains(strings.Join(line.Reasons, " "), reason) {
		t.Fatalf("reasons = %v", line.Reasons)
	}
}

func TestAxesModernOptions(t *testing.T) {
	cases := []struct {
		name, custom string
		check        func(spec.FieldProps) bool
	}{
		{"label", `{"axisLabel":"Requests per second"}`, func(p spec.FieldProps) bool { return p.AxisLabel == "Requests per second" }},
		{"empty label", `{"axisLabel":""}`, func(p spec.FieldProps) bool { return p.AxisLabel == "" }},
		{"soft minimum", `{"axisSoftMin":0}`, func(p spec.FieldProps) bool { return floatOf(p.AxisSoftMin) == 0 && p.AxisSoftMax == nil }},
		{"soft maximum", `{"axisSoftMax":100}`, func(p spec.FieldProps) bool { return floatOf(p.AxisSoftMax) == 100 && p.AxisSoftMin == nil }},
		{"linear scale is the Spec default", `{"scaleDistribution":{"type":"linear"}}`, func(p spec.FieldProps) bool { return p.Scale == "" }},
		{"log scale of base 2", `{"scaleDistribution":{"type":"log","log":2}}`, func(p spec.FieldProps) bool { return p.Scale == "log" && intOf(p.LogBase) == 2 }},
		{"log scale of base 10 is the Spec default", `{"scaleDistribution":{"type":"log","log":10}}`, func(p spec.FieldProps) bool { return p.Scale == "log" && p.LogBase == nil }},
		{"log scale without a base", `{"scaleDistribution":{"type":"log"}}`, func(p spec.FieldProps) bool { return p.Scale == "log" && p.LogBase == nil }},
		{"symlog scale with a linear threshold", `{"scaleDistribution":{"type":"symlog","linearThreshold":10}}`, func(p spec.FieldProps) bool { return p.Scale == "symlog" && floatOf(p.LinearThreshold) == 10 }},
		{"linear scale ignores a stray base", `{"scaleDistribution":{"type":"linear","log":2}}`, func(p spec.FieldProps) bool { return p.Scale == "" && p.LogBase == nil }},
		{"centered zero", `{"axisCenteredZero":true}`, func(p spec.FieldProps) bool { return flagOf(p.AxisCenteredZero) == "true" }},
		{"centered zero off is the Spec default", `{"axisCenteredZero":false}`, func(p spec.FieldProps) bool { return p.AxisCenteredZero == nil }},
		{"width", `{"axisWidth":80}`, func(p spec.FieldProps) bool { return intOf(p.AxisWidth) == 80 }},
		{"width zero is automatic", `{"axisWidth":0}`, func(p spec.FieldProps) bool { return p.AxisWidth == nil }},
		{"grid hidden", `{"axisGridShow":false}`, func(p spec.FieldProps) bool { return flagOf(p.AxisGrid) == "false" }},
		{"grid shown", `{"axisGridShow":true}`, func(p spec.FieldProps) bool { return flagOf(p.AxisGrid) == "true" }},
		{"border", `{"axisBorderShow":true}`, func(p spec.FieldProps) bool { return flagOf(p.AxisBorder) == "true" }},
		{"border off is the Spec default", `{"axisBorderShow":false}`, func(p spec.FieldProps) bool { return p.AxisBorder == nil }},
		{"series color", `{"axisColorMode":"series"}`, func(p spec.FieldProps) bool { return p.AxisColor == "series" }},
		{"text color is the Spec default", `{"axisColorMode":"text"}`, func(p spec.FieldProps) bool { return p.AxisColor == "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := customPanel(t, tc.custom, "")
			requireImported(t, line)
			if !tc.check(panel.FieldProps) {
				t.Fatalf("field props = %+v", panel.FieldProps)
			}
		})
	}
}

func TestAxesUnsupportedAndCutValuesAreReported(t *testing.T) {
	cases := []struct {
		name, custom, path, reason string
	}{
		{"long label", `{"axisLabel":"` + strings.Repeat("a", 120) + `"}`, "fieldConfig.defaults.custom.axisLabel", "The axis label was cut to 100 characters."},
		{"wide axis", `{"axisWidth":900}`, "fieldConfig.defaults.custom.axisWidth", "The axis width was cut to 400 pixels."},
		{"log base 16", `{"scaleDistribution":{"type":"log","log":16}}`, "fieldConfig.defaults.custom.scaleDistribution.log", "Only log base 2 and 10 are supported."},
		{"ordinal scale", `{"scaleDistribution":{"type":"ordinal"}}`, "fieldConfig.defaults.custom.scaleDistribution.type", "The scale type ordinal is not supported yet."},
		{"linear threshold of zero", `{"scaleDistribution":{"type":"symlog","linearThreshold":0}}`, "fieldConfig.defaults.custom.scaleDistribution.linearThreshold", "The linear threshold must be above zero."},
		{"unknown axis color mode", `{"axisColorMode":"scheme"}`, "fieldConfig.defaults.custom.axisColorMode", "The axis color mode scheme is not supported yet."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, line := customPanel(t, tc.custom, "")
			requireReported(t, line, tc.path, tc.reason)
		})
	}
}

func TestAxesHeatmapKeepsItsOwnScaleRule(t *testing.T) {
	panel, line := convertedPanel(t, panelDoc("heatmap", upTarget+`,"fieldConfig":{"defaults":{"custom":{"scaleDistribution":{"type":"log","log":2}}},"overrides":[]}`))
	requireImported(t, line)
	if panel.Scale != "" || panel.Display == nil || panel.Display.YScale != "log" {
		t.Fatalf("field props = %+v, display = %+v", panel.FieldProps, panel.Display)
	}
}

func TestAxesOverridesWriteExplicitValues(t *testing.T) {
	override := func(properties string) string {
		return `{"matcher":{"id":"byName","options":"a"},"properties":[` + properties + `]}`
	}
	cases := []struct {
		name, properties string
		check            func(spec.FieldProps) bool
	}{
		{"label", `{"id":"custom.axisLabel","value":"Latency"}`, func(p spec.FieldProps) bool { return p.AxisLabel == "Latency" }},
		{"soft limits", `{"id":"custom.axisSoftMin","value":0},{"id":"custom.axisSoftMax","value":50}`, func(p spec.FieldProps) bool { return floatOf(p.AxisSoftMin) == 0 && floatOf(p.AxisSoftMax) == 50 }},
		{"linear scale resets a log panel", `{"id":"custom.scaleDistribution","value":{"type":"linear"}}`, func(p spec.FieldProps) bool { return p.Scale == "linear" }},
		{"log base 10 resets a base 2 panel", `{"id":"custom.scaleDistribution","value":{"type":"log","log":10}}`, func(p spec.FieldProps) bool { return p.Scale == "log" && intOf(p.LogBase) == 10 }},
		{"centered zero off", `{"id":"custom.axisCenteredZero","value":false}`, func(p spec.FieldProps) bool { return flagOf(p.AxisCenteredZero) == "false" }},
		{"width", `{"id":"custom.axisWidth","value":60}`, func(p spec.FieldProps) bool { return intOf(p.AxisWidth) == 60 }},
		{"grid", `{"id":"custom.axisGridShow","value":false}`, func(p spec.FieldProps) bool { return flagOf(p.AxisGrid) == "false" }},
		{"border off", `{"id":"custom.axisBorderShow","value":false}`, func(p spec.FieldProps) bool { return flagOf(p.AxisBorder) == "false" }},
		{"text color", `{"id":"custom.axisColorMode","value":"text"}`, func(p spec.FieldProps) bool { return p.AxisColor == "text" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := customPanel(t, `{}`, override(tc.properties))
			requireImported(t, line)
			if len(panel.Overrides) != 1 || !tc.check(panel.Overrides[0].FieldProps) {
				t.Fatalf("overrides = %+v", panel.Overrides)
			}
		})
	}
}

func timeAxisOverride(properties string) string {
	return `{"matcher":{"id":"byType","options":"time"},"properties":[` + properties + `]}`
}

const hideAxis = `{"id":"custom.axisPlacement","value":"hidden"}`

func TestAxesTimeAxisOverrideHidesTheXAxis(t *testing.T) {
	for _, typ := range []string{"timeseries", "trend", "candlestick"} {
		t.Run(typ, func(t *testing.T) {
			doc := panelDoc(typ, upTarget+`,"fieldConfig":{"defaults":{},"overrides":[`+timeAxisOverride(hideAxis)+`]}`)
			panel, line := convertedPanel(t, doc)
			requireImported(t, line)
			if panel.Display == nil || panel.Display.XAxis != "hidden" {
				t.Fatalf("display = %+v", panel.Display)
			}
			if len(panel.Overrides) != 0 {
				t.Fatalf("overrides = %+v", panel.Overrides)
			}
		})
	}
}

func TestAxesTimeAxisOverrideWithOtherPropertiesStaysAnOverride(t *testing.T) {
	doc := panelDoc("timeseries", upTarget+`,"fieldConfig":{"defaults":{},"overrides":[`+timeAxisOverride(hideAxis+`,{"id":"custom.axisLabel","value":"Time"}`)+`]}`)
	panel, line := convertedPanel(t, doc)
	requireImported(t, line)
	if panel.Display != nil && panel.Display.XAxis != "" {
		t.Fatalf("display = %+v", panel.Display)
	}
	if len(panel.Overrides) != 1 || panel.Overrides[0].Match.FieldType != "time" || panel.Overrides[0].AxisLabel != "Time" {
		t.Fatalf("overrides = %+v", panel.Overrides)
	}
}

func TestAxesTimeAxisOverrideShownKeepsTheXAxis(t *testing.T) {
	doc := panelDoc("timeseries", upTarget+`,"fieldConfig":{"defaults":{},"overrides":[`+timeAxisOverride(`{"id":"custom.axisPlacement","value":"bottom"}`)+`]}`)
	panel, _ := convertedPanel(t, doc)
	if panel.Display != nil && panel.Display.XAxis != "" {
		t.Fatalf("display = %+v", panel.Display)
	}
}

func TestAxesLegacyLabelsOnBothAxes(t *testing.T) {
	panel, line := graphPanel(t, `"yaxes":[{"format":"short","label":"Requests","logBase":1},{"format":"ms","label":"Latency","logBase":1}],"seriesOverrides":[{"alias":"b","yaxis":2}]`)
	requireImported(t, line)
	if panel.AxisLabel != "Requests" {
		t.Fatalf("panel label = %q", panel.AxisLabel)
	}
	if len(panel.Overrides) != 1 {
		t.Fatalf("overrides = %+v", panel.Overrides)
	}
	right := panel.Overrides[0]
	if right.Match.Series != "b" || right.Axis != "right" || right.AxisLabel != "Latency" {
		t.Fatalf("right axis override = %+v", right)
	}
}

func TestAxesLegacyLabelIgnoredOnUnusedSecondAxis(t *testing.T) {
	panel, line := graphPanel(t, `"yaxes":[{"format":"short","label":"Requests"},{"format":"ms","label":"Unused"}]`)
	requireImported(t, line)
	if panel.AxisLabel != "Requests" || len(panel.Overrides) != 0 {
		t.Fatalf("panel = %+v", panel.FieldProps)
	}
}

func TestAxesLegacyLogBase(t *testing.T) {
	cases := []struct {
		name, yaxes string
		check       func(spec.FieldProps) bool
	}{
		{"linear", `{"logBase":1}`, func(p spec.FieldProps) bool { return p.Scale == "" }},
		{"base 10", `{"logBase":10}`, func(p spec.FieldProps) bool { return p.Scale == "log" && p.LogBase == nil }},
		{"base 2", `{"logBase":2}`, func(p spec.FieldProps) bool { return p.Scale == "log" && intOf(p.LogBase) == 2 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := graphPanel(t, `"yaxes":[`+tc.yaxes+`]`)
			requireImported(t, line)
			if !tc.check(panel.FieldProps) {
				t.Fatalf("field props = %+v", panel.FieldProps)
			}
		})
	}
}

func TestAxesLegacyLogBaseSixteenIsReported(t *testing.T) {
	panel, line := graphPanel(t, `"yaxes":[{"logBase":16}]`)
	requireReported(t, line, "yaxes[0].logBase", "Only log base 2 and 10 are supported.")
	if panel.Scale != "" {
		t.Fatalf("scale = %q", panel.Scale)
	}
}

func TestAxesLegacySecondAxisScale(t *testing.T) {
	panel, line := graphPanel(t, `"yaxes":[{"logBase":10},{"logBase":1}],"seriesOverrides":[{"alias":"b","yaxis":2}]`)
	requireImported(t, line)
	if panel.Scale != "log" || len(panel.Overrides) != 1 || panel.Overrides[0].Scale != "linear" {
		t.Fatalf("panel = %+v, overrides = %+v", panel.FieldProps, panel.Overrides)
	}
}

func TestAxesLegacyHiddenXAxis(t *testing.T) {
	panel, line := graphPanel(t, `"xaxis":{"show":false,"mode":"time"}`)
	requireImported(t, line)
	if panel.Display == nil || panel.Display.XAxis != "hidden" {
		t.Fatalf("display = %+v", panel.Display)
	}
	shown, line := graphPanel(t, `"xaxis":{"show":true,"mode":"time"}`)
	requireImported(t, line)
	if shown.Display != nil && shown.Display.XAxis != "" {
		t.Fatalf("display = %+v", shown.Display)
	}
}

var plotOnlyOptions = []struct {
	name, custom string
	paths        []string
}{
	{"axis label", `{"axisLabel":"text"}`, []string{"fieldConfig.defaults.custom.axisLabel"}},
	{"soft minimum", `{"axisSoftMin":5}`, []string{"fieldConfig.defaults.custom.axisSoftMin"}},
	{"soft maximum", `{"axisSoftMax":100}`, []string{"fieldConfig.defaults.custom.axisSoftMax"}},
	{"log scale", `{"scaleDistribution":{"type":"log","log":2}}`, []string{"fieldConfig.defaults.custom.scaleDistribution.log", "fieldConfig.defaults.custom.scaleDistribution.type"}},
	{"centered zero", `{"axisCenteredZero":true}`, []string{"fieldConfig.defaults.custom.axisCenteredZero"}},
	{"axis width", `{"axisWidth":80}`, []string{"fieldConfig.defaults.custom.axisWidth"}},
	{"hidden grid", `{"axisGridShow":false}`, []string{"fieldConfig.defaults.custom.axisGridShow"}},
	{"axis border", `{"axisBorderShow":true}`, []string{"fieldConfig.defaults.custom.axisBorderShow"}},
	{"series axis color", `{"axisColorMode":"series"}`, []string{"fieldConfig.defaults.custom.axisColorMode"}},
	{"smooth line", `{"lineInterpolation":"smooth"}`, []string{"fieldConfig.defaults.custom.lineInterpolation"}},
	{"line width", `{"lineWidth":4}`, []string{"fieldConfig.defaults.custom.lineWidth"}},
	{"points", `{"showPoints":"always"}`, []string{"fieldConfig.defaults.custom.showPoints"}},
	{"point size", `{"pointSize":9}`, []string{"fieldConfig.defaults.custom.pointSize"}},
	{"span nulls", `{"spanNulls":true}`, []string{"fieldConfig.defaults.custom.spanNulls"}},
}

var timelineDrawnOptions = map[string]bool{"line width": true, "span nulls": true}

var axisDrawnOptions = map[string]bool{
	"axis label": true, "soft minimum": true, "soft maximum": true, "log scale": true, "centered zero": true,
	"axis width": true, "hidden grid": true, "axis border": true, "series axis color": true, "line width": true,
}

func TestAxisAndLineStyleOptionsAreReportedOnPanelsThatDrawNeither(t *testing.T) {
	for _, typ := range []string{"barchart", "histogram", "state-timeline", "status-history", "xychart"} {
		for _, tc := range plotOnlyOptions {
			if timelineDrawnOptions[tc.name] && (typ == "state-timeline" || typ == "status-history") {
				continue
			}
			if axisDrawnOptions[tc.name] && (typ == "barchart" || typ == "histogram" || typ == "xychart") {
				continue
			}
			t.Run(typ+" "+tc.name, func(t *testing.T) {
				panel, line := customPanelOf(t, typ, tc.custom, "")
				if line.Status != statusPartial || strings.Join(line.Paths, " ") != strings.Join(tc.paths, " ") {
					t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
				}
				if !fieldPropsEmpty(panel.FieldProps) {
					t.Fatalf("field props = %+v", panel.FieldProps)
				}
			})
		}
	}
}

func TestAxisAndLineStyleOverridePropertiesAreReportedOnPanelsThatDrawNeither(t *testing.T) {
	override := `{"matcher":{"id":"byName","options":"a"},"properties":[{"id":"custom.showPoints","value":"never"},{"id":"custom.spanNulls","value":true}]}`
	panel, line := customPanelOf(t, "histogram", `{}`, override)
	if line.Status != statusPartial || len(panel.Overrides) != 0 {
		t.Fatalf("status = %s, paths %v, overrides %+v", line.Status, line.Paths, panel.Overrides)
	}
	want := "fieldConfig.overrides[0].properties[0] fieldConfig.overrides[0].properties[1]"
	if strings.Join(line.Paths, " ") != want {
		t.Fatalf("paths = %v", line.Paths)
	}
}

func TestAxisAndLineStyleOptionsConvertOnPanelsThatDrawThem(t *testing.T) {
	for _, typ := range []string{"timeseries", "trend", "candlestick"} {
		t.Run(typ, func(t *testing.T) {
			panel, line := customPanelOf(t, typ, `{"axisLabel":"Requests","scaleDistribution":{"type":"log","log":2},"lineWidth":4,"spanNulls":true}`, "")
			requireImported(t, line)
			if panel.AxisLabel != "Requests" || panel.Scale != "log" || intOf(panel.LogBase) != 2 || intOf(panel.LineWidth) != 4 || panel.SpanNulls != true {
				t.Fatalf("field props = %+v", panel.FieldProps)
			}
		})
	}
}

var lineStyleOptions = plotOnlyOptions[9:]

func TestLineStyleOptionsAreSilentOnPanelsGrafanaNeverDrawsThemOn(t *testing.T) {
	for _, typ := range []string{"stat", "gauge", "bargauge", "table", "piechart"} {
		for _, tc := range lineStyleOptions {
			t.Run(typ+" "+tc.name, func(t *testing.T) {
				_, line := customPanelOf(t, typ, tc.custom, "")
				requireImported(t, line)
			})
		}
	}
}

func TestLineStyleOverridePropertiesAreSilentOnPanelsGrafanaNeverDrawsThemOn(t *testing.T) {
	override := `{"matcher":{"id":"byName","options":"a"},"properties":[{"id":"custom.lineWidth","value":4},{"id":"custom.showPoints","value":"never"}]}`
	_, line := customPanelOf(t, "stat", `{}`, override)
	requireImported(t, line)
}

func TestAxisOptionsAreStillReportedOnPanelsThatDrawNoAxis(t *testing.T) {
	_, line := customPanelOf(t, "stat", `{"axisLabel":"Requests"}`, "")
	if line.Status != statusPartial || strings.Join(line.Paths, " ") != "fieldConfig.defaults.custom.axisLabel" {
		t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
	}
}
