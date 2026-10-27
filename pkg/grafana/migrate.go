package grafana

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
)

const (
	gridColumns        = 24
	legacyDefaultSpan  = 4
	legacyRowHeight    = 250
	legacyMinHeight    = 90
	legacyHeightStep   = 38
	legacyColumnFactor = gridColumns / 12

	defaultPanelWidth  = 6
	defaultPanelHeight = 3
	gridSanityLimit    = 1e9
)

type gridPos struct{ x, y, w, h int }

type panelNode struct {
	raw        map[string]any
	path       string
	row        bool
	collapsed  bool
	staleKids  bool
	clone      bool
	typ        string
	sourceType string
	title      string
	grafanaID  *int
	pos        gridPos
	moved      bool
	children   []*panelNode
	parent     *panelNode
	rowTitle   *string
	id         string
	ds         panelDS
}

func panelTypeOf(p map[string]any) (typ, sourceType string) {
	sourceType = asString(p["type"])
	switch sourceType {
	case "graph":
		return "timeseries", sourceType
	case "singlestat", "grafana-singlestat-panel":
		if g, ok := asObject(p["gauge"]); ok && truthy(g["show"]) {
			return "gauge", sourceType
		}
		return "stat", sourceType
	case "table-old", "datagrid", "logstable":
		return "table", sourceType
	case "logs-new":
		return "logs", sourceType
	case "grafana-piechart-panel":
		return "piechart", sourceType
	}
	return sourceType, sourceType
}

func isRepeatClone(p map[string]any) bool {
	return p["repeatPanelId"] != nil || p["repeatRowId"] != nil || truthy(p["repeatedByRow"])
}

func isLegacyRepeatClone(row map[string]any) bool {
	return isRepeatClone(row) || (row["repeatIteration"] != nil && !present(row["repeat"]))
}

func newNode(raw map[string]any, path string) *panelNode {
	n := &panelNode{raw: raw, path: path, title: asString(raw["title"])}
	n.typ, n.sourceType = panelTypeOf(raw)
	n.row = n.typ == "row"
	n.collapsed = n.row && truthy(raw["collapsed"])
	n.clone = isRepeatClone(raw)
	if id, ok := asInt(raw["id"]); ok {
		n.grafanaID = &id
	}
	n.pos = parseGridPos(raw["gridPos"])
	return n
}

func parseGridPos(v any) gridPos {
	m, _ := asObject(v)
	read := func(key string, fallback int) int {
		f, ok := asFloat(m[key])
		if !ok {
			return fallback
		}
		return int(math.Floor(min(max(f, -gridSanityLimit), gridSanityLimit)))
	}
	return gridPos{x: read("x", 0), y: read("y", 0), w: read("w", defaultPanelWidth), h: read("h", defaultPanelHeight)}
}

func buildNodes(d map[string]any) ([]*panelNode, bool) {
	panels, _ := asArray(d["panels"])
	rows, _ := asArray(d["rows"])
	if len(panels) == 0 && len(rows) > 0 {
		return legacyNodes(rows), true
	}
	return newStyleNodes(panels, "panels"), false
}

func newStyleNodes(items []any, base string) []*panelNode {
	nodes := make([]*panelNode, 0, len(items))
	for i, item := range items {
		raw, ok := asObject(item)
		if !ok {
			continue
		}
		n := newNode(raw, fmt.Sprintf("%s[%d]", base, i))
		if n.row {
			children, _ := asArray(raw["panels"])
			n.staleKids = !n.collapsed && len(children) > 0
			if !n.collapsed {
				children = nil
			}
			for j, child := range children {
				childRaw, ok := asObject(child)
				if !ok {
					continue
				}
				cn := newNode(childRaw, fmt.Sprintf("%s.panels[%d]", n.path, j))
				if cn.row {
					continue
				}
				n.children = append(n.children, cn)
			}
		}
		nodes = append(nodes, n)
	}
	return nodes
}

var leadingInteger = regexp.MustCompile(`^\s*[+-]?[0-9]+`)

func legacyPixels(v any) int {
	switch h := v.(type) {
	case string:
		if m := leadingInteger.FindString(h); m != "" {
			if n, err := strconv.Atoi(m); err == nil {
				return n
			}
		}
	default:
		if f, ok := asFloat(v); ok {
			return int(min(max(f, -gridSanityLimit), gridSanityLimit))
		}
	}
	return 0
}

func legacyGridUnits(pixels int) int {
	return int(math.Ceil(float64(max(pixels, legacyMinHeight)) / legacyHeightStep))
}

func legacyGridHeight(v any, fallback int) int {
	if px := legacyPixels(v); px > 0 {
		return legacyGridUnits(px)
	}
	return legacyGridUnits(fallback)
}

func legacyMaxPanelID(rows []any) int {
	maxID := 0
	for _, r := range rows {
		row, _ := asObject(r)
		items, _ := asArray(row["panels"])
		for _, item := range items {
			p, _ := asObject(item)
			if id, ok := asInt(p["id"]); ok {
				maxID = max(maxID, id)
			}
		}
	}
	return maxID
}

func legacyShowsRows(rows []any) bool {
	for _, r := range rows {
		if row, ok := asObject(r); ok && (truthy(row["collapse"]) || truthy(row["showTitle"]) || present(row["repeat"])) {
			return true
		}
	}
	return false
}

type legacyBuilder struct {
	out       []*panelNode
	y         int
	nextRowID int
	showRows  bool
}

func legacyNodes(rows []any) []*panelNode {
	b := &legacyBuilder{nextRowID: legacyMaxPanelID(rows) + 1, showRows: legacyShowsRows(rows)}
	for i, r := range rows {
		if row, ok := asObject(r); ok {
			b.addRow(row, fmt.Sprintf("rows[%d]", i))
		}
	}
	return b.out
}

func (b *legacyBuilder) addRow(row map[string]any, path string) {
	if isLegacyRepeatClone(row) {
		b.out = append(b.out, &panelNode{raw: row, path: path, row: true, clone: true, typ: "row", sourceType: "row", pos: gridPos{y: b.y, w: gridColumns, h: 1}})
		return
	}
	collapsed := truthy(row["collapse"])
	var rowNode *panelNode
	if b.showRows {
		id := b.nextRowID
		b.nextRowID++
		rowNode = &panelNode{
			raw:        map[string]any{"repeat": row["repeat"]},
			path:       path,
			row:        true,
			collapsed:  collapsed,
			typ:        "row",
			sourceType: "row",
			title:      asString(row["title"]),
			grafanaID:  &id,
			pos:        gridPos{x: 0, y: b.y, w: gridColumns, h: 1},
		}
		b.out = append(b.out, rowNode)
		b.y++
	}
	height := b.addPanels(row, path, rowNode, collapsed)
	if !collapsed || rowNode == nil {
		b.y += height
	}
}

func (b *legacyBuilder) addPanels(row map[string]any, path string, rowNode *panelNode, collapsed bool) int {
	rowHeight := legacyGridHeight(row["height"], legacyRowHeight)
	items, _ := asArray(row["panels"])
	x, lineHeight, packed := 0, 0, 0
	for j, item := range items {
		p, ok := asObject(item)
		if !ok {
			continue
		}
		n := newNode(p, fmt.Sprintf("%s.panels[%d]", path, j))
		span := legacyDefaultSpan
		if f, ok := asFloat(p["span"]); ok && f > 0 {
			span = int(math.Floor(f))
		}
		w := min(span*legacyColumnFactor, gridColumns)
		h := rowHeight
		if px := legacyPixels(p["height"]); px > 0 {
			h = legacyGridUnits(px)
		}
		if x+w > gridColumns {
			x = 0
			packed += lineHeight
			lineHeight = 0
		}
		n.pos = gridPos{x: x, y: b.y + packed, w: w, h: h}
		x += w
		lineHeight = max(lineHeight, h)
		if rowNode != nil && collapsed {
			rowNode.children = append(rowNode.children, n)
		} else {
			b.out = append(b.out, n)
		}
	}
	return packed + lineHeight
}
