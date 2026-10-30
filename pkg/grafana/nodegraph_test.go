package grafana

import (
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestNodeGraphLayoutAndZoomImport(t *testing.T) {
	cases := []struct {
		name, options, display string
		paths                  []string
	}{
		{name: "force layout", options: `"layoutAlgorithm":"force"`, display: `{"layout":"force"}`},
		{name: "grid layout", options: `"layoutAlgorithm":"grid"`, display: `{"layout":"grid"}`},
		{name: "layered is the Spec default", options: `"layoutAlgorithm":"layered"`, display: "null"},
		{name: "greedy zoom turns the wheel on", options: `"zoomMode":"greedy"`, display: `{"wheelZoom":true}`},
		{name: "cooperative zoom is Grafana's default", options: `"zoomMode":"cooperative"`, display: "null"},
		{name: "force layout with greedy zoom", options: `"layoutAlgorithm":"force","zoomMode":"greedy"`, display: `{"layout":"force","wheelZoom":true}`},
		{name: "unknown layout is reported", options: `"layoutAlgorithm":"radial"`, display: "null", paths: []string{"options.layoutAlgorithm"}},
		{name: "unknown zoom mode is reported", options: `"zoomMode":"sticky"`, display: "null", paths: []string{"options.zoomMode"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := convertedPanel(t, panelDoc("nodeGraph", upTarget+`,"options":{`+tc.options+`}`))
			if panel.Type != spec.NodeGraph {
				t.Fatalf("type = %s", panel.Type)
			}
			if got := displayJSON(t, panel); got != tc.display {
				t.Errorf("display = %s, want %s", got, tc.display)
			}
			if !slices.Equal(line.Paths, tc.paths) {
				t.Errorf("paths = %v, want %v (reasons %v)", line.Paths, tc.paths, line.Reasons)
			}
		})
	}
}
