package grafana

import (
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func barPanel(t *testing.T, typ, members string) (spec.Panel, PanelLine) {
	t.Helper()
	return convertedPanel(t, panelDoc(typ, upTarget+","+members))
}

func TestBarChartTooltipSortIsWrittenForBothDirections(t *testing.T) {
	for _, sort := range []string{"asc", "desc"} {
		t.Run(sort, func(t *testing.T) {
			panel, line := barPanel(t, "barchart", `"options":{"tooltip":{"mode":"multi","sort":"`+sort+`"}}`)
			if !clean(line) || displayOf(t, panel).TooltipSort != sort {
				t.Fatalf("status %s, paths %v, display %+v", line.Status, line.Paths, panel.Display)
			}
		})
	}
	panel, line := barPanel(t, "barchart", `"options":{"tooltip":{"mode":"multi","sort":"none"}}`)
	if !clean(line) || displayOf(t, panel).TooltipSort != "" {
		t.Fatalf("none is the Spec default: status %s, display %+v", line.Status, panel.Display)
	}
}

func TestBarChartLegendAndTooltipOptions(t *testing.T) {
	panel, line := barPanel(t, "barchart", `"options":{"legend":{"displayMode":"table","placement":"right","calcs":["max","mean"],"sortBy":"Mean","sortDesc":true,"width":240},"tooltip":{"mode":"multi","hideZeros":true,"maxWidth":300,"maxHeight":200}}`)
	if !clean(line) {
		t.Fatalf("status %s, paths %v", line.Status, line.Paths)
	}
	d := displayOf(t, panel)
	if d.Legend != "right" || d.LegendMode != "" || !slices.Equal(d.LegendValues, []string{"max", "mean"}) {
		t.Fatalf("legend = %+v", d)
	}
	if d.LegendSortBy != "mean" || d.LegendSortDesc == nil || !*d.LegendSortDesc || d.LegendWidth != 240 {
		t.Fatalf("legend sort and width = %+v", d)
	}
	if d.Tooltip != "all" || d.TooltipHideZeros == nil || !*d.TooltipHideZeros || d.TooltipMaxWidth != 300 || d.TooltipMaxHeight != 200 {
		t.Fatalf("tooltip = %+v", d)
	}
}

func TestBarChartTableLegendWithoutValuesIsATable(t *testing.T) {
	panel, line := barPanel(t, "barchart", `"options":{"legend":{"displayMode":"table","placement":"bottom","showLegend":true}}`)
	if !clean(line) || displayOf(t, panel).LegendMode != "table" {
		t.Fatalf("status %s, display %+v", line.Status, panel.Display)
	}
}

func TestBarChartLabelAndBarOptionsOutsideTheirRangeAreReported(t *testing.T) {
	cases := []struct{ name, options, path string }{
		{"rotation", `{"xTickLabelRotation":120}`, "options.xTickLabelRotation"},
		{"spacing", `{"xTickLabelSpacing":900}`, "options.xTickLabelSpacing"},
		{"max length", `{"xTickLabelMaxLength":500}`, "options.xTickLabelMaxLength"},
		{"radius", `{"barRadius":0.9}`, "options.barRadius"},
		{"value size", `{"text":{"valueSize":2}}`, "options.text.valueSize"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := barPanel(t, "barchart", `"options":`+tc.options)
			if line.Status != statusPartial || !slices.Equal(line.Paths, []string{tc.path}) {
				t.Fatalf("status %s, paths %v", line.Status, line.Paths)
			}
			if panel.Display != nil {
				t.Fatalf("display = %+v, want none", panel.Display)
			}
		})
	}
}

func TestBarChartDefaultsAreNotWritten(t *testing.T) {
	panel, line := barPanel(t, "barchart", `"options":{"xTickLabelRotation":0,"xTickLabelSpacing":0,"xTickLabelMaxLength":0,"barRadius":0,"fullHighlight":false}`)
	if !clean(line) || panel.Display != nil {
		t.Fatalf("status %s, paths %v, display %+v", line.Status, line.Paths, panel.Display)
	}
}

func TestBarChartAndHistogramDrawTheValueAxisProps(t *testing.T) {
	custom := `{"axisLabel":"Count","axisSoftMin":0,"axisSoftMax":50,"axisWidth":90,"axisGridShow":false,"axisBorderShow":true,"axisCenteredZero":true,"axisColorMode":"series","scaleDistribution":{"type":"log","log":2},"axisPlacement":"right"}`
	for _, typ := range []string{"barchart", "histogram"} {
		t.Run(typ, func(t *testing.T) {
			panel, line := customPanelOf(t, typ, custom, "")
			if !clean(line) {
				t.Fatalf("status %s, paths %v", line.Status, line.Paths)
			}
			p := panel.FieldProps
			if p.AxisLabel != "Count" || p.Axis != "right" || p.Scale != "log" || intOf(p.LogBase) != 2 || intOf(p.AxisWidth) != 90 {
				t.Fatalf("props = %+v", p)
			}
			if p.AxisGrid == nil || *p.AxisGrid || p.AxisBorder == nil || !*p.AxisBorder || p.AxisCenteredZero == nil || !*p.AxisCenteredZero {
				t.Fatalf("props = %+v", p)
			}
		})
	}
}

func TestBarChartAndHistogramLineWidthIsTheBorderWidth(t *testing.T) {
	for _, typ := range []string{"barchart", "histogram"} {
		t.Run(typ, func(t *testing.T) {
			panel, line := customPanelOf(t, typ, `{"lineWidth":1,"fillOpacity":80}`, "")
			if !clean(line) || intOf(panel.LineWidth) != 1 || intOf(panel.Fill) != 80 {
				t.Fatalf("status %s, paths %v, props %+v", line.Status, line.Paths, panel.FieldProps)
			}
		})
	}
}

func TestBarChartAndHistogramReportATransparentFill(t *testing.T) {
	for _, typ := range []string{"barchart", "histogram"} {
		t.Run(typ, func(t *testing.T) {
			_, line := customPanelOf(t, typ, `{"fillOpacity":0}`, "")
			if line.Status != statusPartial || !slices.Contains(line.Reasons, opaqueFillReason) {
				t.Fatalf("status %s, paths %v, reasons %v", line.Status, line.Paths, line.Reasons)
			}
		})
	}
}

func TestBarChartAndHistogramStillReportTheLineProps(t *testing.T) {
	for _, typ := range []string{"barchart", "histogram"} {
		for _, custom := range []string{`{"lineInterpolation":"smooth"}`, `{"showPoints":"always"}`, `{"spanNulls":true}`} {
			t.Run(typ+custom, func(t *testing.T) {
				_, line := customPanelOf(t, typ, custom, "")
				if line.Status != statusPartial {
					t.Fatalf("status %s, paths %v", line.Status, line.Paths)
				}
			})
		}
	}
}

func TestHistogramWritesGrafanasOwnValuesWhereTheyDifferFromTheRenderer(t *testing.T) {
	panel, line := barPanel(t, "histogram", `"options":{}`)
	if !clean(line) {
		t.Fatalf("status %s, paths %v", line.Status, line.Paths)
	}
	d := displayOf(t, panel)
	if d.Stack != "none" || d.Tooltip != "single" || d.TooltipHideZeros == nil || *d.TooltipHideZeros {
		t.Fatalf("display = %+v, want stack none, tooltip single and hideZeros false written", d)
	}
}

func TestHistogramStackingComesFromTheFieldConfig(t *testing.T) {
	for _, mode := range []string{"normal", "percent", "none"} {
		t.Run(mode, func(t *testing.T) {
			panel, line := customPanelOf(t, "histogram", `{"stacking":{"mode":"`+mode+`","group":"A"}}`, "")
			want := mode
			if mode == "normal" {
				want = ""
			}
			if !clean(line) || displayOf(t, panel).Stack != want {
				t.Fatalf("stack = %q, want %q (paths %v)", displayOf(t, panel).Stack, want, line.Paths)
			}
			if !fieldPropsEmpty(panel.FieldProps) {
				t.Fatalf("stack leaked into field props: %+v", panel.FieldProps)
			}
		})
	}
}

func TestHistogramStackingOverrideIsReported(t *testing.T) {
	override := `{"matcher":{"id":"byName","options":"a"},"properties":[{"id":"custom.stacking","value":{"mode":"percent"}}]}`
	_, line := customPanelOf(t, "histogram", `{}`, override)
	if line.Status != statusPartial || !slices.Contains(line.Reasons, "Stacking applies to the whole histogram, not to one series.") {
		t.Fatalf("status %s, paths %v, reasons %v", line.Status, line.Paths, line.Reasons)
	}
}

func TestHistogramBucketOffsetCombineLegendAndTooltip(t *testing.T) {
	panel, line := barPanel(t, "histogram", `"options":{"bucketOffset":2.5,"combine":true,"legend":{"displayMode":"table","placement":"right","calcs":["sum"],"width":200},"tooltip":{"mode":"multi","sort":"desc","hideZeros":true}}`)
	if !clean(line) {
		t.Fatalf("status %s, paths %v", line.Status, line.Paths)
	}
	d := displayOf(t, panel)
	if d.BucketOffset == nil || *d.BucketOffset != 2.5 || d.Combine == nil || !*d.Combine {
		t.Fatalf("buckets = %+v", d)
	}
	if d.Legend != "right" || !slices.Equal(d.LegendValues, []string{"sum"}) || d.LegendWidth != 200 {
		t.Fatalf("legend = %+v", d)
	}
	if d.Tooltip != "" || d.TooltipSort != "desc" || d.TooltipHideZeros != nil {
		t.Fatalf("tooltip = %+v (all and hideZeros true are the Spec defaults)", d)
	}
}

func TestHistogramHidesItsLegendAndTooltip(t *testing.T) {
	panel, line := barPanel(t, "histogram", `"options":{"legend":{"showLegend":false},"tooltip":{"mode":"none"}}`)
	if !clean(line) {
		t.Fatalf("status %s, paths %v", line.Status, line.Paths)
	}
	if d := displayOf(t, panel); d.Legend != "hidden" || d.Tooltip != "hidden" {
		t.Fatalf("display = %+v", d)
	}
}

func TestBarChartAndHistogramHideSeriesFromTheLegendAndTooltip(t *testing.T) {
	for _, typ := range []string{"barchart", "histogram"} {
		t.Run(typ, func(t *testing.T) {
			panel, line := customPanelOf(t, typ, `{"hideFrom":{"legend":true,"tooltip":true,"viz":false}}`, "")
			if !clean(line) || panel.HideInLegend == nil || !*panel.HideInLegend || panel.HideInTooltip == nil || !*panel.HideInTooltip {
				t.Fatalf("status %s, paths %v, props %+v", line.Status, line.Paths, panel.FieldProps)
			}
		})
	}
}
