package tools

import "github.com/slashobserve/slashobserve/pkg/spec"

var defaultSizes = map[spec.PanelType]spec.GridPos{
	spec.Timeseries: {W: 12, H: 8},
	spec.Stat:       {W: 6, H: 4},
	spec.Gauge:      {W: 6, H: 4},
	spec.Table:      {W: 24, H: 8},
}

var fallbackSize = spec.GridPos{W: 12, H: 8}

func defaultSize(t spec.PanelType) spec.GridPos {
	if size, ok := defaultSizes[t]; ok {
		return size
	}
	return fallbackSize
}
