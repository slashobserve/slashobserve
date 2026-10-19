package grafana

import (
	"slices"
	"strconv"
)

const movedReason = "Moved or resized to fit the grid."

const (
	maxGridY      = 10000
	maxGridHeight = 100
)

func arrange(top []*panelNode) ([]*panelNode, []string) {
	if allPositioned(top) {
		slices.SortStableFunc(top, func(a, b *panelNode) int {
			if a.pos.y != b.pos.y {
				return a.pos.y - b.pos.y
			}
			return a.pos.x - b.pos.x
		})
	}
	nested := nestRows(top)
	kept, clones := dropClones(nested)
	assignIDs(kept)
	fitGrid(kept)
	return kept, clones
}

func allPositioned(nodes []*panelNode) bool {
	for _, n := range nodes {
		if !n.hasPos {
			return false
		}
	}
	return true
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

func fitGrid(nodes []*panelNode) {
	bottom := 0
	place := func(n *panelNode) {
		if !n.hasPos {
			n.pos = gridPos{x: 0, y: bottom, w: 12, h: 8}
			n.hasPos = true
		}
		clampPos(n)
		bottom = max(bottom, n.pos.y+n.pos.h)
	}
	for _, n := range nodes {
		if n.collapsed {
			pullUp(n)
		}
		place(n)
		for _, child := range n.children {
			place(child)
		}
	}
	var layout []*panelNode
	for _, n := range nodes {
		layout = append(layout, n)
		if !n.collapsed {
			layout = append(layout, n.children...)
		}
	}
	separate(layout)
	for _, n := range nodes {
		if n.collapsed {
			separate(n.children)
		}
	}
}

func pullUp(row *panelNode) {
	if len(row.children) == 0 || !row.hasPos {
		return
	}
	lowest, highest := row.children[0].pos.y, row.children[0].pos.y
	for _, child := range row.children {
		lowest, highest = min(lowest, child.pos.y), max(highest, child.pos.y)
	}
	if highest <= maxGridY {
		return
	}
	shift := lowest - (row.pos.y + 1)
	for _, child := range row.children {
		child.pos.y -= shift
		child.moved = true
	}
}

func clampPos(n *panelNode) {
	p := n.pos
	if n.row {
		p.h = 1
		p.x, p.w = 0, gridColumns
	}
	p.x = min(max(p.x, 0), gridColumns-1)
	p.w = min(max(p.w, 1), gridColumns-p.x)
	p.y = min(max(p.y, 0), maxGridY)
	p.h = min(max(p.h, 1), maxGridHeight)
	if p != n.pos && !n.row {
		n.moved = true
	}
	n.pos = p
}

func overlaps(a, b gridPos) bool {
	return a.x < b.x+b.w && b.x < a.x+a.w && a.y < b.y+b.h && b.y < a.y+a.h
}

func separate(layout []*panelNode) {
	for i, n := range layout {
		for moved := true; moved; {
			moved = false
			for _, earlier := range layout[:i] {
				if overlaps(earlier.pos, n.pos) {
					n.pos.y = earlier.pos.y + earlier.pos.h
					n.moved = true
					moved = true
				}
			}
		}
	}
}
