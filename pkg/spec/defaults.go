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
	ThresholdMode     string  `json:"thresholdMode"`
	ColorBy           string  `json:"colorBy"`
	Style             string  `json:"style"`
	Fill              int     `json:"fill"`
	LineStyle         string  `json:"lineStyle"`
	LineInterpolation string  `json:"lineInterpolation"`
	LineWidth         int     `json:"lineWidth"`
	Points            string  `json:"points"`
	PointSize         int     `json:"pointSize"`
	PointShape        string  `json:"pointShape"`
	PointStrokeWidth  int     `json:"pointStrokeWidth"`
	SpanNulls         bool    `json:"spanNulls"`
	Stack             string  `json:"stack"`
	Axis              string  `json:"axis"`
	Hidden            bool    `json:"hidden"`
	Scale             string  `json:"scale"`
	LogBase           int     `json:"logBase"`
	AxisCenteredZero  bool    `json:"axisCenteredZero"`
	AxisBorder        bool    `json:"axisBorder"`
	AxisColor         string  `json:"axisColor"`
	HideInLegend      bool    `json:"hideInLegend"`
	HideInTooltip     bool    `json:"hideInTooltip"`
	StackGroup        string  `json:"stackGroup"`
	NullAsZero        bool    `json:"nullAsZero"`
	BarAlignment      string  `json:"barAlignment"`
	BarWidthFactor    float64 `json:"barWidthFactor"`
	BarMaxWidth       int     `json:"barMaxWidth"`
	ThresholdsStyle   string  `json:"thresholdsStyle"`
	Align             string  `json:"align"`
	Filterable        bool    `json:"filterable"`
	Inspect           bool    `json:"inspect"`
	HideHeader        bool    `json:"hideHeader"`
	FieldMinMax       bool    `json:"fieldMinMax"`
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

type TimePickerDefaults struct {
	RefreshIntervals []string `json:"refreshIntervals"`
}

type CanvasElementDefaults struct {
	FontSize    int     `json:"fontSize"`
	Align       string  `json:"align"`
	VAlign      string  `json:"valign"`
	BorderWidth int     `json:"borderWidth"`
	Radius      int     `json:"radius"`
	Rotation    float64 `json:"rotation"`
}

type CanvasConnectionDefaults struct {
	Width     int    `json:"width"`
	Path      string `json:"path"`
	LineStyle string `json:"lineStyle"`
	Arrow     string `json:"arrow"`
}

type TextDefaults struct {
	Mode string `json:"mode"`
}

type ActionDefaults struct {
	Method      string `json:"method"`
	ContentType string `json:"contentType"`
}

type DefaultsTable struct {
	FieldProps       FieldPropsDefaults       `json:"fieldProps"`
	Query            QueryDefaults            `json:"query"`
	Variable         VariableDefaults         `json:"variable"`
	Row              RowDefaults              `json:"row"`
	Link             LinkDefaults             `json:"link"`
	Dashboard        DashboardDefaults        `json:"dashboard"`
	TimePicker       TimePickerDefaults       `json:"timePicker"`
	Display          map[PanelType]Display    `json:"display"`
	CanvasElement    CanvasElementDefaults    `json:"canvasElement"`
	CanvasConnection CanvasConnectionDefaults `json:"canvasConnection"`
	Text             TextDefaults             `json:"text"`
	Action           ActionDefaults           `json:"action"`
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
	table.TimePicker.RefreshIntervals = slices.Clone(table.TimePicker.RefreshIntervals)
	for panelType, display := range table.Display {
		display.LegendValues = slices.Clone(display.LegendValues)
		display.Footer = slices.Clone(display.Footer)
		display.FooterFields = slices.Clone(display.FooterFields)
		display.FooterCountRows = clonePointer(display.FooterCountRows)
		display.Pagination = clonePointer(display.Pagination)
		display.SortDesc = clonePointer(display.SortDesc)
		display.TypeIcons = clonePointer(display.TypeIcons)
		display.Graph = clonePointer(display.Graph)
		display.ThresholdMarkers = clonePointer(display.ThresholdMarkers)
		display.Header = clonePointer(display.Header)
		display.Calculate = clonePointer(display.Calculate)
		display.BucketSize = clonePointer(display.BucketSize)
		display.MergeValues = clonePointer(display.MergeValues)
		display.Wrap = clonePointer(display.Wrap)
		display.ShowTime = clonePointer(display.ShowTime)
		display.ShowLabels = clonePointer(display.ShowLabels)
		display.ShowCommonLabels = clonePointer(display.ShowCommonLabels)
		display.Details = clonePointer(display.Details)
		display.Prettify = clonePointer(display.Prettify)
		display.Controls = clonePointer(display.Controls)
		display.Highlight = clonePointer(display.Highlight)
		display.IncludeAllFields = clonePointer(display.IncludeAllFields)
		display.BarWidth = clonePointer(display.BarWidth)
		display.GroupWidth = clonePointer(display.GroupWidth)
		display.Lat = clonePointer(display.Lat)
		display.Lon = clonePointer(display.Lon)
		display.Zoom = clonePointer(display.Zoom)
		display.KeepTime = clonePointer(display.KeepTime)
		display.IncludeVars = clonePointer(display.IncludeVars)
		display.BasemapLabels = clonePointer(display.BasemapLabels)
		display.BasemapOpacity = clonePointer(display.BasemapOpacity)
		display.MinZoom = clonePointer(display.MinZoom)
		display.MaxZoom = clonePointer(display.MaxZoom)
		display.FitLastOnly = clonePointer(display.FitLastOnly)
		display.FitPadding = clonePointer(display.FitPadding)
		display.SharedView = clonePointer(display.SharedView)
		display.ShowZoom = clonePointer(display.ShowZoom)
		display.MouseWheelZoom = clonePointer(display.MouseWheelZoom)
		display.WheelZoom = clonePointer(display.WheelZoom)
		display.ShowAttribution = clonePointer(display.ShowAttribution)
		display.ShowScale = clonePointer(display.ShowScale)
		display.ShowMeasure = clonePointer(display.ShowMeasure)
		display.ShowDebug = clonePointer(display.ShowDebug)
		display.BorderWidth = clonePointer(display.BorderWidth)
		display.BorderRadius = clonePointer(display.BorderRadius)
		display.PanZoom = clonePointer(display.PanZoom)
		display.InfinitePan = clonePointer(display.InfinitePan)
		display.BackgroundImage = clonePointer(display.BackgroundImage)
		display.Layers = slices.Clone(display.Layers)
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
