package tools

import (
	"cmp"
	"encoding/json"
	"math"
	"slices"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

type slot struct {
	ID       string
	Row      string
	RowTitle string
	W, H     int
	Prefer   *spec.GridPos
}

type layoutPlan struct {
	Positions map[string]spec.GridPos
	NewRows   map[string]spec.GridPos
	Moves     []spec.PanelPlacement
}

type layoutItem struct {
	id        string
	section   string
	isRow     bool
	collapsed bool
	hidden    bool
	pos       spec.GridPos
}

type workingLayout struct {
	items []layoutItem
}

func place(d *spec.Dashboard, slots []slot) layoutPlan {
	lay := newWorkingLayout(d)
	for _, s := range slots {
		lay.remove(s.ID)
	}
	plan := layoutPlan{Positions: map[string]spec.GridPos{}, NewRows: map[string]spec.GridPos{}}
	for _, s := range slots {
		lay.put(s, plan.NewRows)
	}
	for _, s := range slots {
		plan.Positions[s.ID] = lay.find(s.ID).pos
	}
	plan.Moves = lay.movesFrom(d)
	return plan
}

func newWorkingLayout(d *spec.Dashboard) *workingLayout {
	lay := &workingLayout{}
	for _, e := range d.AllPanels() {
		item := layoutItem{id: e.Panel.ID, isRow: e.Panel.Type == spec.Row, collapsed: e.Panel.Collapsed, pos: e.Panel.GridPos}
		if e.Row != nil {
			item.section = e.Row.ID
			item.hidden = e.Row.Collapsed
		}
		lay.items = append(lay.items, item)
	}
	return lay
}

func (l *workingLayout) find(id string) *layoutItem {
	for i := range l.items {
		if l.items[i].id == id {
			return &l.items[i]
		}
	}
	return nil
}

func (l *workingLayout) remove(id string) {
	l.items = slices.DeleteFunc(l.items, func(it layoutItem) bool { return it.id == id })
}

func (l *workingLayout) put(s slot, newRows map[string]spec.GridPos) {
	base, collapsed := 0, false
	if s.Row != "" {
		row := l.find(s.Row)
		if row == nil || !row.isRow {
			row = l.addRow(s.Row, newRows)
		}
		base, collapsed = row.pos.Y+row.pos.H, row.collapsed
	}
	scan := l.scan(s.Row, base)
	pos, ok := wantedSpot(scan, s, collapsed)
	if !ok {
		pos = firstFree(scan, s.W, s.H, collapsed)
	}
	if delta := pos.Y + s.H - scan.limit; !collapsed && delta > 0 {
		l.shiftDown(s.Row, scan.limit, delta)
	}
	l.items = append(l.items, layoutItem{id: s.ID, section: s.Row, pos: pos})
}

func (l *workingLayout) addRow(id string, newRows map[string]spec.GridPos) *layoutItem {
	rowY := 0
	for _, it := range l.items {
		if !it.hidden {
			rowY = max(rowY, it.pos.Y+it.pos.H)
		}
	}
	pos := spec.GridPos{X: 0, Y: rowY, W: spec.GridColumns, H: 1}
	newRows[id] = pos
	l.items = append(l.items, layoutItem{id: id, isRow: true, pos: pos})
	return &l.items[len(l.items)-1]
}

func isMember(it layoutItem, section string) bool {
	return !it.isRow && it.section == section
}

func (l *workingLayout) shiftDown(section string, limit, delta int) {
	shiftedRows := map[string]bool{}
	for i := range l.items {
		it := &l.items[i]
		if isMember(*it, section) || it.hidden || it.pos.Y < limit {
			continue
		}
		it.pos.Y += delta
		if it.isRow && it.collapsed {
			shiftedRows[it.id] = true
		}
	}
	for i := range l.items {
		if l.items[i].hidden && shiftedRows[l.items[i].section] {
			l.items[i].pos.Y += delta
		}
	}
}

func (l *workingLayout) movesFrom(d *spec.Dashboard) []spec.PanelPlacement {
	var moves []spec.PanelPlacement
	for _, e := range d.AllPanels() {
		now := l.find(e.Panel.ID)
		if now == nil {
			continue
		}
		was := ""
		if e.Row != nil {
			was = e.Row.ID
		}
		changedRow := now.section != was
		if now.pos == e.Panel.GridPos && !changedRow {
			continue
		}
		move := spec.PanelPlacement{ID: now.id, GridPos: gridPosJSON(now.pos)}
		if changedRow {
			section := now.section
			move.Row = &section
		}
		moves = append(moves, move)
	}
	return moves
}

type sectionScan struct {
	members []spec.GridPos
	outside []spec.GridPos
	base    int
	bottom  int
	limit   int
}

func (l *workingLayout) scan(section string, base int) sectionScan {
	scan := sectionScan{base: base, bottom: base, limit: math.MaxInt}
	for _, it := range l.items {
		switch {
		case isMember(it, section):
			scan.members = append(scan.members, it.pos)
			scan.bottom = max(scan.bottom, it.pos.Y+it.pos.H)
		case !it.hidden:
			scan.outside = append(scan.outside, it.pos)
		}
	}
	for _, g := range scan.outside {
		if g.Y >= scan.bottom {
			scan.limit = min(scan.limit, g.Y)
		}
	}
	return scan
}

func blockersOf(scan sectionScan, collapsed bool) []spec.GridPos {
	if collapsed {
		return scan.members
	}
	return append(slices.Clone(scan.members), above(scan.outside, scan.limit)...)
}

func wantedSpot(scan sectionScan, s slot, collapsed bool) (spec.GridPos, bool) {
	want := s.Prefer
	if want == nil || want.X < 0 || want.X+s.W > spec.GridColumns || want.Y < scan.base || want.Y > scan.bottom {
		return spec.GridPos{}, false
	}
	candidate := spec.GridPos{X: want.X, Y: want.Y, W: s.W, H: s.H}
	return candidate, !overlapsAny(candidate, blockersOf(scan, collapsed))
}

func firstFree(scan sectionScan, w, h int, collapsed bool) spec.GridPos {
	blockers := blockersOf(scan, collapsed)
	end := scan.bottom
	for _, g := range blockers {
		end = max(end, g.Y+g.H)
	}
	for y := scan.base; y < end; y++ {
		for x := 0; x+w <= spec.GridColumns; x++ {
			candidate := spec.GridPos{X: x, Y: y, W: w, H: h}
			if !overlapsAny(candidate, blockers) {
				return candidate
			}
		}
	}
	return spec.GridPos{X: 0, Y: end, W: w, H: h}
}

func overlapsAny(g spec.GridPos, others []spec.GridPos) bool {
	for _, o := range others {
		if g.X < o.X+o.W && o.X < g.X+g.W && g.Y < o.Y+o.H && o.Y < g.Y+g.H {
			return true
		}
	}
	return false
}

func above(items []spec.GridPos, limit int) []spec.GridPos {
	var out []spec.GridPos
	for _, g := range items {
		if g.Y < limit {
			out = append(out, g)
		}
	}
	return out
}

func gridPosJSON(g spec.GridPos) json.RawMessage {
	raw, _ := json.Marshal(g)
	return raw
}

type gap struct {
	Row string
	Pos spec.GridPos
}

func gaps(d *spec.Dashboard) []gap {
	var loose, beside []spec.GridPos
	for _, p := range d.Panels {
		if p.Type != spec.Row {
			loose = append(loose, p.GridPos)
			continue
		}
		band := p.GridPos
		if !p.Collapsed {
			for _, c := range p.Panels {
				band.H = max(band.H, c.GridPos.Y+c.GridPos.H-band.Y)
			}
		}
		beside = append(beside, band)
	}
	found := sectionGaps("", 0, loose, beside)
	for _, row := range d.Panels {
		if row.Type != spec.Row || row.Collapsed {
			continue
		}
		members := make([]spec.GridPos, len(row.Panels))
		for i, c := range row.Panels {
			members[i] = c.GridPos
		}
		found = append(found, sectionGaps(row.ID, row.GridPos.Y+row.GridPos.H, members, nil)...)
	}
	return found
}

type columnRun struct{ from, to int }

func sectionGaps(rowID string, base int, members, beside []spec.GridPos) []gap {
	if len(members) == 0 {
		return nil
	}
	bottom := 0
	for _, m := range members {
		bottom = max(bottom, m.Y+m.H)
	}
	var rects []spec.GridPos
	open := map[columnRun]int{}
	for y := base; y <= bottom; y++ {
		var runs []columnRun
		if y < bottom {
			runs = freeRuns(append(slices.Clone(members), beside...), y)
		}
		for run, from := range open {
			if !slices.Contains(runs, run) {
				rects = append(rects, spec.GridPos{X: run.from, Y: from, W: run.to - run.from, H: y - from})
				delete(open, run)
			}
		}
		for _, run := range runs {
			if _, ok := open[run]; !ok {
				open[run] = y
			}
		}
	}
	slices.SortFunc(rects, func(a, b spec.GridPos) int { return cmp.Or(cmp.Compare(a.Y, b.Y), cmp.Compare(a.X, b.X)) })
	var found []gap
	for _, r := range rects {
		startsALine := slices.ContainsFunc(members, func(m spec.GridPos) bool { return m.Y == r.Y })
		hasPanelsBelow := slices.ContainsFunc(members, func(m spec.GridPos) bool { return m.Y >= r.Y+r.H })
		if !startsALine || hasPanelsBelow {
			found = append(found, gap{Row: rowID, Pos: r})
		}
	}
	return found
}

func freeRuns(members []spec.GridPos, y int) []columnRun {
	var covered [spec.GridColumns]bool
	for _, m := range members {
		if m.Y <= y && y < m.Y+m.H {
			for x := max(m.X, 0); x < min(m.X+m.W, spec.GridColumns); x++ {
				covered[x] = true
			}
		}
	}
	var runs []columnRun
	for x := 0; x < spec.GridColumns; x++ {
		if covered[x] {
			continue
		}
		if n := len(runs); n > 0 && runs[n-1].to == x {
			runs[n-1].to = x + 1
			continue
		}
		runs = append(runs, columnRun{from: x, to: x + 1})
	}
	return runs
}
