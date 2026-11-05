package tools

import (
	"encoding/json"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

var defaultSizes = map[spec.PanelType]spec.GridPos{
	spec.Stat:          {W: 6, H: 4},
	spec.Gauge:         {W: 6, H: 4},
	spec.Timeseries:    {W: 12, H: 8},
	spec.BarGauge:      {W: 12, H: 8},
	spec.BarChart:      {W: 12, H: 8},
	spec.Heatmap:       {W: 12, H: 8},
	spec.Histogram:     {W: 12, H: 8},
	spec.PieChart:      {W: 8, H: 8},
	spec.Table:         {W: 24, H: 8},
	spec.StateTimeline: {W: 24, H: 6},
	spec.StatusHistory: {W: 24, H: 6},
	spec.Logs:          {W: 24, H: 10},
	spec.Traces:        {W: 24, H: 10},
	spec.NodeGraph:     {W: 24, H: 10},
	spec.Geomap:        {W: 24, H: 10},
	spec.Text:          {W: 24, H: 2},
}

var fallbackSize = spec.GridPos{W: 12, H: 8}

const headlineHeight = 7

type size struct{ W, H int }

func defaultSize(t spec.PanelType) spec.GridPos {
	if size, ok := defaultSizes[t]; ok {
		return size
	}
	return fallbackSize
}

func givenSize(gridPos json.RawMessage) size {
	var pos struct {
		W *float64 `json:"w"`
		H *float64 `json:"h"`
	}
	if json.Unmarshal(gridPos, &pos) != nil {
		return size{}
	}
	var given size
	given.W, _ = wholeWithin(pos.W, 1, maxPanelW)
	given.H, _ = wholeWithin(pos.H, 1, maxPanelH)
	return given
}

func (s size) or(fallback spec.GridPos) (w, h int) {
	w, h = s.W, s.H
	if w == 0 {
		w = fallback.W
	}
	if h == 0 {
		h = fallback.H
	}
	return w, h
}

func chooseSize(t spec.PanelType, given size, open openLine) spec.GridPos {
	def := defaultSize(t)
	w, h := given.W, given.H
	same := open.W > 0 && open.Type == t
	if w == 0 {
		w = def.W
		if same && open.LastW <= open.W {
			w = open.LastW
		}
	}
	if h == 0 {
		h = def.H
		switch {
		case open.W > 0 && w <= open.W && (same || open.H > def.H):
			h = open.H
		case t == spec.Timeseries && w == spec.GridColumns:
			h = headlineHeight
		}
	}
	return spec.GridPos{W: w, H: h}
}
