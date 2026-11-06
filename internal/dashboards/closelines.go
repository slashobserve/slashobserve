package dashboards

import (
	"cmp"
	"encoding/json"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func closeLines(d *spec.Dashboard) (*spec.Dashboard, error) {
	sections := sectionsOf(d)
	widened := map[string]spec.GridPos{}
	for _, section := range sections {
		for id, pos := range widenedLines(section) {
			widened[id] = pos
		}
	}
	var patch spec.PanelPatch
	for _, section := range sections {
		for _, p := range section {
			pos, isWidened := widened[p.ID]
			if !isWidened {
				pos = p.GridPos
			}
			switch {
			case wantsFullWidthLegend(p, pos):
				raw, err := json.Marshal(withRightLegend(p, pos))
				if err != nil {
					return nil, err
				}
				patch.Upsert = append(patch.Upsert, raw)
			case isWidened:
				raw, err := json.Marshal(pos)
				if err != nil {
					return nil, err
				}
				patch.Layout = append(patch.Layout, spec.PanelPlacement{ID: p.ID, GridPos: raw})
			}
		}
	}
	if len(patch.Upsert) == 0 && len(patch.Layout) == 0 {
		return d, nil
	}
	return spec.ApplyPanelPatch(d, patch)
}

func sectionsOf(d *spec.Dashboard) [][]spec.Panel {
	var loose []spec.Panel
	var sections [][]spec.Panel
	for _, p := range d.Panels {
		if p.Type == spec.Row {
			sections = append(sections, p.Panels)
			continue
		}
		loose = append(loose, p)
	}
	return append([][]spec.Panel{loose}, sections...)
}

func widenedLines(section []spec.Panel) map[string]spec.GridPos {
	byY := map[int][]spec.Panel{}
	for _, p := range section {
		byY[p.GridPos.Y] = append(byY[p.GridPos.Y], p)
	}
	widened := map[string]spec.GridPos{}
	for y, line := range byY {
		if !isCleanLine(section, y, line) {
			continue
		}
		total := 0
		for _, p := range line {
			total += p.GridPos.W
		}
		if total >= spec.GridColumns {
			continue
		}
		for id, pos := range widenLine(line, total) {
			widened[id] = pos
		}
	}
	return widened
}

func isCleanLine(section []spec.Panel, y int, line []spec.Panel) bool {
	h := line[0].GridPos.H
	for _, p := range line {
		if p.GridPos.H != h {
			return false
		}
	}
	return !slices.ContainsFunc(section, func(other spec.Panel) bool {
		return other.GridPos.Y != y && other.GridPos.Y < y+h && y < other.GridPos.Y+other.GridPos.H
	})
}

func widenLine(line []spec.Panel, total int) map[string]spec.GridPos {
	ordered := slices.Clone(line)
	slices.SortFunc(ordered, func(a, b spec.Panel) int { return cmp.Compare(a.GridPos.X, b.GridPos.X) })
	widths := make([]int, len(ordered))
	sum := 0
	for i, p := range ordered {
		widths[i] = p.GridPos.W * spec.GridColumns / total
		sum += widths[i]
	}
	for i := range spec.GridColumns - sum {
		widths[i]++
	}
	out := map[string]spec.GridPos{}
	x := 0
	for i, p := range ordered {
		out[p.ID] = spec.GridPos{X: x, Y: p.GridPos.Y, W: widths[i], H: p.GridPos.H}
		x += widths[i]
	}
	return out
}

func wantsFullWidthLegend(p spec.Panel, pos spec.GridPos) bool {
	if p.Type != spec.Timeseries || pos.W != spec.GridColumns || (p.Display != nil && p.Display.Legend != "") {
		return false
	}
	return len(p.Queries) > 1 || slices.ContainsFunc(p.Queries, func(q spec.Query) bool { return strings.Contains(q.Legend, "{{") })
}

func withRightLegend(p spec.Panel, pos spec.GridPos) spec.Panel {
	display := spec.Display{}
	if p.Display != nil {
		display = *p.Display
	}
	display.Legend = "right"
	p.Display = &display
	p.GridPos = pos
	return p
}
