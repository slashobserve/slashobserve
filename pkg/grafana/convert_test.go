package grafana

import (
	"fmt"
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const panelFrame = `"id":1,"title":"T","gridPos":{"x":0,"y":0,"w":12,"h":8},"datasource":{"type":"prometheus","uid":"prom"}`

const upTarget = `"targets":[{"refId":"A","expr":"up"}]`

func panelDoc(typ, members string) string {
	return fmt.Sprintf(`{"title":"T","schemaVersion":39,"panels":[{%s,"type":%q,%s}]}`, panelFrame, typ, members)
}

func convertedPanel(t *testing.T, doc string) (spec.Panel, PanelLine) {
	t.Helper()
	res := convertDoc(t, doc)
	if !res.Ready() {
		t.Fatalf("result is not ready: %+v", res.Datasources)
	}
	entry, ok := res.Spec.FindPanel("p1")
	if !ok {
		t.Fatalf("no panel p1 in the Spec\n%s", doc)
	}
	return *entry.Panel, panelLine(t, res, "p1")
}

func graphPanel(t *testing.T, members string) (spec.Panel, PanelLine) {
	t.Helper()
	return convertedPanel(t, panelDoc("graph", upTarget+","+members))
}

func singlestatPanel(t *testing.T, members string) (spec.Panel, PanelLine) {
	t.Helper()
	return convertedPanel(t, panelDoc("singlestat", upTarget+","+members))
}

func displayOf(t *testing.T, panel spec.Panel) spec.Display {
	t.Helper()
	if panel.Display == nil {
		t.Fatalf("panel has no display: %+v", panel)
	}
	return *panel.Display
}

func TestLegacyGraphLegendSurvivesTheDisplayStep(t *testing.T) {
	cases := []struct{ name, legend, want string }{
		{"hidden", `{"show":false}`, "hidden"},
		{"right side", `{"show":true,"rightSide":true}`, "right"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := graphPanel(t, `"legend":`+tc.legend)
			if got := displayOf(t, panel).Legend; got != tc.want {
				t.Fatalf("legend = %q, want %q", got, tc.want)
			}
			if line.Status != statusImported {
				t.Fatalf("status = %s, paths %v", line.Status, line.Paths)
			}
		})
	}
}

func TestSinglestatColorModeSurvivesTheDisplayStep(t *testing.T) {
	cases := []struct{ name, members, want string }{
		{"background", `"colorBackground":true`, "background"},
		{"neither", `"colorBackground":false,"colorValue":false`, "none"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, _ := singlestatPanel(t, tc.members)
			if got := displayOf(t, panel).ColorMode; got != tc.want {
				t.Fatalf("colorMode = %q, want %q", got, tc.want)
			}
		})
	}
}

func thresholdText(steps []spec.Threshold) []string {
	out := make([]string, len(steps))
	for i, step := range steps {
		out[i] = step.Color
		if step.Value != nil {
			out[i] += fmt.Sprintf("@%g", *step.Value)
		}
	}
	return out
}

func TestLegacyGraphThresholdsKeepTheirColorMode(t *testing.T) {
	cases := []struct{ mode, want string }{
		{"critical", "critical@80"},
		{"warning", "warning@80"},
		{"ok", "ok@80"},
	}
	for _, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			panel, line := graphPanel(t, `"thresholds":[{"colorMode":"`+tc.mode+`","op":"gt","value":80,"fill":true,"line":true}]`)
			if got := thresholdText(panel.Thresholds); !slices.Equal(got, []string{tc.want}) {
				t.Fatalf("thresholds = %v, want [%s]", got, tc.want)
			}
			if len(line.Paths) != 0 {
				t.Fatalf("paths = %v, reasons = %v", line.Paths, line.Reasons)
			}
		})
	}
}
