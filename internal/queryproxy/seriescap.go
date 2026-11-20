package queryproxy

import (
	"math"
	"slices"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const MaxPanelSeries = 50

func CapsSeries(t spec.PanelType) bool {
	return t == spec.Timeseries || t == spec.Trend
}

type seriesUnit struct {
	frame, field int
	peak         float64
}

func CapSeries(frames []datasource.Frame, limit int) (kept []datasource.Frame, shown, total int) {
	var units []seriesUnit
	for i, f := range frames {
		if !f.IsSeries() {
			continue
		}
		for j := 1; j < len(f.Fields); j++ {
			units = append(units, seriesUnit{frame: i, field: j, peak: peak(f.Fields[j])})
		}
	}
	if len(units) <= limit {
		return frames, len(units), len(units)
	}
	ranked := slices.Clone(units)
	slices.SortStableFunc(ranked, func(a, b seriesUnit) int {
		switch {
		case a.peak > b.peak:
			return -1
		case a.peak < b.peak:
			return 1
		}
		return 0
	})
	drop := map[[2]int]bool{}
	for _, u := range ranked[limit:] {
		drop[[2]int{u.frame, u.field}] = true
	}
	for i, f := range frames {
		if !f.IsSeries() {
			kept = append(kept, f)
			continue
		}
		fields := []datasource.Field{f.Fields[0]}
		for j := 1; j < len(f.Fields); j++ {
			if !drop[[2]int{i, j}] {
				fields = append(fields, f.Fields[j])
			}
		}
		if len(fields) > 1 {
			f.Fields = fields
			kept = append(kept, f)
		}
	}
	return kept, limit, len(units)
}

func peak(field datasource.Field) float64 {
	best := math.Inf(-1)
	for _, v := range field.Values {
		if n, ok := v.(float64); ok && n > best {
			best = n
		}
	}
	return best
}
