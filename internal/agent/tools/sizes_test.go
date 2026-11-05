package tools

import (
	"encoding/json"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestDefaultSizes(t *testing.T) {
	tests := []struct {
		panelType spec.PanelType
		w, h      int
	}{
		{spec.Stat, 6, 4},
		{spec.Gauge, 6, 4},
		{spec.Timeseries, 12, 8},
		{spec.BarGauge, 12, 8},
		{spec.BarChart, 12, 8},
		{spec.Heatmap, 12, 8},
		{spec.Histogram, 12, 8},
		{spec.PieChart, 8, 8},
		{spec.Table, 24, 8},
		{spec.StateTimeline, 24, 6},
		{spec.StatusHistory, 24, 6},
		{spec.Logs, 24, 10},
		{spec.Traces, 24, 10},
		{spec.NodeGraph, 24, 10},
		{spec.Geomap, 24, 10},
		{spec.Text, 24, 2},
		{spec.Trend, 12, 8},
	}
	for _, tt := range tests {
		t.Run(string(tt.panelType), func(t *testing.T) {
			got := defaultSize(tt.panelType)
			if got.W != tt.w || got.H != tt.h {
				t.Errorf("size = %dx%d, want %dx%d", got.W, got.H, tt.w, tt.h)
			}
		})
	}
}

func TestGivenSizeKeepsEachValidField(t *testing.T) {
	tests := []struct {
		name    string
		gridPos string
		want    size
	}{
		{"empty object", `{}`, size{}},
		{"only a width", `{"w":8}`, size{W: 8}},
		{"a zero height", `{"w":8,"h":0}`, size{W: 8}},
		{"a width too large", `{"w":25,"h":6}`, size{H: 6}},
		{"a fractional width", `{"w":7.5,"h":4}`, size{H: 4}},
		{"a text width makes the object unreadable", `{"w":"8","h":4}`, size{}},
		{"the largest size", `{"w":24,"h":100}`, size{W: 24, H: 100}},
		{"a height too large", `{"w":6,"h":101}`, size{W: 6}},
		{"a position beside the size", `{"x":3,"y":2,"w":6,"h":4}`, size{W: 6, H: 4}},
		{"absent", ``, size{}},
		{"not an object", `[1]`, size{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := givenSize(json.RawMessage(tt.gridPos)); got != tt.want {
				t.Errorf("size = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestChooseSize(t *testing.T) {
	tests := []struct {
		name      string
		panelType spec.PanelType
		given     size
		open      openLine
		want      spec.GridPos
	}{
		{"default on an empty section", spec.Stat, size{}, openLine{}, spec.GridPos{W: 6, H: 4}},
		{"a stat beside a chart takes its height", spec.Stat, size{}, openLine{X: 12, Y: 1, W: 12, H: 8, Type: spec.Timeseries, LastW: 12}, spec.GridPos{W: 6, H: 8}},
		{"a stat strip keeps its width and height", spec.Stat, size{}, openLine{X: 8, Y: 1, W: 16, H: 3, Type: spec.Stat, LastW: 4}, spec.GridPos{W: 4, H: 3}},
		{"a chart after an 8-wide chart makes thirds", spec.Timeseries, size{}, openLine{X: 8, Y: 1, W: 16, H: 8, Type: spec.Timeseries, LastW: 8}, spec.GridPos{W: 8, H: 8}},
		{"a chart beside a short strip keeps its height", spec.Timeseries, size{}, openLine{X: 12, Y: 1, W: 12, H: 4, Type: spec.Stat, LastW: 6}, spec.GridPos{W: 12, H: 8}},
		{"a chart after a 16-wide focus chart does not fit and keeps the default", spec.Timeseries, size{}, openLine{X: 16, Y: 1, W: 8, H: 8, Type: spec.Timeseries, LastW: 16}, spec.GridPos{W: 12, H: 8}},
		{"a sent width alone gets the line's height", spec.Stat, size{W: 4}, openLine{X: 12, Y: 1, W: 12, H: 10, Type: spec.BarGauge, LastW: 12}, spec.GridPos{W: 4, H: 10}},
		{"a full-width chart gets the headline height", spec.Timeseries, size{W: 24}, openLine{}, spec.GridPos{W: 24, H: 7}},
		{"a sent size is kept", spec.Stat, size{W: 4, H: 3}, openLine{X: 12, Y: 1, W: 12, H: 8, Type: spec.Timeseries, LastW: 12}, spec.GridPos{W: 4, H: 3}},
		{"a sent height is kept beside a taller line", spec.Stat, size{H: 3}, openLine{X: 12, Y: 1, W: 12, H: 8, Type: spec.Timeseries, LastW: 12}, spec.GridPos{W: 6, H: 3}},
		{"a table never joins a line", spec.Table, size{}, openLine{X: 12, Y: 1, W: 12, H: 8, Type: spec.Timeseries, LastW: 12}, spec.GridPos{W: 24, H: 8}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := chooseSize(tt.panelType, tt.given, tt.open); got != tt.want {
				t.Errorf("size = %+v, want %+v", got, tt.want)
			}
		})
	}
}
