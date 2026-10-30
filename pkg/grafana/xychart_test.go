package grafana

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func xyPanel(t *testing.T, members string) (spec.Panel, PanelLine) {
	t.Helper()
	return convertedPanel(t, panelDoc("xychart", upTarget+","+members))
}

func byName(name string) string {
	return fmt.Sprintf(`{"matcher":{"id":"byName","options":%q}}`, name)
}

func TestXYChartManualMappingImportsEveryMatcher(t *testing.T) {
	members := `"options":{"mapping":"manual","series":[{` +
		`"frame":{"matcher":{"id":"byRefId","options":"A"}},` +
		`"x":` + byName("severity") + `,` +
		`"y":{"matcher":{"id":"byRegexp","options":"^lat"}},` +
		`"size":` + byName("count") + `,` +
		`"color":` + byName("score") + `,` +
		`"name":{"fixed":"Latency"}}]}`
	panel, line := xyPanel(t, members)
	if !clean(line) {
		t.Fatalf("status %s, paths %v", line.Status, line.Paths)
	}
	want := []spec.XYSeries{{Query: "A", X: "severity", Y: "/^lat/", Size: "count", Color: "score", Name: "Latency"}}
	if got := displayOf(t, panel).Series; !slices.Equal(got, want) {
		t.Fatalf("series = %+v, want %+v", got, want)
	}
}

func TestXYChartManualMappingWithoutSeriesIsOneDefaultSeries(t *testing.T) {
	panel, line := xyPanel(t, `"options":{"mapping":"manual"}`)
	if !clean(line) {
		t.Fatalf("status %s, paths %v", line.Status, line.Paths)
	}
	if got := displayOf(t, panel).Series; !slices.Equal(got, []spec.XYSeries{{}}) {
		t.Fatalf("series = %+v, want one empty entry", got)
	}
}

func TestXYChartFrameMatchers(t *testing.T) {
	cases := []struct {
		name, frame string
		want        spec.XYSeries
	}{
		{"by name", `{"matcher":{"id":"byName","options":"Value"}}`, spec.XYSeries{Frame: "Value"}},
		{"by regexp", `{"matcher":{"id":"byRegexp","options":"^pod-"}}`, spec.XYSeries{Frame: "/^pod-/"}},
		{"by ref id", `{"matcher":{"id":"byRefId","options":"A"}}`, spec.XYSeries{Query: "A"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := xyPanel(t, `"options":{"mapping":"manual","series":[{"frame":`+tc.frame+`}]}`)
			if !clean(line) {
				t.Fatalf("status %s, paths %v", line.Status, line.Paths)
			}
			if got := displayOf(t, panel).Series; !slices.Equal(got, []spec.XYSeries{tc.want}) {
				t.Fatalf("series = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestXYChartYMatchers(t *testing.T) {
	cases := []struct{ name, matcher, want string }{
		{"one name", `{"matcher":{"id":"byNames","options":{"mode":"include","names":["a.b"]}}}`, "/^(?:a\\.b)$/"},
		{"bare list of names", `{"matcher":{"id":"byNames","options":["a","b"]}}`, "/^(?:a|b)$/"},
		{"several names", `{"matcher":{"id":"byNames","options":{"mode":"include","names":["a","b"]}}}`, "/^(?:a|b)$/"},
		{"all but some names", `{"matcher":{"id":"byNames","options":{"mode":"exclude","names":["a","b"]}}}`, "/^(?!(?:a|b)$).*$/"},
		{"one field", byName("latency"), "latency"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := xyPanel(t, `"options":{"mapping":"manual","series":[{"y":`+tc.matcher+`}]}`)
			if !clean(line) {
				t.Fatalf("status %s, paths %v", line.Status, line.Paths)
			}
			if got := displayOf(t, panel).Series; len(got) != 1 || got[0].Y != tc.want {
				t.Fatalf("series = %+v, want y %q", got, tc.want)
			}
		})
	}
}

func TestXYChartMatchersWithoutAnEquivalentAreReportedAndTheRestKept(t *testing.T) {
	members := `"options":{"mapping":"manual","series":[{"x":` + byName("t") + `,` +
		`"y":{"matcher":{"id":"byType","options":"number"}},` +
		`"size":{"matcher":{"id":"byRegexp","options":"n"}},` +
		`"frame":{"matcher":{"id":"byIndex","options":0}}}]}`
	panel, line := xyPanel(t, members)
	want := "options.series[0].frame options.series[0].size options.series[0].y"
	if line.Status != statusPartial || strings.Join(slices.Sorted(slices.Values(line.Paths)), " ") != want {
		t.Fatalf("status %s, paths %v, want %s", line.Status, line.Paths, want)
	}
	if got := displayOf(t, panel).Series; !slices.Equal(got, []spec.XYSeries{{X: "t"}}) {
		t.Fatalf("series = %+v", got)
	}
}

func TestXYChartAMatcherNameStartingWithASlashIsReported(t *testing.T) {
	_, line := xyPanel(t, `"options":{"mapping":"manual","series":[{"y":`+byName("/odd")+`}]}`)
	if line.Status != statusPartial || strings.Join(line.Paths, " ") != "options.series[0].y" {
		t.Fatalf("status %s, paths %v", line.Status, line.Paths)
	}
}

func TestXYChartMoreThanTwentySeriesKeepTwentyAndReportTheRest(t *testing.T) {
	entries := make([]string, 22)
	for i := range entries {
		entries[i] = `{"x":` + byName(fmt.Sprintf("f%d", i)) + `}`
	}
	panel, line := xyPanel(t, `"options":{"mapping":"manual","series":[`+strings.Join(entries, ",")+`]}`)
	if line.Status != statusPartial || strings.Join(line.Paths, " ") != "options.series" {
		t.Fatalf("status %s, paths %v", line.Status, line.Paths)
	}
	got := displayOf(t, panel).Series
	if len(got) != 20 || got[19].X != "f19" {
		t.Fatalf("series = %d entries, last %+v", len(got), got[len(got)-1])
	}
}

func TestXYChartOlderSeriesMappingShape(t *testing.T) {
	members := `"options":{"seriesMapping":"manual","series":[{"x":"time","y":"value","name":"Older","pointColor":{"field":"hue"},"pointSize":{"field":"mass"}}]}`
	panel, line := xyPanel(t, members)
	if !clean(line) {
		t.Fatalf("status %s, paths %v", line.Status, line.Paths)
	}
	want := []spec.XYSeries{{X: "time", Y: "value", Size: "mass", Color: "hue", Name: "Older"}}
	if got := displayOf(t, panel).Series; !slices.Equal(got, want) {
		t.Fatalf("series = %+v, want %+v", got, want)
	}
}

func TestXYChartOlderDimsShape(t *testing.T) {
	panel, line := xyPanel(t, `"options":{"dims":{"frame":0,"x":"time","exclude":["a","b"]}}`)
	if line.Status != statusPartial || strings.Join(line.Paths, " ") != "options.dims.frame" {
		t.Fatalf("status %s, paths %v", line.Status, line.Paths)
	}
	d := displayOf(t, panel)
	if d.XField != "time" || !slices.Equal(d.Series, []spec.XYSeries{{X: "time", Y: "/^(?!(?:a|b)$).*$/"}}) {
		t.Fatalf("display = %+v", d)
	}
}

func TestXYChartAutoMappingSeriesAreStillReported(t *testing.T) {
	_, line := xyPanel(t, `"options":{"mapping":"auto","series":[{"x":`+byName("t")+`}]}`)
	if line.Status != statusPartial {
		t.Fatalf("status %s, paths %v", line.Status, line.Paths)
	}
}

func TestXYChartPointSizeRange(t *testing.T) {
	custom := func(size string) string {
		return `"fieldConfig":{"defaults":{"custom":{"pointSize":` + size + `}},"overrides":[]}`
	}
	cases := []struct {
		name, size string
		min, max   int
		paths      []string
	}{
		{"both", `{"fixed":8,"min":2,"max":50}`, 2, 50, nil},
		{"only the largest", `{"max":90}`, 0, 90, nil},
		{"above the largest allowed", `{"min":2,"max":120}`, 2, 0, []string{"fieldConfig.defaults.custom.pointSize.max"}},
		{"smallest above largest", `{"min":30,"max":10}`, 0, 0, []string{"fieldConfig.defaults.custom.pointSize.max", "fieldConfig.defaults.custom.pointSize.min"}},
		{"fraction", `{"min":1.5}`, 0, 0, []string{"fieldConfig.defaults.custom.pointSize.min"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := xyPanel(t, custom(tc.size))
			var d spec.Display
			if panel.Display != nil {
				d = *panel.Display
			}
			if d.PointSizeMin != tc.min || d.PointSizeMax != tc.max {
				t.Fatalf("range = %d..%d, want %d..%d", d.PointSizeMin, d.PointSizeMax, tc.min, tc.max)
			}
			if !slices.Equal(slices.Sorted(slices.Values(line.Paths)), tc.paths) {
				t.Fatalf("paths = %v, want %v", line.Paths, tc.paths)
			}
		})
	}
}

func TestXYChartPointAndLineFieldProps(t *testing.T) {
	cases := []struct {
		name, custom string
		check        func(spec.FieldProps) bool
	}{
		{"square points", `{"pointShape":"square"}`, func(p spec.FieldProps) bool { return p.PointShape == "square" }},
		{"circle points are the default", `{"pointShape":"circle"}`, func(p spec.FieldProps) bool { return p.PointShape == "" }},
		{"stroke", `{"pointStrokeWidth":3}`, func(p spec.FieldProps) bool { return intOf(p.PointStrokeWidth) == 3 }},
		{"Grafana's own stroke of 1 is kept", `{"pointStrokeWidth":1}`, func(p spec.FieldProps) bool { return intOf(p.PointStrokeWidth) == 1 }},
		{"no stroke is the default", `{"pointStrokeWidth":0}`, func(p spec.FieldProps) bool { return p.PointStrokeWidth == nil }},
		{"line width", `{"lineWidth":3}`, func(p spec.FieldProps) bool { return intOf(p.LineWidth) == 3 }},
		{"line width 2 is kept, not the canvas look", `{"lineWidth":2}`, func(p spec.FieldProps) bool { return intOf(p.LineWidth) == 2 }},
		{"Grafana's line width is absent", `{"lineWidth":1}`, func(p spec.FieldProps) bool { return p.LineWidth == nil }},
		{"dashed lines", `{"lineStyle":{"fill":"dash"}}`, func(p spec.FieldProps) bool { return p.LineStyle == "dashed" }},
		{"fill", `{"fillOpacity":30}`, func(p spec.FieldProps) bool { return intOf(p.Fill) == 30 }},
		{"hidden from the legend", `{"hideFrom":{"legend":true}}`, func(p spec.FieldProps) bool { return p.HideInLegend != nil && *p.HideInLegend }},
		{"hidden from the tooltip", `{"hideFrom":{"tooltip":true}}`, func(p spec.FieldProps) bool { return p.HideInTooltip != nil && *p.HideInTooltip }},
		{"axis on the right", `{"axisPlacement":"right"}`, func(p spec.FieldProps) bool { return p.Axis == "right" }},
		{"log axis", `{"scaleDistribution":{"type":"log","log":2}}`, func(p spec.FieldProps) bool { return p.Scale == "log" && intOf(p.LogBase) == 2 }},
		{"symlog axis", `{"scaleDistribution":{"type":"symlog","linearThreshold":10}}`, func(p spec.FieldProps) bool { return p.Scale == "symlog" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := customPanelOf(t, "xychart", tc.custom, "")
			if !clean(line) || !tc.check(panel.FieldProps) {
				t.Fatalf("status %s, paths %v, field props %+v", line.Status, line.Paths, panel.FieldProps)
			}
		})
	}
}

func TestXYChartPointPropsOverrideAPanelValue(t *testing.T) {
	override := `{"matcher":{"id":"byName","options":"a"},"properties":[{"id":"custom.pointShape","value":"circle"},{"id":"custom.lineWidth","value":1},{"id":"custom.pointStrokeWidth","value":0}]}`
	panel, line := customPanelOf(t, "xychart", `{"pointShape":"square","lineWidth":4,"pointStrokeWidth":2}`, override)
	if !clean(line) {
		t.Fatalf("status %s, paths %v", line.Status, line.Paths)
	}
	got := onlyOverride(t, panel)
	if got.PointShape != "circle" || intOf(got.LineWidth) != 1 || intOf(got.PointStrokeWidth) != 0 {
		t.Fatalf("override = %+v", got.FieldProps)
	}
}

func TestPointPropsAreReportedOnPanelsThatDrawNoPoints(t *testing.T) {
	for _, typ := range []string{"timeseries", "barchart", "state-timeline"} {
		t.Run(typ, func(t *testing.T) {
			_, line := customPanelOf(t, typ, `{"pointShape":"square","pointStrokeWidth":2}`, "")
			want := "fieldConfig.defaults.custom.pointShape fieldConfig.defaults.custom.pointStrokeWidth"
			if line.Status != statusPartial || strings.Join(line.Paths, " ") != want {
				t.Fatalf("status %s, paths %v", line.Status, line.Paths)
			}
		})
	}
}

func TestXYChartTransparentFillIsReportedBecauseItDrawsOpaque(t *testing.T) {
	_, line := customPanelOf(t, "xychart", `{"fillOpacity":0}`, "")
	if line.Status != statusPartial || strings.Join(line.Paths, " ") != "fieldConfig.defaults.custom.fillOpacity" {
		t.Fatalf("status %s, paths %v", line.Status, line.Paths)
	}
}

func TestXYChartLineStylesTheRendererCannotDrawAreReported(t *testing.T) {
	cases := []struct{ name, custom, path string }{
		{"square", `{"lineStyle":{"fill":"square"}}`, "fieldConfig.defaults.custom.lineStyle.fill"},
		{"dash pattern", `{"lineStyle":{"fill":"dash","dash":[10,15]}}`, "fieldConfig.defaults.custom.lineStyle.dash"},
		{"smooth", `{"lineInterpolation":"smooth"}`, "fieldConfig.defaults.custom.lineInterpolation"},
		{"ordinal scale", `{"scaleDistribution":{"type":"ordinal"}}`, "fieldConfig.defaults.custom.scaleDistribution.type"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, line := customPanelOf(t, "xychart", tc.custom, "")
			if line.Status != statusPartial || strings.Join(line.Paths, " ") != tc.path {
				t.Fatalf("status %s, paths %v", line.Status, line.Paths)
			}
		})
	}
}

func TestXYChartLegendAndTooltipOptions(t *testing.T) {
	members := `"options":{"legend":{"displayMode":"table","placement":"right","calcs":["mean","max"],"sortBy":"Mean","sortDesc":true,"width":240},` +
		`"tooltip":{"mode":"multi","sort":"asc","hideZeros":true,"maxWidth":300,"maxHeight":200}}`
	panel, line := xyPanel(t, members)
	if !clean(line) {
		t.Fatalf("status %s, paths %v", line.Status, line.Paths)
	}
	d := displayOf(t, panel)
	if d.Legend != "right" || !slices.Equal(d.LegendValues, []string{"mean", "max"}) || d.LegendSortBy != "mean" || d.LegendSortDesc == nil || !*d.LegendSortDesc || d.LegendWidth != 240 {
		t.Fatalf("legend = %+v", d)
	}
	if d.Tooltip != "all" || d.TooltipSort != "asc" || d.TooltipHideZeros == nil || !*d.TooltipHideZeros || d.TooltipMaxWidth != 300 || d.TooltipMaxHeight != 200 {
		t.Fatalf("tooltip = %+v", d)
	}
}

func TestXYChartTableLegendWithoutValuesIsATable(t *testing.T) {
	panel, line := xyPanel(t, `"options":{"legend":{"displayMode":"table","placement":"bottom","showLegend":true}}`)
	if !clean(line) || displayOf(t, panel).LegendMode != "table" {
		t.Fatalf("status %s, display %+v", line.Status, panel.Display)
	}
}

func TestXYChartTooltipSortIsWrittenInSeriesOrderTerms(t *testing.T) {
	for _, sort := range []string{"asc", "desc"} {
		t.Run(sort, func(t *testing.T) {
			panel, line := xyPanel(t, `"options":{"tooltip":{"mode":"multi","sort":"`+sort+`"}}`)
			if !clean(line) || displayOf(t, panel).TooltipSort != sort {
				t.Fatalf("status %s, display %+v", line.Status, panel.Display)
			}
		})
	}
}
