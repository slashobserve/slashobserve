package tools

import (
	"encoding/json"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

type placement struct {
	Panel  spec.GridPos
	Row    *spec.GridPos
	Shifts []spec.PanelPlacement
}

func place(d *spec.Dashboard, rowID string, w, h int) placement {
	entries := d.AllPanels()
	if rowID == "" {
		return placeInSection(entries, func(e spec.PanelEntry) bool {
			return e.Row == nil && e.Panel.Type != spec.Row
		}, 0, false, w, h)
	}
	for i := range d.Panels {
		row := &d.Panels[i]
		if row.Type == spec.Row && row.ID == rowID {
			return placeInSection(entries, func(e spec.PanelEntry) bool {
				return e.Row == row
			}, row.GridPos.Y+row.GridPos.H, row.Collapsed, w, h)
		}
	}
	return placeInNewRow(entries, w, h)
}

func inLayout(e spec.PanelEntry) bool {
	return e.Row == nil || !e.Row.Collapsed
}

func placeInNewRow(entries []spec.PanelEntry, w, h int) placement {
	rowY := 0
	for _, e := range entries {
		if inLayout(e) {
			rowY = max(rowY, e.Panel.GridPos.Y+e.Panel.GridPos.H)
		}
	}
	return placement{
		Panel: spec.GridPos{X: 0, Y: rowY + 1, W: w, H: h},
		Row:   &spec.GridPos{X: 0, Y: rowY, W: spec.GridColumns, H: 1},
	}
}

type sectionScan struct {
	members []spec.GridPos
	outside []spec.GridPos
	last    spec.GridPos
	bottom  int
}

func scanSection(entries []spec.PanelEntry, inSection func(spec.PanelEntry) bool, base int) sectionScan {
	scan := sectionScan{bottom: base}
	for _, e := range entries {
		g := e.Panel.GridPos
		switch {
		case inSection(e):
			if len(scan.members) == 0 || g.Y > scan.last.Y || g.Y == scan.last.Y && g.X > scan.last.X {
				scan.last = g
			}
			scan.members = append(scan.members, g)
			scan.bottom = max(scan.bottom, g.Y+g.H)
		case inLayout(e):
			scan.outside = append(scan.outside, g)
		}
	}
	return scan
}

func (s sectionScan) candidate(w, h int, collapsed bool) spec.GridPos {
	wrapped := spec.GridPos{X: 0, Y: s.bottom, W: w, H: h}
	if len(s.members) == 0 || s.last.X+s.last.W+w > spec.GridColumns {
		return wrapped
	}
	beside := spec.GridPos{X: s.last.X + s.last.W, Y: s.last.Y, W: w, H: h}
	if overlapsAny(beside, s.members) || !collapsed && overlapsAny(beside, above(s.outside, s.bottom)) {
		return wrapped
	}
	return beside
}

func placeInSection(entries []spec.PanelEntry, inSection func(spec.PanelEntry) bool, base int, collapsed bool, w, h int) placement {
	scan := scanSection(entries, inSection, base)
	candidate := scan.candidate(w, h, collapsed)
	delta := candidate.Y + h - scan.bottom
	if collapsed || delta <= 0 {
		return placement{Panel: candidate}
	}
	var shifts []spec.PanelPlacement
	for _, e := range entries {
		if inSection(e) || !movesDown(e, scan.bottom) {
			continue
		}
		g := e.Panel.GridPos
		g.Y += delta
		shifts = append(shifts, spec.PanelPlacement{ID: e.Panel.ID, GridPos: gridPosJSON(g)})
	}
	return placement{Panel: candidate, Shifts: shifts}
}

func movesDown(e spec.PanelEntry, bottom int) bool {
	if e.Row != nil && e.Row.Collapsed {
		return e.Row.GridPos.Y >= bottom
	}
	return e.Panel.GridPos.Y >= bottom
}

func overlapsAny(g spec.GridPos, others []spec.GridPos) bool {
	for _, o := range others {
		if g.X < o.X+o.W && o.X < g.X+g.W && g.Y < o.Y+o.H && o.Y < g.Y+g.H {
			return true
		}
	}
	return false
}

func above(items []spec.GridPos, bottom int) []spec.GridPos {
	var out []spec.GridPos
	for _, g := range items {
		if g.Y < bottom {
			out = append(out, g)
		}
	}
	return out
}

func gridPosJSON(g spec.GridPos) json.RawMessage {
	raw, _ := json.Marshal(g)
	return raw
}
