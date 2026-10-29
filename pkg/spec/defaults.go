package spec

import (
	"bytes"
	"encoding/json"
	"maps"
	"slices"
	"sync"

	"github.com/slashobserve/slashobserve/spec/schema"
)

type FieldPropsDefaults struct {
	ThresholdMode string `json:"thresholdMode"`
	Style         string `json:"style"`
	Fill          int    `json:"fill"`
	LineStyle     string `json:"lineStyle"`
	Stack         string `json:"stack"`
	Axis          string `json:"axis"`
	Hidden        bool   `json:"hidden"`
}

type QueryDefaults struct {
	Instant bool   `json:"instant"`
	Hide    bool   `json:"hide"`
	Format  string `json:"format"`
}

type VariableDefaults struct {
	Refresh    string `json:"refresh"`
	Multi      bool   `json:"multi"`
	IncludeAll bool   `json:"includeAll"`
}

type RowDefaults struct {
	Collapsed bool `json:"collapsed"`
}

type LinkDefaults struct {
	NewTab bool `json:"newTab"`
}

type DashboardDefaults struct {
	CursorSync CursorSync `json:"cursorSync"`
}

type CanvasElementDefaults struct {
	FontSize int    `json:"fontSize"`
	Align    string `json:"align"`
}

type CanvasConnectionDefaults struct {
	Width int `json:"width"`
}

type DefaultsTable struct {
	FieldProps       FieldPropsDefaults       `json:"fieldProps"`
	Query            QueryDefaults            `json:"query"`
	Variable         VariableDefaults         `json:"variable"`
	Row              RowDefaults              `json:"row"`
	Link             LinkDefaults             `json:"link"`
	Dashboard        DashboardDefaults        `json:"dashboard"`
	Display          map[PanelType]Display    `json:"display"`
	CanvasElement    CanvasElementDefaults    `json:"canvasElement"`
	CanvasConnection CanvasConnectionDefaults `json:"canvasConnection"`
	InstantPanels    []PanelType              `json:"instantPanels"`
}

var defaults = sync.OnceValue(func() DefaultsTable {
	var table DefaultsTable
	dec := json.NewDecoder(bytes.NewReader(schema.Defaults))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&table); err != nil {
		panic("spec: embedded defaults are not valid: " + err.Error())
	}
	if len(table.Display) == 0 {
		panic("spec: embedded defaults have no display table")
	}
	return table
})

func Defaults() DefaultsTable {
	table := defaults()
	table.Display = maps.Clone(table.Display)
	table.InstantPanels = slices.Clone(table.InstantPanels)
	for panelType, display := range table.Display {
		display.LegendValues = slices.Clone(display.LegendValues)
		display.Footer = slices.Clone(display.Footer)
		display.Graph = clonePointer(display.Graph)
		display.ThresholdMarkers = clonePointer(display.ThresholdMarkers)
		display.Header = clonePointer(display.Header)
		display.Calculate = clonePointer(display.Calculate)
		display.BucketSize = clonePointer(display.BucketSize)
		display.MergeValues = clonePointer(display.MergeValues)
		display.Wrap = clonePointer(display.Wrap)
		display.ShowTime = clonePointer(display.ShowTime)
		display.BarWidth = clonePointer(display.BarWidth)
		display.GroupWidth = clonePointer(display.GroupWidth)
		display.Lat = clonePointer(display.Lat)
		display.Lon = clonePointer(display.Lon)
		display.Zoom = clonePointer(display.Zoom)
		display.KeepTime = clonePointer(display.KeepTime)
		display.IncludeVars = clonePointer(display.IncludeVars)
		display.Elements = slices.Clone(display.Elements)
		display.Connections = slices.Clone(display.Connections)
		display.Tags = slices.Clone(display.Tags)
		table.Display[panelType] = display
	}
	return table
}

var instantPanels = sync.OnceValue(func() map[PanelType]bool {
	set := map[PanelType]bool{}
	for _, t := range defaults().InstantPanels {
		set[t] = true
	}
	return set
})

func InstantByDefault(t PanelType) bool {
	return instantPanels()[t]
}

func clonePointer[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
