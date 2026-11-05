package tools

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/prompts"
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

const guideSizesPrefix = "- Default sizes, `w`x`h`: "
const guideSizesSuffix = "; a row is 24x1."

func impliedGuideLine() string {
	bySize := map[string][]string{}
	for panelType, g := range defaultSizes {
		key := fmt.Sprintf("%dx%d", g.W, g.H)
		bySize[key] = append(bySize[key], string(panelType))
	}
	var groups []string
	for key, names := range bySize {
		slices.Sort(names)
		groups = append(groups, strings.Join(names, ", ")+" "+key)
	}
	slices.Sort(groups)
	groups = append(groups, fmt.Sprintf("others %dx%d", fallbackSize.W, fallbackSize.H))
	return guideSizesPrefix + strings.Join(groups, "; ") + guideSizesSuffix
}

func TestTheGuideStatesEveryDefaultSize(t *testing.T) {
	var line string
	for _, l := range strings.Split(prompts.SpecGuide(), "\n") {
		if strings.HasPrefix(l, guideSizesPrefix) {
			line = l
		}
	}
	if line == "" || !strings.HasSuffix(line, guideSizesSuffix) {
		t.Fatalf("the guide has no default sizes line ending in %q; the table implies %q", guideSizesSuffix, impliedGuideLine())
	}
	stated := map[spec.PanelType]bool{}
	others := false
	for group := range strings.SplitSeq(strings.TrimSuffix(strings.TrimPrefix(line, guideSizesPrefix), guideSizesSuffix), "; ") {
		fields := strings.Fields(group)
		var w, h int
		if _, err := fmt.Sscanf(fields[len(fields)-1], "%dx%d", &w, &h); err != nil {
			t.Fatalf("group %q has no WxH size: %v", group, err)
		}
		got := spec.GridPos{W: w, H: h}
		names := strings.NewReplacer(" and ", ", ").Replace(strings.Join(fields[:len(fields)-1], " "))
		for name := range strings.SplitSeq(names, ", ") {
			if name == "others" {
				others = true
				if got != fallbackSize {
					t.Errorf("the guide gives others %+v, fallbackSize is %+v; the table implies %q", got, fallbackSize, impliedGuideLine())
				}
				continue
			}
			want, ok := defaultSizes[spec.PanelType(name)]
			if !ok || want != got {
				t.Errorf("the guide gives %s %+v, defaultSizes has %+v (listed %v); the table implies %q", name, got, want, ok, impliedGuideLine())
			}
			stated[spec.PanelType(name)] = true
		}
	}
	if !others {
		t.Errorf("the guide does not state the size of other types; the table implies %q", impliedGuideLine())
	}
	for panelType := range defaultSizes {
		if !stated[panelType] {
			t.Errorf("the guide does not state the size of %s; the table implies %q", panelType, impliedGuideLine())
		}
	}
}
