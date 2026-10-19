package grafana

import (
	"fmt"
	"strings"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

type builtPanel struct {
	node     *panelNode
	panel    spec.Panel
	line     PanelLine
	children []*builtPanel
	repeat   string
}

func (bp *builtPanel) isRow() bool {
	return bp.node.row
}

func (bp *builtPanel) isPlaceholder() bool {
	return bp.panel.Type == spec.Placeholder
}

func (c *converter) convertPanel(n *panelNode) *builtPanel {
	return placeholderPanel(n, c.unsupportedReason(n))
}

func (c *converter) unsupportedReason(n *panelNode) string {
	if len(n.ds.targets) > 0 {
		unsupported := true
		for _, ref := range n.ds.targets {
			if ref.supported() {
				unsupported = false
			}
		}
		if unsupported {
			return n.ds.targets[0].label() + " is not supported yet."
		}
	}
	return fmt.Sprintf("Grafana panel type %s is not supported yet.", n.sourceType)
}

func placeholderPanel(n *panelNode, reason string) *builtPanel {
	panel := spec.Panel{
		ID:      n.id,
		Type:    spec.Placeholder,
		GridPos: spec.GridPos{X: n.pos.x, Y: n.pos.y, W: n.pos.w, H: n.pos.h},
	}
	if title := strings.TrimSpace(n.title); title != "" {
		panel.Title, _ = cutRunes(title, 200)
	}
	panel.Description, _ = cutRunes(asString(n.raw["description"]), 2000)
	panel.Reason, _ = cutRunes(reason, 500)
	panel.SourceType, _ = cutRunes(n.sourceType, 100)

	line := PanelLine{
		PanelID:     n.id,
		GrafanaID:   n.grafanaID,
		GrafanaPath: n.path,
		Title:       n.title,
		Type:        string(spec.Placeholder),
		SourceType:  n.sourceType,
		Row:         n.rowTitle,
		Status:      statusPlaceholder,
		Reasons:     []string{reason},
		Paths:       []string{},
		Queries:     []QueryLine{},
	}
	if n.moved {
		line.Reasons = append(line.Reasons, movedReason)
		line.Paths = append(line.Paths, "gridPos")
	}
	targets, _ := asArray(n.raw["targets"])
	for _, item := range targets {
		target, _ := asObject(item)
		line.Queries = append(line.Queries, QueryLine{
			RefID:    asString(target["refId"]),
			Rewrites: []string{},
			Dropped:  ptr("The panel is a placeholder, so its query is not imported."),
		})
	}
	return &builtPanel{node: n, panel: panel, line: line}
}
