package grafana

import (
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func overrideOf(properties string) string {
	return `{"matcher":{"id":"byName","options":"a"},"properties":[` + properties + `]}`
}

func firstOverrideProps(t *testing.T, panel spec.Panel) spec.FieldProps {
	t.Helper()
	if len(panel.Overrides) != 1 {
		t.Fatalf("overrides = %+v", panel.Overrides)
	}
	return panel.Overrides[0].FieldProps
}

func TestInsertNullsConvertsOnEveryDrawingType(t *testing.T) {
	cases := []struct {
		name, custom string
		want         any
	}{
		{"true is the query interval", `{"insertNulls":true}`, true},
		{"a number is milliseconds", `{"insertNulls":60000}`, float64(60000)},
		{"false is Grafana's default", `{"insertNulls":false}`, nil},
		{"zero is Grafana's default", `{"insertNulls":0}`, nil},
	}
	for _, typ := range []string{"timeseries", "trend", "candlestick", "state-timeline", "status-history"} {
		for _, tc := range cases {
			t.Run(typ+" "+tc.name, func(t *testing.T) {
				panel, line := customPanelOf(t, typ, tc.custom, "")
				requireImported(t, line)
				if panel.InsertNulls != tc.want {
					t.Fatalf("insertNulls = %v, want %v", panel.InsertNulls, tc.want)
				}
			})
		}
	}
}

func TestInsertNullsTrueInAnOverrideConverts(t *testing.T) {
	panel, line := customPanel(t, `{}`, overrideOf(`{"id":"custom.insertNulls","value":true}`))
	requireImported(t, line)
	if got := firstOverrideProps(t, panel); got.InsertNulls != true {
		t.Fatalf("override props = %+v", got)
	}
}

func TestInsertNullsOverrideSetsTheGap(t *testing.T) {
	panel, line := customPanel(t, `{}`, overrideOf(`{"id":"custom.insertNulls","value":30000}`))
	requireImported(t, line)
	if got := firstOverrideProps(t, panel); got.InsertNulls != float64(30000) {
		t.Fatalf("override props = %+v", got)
	}
}

func TestInsertNullsOverrideCannotTurnOffThePanelsNulls(t *testing.T) {
	_, line := customPanel(t, `{"insertNulls":60000}`, overrideOf(`{"id":"custom.insertNulls","value":false}`))
	if line.Status != statusPartial {
		t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
	}
}

func TestInsertNullsOverrideCannotTurnOffTheQueryIntervalNulls(t *testing.T) {
	_, line := customPanel(t, `{"insertNulls":true}`, overrideOf(`{"id":"custom.insertNulls","value":false}`))
	if line.Status != statusPartial {
		t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
	}
}

func TestPlotOnlyPropsAreReportedOnBarChartAndHistogram(t *testing.T) {
	customs := map[string]string{
		"insertNulls":    `{"insertNulls":60000}`,
		"barAlignment":   `{"barAlignment":1}`,
		"barWidthFactor": `{"barWidthFactor":0.8}`,
		"barMaxWidth":    `{"barMaxWidth":20}`,
	}
	for _, typ := range []string{"barchart", "histogram"} {
		for name, custom := range customs {
			t.Run(typ+" "+name, func(t *testing.T) {
				_, line := customPanelOf(t, typ, custom, "")
				if line.Status != statusPartial || len(line.Paths) != 1 {
					t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
				}
			})
		}
	}
}

func TestBarPropsConvertWhenBarsAreDrawn(t *testing.T) {
	cases := []struct {
		name, custom string
		check        func(spec.FieldProps) bool
	}{
		{"alignment before", `{"drawStyle":"bars","barAlignment":-1}`, func(p spec.FieldProps) bool { return p.BarAlignment == "before" }},
		{"alignment after", `{"drawStyle":"bars","barAlignment":1}`, func(p spec.FieldProps) bool { return p.BarAlignment == "after" }},
		{"alignment center is the default", `{"drawStyle":"bars","barAlignment":0}`, func(p spec.FieldProps) bool { return p.BarAlignment == "" }},
		{"width factor", `{"drawStyle":"bars","barWidthFactor":0.8}`, func(p spec.FieldProps) bool { return floatOf(p.BarWidthFactor) == 0.8 }},
		{"width factor is clamped", `{"drawStyle":"bars","barWidthFactor":0.001}`, func(p spec.FieldProps) bool { return floatOf(p.BarWidthFactor) == 0.05 }},
		{"canvas width factor is the default", `{"drawStyle":"bars","barWidthFactor":0.6}`, func(p spec.FieldProps) bool { return p.BarWidthFactor == nil }},
		{"max width", `{"drawStyle":"bars","barMaxWidth":20}`, func(p spec.FieldProps) bool { return p.BarMaxWidth != nil && *p.BarMaxWidth == 20 }},
		{"max width is clamped", `{"drawStyle":"bars","barMaxWidth":900}`, func(p spec.FieldProps) bool { return p.BarMaxWidth != nil && *p.BarMaxWidth == 200 }},
		{"canvas max width is the default", `{"drawStyle":"bars","barMaxWidth":64}`, func(p spec.FieldProps) bool { return p.BarMaxWidth == nil }},
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

func TestBarPropsAreDroppedWhenNoBarsAreDrawn(t *testing.T) {
	panel, line := customPanel(t, `{"drawStyle":"line","barAlignment":1,"barWidthFactor":0.8,"barMaxWidth":20}`, "")
	requireImported(t, line)
	if !fieldPropsEmpty(panel.FieldProps) {
		t.Fatalf("field props = %+v", panel.FieldProps)
	}
}

func TestBarPropsAreReportedOrIgnoredOnTypesThatDrawNone(t *testing.T) {
	panel, _ := customPanelOf(t, "stat", `{"barAlignment":1,"barWidthFactor":0.8}`, "")
	if panel.BarAlignment != "" || panel.BarWidthFactor != nil {
		t.Fatalf("field props = %+v", panel.FieldProps)
	}
}

func TestBarPropsOverrideWritesExplicitValues(t *testing.T) {
	properties := `{"id":"custom.drawStyle","value":"bars"},{"id":"custom.barAlignment","value":0},{"id":"custom.barWidthFactor","value":0.6},{"id":"custom.barMaxWidth","value":64}`
	panel, line := customPanel(t, `{"drawStyle":"bars","barAlignment":1,"barWidthFactor":0.9,"barMaxWidth":30}`, overrideOf(properties))
	requireImported(t, line)
	got := firstOverrideProps(t, panel)
	if got.BarAlignment != "center" || floatOf(got.BarWidthFactor) != 0.6 || got.BarMaxWidth == nil || *got.BarMaxWidth != 64 {
		t.Fatalf("override props = %+v", got)
	}
}

func TestThresholdsStyleModes(t *testing.T) {
	for _, mode := range []string{"off", "line", "dashed", "area", "line+area", "dashed+area", "series"} {
		t.Run(mode, func(t *testing.T) {
			panel, line := customPanel(t, `{"thresholdsStyle":{"mode":"`+mode+`"}}`, "")
			requireImported(t, line)
			if panel.ThresholdsStyle != mode {
				t.Fatalf("field props = %+v", panel.FieldProps)
			}
		})
	}
}

func TestThresholdsStyleUnknownModeIsReported(t *testing.T) {
	_, line := customPanel(t, `{"thresholdsStyle":{"mode":"glow"}}`, "")
	if line.Status != statusPartial || len(line.Paths) != 1 || line.Paths[0] != "fieldConfig.defaults.custom.thresholdsStyle.mode" {
		t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
	}
}

func TestThresholdsStyleOverride(t *testing.T) {
	panel, line := customPanel(t, `{}`, overrideOf(`{"id":"custom.thresholdsStyle","value":{"mode":"area"}}`))
	requireImported(t, line)
	if got := firstOverrideProps(t, panel); got.ThresholdsStyle != "area" {
		t.Fatalf("override props = %+v", got)
	}
}

func TestThresholdsStyleOnTypesThatDrawNoneStaysSilentOrReported(t *testing.T) {
	panel, _ := customPanelOf(t, "stat", `{"thresholdsStyle":{"mode":"area"}}`, "")
	if panel.ThresholdsStyle != "" {
		t.Fatalf("field props = %+v", panel.FieldProps)
	}
}

func TestNullPointModeNullAsZeroConverts(t *testing.T) {
	panel, line := graphPanel(t, `"nullPointMode":"null as zero"`)
	requireImported(t, line)
	if panel.NullAsZero == nil || !*panel.NullAsZero {
		t.Fatalf("field props = %+v", panel.FieldProps)
	}
}

func TestSeriesOverrideNullAsZeroConverts(t *testing.T) {
	panel, line := graphPanel(t, `"seriesOverrides":[{"alias":"a","nullPointMode":"null as zero"}]`)
	requireImported(t, line)
	got := firstOverrideProps(t, panel)
	if got.NullAsZero == nil || !*got.NullAsZero {
		t.Fatalf("override props = %+v", got)
	}
}

func TestSeriesOverrideResetsThePanelsNullAsZero(t *testing.T) {
	for _, mode := range []string{"connected", "null"} {
		t.Run(mode, func(t *testing.T) {
			panel, line := graphPanel(t, `"nullPointMode":"null as zero","seriesOverrides":[{"alias":"a","nullPointMode":"`+mode+`"}]`)
			requireImported(t, line)
			got := firstOverrideProps(t, panel)
			if got.NullAsZero == nil || *got.NullAsZero {
				t.Fatalf("override props = %+v", got)
			}
		})
	}
}

func TestSeriesOverrideWithoutPanelNullAsZeroWritesNoReset(t *testing.T) {
	panel, line := graphPanel(t, `"seriesOverrides":[{"alias":"a","nullPointMode":"null"}]`)
	requireImported(t, line)
	if got := firstOverrideProps(t, panel); got.NullAsZero != nil {
		t.Fatalf("override props = %+v", got)
	}
}

func candlestickPanel(t *testing.T, options string) (spec.Panel, PanelLine) {
	t.Helper()
	return convertedPanel(t, panelDoc("candlestick", upTarget+`,"options":{`+options+`}`))
}

func TestCandlestickIncludeAllFields(t *testing.T) {
	panel, line := candlestickPanel(t, `"includeAllFields":true`)
	requireImported(t, line)
	if panel.Display == nil || panel.Display.IncludeAllFields == nil || !*panel.Display.IncludeAllFields {
		t.Fatalf("display = %+v", panel.Display)
	}
}

func TestCandlestickIncludeAllFieldsOffIsSilent(t *testing.T) {
	panel, line := candlestickPanel(t, `"includeAllFields":false`)
	requireImported(t, line)
	if panel.Display != nil && panel.Display.IncludeAllFields != nil {
		t.Fatalf("display = %+v", panel.Display)
	}
}

func TestCandlestickExtraSeriesTakeTheTimeSeriesProps(t *testing.T) {
	cases := []struct {
		name, custom string
		check        func(spec.FieldProps) bool
	}{
		{"point size", `{"showPoints":"always","pointSize":8}`, func(p spec.FieldProps) bool { return intOf(p.PointSize) == 8 }},
		{"fill below to", `{"fillBelowTo":"down"}`, func(p spec.FieldProps) bool { return p.FillBelowTo == "down" }},
		{"constant transform", `{"transform":"constant"}`, func(p spec.FieldProps) bool { return p.ValueTransform == "constant" }},
		{"negative Y transform", `{"transform":"negative-Y"}`, func(p spec.FieldProps) bool { return p.ValueTransform == "negativeY" }},
		{"stacking group", `{"stacking":{"mode":"normal","group":"B"}}`, func(p spec.FieldProps) bool { return p.Stack == "normal" && p.StackGroup == "B" }},
		{"hide from legend", `{"hideFrom":{"legend":true,"tooltip":false,"viz":false}}`, func(p spec.FieldProps) bool {
			return p.HideInLegend != nil && *p.HideInLegend && p.HideInTooltip == nil
		}},
		{"hide from tooltip", `{"hideFrom":{"legend":false,"tooltip":true,"viz":false}}`, func(p spec.FieldProps) bool {
			return p.HideInTooltip != nil && *p.HideInTooltip && p.HideInLegend == nil
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := customPanelOf(t, "candlestick", tc.custom, "")
			requireImported(t, line)
			if !tc.check(panel.FieldProps) {
				t.Fatalf("props = %+v", panel.FieldProps)
			}
		})
	}
}

func TestPlotDrawnPropsAreReportedOnTimelinesAndPieCharts(t *testing.T) {
	cases := []struct{ name, custom, path string }{
		{"threshold style", `{"thresholdsStyle":{"mode":"line"}}`, "fieldConfig.defaults.custom.thresholdsStyle.mode"},
		{"bar alignment", `{"barAlignment":1}`, "fieldConfig.defaults.custom.barAlignment"},
		{"bar width factor", `{"barWidthFactor":0.8}`, "fieldConfig.defaults.custom.barWidthFactor"},
		{"bar maximum width", `{"barMaxWidth":20}`, "fieldConfig.defaults.custom.barMaxWidth"},
	}
	for _, typ := range []string{"state-timeline", "status-history", "piechart"} {
		for _, tc := range cases {
			t.Run(typ+" "+tc.name, func(t *testing.T) {
				panel, line := customPanelOf(t, typ, tc.custom, "")
				if line.Status != statusPartial || strings.Join(line.Paths, " ") != tc.path {
					t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
				}
				if !fieldPropsEmpty(panel.FieldProps) {
					t.Fatalf("field props = %+v", panel.FieldProps)
				}
			})
		}
	}
}

func TestInsertNullsIsReportedOnPieChartsAndInAnOverrideOnOtherTypes(t *testing.T) {
	cases := []struct{ name, typ, custom, overrides, path string }{
		{"pie chart", "piechart", `{"insertNulls":true}`, "", "fieldConfig.defaults.custom.insertNulls"},
		{"pie chart override", "piechart", `{}`, overrideOf(`{"id":"custom.insertNulls","value":60000}`), "fieldConfig.overrides[0].properties[0]"},
		{"table override", "table", `{}`, overrideOf(`{"id":"custom.insertNulls","value":true}`), "fieldConfig.overrides[0].properties[0]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := customPanelOf(t, tc.typ, tc.custom, tc.overrides)
			if line.Status != statusPartial || strings.Join(line.Paths, " ") != tc.path {
				t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
			}
			if !fieldPropsEmpty(panel.FieldProps) {
				t.Fatalf("field props = %+v", panel.FieldProps)
			}
		})
	}
}

func TestSeriesStylePropsConvertOnTimeSeries(t *testing.T) {
	cases := []struct {
		name, custom string
		check        func(spec.FieldProps) bool
	}{
		{"line color", `{"lineColor":"#ff0000"}`, func(p spec.FieldProps) bool { return p.LineColor == "#ff0000" }},
		{"line color from a rgb value", `{"lineColor":"rgb(255, 0, 0)"}`, func(p spec.FieldProps) bool { return strings.EqualFold(p.LineColor, "#ff0000") }},
		{"fill color from a palette name", `{"fillColor":"blue"}`, func(p spec.FieldProps) bool { return p.FillColor == "#5794F2" }},
		{"point color", `{"pointColor":"#0000ff"}`, func(p spec.FieldProps) bool { return p.PointColor == "#0000ff" }},
		{"circle symbol", `{"pointSymbol":"circle"}`, func(p spec.FieldProps) bool { return p.PointShape == "circle" }},
		{"square symbol", `{"pointSymbol":"square"}`, func(p spec.FieldProps) bool { return p.PointShape == "square" }},
		{"circle marker path", `{"pointSymbol":"img/icons/marker/circle.svg"}`, func(p spec.FieldProps) bool { return p.PointShape == "circle" }},
		{"square marker path", `{"pointSymbol":"img/icons/marker/square.svg"}`, func(p spec.FieldProps) bool { return p.PointShape == "square" }},
		{"show values", `{"showValues":true}`, func(p spec.FieldProps) bool { return p.ShowValues != nil && *p.ShowValues }},
		{"hidden values are Grafana's default", `{"showValues":false}`, func(p spec.FieldProps) bool { return p.ShowValues == nil }},
	}
	for _, typ := range []string{"timeseries", "trend", "candlestick"} {
		for _, tc := range cases {
			t.Run(typ+" "+tc.name, func(t *testing.T) {
				panel, line := customPanelOf(t, typ, tc.custom, "")
				requireImported(t, line)
				if !tc.check(panel.FieldProps) {
					t.Fatalf("field props = %+v", panel.FieldProps)
				}
			})
		}
	}
}

func TestSeriesStyleColorAndSymbolProblemsAreReported(t *testing.T) {
	cases := []struct{ name, custom, path, reason string }{
		{"unknown line color", `{"lineColor":"not-a-color"}`, "fieldConfig.defaults.custom.lineColor", "This line color is not supported yet."},
		{"unknown fill color", `{"fillColor":"nope"}`, "fieldConfig.defaults.custom.fillColor", "This fill color is not supported yet."},
		{"unknown point color", `{"pointColor":""}`, "fieldConfig.defaults.custom.pointColor", "This point color is not supported yet."},
		{"unknown point symbol", `{"pointSymbol":"img/icons/marker/diamond.svg"}`, "fieldConfig.defaults.custom.pointSymbol", "The point symbol img/icons/marker/diamond.svg is not supported yet."},
		{"point symbol that only ends like a marker", `{"pointSymbol":"notsquare.svg"}`, "fieldConfig.defaults.custom.pointSymbol", "The point symbol notsquare.svg is not supported yet."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := customPanel(t, tc.custom, "")
			requireReported(t, line, tc.path, tc.reason)
			if !fieldPropsEmpty(panel.FieldProps) {
				t.Fatalf("field props = %+v", panel.FieldProps)
			}
		})
	}
}

func TestSeriesStylePropsAreReportedOutsideTheSeriesPanels(t *testing.T) {
	cases := []struct{ name, custom, path string }{
		{"line color", `{"lineColor":"#ff0000"}`, "fieldConfig.defaults.custom.lineColor"},
		{"fill color", `{"fillColor":"blue"}`, "fieldConfig.defaults.custom.fillColor"},
		{"point color", `{"pointColor":"#0000ff"}`, "fieldConfig.defaults.custom.pointColor"},
		{"point symbol", `{"pointSymbol":"circle"}`, "fieldConfig.defaults.custom.pointSymbol"},
		{"show values", `{"showValues":true}`, "fieldConfig.defaults.custom.showValues"},
	}
	for _, typ := range []string{"state-timeline", "barchart", "xychart", "piechart"} {
		for _, tc := range cases {
			t.Run(typ+" "+tc.name, func(t *testing.T) {
				panel, line := customPanelOf(t, typ, tc.custom, "")
				if line.Status != statusPartial || strings.Join(line.Paths, " ") != tc.path {
					t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
				}
				if !fieldPropsEmpty(panel.FieldProps) {
					t.Fatalf("field props = %+v", panel.FieldProps)
				}
			})
		}
	}
}

func TestShowValuesOverrideResetsThePanel(t *testing.T) {
	off := overrideOf(`{"id":"custom.showValues","value":false}`)
	panel, line := customPanel(t, `{"showValues":true}`, off)
	requireImported(t, line)
	reset := firstOverrideProps(t, panel).ShowValues
	if reset == nil || *reset {
		t.Fatalf("override props = %+v", firstOverrideProps(t, panel))
	}
	panel, line = customPanel(t, `{}`, off)
	requireImported(t, line)
	if len(panel.Overrides) != 0 {
		t.Fatalf("overrides = %+v", panel.Overrides)
	}
}

func TestSeriesStyleOverridesConvert(t *testing.T) {
	override := overrideOf(`{"id":"custom.lineColor","value":"#ff0000"},{"id":"custom.fillColor","value":"blue"},{"id":"custom.pointColor","value":"#0000ff"},{"id":"custom.pointSymbol","value":"square"},{"id":"custom.showValues","value":true}`)
	panel, line := customPanel(t, `{}`, override)
	requireImported(t, line)
	got := firstOverrideProps(t, panel)
	if got.LineColor != "#ff0000" || got.FillColor != "#5794F2" || got.PointColor != "#0000ff" || got.PointShape != "square" || got.ShowValues == nil || !*got.ShowValues {
		t.Fatalf("override props = %+v", got)
	}
}
