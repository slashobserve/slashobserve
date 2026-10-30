package grafana

import (
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func customPanel(t *testing.T, defaults, overrides string) (spec.Panel, PanelLine) {
	t.Helper()
	return customPanelOf(t, "timeseries", defaults, overrides)
}

func customPanelOf(t *testing.T, typ, defaults, overrides string) (spec.Panel, PanelLine) {
	t.Helper()
	return convertedPanel(t, panelDoc(typ, upTarget+`,"fieldConfig":{"defaults":{"custom":`+defaults+`},"overrides":[`+overrides+`]}`))
}

func intOf(p *int) int {
	if p == nil {
		return -1
	}
	return *p
}

func requireImported(t *testing.T, line PanelLine) {
	t.Helper()
	if line.Status != statusImported {
		t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
	}
}

func TestLineStyleModernOptions(t *testing.T) {
	cases := []struct {
		name, custom string
		check        func(spec.FieldProps) bool
	}{
		{"smooth", `{"lineInterpolation":"smooth"}`, func(p spec.FieldProps) bool { return p.LineInterpolation == "smooth" }},
		{"step before", `{"lineInterpolation":"stepBefore"}`, func(p spec.FieldProps) bool { return p.LineInterpolation == "stepBefore" }},
		{"step after", `{"lineInterpolation":"stepAfter"}`, func(p spec.FieldProps) bool { return p.LineInterpolation == "stepAfter" }},
		{"linear is the Spec default", `{"lineInterpolation":"linear"}`, func(p spec.FieldProps) bool { return p.LineInterpolation == "" }},
		{"line width 3", `{"lineWidth":3}`, func(p spec.FieldProps) bool { return intOf(p.LineWidth) == 3 }},
		{"line width 0", `{"lineWidth":0}`, func(p spec.FieldProps) bool { return intOf(p.LineWidth) == 0 }},
		{"line width is clamped", `{"lineWidth":14}`, func(p spec.FieldProps) bool { return intOf(p.LineWidth) == 10 }},
		{"Grafana's line width 1 keeps the canvas look", `{"lineWidth":1}`, func(p spec.FieldProps) bool { return p.LineWidth == nil }},
		{"line width 2 is the Spec default", `{"lineWidth":2}`, func(p spec.FieldProps) bool { return p.LineWidth == nil }},
		{"points always", `{"showPoints":"always"}`, func(p spec.FieldProps) bool { return p.Points == "always" }},
		{"points never", `{"showPoints":"never"}`, func(p spec.FieldProps) bool { return p.Points == "never" }},
		{"points auto", `{"showPoints":"auto"}`, func(p spec.FieldProps) bool { return p.Points == "" }},
		{"point size with always", `{"showPoints":"always","pointSize":8}`, func(p spec.FieldProps) bool { return intOf(p.PointSize) == 8 }},
		{"point size with the points draw style", `{"drawStyle":"points","pointSize":9}`, func(p spec.FieldProps) bool { return p.Style == "points" && intOf(p.PointSize) == 9 }},
		{"point size is clamped", `{"showPoints":"always","pointSize":99}`, func(p spec.FieldProps) bool { return intOf(p.PointSize) == 40 }},
		{"Grafana's point size 5 keeps the canvas look", `{"showPoints":"always","pointSize":5}`, func(p spec.FieldProps) bool { return p.PointSize == nil }},
		{"point size with points never shown is ignored", `{"showPoints":"never","pointSize":12}`, func(p spec.FieldProps) bool { return p.PointSize == nil }},
		{"point size with bars is ignored", `{"drawStyle":"bars","pointSize":12}`, func(p spec.FieldProps) bool { return p.PointSize == nil }},
		{"Grafana's point size 5 with automatic points is its default", `{"showPoints":"auto","pointSize":5}`, func(p spec.FieldProps) bool { return p.PointSize == nil }},
		{"span nulls true", `{"spanNulls":true}`, func(p spec.FieldProps) bool { return p.SpanNulls == true }},
		{"span nulls as milliseconds", `{"spanNulls":3600000}`, func(p spec.FieldProps) bool { return p.SpanNulls == float64(3600000) }},
		{"span nulls false", `{"spanNulls":false}`, func(p spec.FieldProps) bool { return p.SpanNulls == nil }},
		{"span nulls zero", `{"spanNulls":0}`, func(p spec.FieldProps) bool { return p.SpanNulls == nil }},
		{"gradient mode is drawn flat", `{"gradientMode":"opacity"}`, fieldPropsEmpty},
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

func TestLineStyleGradientModesAreAllSilent(t *testing.T) {
	for _, mode := range []string{"none", "opacity", "hue", "scheme"} {
		t.Run(mode, func(t *testing.T) {
			panel, line := customPanel(t, `{"gradientMode":"`+mode+`"}`, "")
			requireImported(t, line)
			if !fieldPropsEmpty(panel.FieldProps) {
				t.Fatalf("field props = %+v", panel.FieldProps)
			}
		})
	}
}

func TestLineStyleUnknownInterpolationIsReported(t *testing.T) {
	_, line := customPanel(t, `{"lineInterpolation":"cubic"}`, "")
	if line.Status != statusPartial || len(line.Paths) != 1 || line.Paths[0] != "fieldConfig.defaults.custom.lineInterpolation" {
		t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
	}
	if !strings.Contains(strings.Join(line.Reasons, " "), "The line interpolation cubic is not supported yet.") {
		t.Fatalf("reasons = %v", line.Reasons)
	}
}

func TestLineStyleOverridesWriteExplicitValues(t *testing.T) {
	override := func(properties string) string {
		return `{"matcher":{"id":"byName","options":"a"},"properties":[` + properties + `]}`
	}
	cases := []struct {
		name, properties string
		check            func(spec.FieldProps) bool
	}{
		{"linear over a smooth default", `{"id":"custom.lineInterpolation","value":"linear"}`, func(p spec.FieldProps) bool { return p.LineInterpolation == "linear" }},
		{"line width", `{"id":"custom.lineWidth","value":4}`, func(p spec.FieldProps) bool { return intOf(p.LineWidth) == 4 }},
		{"Grafana's line width 1 becomes the canvas width", `{"id":"custom.lineWidth","value":1}`, func(p spec.FieldProps) bool { return intOf(p.LineWidth) == 2 }},
		{"points auto", `{"id":"custom.showPoints","value":"auto"}`, func(p spec.FieldProps) bool { return p.Points == "auto" }},
		{"point size before show points", `{"id":"custom.pointSize","value":7},{"id":"custom.showPoints","value":"always"}`, func(p spec.FieldProps) bool { return intOf(p.PointSize) == 7 && p.Points == "always" }},
		{"span nulls off", `{"id":"custom.spanNulls","value":false}`, func(p spec.FieldProps) bool { return p.SpanNulls == false }},
		{"span nulls as milliseconds", `{"id":"custom.spanNulls","value":60000}`, func(p spec.FieldProps) bool { return p.SpanNulls == float64(60000) }},
		{"gradient mode", `{"id":"custom.gradientMode","value":"hue"}`, fieldPropsEmpty},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := customPanel(t, `{}`, override(tc.properties))
			requireImported(t, line)
			var got spec.FieldProps
			if len(panel.Overrides) == 1 {
				got = panel.Overrides[0].FieldProps
			} else if len(panel.Overrides) != 0 || !fieldPropsEmpty(panel.FieldProps) {
				t.Fatalf("overrides = %+v", panel.Overrides)
			}
			if !tc.check(got) {
				t.Fatalf("override props = %+v", got)
			}
		})
	}
}

func TestLineStyleLegacyGraphOptions(t *testing.T) {
	cases := []struct {
		name, members string
		check         func(spec.FieldProps) bool
	}{
		{"stepped line draws step after", `"steppedLine":true`, func(p spec.FieldProps) bool { return p.LineInterpolation == "stepAfter" }},
		{"not stepped", `"steppedLine":false`, func(p spec.FieldProps) bool { return p.LineInterpolation == "" }},
		{"connected null points", `"nullPointMode":"connected"`, func(p spec.FieldProps) bool { return p.SpanNulls == true }},
		{"null points", `"nullPointMode":"null"`, func(p spec.FieldProps) bool { return p.SpanNulls == nil }},
		{"line width", `"linewidth":3`, func(p spec.FieldProps) bool { return intOf(p.LineWidth) == 3 }},
		{"Grafana's line width 1", `"linewidth":1`, func(p spec.FieldProps) bool { return p.LineWidth == nil }},
		{"points with a radius", `"points":true,"pointradius":3`, func(p spec.FieldProps) bool { return p.Points == "always" && intOf(p.PointSize) == 6 }},
		{"points with the canvas radius", `"points":true,"pointradius":2`, func(p spec.FieldProps) bool { return p.Points == "always" && p.PointSize == nil }},
		{"no points", `"points":false,"pointradius":3`, func(p spec.FieldProps) bool { return p.Points == "" && p.PointSize == nil }},
		{"points only", `"lines":false,"points":true,"pointradius":4`, func(p spec.FieldProps) bool {
			return p.Style == "points" && p.Points == "" && intOf(p.PointSize) == 8
		}},
		{"fill gradient is drawn flat", `"fillGradient":6`, fieldPropsEmpty},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := graphPanel(t, tc.members)
			requireImported(t, line)
			if !tc.check(panel.FieldProps) {
				t.Fatalf("field props = %+v", panel.FieldProps)
			}
		})
	}
}

func TestLineStyleLegacySeriesOverrides(t *testing.T) {
	cases := []struct {
		name, override string
		check          func(spec.FieldProps) bool
	}{
		{"line width", `"linewidth":3`, func(p spec.FieldProps) bool { return intOf(p.LineWidth) == 3 }},
		{"stepped line", `"steppedLine":true`, func(p spec.FieldProps) bool { return p.LineInterpolation == "stepAfter" }},
		{"not stepped", `"steppedLine":false`, func(p spec.FieldProps) bool { return p.LineInterpolation == "linear" }},
		{"points beside lines", `"points":true,"pointradius":3`, func(p spec.FieldProps) bool { return p.Points == "always" && intOf(p.PointSize) == 6 }},
		{"connected null points", `"nullPointMode":"connected"`, func(p spec.FieldProps) bool { return p.SpanNulls == true }},
		{"null points", `"nullPointMode":"null"`, func(p spec.FieldProps) bool { return p.SpanNulls == false }},
		{"fill gradient", `"linewidth":2,"fillGradient":4`, func(p spec.FieldProps) bool { return intOf(p.LineWidth) == 2 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := graphPanel(t, `"seriesOverrides":[{"alias":"a",`+tc.override+`}]`)
			requireImported(t, line)
			if len(panel.Overrides) != 1 || panel.Overrides[0].Match.Series != "a" {
				t.Fatalf("overrides = %+v", panel.Overrides)
			}
			if !tc.check(panel.Overrides[0].FieldProps) {
				t.Fatalf("override props = %+v", panel.Overrides[0].FieldProps)
			}
		})
	}
}

func TestPointSizeOfAutomaticPointsIsReported(t *testing.T) {
	for _, custom := range []string{`{"pointSize":12}`, `{"showPoints":"auto","pointSize":12}`} {
		t.Run(custom, func(t *testing.T) {
			panel, line := customPanel(t, custom, "")
			requireReported(t, line, "fieldConfig.defaults.custom.pointSize", "A point size applies only to points")
			if panel.PointSize != nil {
				t.Fatalf("point size = %d", *panel.PointSize)
			}
		})
	}
}

func TestLineStyleFillAndDash(t *testing.T) {
	cases := []struct {
		name, lineStyle string
		want            string
		path, reason    string
	}{
		{"dash with Grafana's own pattern", `{"fill":"dash","dash":[10,10]}`, "dashed", "", ""},
		{"dash without a pattern", `{"fill":"dash"}`, "dashed", "", ""},
		{"dash with another pattern", `{"fill":"dash","dash":[10,15]}`, "dashed", "fieldConfig.defaults.custom.lineStyle.dash", "one pattern"},
		{"dot", `{"fill":"dot"}`, "dotted", "", ""},
		{"solid is the Spec default and ignores a pattern", `{"fill":"solid","dash":[10,15]}`, "", "", ""},
		{"square", `{"fill":"square"}`, "", "fieldConfig.defaults.custom.lineStyle.fill", "The line style square is not supported yet."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := customPanel(t, `{"lineStyle":`+tc.lineStyle+`}`, "")
			if panel.LineStyle != tc.want {
				t.Fatalf("line style = %q, want %q", panel.LineStyle, tc.want)
			}
			if tc.path == "" {
				requireImported(t, line)
				return
			}
			requireReported(t, line, tc.path, tc.reason)
		})
	}
}

func TestAxisPlacement(t *testing.T) {
	cases := []struct {
		name, placement, want, reason string
	}{
		{"right", "right", "right", ""},
		{"hidden", "hidden", "hidden", ""},
		{"left is the Spec default", "left", "", ""},
		{"auto is Grafana's default", "auto", "", ""},
		{"top", "top", "", "The axis placement top is not supported yet."},
		{"bottom", "bottom", "", "The axis placement bottom is not supported yet."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := customPanel(t, `{"axisPlacement":"`+tc.placement+`"}`, "")
			if panel.Axis != tc.want {
				t.Fatalf("axis = %q, want %q", panel.Axis, tc.want)
			}
			if tc.reason == "" {
				requireImported(t, line)
				return
			}
			requireReported(t, line, "fieldConfig.defaults.custom.axisPlacement", tc.reason)
		})
	}
}
