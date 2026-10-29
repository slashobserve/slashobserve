package grafana

import (
	"cmp"
	"slices"
	"strconv"
)

const movedReason = "Moved or resized to fit the grid."

const (
	maxGridY       = 10000
	maxGridHeight  = 100
	overflowHeight = 25
)

func arrange(top []*panelNode) ([]*panelNode, []string) {
	slices.SortStableFunc(top, func(a, b *panelNode) int {
		return cmp.Or(cmp.Compare(a.pos.y, b.pos.y), cmp.Compare(a.pos.x, b.pos.x))
	})
	nested := nestRows(top)
	kept, clones := dropClones(nested)
	assignIDs(kept)
	fitGrid(kept)
	return kept, clones
}

func nestRows(top []*panelNode) []*panelNode {
	var out []*panelNode
	var open *panelNode
	for _, n := range top {
		switch {
		case n.row:
			out = append(out, n)
			open = nil
			if !n.collapsed {
				open = n
			}
		case open != nil:
			open.children = append(open.children, n)
		default:
			out = append(out, n)
		}
	}
	return out
}

func dropClones(nodes []*panelNode) ([]*panelNode, []string) {
	var kept []*panelNode
	var clones []string
	for _, n := range nodes {
		if n.clone {
			clones = append(clones, n.path)
			for _, child := range n.children {
				clones = append(clones, child.path)
			}
			continue
		}
		var children []*panelNode
		for _, child := range n.children {
			if child.clone {
				clones = append(clones, child.path)
				continue
			}
			child.parent = n
			children = append(children, child)
		}
		n.children = children
		kept = append(kept, n)
	}
	return kept, clones
}

func assignIDs(nodes []*panelNode) {
	used := map[string]int{}
	unnumbered := 0
	assign := func(n *panelNode) {
		var base string
		if n.grafanaID != nil {
			base = "p" + strconv.Itoa(*n.grafanaID)
		} else {
			unnumbered++
			base = "pn" + strconv.Itoa(unnumbered)
		}
		used[base]++
		n.id = base
		if used[base] > 1 {
			n.id = base + "_" + strconv.Itoa(used[base])
		}
	}
	for _, n := range nodes {
		assign(n)
		for _, child := range n.children {
			assign(child)
		}
	}
}

type placement struct {
	pos   gridPos
	moved bool
}

func fitGrid(nodes []*panelNode) {
	for _, limit := range []int{maxGridHeight, overflowHeight} {
		placed, fits := placeGrid(nodes, limit)
		if !fits && limit != overflowHeight {
			continue
		}
		for n, p := range placed {
			n.pos, n.moved = p.pos, p.moved
		}
		return
	}
}

func placeGrid(nodes []*panelNode, heightCap int) (map[*panelNode]placement, bool) {
	placed := map[*panelNode]placement{}
	var layout []*panelNode
	for _, n := range nodes {
		layout = append(layout, n)
		if !n.collapsed {
			layout = append(layout, n.children...)
		}
	}
	fits := placeGroup(layout, 0, 0, heightCap, placed)
	for _, row := range nodes {
		if !row.collapsed || len(row.children) == 0 {
			continue
		}
		first := row.children[0].pos.y
		fits = placeGroup(row.children, placed[row].pos.y+1, first, heightCap, placed) && fits
	}
	return placed, fits
}

func placeGroup(group []*panelNode, base, shift, heightCap int, out map[*panelNode]placement) bool {
	raw := make([]gridPos, len(group))
	fitted := make([]gridPos, len(group))
	for i, n := range group {
		raw[i] = n.pos
		raw[i].y -= shift
		if n.row {
			raw[i].x, raw[i].w, raw[i].h = 0, gridColumns, 1
		}
		fitted[i] = clampPos(raw[i], n.row, heightCap)
	}
	want, got := compactVertical(raw), compactVertical(fitted)
	fits := true
	for i, n := range group {
		pos := got[i]
		pos.y += base
		out[n] = placement{pos: pos, moved: !n.row && got[i] != want[i]}
		fits = fits && pos.y <= maxGridY
	}
	return fits
}

func clampPos(p gridPos, row bool, heightCap int) gridPos {
	p.x = min(max(p.x, 0), gridColumns-1)
	p.w = min(max(p.w, 1), gridColumns-p.x)
	p.y = max(p.y, 0)
	p.h = min(max(p.h, 1), heightCap)
	if row {
		p.x, p.w, p.h = 0, gridColumns, 1
	}
	return p
}

func overlaps(a, b gridPos) bool {
	return a.x < b.x+b.w && b.x < a.x+a.w && a.y < b.y+b.h && b.y < a.y+a.h
}

func compactVertical(in []gridPos) []gridPos {
	cur := slices.Clone(in)
	order := make([]int, len(in))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int {
		return cmp.Or(cmp.Compare(in[a].y, in[b].y), cmp.Compare(in[a].x, in[b].x))
	})
	var placed []int
	for k, i := range order {
		l := cur[i]
		bottom := 0
		for _, j := range placed {
			bottom = max(bottom, cur[j].y+cur[j].h)
		}
		l.y = min(bottom, l.y)
		if l.y > 0 && firstCollision(cur, placed, l) < 0 {
			l.y = rowAfterMovingUp(cur, placed, l)
		}
		for c := firstCollision(cur, placed, l); c >= 0; c = firstCollision(cur, placed, l) {
			pushDown(cur, order, k, &l, cur[c].y+cur[c].h)
		}
		l.y, l.x = max(l.y, 0), max(l.x, 0)
		cur[i] = l
		placed = append(placed, i)
	}
	return cur
}

func rowAfterMovingUp(cur []gridPos, placed []int, l gridPos) int {
	y := 0
	for _, j := range placed {
		other := cur[j]
		if other.x >= l.x+l.w || l.x >= other.x+other.w {
			continue
		}
		lowest, highest := other.y-l.h+1, other.y+other.h-1
		if lowest <= highest && lowest <= l.y-1 {
			y = max(y, min(highest, l.y-1))
		}
	}
	return y
}

func firstCollision(cur []gridPos, placed []int, l gridPos) int {
	for _, j := range placed {
		if overlaps(l, cur[j]) {
			return j
		}
	}
	return -1
}

func pushDown(cur []gridPos, order []int, k int, item *gridPos, moveTo int) {
	item.y++
	for j := k + 1; j < len(order); j++ {
		other := &cur[order[j]]
		if other.y > item.y+item.h {
			break
		}
		if overlaps(*item, *other) {
			pushDown(cur, order, j, other, moveTo+item.h)
		}
	}
	item.y = moveTo
}
