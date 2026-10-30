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

func TestNodeGraphStatUnitsApplyOnlyWhenNodesAndEdgesAgree(t *testing.T) {
	cases := []struct {
		name, options string
		units         []string
		paths         []string
		reasons       []string
	}{
		{name: "equal units become one override", options: `"nodes":{"mainStatUnit":"ms"},"edges":{"mainStatUnit":"ms"}`, units: []string{"ms"}},
		{name: "a node unit alone applies", options: `"nodes":{"secondaryStatUnit":"percent"}`, units: []string{"percent"}},
		{name: "an edge unit alone applies", options: `"edges":{"mainStatUnit":"bytes"}`, units: []string{"bytes"}},
		{
			name:    "differing units apply neither and say so",
			options: `"nodes":{"mainStatUnit":"ms"},"edges":{"mainStatUnit":"s"}`,
			paths:   []string{"options.nodes.mainStatUnit", "options.edges.mainStatUnit"},
			reasons: []string{"The node and edge units differ, so neither is applied."},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := convertedPanel(t, panelDoc("nodeGraph", upTarget+`,"options":{`+tc.options+`}`))
			var units []string
			for _, o := range panel.Overrides {
				units = append(units, o.Unit)
			}
			if !slices.Equal(units, tc.units) {
				t.Errorf("override units = %v, want %v", units, tc.units)
			}
			if !slices.Equal(line.Paths, tc.paths) {
				t.Errorf("paths = %v, want %v", line.Paths, tc.paths)
			}
			if !slices.Equal(slices.Compact(line.Reasons), tc.reasons) {
				t.Errorf("reasons = %v, want %v", line.Reasons, tc.reasons)
			}
		})
	}
}
