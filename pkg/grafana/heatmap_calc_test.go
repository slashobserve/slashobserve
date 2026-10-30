package grafana

import (
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func legacyHeatmapDisplay(t *testing.T, members string) (spec.Display, PanelLine) {
	t.Helper()
	doc := strings.Replace(panelDoc("heatmap", upTarget+`,`+members), `"schemaVersion":39`, `"schemaVersion":30`, 1)
	panel, line := convertedPanel(t, doc)
	return displayOf(t, panel), line
}

func sameBuckets(got *spec.Buckets, mode string, value float64) bool {
	return got != nil && got.Mode == mode && got.Value != nil && *got.Value == value
}

func TestHeatmapBucketsAreImportedPerAxis(t *testing.T) {
	cases := []struct {
		name, options string
		check         func(spec.Display) bool
	}{
		{"an x size in minutes is seconds", `{"calculate":true,"calculation":{"xBuckets":{"mode":"size","value":"1m"}}}`, func(d spec.Display) bool {
			return sameBuckets(d.XBuckets, "size", 60)
		}},
		{"an x size without a unit is seconds", `{"calculate":true,"calculation":{"xBuckets":{"mode":"size","value":"10"}}}`, func(d spec.Display) bool {
			return sameBuckets(d.XBuckets, "size", 10)
		}},
		{"an x count", `{"calculate":true,"calculation":{"xBuckets":{"mode":"count","value":"40"}}}`, func(d spec.Display) bool {
			return sameBuckets(d.XBuckets, "count", 40)
		}},
		{"a y count", `{"calculate":true,"calculation":{"yBuckets":{"mode":"count","value":30}}}`, func(d spec.Display) bool {
			return sameBuckets(d.YBuckets, "count", 30)
		}},
		{"a y size", `{"calculate":true,"calculation":{"yBuckets":{"mode":"size","value":"0.5"}}}`, func(d spec.Display) bool {
			return sameBuckets(d.YBuckets, "size", 0.5)
		}},
		{"a mode without a value is automatic", `{"calculate":true,"calculation":{"yBuckets":{"mode":"size"}}}`, func(d spec.Display) bool {
			return d.YBuckets != nil && d.YBuckets.Mode == "size" && d.YBuckets.Value == nil
		}},
		{"a count without a value is the default row count", `{"calculate":true,"calculation":{"yBuckets":{"mode":"count"}}}`, func(d spec.Display) bool {
			return d.YBuckets == nil
		}},
		{"a count without a value is one column per sample", `{"calculate":true,"calculation":{"xBuckets":{"mode":"count","value":""}}}`, func(d spec.Display) bool {
			return d.XBuckets == nil
		}},
		{"a value without a mode is a size", `{"calculate":true,"calculation":{"yBuckets":{"value":"5"}}}`, func(d spec.Display) bool {
			return sameBuckets(d.YBuckets, "size", 5)
		}},
		{"an empty value is automatic", `{"calculate":true,"calculation":{"yBuckets":{"value":""}}}`, func(d spec.Display) bool {
			return d.YBuckets == nil
		}},
		{"a log y scale with its base", `{"calculate":true,"calculation":{"yBuckets":{"scale":{"type":"log","log":10}}}}`, func(d spec.Display) bool {
			return d.YBucketScale == "log" && d.YBucketLogBase == 10
		}},
		{"log base 2 is the default", `{"calculate":true,"calculation":{"yBuckets":{"scale":{"type":"symlog","log":2}}}}`, func(d spec.Display) bool {
			return d.YBucketScale == "symlog" && d.YBucketLogBase == 0
		}},
		{"a linear y scale is the default", `{"calculate":true,"calculation":{"yBuckets":{"scale":{"type":"linear"}}}}`, func(d spec.Display) bool {
			return d.YBucketScale == "" && d.YBucketLogBase == 0
		}},
		{"a linear x scale is the default", `{"calculate":true,"calculation":{"xBuckets":{"scale":{"type":"linear","log":2}}}}`, func(d spec.Display) bool {
			return d.XBuckets == nil
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

func TestHeatmapLinearYScaleOverridesALogAxis(t *testing.T) {
	doc := panelDoc("heatmap", upTarget+`,"fieldConfig":{"defaults":{"custom":{"scaleDistribution":{"type":"log","log":10}}},"overrides":[]},"options":{"calculate":true,"calculation":{"yBuckets":{"scale":{"type":"linear"}}}}`)
	panel, line := convertedPanel(t, doc)
	requireImported(t, line)
	d := displayOf(t, panel)
	if d.YScale != "log" || d.YBucketScale != "linear" || d.YBucketLogBase != 10 {
		t.Fatalf("display = %+v", d)
	}
}

func TestHeatmapLogBase2ResetsALogBase10Axis(t *testing.T) {
	doc := panelDoc("heatmap", upTarget+`,"fieldConfig":{"defaults":{"custom":{"scaleDistribution":{"type":"log","log":10}}},"overrides":[]},"options":{"calculate":true,"calculation":{"yBuckets":{"scale":{"type":"log","log":2}}}}`)
	panel, line := convertedPanel(t, doc)
	requireImported(t, line)
	d := displayOf(t, panel)
	if d.YScale != "log" || d.YBucketScale != "log" || d.YBucketLogBase != 0 {
		t.Fatalf("display = %+v", d)
	}
}

func TestHeatmapBucketsGrafanaCannotMapAreReported(t *testing.T) {
	cases := []struct {
		name, options, path string
		check               func(spec.Display) bool
	}{
		{"a log time scale", `{"calculate":true,"calculation":{"xBuckets":{"scale":{"type":"log"}}}}`, "options.calculation.xBuckets.scale.type", func(d spec.Display) bool { return d.XBuckets == nil }},
		{"a symlog time scale", `{"calculate":true,"calculation":{"xBuckets":{"scale":{"type":"symlog"}}}}`, "options.calculation.xBuckets.scale.type", func(d spec.Display) bool { return d.XBuckets == nil }},
		{"a size that is not a duration", `{"calculate":true,"calculation":{"xBuckets":{"mode":"size","value":"soon"}}}`, "options.calculation.xBuckets.value", func(d spec.Display) bool {
			return d.XBuckets != nil && d.XBuckets.Value == nil
		}},
		{"a negative count", `{"calculate":true,"calculation":{"yBuckets":{"mode":"count","value":-3}}}`, "options.calculation.yBuckets.value", func(d spec.Display) bool {
			return d.YBuckets == nil
		}},
		{"a log base other than 2 and 10", `{"calculate":true,"calculation":{"yBuckets":{"scale":{"type":"log","log":5}}}}`, "options.calculation.yBuckets.scale.log", func(d spec.Display) bool {
			return d.YBucketScale == "log" && d.YBucketLogBase == 0
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, line := heatmapOptions(t, tc.options)
			if line.Status != statusPartial || !slices.Contains(line.Paths, tc.path) {
				t.Fatalf("status %s, paths %v", line.Status, line.Paths)
			}
			if !tc.check(d) {
				t.Fatalf("display = %+v", d)
			}
		})
	}
}

func TestHeatmapYAxisOptions(t *testing.T) {
	cases := []struct {
		name, options string
		check         func(spec.Display) bool
	}{
		{"a right axis", `{"yAxis":{"axisPlacement":"right"}}`, func(d spec.Display) bool { return d.YAxis == "right" }},
		{"a hidden axis", `{"yAxis":{"axisPlacement":"hidden"}}`, func(d spec.Display) bool { return d.YAxis == "hidden" }},
		{"left and auto are the default", `{"yAxis":{"axisPlacement":"auto"}}`, func(d spec.Display) bool { return d.YAxis == "" }},
		{"unit, decimals and range", `{"yAxis":{"unit":"s","decimals":2,"min":0,"max":"10"}}`, func(d spec.Display) bool {
			return d.YUnit == "s" && d.YDecimals != nil && *d.YDecimals == 2 && d.YMin != nil && *d.YMin == 0 && d.YMax != nil && *d.YMax == 10
		}},
		{"width and label", `{"yAxis":{"axisWidth":80,"axisLabel":" Latency "}}`, func(d spec.Display) bool {
			return d.YAxisWidth != nil && *d.YAxisWidth == 80 && d.YAxisLabel == "Latency"
		}},
		{"reverse", `{"yAxis":{"reverse":true}}`, func(d spec.Display) bool { return d.YReverse != nil && *d.YReverse }},
		{"reverse false is the default", `{"yAxis":{"reverse":false}}`, func(d spec.Display) bool { return d.YReverse == nil }},
		{"a rows layout", `{"rowsFrame":{"layout":"ge"}}`, func(d spec.Display) bool { return d.RowsLayout == "ge" }},
		{"auto rows layout is the default", `{"rowsFrame":{"layout":"auto"}}`, func(d spec.Display) bool { return d.RowsLayout == "" }},
		{"the default selection mode", `{"selectionMode":"x"}`, func(d spec.Display) bool { return true }},
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

func TestHeatmapYAxisPlacementsTheRendererCannotDrawAreReported(t *testing.T) {
	for _, placement := range []string{"top", "bottom"} {
		d, line := heatmapOptions(t, `{"yAxis":{"axisPlacement":"`+placement+`"}}`)
		if line.Status != statusPartial || !slices.Contains(line.Paths, "options.yAxis.axisPlacement") || d.YAxis != "" {
			t.Fatalf("%s: status %s, paths %v, display %+v", placement, line.Status, line.Paths, d)
		}
	}
}

func TestHeatmapSelectionModesOtherThanTimeAreReported(t *testing.T) {
	for _, mode := range []string{"xy", "y"} {
		_, line := heatmapOptions(t, `{"selectionMode":"`+mode+`"}`)
		if line.Status != statusPartial || !slices.Contains(line.Paths, "options.selectionMode") {
			t.Fatalf("%s: status %s, paths %v", mode, line.Status, line.Paths)
		}
	}
}

func TestHeatmapTooltipOptions(t *testing.T) {
	cases := []struct {
		name, options string
		check         func(spec.Display) bool
	}{
		{"multi lists every row", `{"tooltip":{"mode":"multi"}}`, func(d spec.Display) bool { return d.Tooltip == "all" }},
		{"none hides it", `{"tooltip":{"mode":"none"}}`, func(d spec.Display) bool { return d.Tooltip == "hidden" }},
		{"single is the default", `{"tooltip":{"mode":"single"}}`, func(d spec.Display) bool { return d.Tooltip == "" }},
		{"histogram and color scale", `{"tooltip":{"yHistogram":true,"showColorScale":true}}`, func(d spec.Display) bool {
			return d.TooltipHistogram != nil && *d.TooltipHistogram && d.TooltipColorScale != nil && *d.TooltipColorScale
		}},
		{"both extras off are the default", `{"tooltip":{"yHistogram":false,"showColorScale":false}}`, func(d spec.Display) bool {
			return d.TooltipHistogram == nil && d.TooltipColorScale == nil
		}},
		{"size limits", `{"tooltip":{"maxWidth":300,"maxHeight":200}}`, func(d spec.Display) bool {
			return d.TooltipMaxWidth == 300 && d.TooltipMaxHeight == 200
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

func TestHeatmapExemplarColor(t *testing.T) {
	d, line := heatmapOptions(t, `{"exemplars":{"color":"red"}}`)
	requireImported(t, line)
	if d.ExemplarColor != "critical" {
		t.Fatalf("display = %+v", d)
	}
	d, line = heatmapOptions(t, `{"exemplars":{"color":"rgba(255,0,255,0.7)"}}`)
	requireImported(t, line)
	if d.ExemplarColor != "" {
		t.Fatalf("Grafana's own exemplar color was written: %+v", d)
	}
	d, line = heatmapOptions(t, `{"exemplars":{"color":"purple"}}`)
	if line.Status != statusPartial || !slices.Contains(line.Paths, "options.exemplars.color") || d.ExemplarColor != "" {
		t.Fatalf("status %s, paths %v, display %+v", line.Status, line.Paths, d)
	}
}

func TestHeatmapLogAndSymlogYScales(t *testing.T) {
	for _, scale := range []string{"log", "symlog"} {
		doc := panelDoc("heatmap", upTarget+`,"fieldConfig":{"defaults":{"custom":{"scaleDistribution":{"type":"`+scale+`","log":2}}},"overrides":[]}`)
		panel, line := convertedPanel(t, doc)
		requireImported(t, line)
		d := displayOf(t, panel)
		if d.YScale != scale || d.YBucketLogBase != 0 {
			t.Fatalf("%s: display = %+v", scale, d)
		}
	}
}

func TestHeatmapWarmSchemeHasARamp(t *testing.T) {
	d, line := heatmapOptions(t, `{"color":{"mode":"scheme","scheme":"Warm"}}`)
	requireImported(t, line)
	if d.ColorScheme != "warm" {
		t.Fatalf("display = %+v", d)
	}
}

func TestHeatmapExemplarsAreKeptOnRangeTargets(t *testing.T) {
	panel, line := exemplarPanel(t, "heatmap", `{"refId":"A","expr":"up","exemplar":true}`)
	if !panel.Queries[0].Exemplar {
		t.Fatalf("query = %+v, want exemplar kept", panel.Queries[0])
	}
	if line.Status != statusImported || len(line.Paths) != 0 {
		t.Fatalf("status = %s, paths %v, want imported", line.Status, line.Paths)
	}
}

func TestLegacyHeatmapWithoutOptionsImportsItsOwnDefaults(t *testing.T) {
	d, line := legacyHeatmapDisplay(t, `"dataFormat":"tsbuckets"`)
	requireImported(t, line)
	if d.Calculate != nil && *d.Calculate {
		t.Fatalf("display = %+v, want pre-bucketed rows", d)
	}
	if d.ColorMode != "scheme" || d.ColorScheme != "redYellowGreen" || d.CellGap != 1 || d.HideBelow == nil || *d.HideBelow != -1 {
		t.Fatalf("display = %+v", d)
	}
}

func TestLegacyHeatmapTimeSeriesFormatCalculatesItsBuckets(t *testing.T) {
	d, line := legacyHeatmapDisplay(t, `"dataFormat":"timeseries","xBucketNumber":60,"xBucketSize":"5m","yBucketSize":2,"yBucketBound":"upper"`)
	requireImported(t, line)
	if d.Calculate == nil || !*d.Calculate || !sameBuckets(d.XBuckets, "count", 60) || !sameBuckets(d.YBuckets, "size", 2) || d.RowsLayout != "" {
		t.Fatalf("display = %+v", d)
	}
	d, line = legacyHeatmapDisplay(t, `"dataFormat":"timeseries","xBucketSize":"5m","yBucketNumber":null`)
	requireImported(t, line)
	if !sameBuckets(d.XBuckets, "size", 300) || d.YBuckets != nil {
		t.Fatalf("display = %+v", d)
	}
}

func TestLegacyHeatmapBucketBounds(t *testing.T) {
	cases := map[string]string{"upper": "le", "lower": "ge", "middle": "", "auto": ""}
	for bound, want := range cases {
		d, line := legacyHeatmapDisplay(t, `"dataFormat":"tsbuckets","yBucketBound":"`+bound+`"`)
		requireImported(t, line)
		if d.RowsLayout != want {
			t.Fatalf("%s: display = %+v, want layout %q", bound, d, want)
		}
	}
}

func TestLegacyHeatmapColors(t *testing.T) {
	cases := []struct {
		name, color string
		check       func(spec.Display) bool
	}{
		{"a spectrum scheme", `{"mode":"spectrum","colorScheme":"interpolateBlues","cardColor":"#b4ff00","colorScale":"sqrt"}`, func(d spec.Display) bool {
			return d.ColorMode == "scheme" && d.ColorScheme == "" && d.ColorFill == "" && d.ColorScale == ""
		}},
		{"a scheme of another ramp", `{"mode":"spectrum","colorScheme":"interpolateViridis"}`, func(d spec.Display) bool {
			return d.ColorScheme == "viridis"
		}},
		{"opacity with a square root scale", `{"mode":"opacity","cardColor":"#112233","colorScale":"sqrt","exponent":0.8}`, func(d spec.Display) bool {
			return d.ColorMode == "" && d.ColorFill == "#112233" && d.ColorScale == "exponential" && d.ColorExponent != nil && *d.ColorExponent == 0.8
		}},
		{"opacity with a linear scale", `{"mode":"opacity","cardColor":"#112233","colorScale":"linear","exponent":0.8}`, func(d spec.Display) bool {
			return d.ColorScale == "" && d.ColorExponent == nil
		}},
		{"a range", `{"mode":"spectrum","min":1,"max":9}`, func(d spec.Display) bool {
			return d.ColorMin != nil && *d.ColorMin == 1 && d.ColorMax != nil && *d.ColorMax == 9
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, line := legacyHeatmapDisplay(t, `"dataFormat":"tsbuckets","color":`+tc.color)
			requireImported(t, line)
			if !tc.check(d) {
				t.Fatalf("display = %+v", d)
			}
		})
	}
}

func TestLegacyHeatmapAxesAndCards(t *testing.T) {
	d, line := legacyHeatmapDisplay(t, `"dataFormat":"tsbuckets","cards":{"cardPadding":3,"cardRound":2},"hideZeroBuckets":true,"reverseYBuckets":true,"yAxis":{"format":"s","decimals":1,"min":0,"max":50,"logBase":2,"show":true,"splitFactor":null},"xAxis":{"show":true},"tooltip":{"show":true,"showHistogram":true},"tooltipDecimals":null,"legend":{"show":false},"highlightCards":true,"heatmap":{}`)
	requireImported(t, line)
	if d.CellGap != 3 || d.CellRadius != 2 || d.HideBelow != nil || d.YReverse == nil || !*d.YReverse {
		t.Fatalf("display = %+v", d)
	}
	if d.YUnit != "s" || d.YDecimals == nil || *d.YDecimals != 1 || d.YMin == nil || *d.YMin != 0 || d.YMax == nil || *d.YMax != 50 {
		t.Fatalf("display = %+v", d)
	}
	if d.YScale != "log" || d.YBucketLogBase != 0 || d.TooltipHistogram == nil || !*d.TooltipHistogram || d.Legend != "hidden" || d.YAxis != "" {
		t.Fatalf("display = %+v", d)
	}
}

func TestLegacyHeatmapHidesWhatItIsTold(t *testing.T) {
	d, line := legacyHeatmapDisplay(t, `"dataFormat":"tsbuckets","tooltip":{"show":false},"yAxis":{"show":false}`)
	requireImported(t, line)
	if d.Tooltip != "hidden" || d.YAxis != "hidden" {
		t.Fatalf("display = %+v", d)
	}
}

func TestLegacyHeatmapOptionsTheRendererCannotDrawAreReported(t *testing.T) {
	cases := []struct{ name, members, path string }{
		{"a hidden time axis", `"xAxis":{"show":false}`, "xAxis.show"},
		{"tooltip decimals", `"tooltipDecimals":2`, "tooltipDecimals"},
		{"a split factor", `"yAxis":{"splitFactor":2}`, "yAxis.splitFactor"},
		{"a log base of 5", `"yAxis":{"logBase":5}`, "yAxis.logBase"},
		{"a card color with no token", `"color":{"mode":"opacity","cardColor":"purple"}`, "color.cardColor"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, line := legacyHeatmapDisplay(t, `"dataFormat":"tsbuckets",`+tc.members)
			if line.Status != statusPartial || !slices.Contains(line.Paths, tc.path) {
				t.Fatalf("status %s, paths %v", line.Status, line.Paths)
			}
		})
	}
}
