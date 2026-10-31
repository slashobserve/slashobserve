package grafana

import (
	"strings"
	"testing"
)

const (
	oneStep  = `{"mode":"absolute","steps":[{"color":"light-blue","value":null}]}`
	twoSteps = `{"mode":"absolute","steps":[{"color":"light-blue","value":null},{"color":"red","value":10}]}`
)

func thresholdColorPanel(t *testing.T, typ, thresholds, overrides string) (panelColors, PanelLine) {
	t.Helper()
	fieldConfig := `"fieldConfig":{"defaults":{"color":{"mode":"thresholds"},"thresholds":` + thresholds + `},"overrides":[` + overrides + `]}`
	panel, line := convertedPanel(t, panelDoc(typ, upTarget+`,`+fieldConfig))
	colors := panelColors{color: panel.Color, mode: panel.ColorMode}
	if len(panel.Overrides) > 0 {
		colors.overrideColor, colors.overrideMode = panel.Overrides[0].Color, panel.Overrides[0].ColorMode
	}
	return colors, line
}

type panelColors struct{ color, mode, overrideColor, overrideMode string }

func TestHistogramThresholdColorWithOnlyABaseStepImportsAsAFixedColor(t *testing.T) {
	colors, line := thresholdColorPanel(t, "histogram", oneStep, "")
	requireImported(t, line)
	if colors.color != "#8AB8FF" || colors.mode != "" {
		t.Fatalf("color %q mode %q, want the light blue and no mode", colors.color, colors.mode)
	}
}

func TestHistogramThresholdColorWithMoreStepsIsStillReported(t *testing.T) {
	colors, line := thresholdColorPanel(t, "histogram", twoSteps, "")
	requireReported(t, line, "fieldConfig.defaults.color", "The color mode thresholds is not drawn on histogram panels.")
	if colors.color != "" {
		t.Fatalf("color = %q, want none", colors.color)
	}
}

func TestThresholdColorOnOtherPanelsIsUnaffectedByTheHistogramRule(t *testing.T) {
	for _, typ := range []string{"timeseries", "stat"} {
		t.Run(typ, func(t *testing.T) {
			colors, line := thresholdColorPanel(t, typ, oneStep, "")
			requireImported(t, line)
			if colors.color != "" {
				t.Fatalf("color = %q, want none", colors.color)
			}
		})
	}
}

func TestHistogramThresholdColorOverridesFollowTheSameRule(t *testing.T) {
	override := func(thresholds string) string {
		props := []string{property("color", `{"mode":"thresholds"}`)}
		if thresholds != "" {
			props = append(props, property("thresholds", thresholds))
		}
		return `{"matcher":{"id":"byName","options":"up"},"properties":[` + strings.Join(props, ",") + `]}`
	}
	cases := []struct {
		name, defaults, override string
		wantColor                string
	}{
		{"one step in the override", twoSteps, override(oneStep), "#8AB8FF"},
		{"override without thresholds uses the defaults", oneStep, override(""), "#8AB8FF"},
		{"two steps in the override", oneStep, override(twoSteps), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			colors, line := thresholdColorPanel(t, "histogram", tc.defaults, tc.override)
			if tc.wantColor == "" {
				if line.Status != statusPartial || colors.overrideColor != "" {
					t.Fatalf("status %s, override color %q, want the color mode reported", line.Status, colors.overrideColor)
				}
				return
			}
			if colors.overrideColor != tc.wantColor {
				t.Fatalf("override color %q, want %q", colors.overrideColor, tc.wantColor)
			}
		})
	}
}
