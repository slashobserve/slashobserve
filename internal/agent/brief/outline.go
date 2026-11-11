package brief

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	outlineThresholdBytes = 16 << 10
	maxOutlineTargets     = 20
)

func needsOutline(specBytes int) bool {
	return specBytes > outlineThresholdBytes
}

func outlineTargets(d *spec.Dashboard, panelID, prompt string) []string {
	mentioned := map[string]bool{}
	for _, token := range strings.FieldsFunc(prompt, func(r rune) bool { return !isPanelIDRune(r) }) {
		mentioned[token] = true
	}
	var targets []string
	if _, ok := d.FindPanel(panelID); ok {
		targets = append(targets, panelID)
	}
	for _, e := range d.AllPanels() {
		if len(targets) == maxOutlineTargets {
			break
		}
		if mentioned[e.Panel.ID] && !slices.Contains(targets, e.Panel.ID) {
			targets = append(targets, e.Panel.ID)
		}
	}
	return targets
}

func isPanelIDRune(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-'
}

func writeOutline(b *strings.Builder, d *spec.Dashboard, targets []string) {
	fmt.Fprintf(b, "Title: %s\n", mustJSON(d.Title))
	fmt.Fprintf(b, "Time: %s\n", mustJSON(d.Time))
	fmt.Fprintf(b, "Refresh: %s\n", mustJSON(d.Refresh))
	fmt.Fprintf(b, "Tags: %s\n", mustJSON(nonNil(d.Tags)))
	fmt.Fprintf(b, "Links: %s\n", mustJSON(nonNil(d.Links)))
	fmt.Fprintf(b, "Variables: %s\n", mustJSON(nonNil(d.Variables)))
	b.WriteString("Panels (id type title, row, position as x y w h):")
	for _, e := range d.AllPanels() {
		p := e.Panel
		fmt.Fprintf(b, "\n- %s %s %q", p.ID, p.Type, p.Title)
		if e.Row != nil {
			fmt.Fprintf(b, " row=%s", e.Row.ID)
		}
		fmt.Fprintf(b, " x%d y%d w%d h%d", p.GridPos.X, p.GridPos.Y, p.GridPos.W, p.GridPos.H)
	}
	if len(targets) == 0 {
		return
	}
	b.WriteString("\nPanels in full, rows without their children:\n```json\n")
	full := make([]spec.Panel, 0, len(targets))
	for _, id := range targets {
		if e, ok := d.FindPanel(id); ok {
			p := *e.Panel
			p.Panels = nil
			full = append(full, p)
		}
	}
	b.WriteString(mustJSON(full))
	b.WriteString("\n```")
}

func nonNil[S ~[]E, E any](s S) S {
	if s == nil {
		return S{}
	}
	return s
}

func mustJSON(v any) string {
	body, err := json.Marshal(v)
	if err != nil {
		panic("agent: spec value does not marshal: " + err.Error())
	}
	return string(body)
}
