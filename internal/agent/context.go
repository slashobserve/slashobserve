package agent

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/internal/agent/prompts"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

type revision struct {
	dashboardID string
	number      int
	spec        *spec.Dashboard
	unverified  dashboards.Unverified
	conversion  []tools.Attention
}

func systemFor(statuses []datasource.Status) string {
	var types []spec.DatasourceType
	for _, s := range statuses {
		if t := spec.DatasourceType(s.Type); !slices.Contains(types, t) {
			types = append(types, t)
		}
	}
	return prompts.Build(types)
}

func buildContext(statuses []datasource.Status, mode tools.Mode, source revisions.Source, panelID, prompt string, current *revision, sd seed) (string, error) {
	var b strings.Builder
	writeDatasources(&b, statuses)
	if mode == tools.ModeCreate && sd.build == nil {
		b.WriteString("\nCreate a new dashboard for the request, or answer it with save_finding if it only asks a question.")
		writeSeedNote(&b, sd)
		return b.String(), nil
	}
	body, err := json.Marshal(current.spec)
	if err != nil {
		return "", fmt.Errorf("marshal current spec: %w", err)
	}
	writeIntro(&b, mode, current, sd.build != nil)
	if needsOutline(len(body)) {
		b.WriteString(" The Spec is too large to send whole, so this is its outline: read the full JSON of other panels with get_panels.\n")
		writeOutline(&b, current.spec, outlineTargets(current.spec, panelID, prompt))
	} else {
		fmt.Fprintf(&b, "\n```json\n%s\n```", body)
	}
	if mode == tools.ModeCreate {
		writeBuild(&b, sd)
		return b.String(), nil
	}
	writeUnverified(&b, current)
	writeConversion(&b, current)
	if mode == tools.ModeSuggest {
		writeSuggestRequest(&b, panelID, current)
		return b.String(), nil
	}
	writePanelRequest(&b, source, panelID, current)
	if sd.build != nil {
		writeBuild(&b, sd)
	} else {
		writeSeedNote(&b, sd)
	}
	return b.String(), nil
}

func writeDatasources(b *strings.Builder, statuses []datasource.Status) {
	b.WriteString("Configured datasources:\n")
	sorted := slices.Clone(statuses)
	slices.SortFunc(sorted, func(a, b datasource.Status) int { return strings.Compare(a.UID, b.UID) })
	for _, s := range sorted {
		state := "unhealthy"
		if s.Healthy {
			state = "healthy"
		}
		fmt.Fprintf(b, "- %s (%s): %s\n", s.UID, s.Type, state)
	}
}

func writeIntro(b *strings.Builder, mode tools.Mode, current *revision, hasBuild bool) {
	switch {
	case mode == tools.ModeSuggest:
		fmt.Fprintf(b, "\nYou are proposing a change to dashboard %s, version %d.", current.dashboardID, current.number)
	case mode == tools.ModeCreate:
		b.WriteString("\nCreate a new dashboard for the request. This is your build of the new dashboard, not saved as a version yet.")
	case hasBuild:
		fmt.Fprintf(b, "\nYou are revising dashboard %s, currently version %d. This is your build of the revision, not saved as a version yet.", current.dashboardID, current.number)
	default:
		fmt.Fprintf(b, "\nYou are revising dashboard %s, currently version %d. Change it for the request, or answer it with save_finding if it only asks a question.", current.dashboardID, current.number)
	}
}

func writeSuggestRequest(b *strings.Builder, panelID string, current *revision) {
	title, _ := panelTitle(current.spec, panelID)
	if title == "" {
		title = panelID
	}
	fmt.Fprintf(b, "\nPropose a change to panel %s titled %q for the request. Call update_panels with only that panel, with id %s, in upsert; everything else stays as in version %d, and nothing is saved.", panelID, title, panelID, current.number)
}

func writePanelRequest(b *strings.Builder, source revisions.Source, panelID string, current *revision) {
	if panelID == "" {
		return
	}
	fromComment := source == revisions.SourceComment
	if title, ok := panelTitle(current.spec, panelID); ok {
		if fromComment {
			fmt.Fprintf(b, "\nThe request is a comment on panel %s titled %q.", panelID, title)
		} else {
			fmt.Fprintf(b, "\nThe request is about panel %s titled %q.", panelID, title)
		}
	} else if fromComment {
		fmt.Fprintf(b, "\nPanel %s from the comment is not in the latest version.", panelID)
	} else {
		fmt.Fprintf(b, "\nPanel %s from the request is not in the latest version.", panelID)
	}
}

const (
	maxListLines = 60
	maxListBytes = 8 << 10
)

func writeUnverified(b *strings.Builder, current *revision) {
	u := current.unverified
	if u.Empty() {
		return
	}
	var lines []string
	for _, p := range u.Panels {
		title, _ := panelTitle(current.spec, p.PanelID)
		lines = append(lines, fmt.Sprintf("- panel %s %q: %s", p.PanelID, title, firstIssue(p.Issues)))
	}
	for _, v := range u.Variables {
		lines = append(lines, fmt.Sprintf("- variable %s: %s", v.Name, firstIssue(v.Issues)))
	}
	fmt.Fprintf(b, "\nUnverified in version %d: their queries failed when it was saved. A save keeps them unverified while their queries and the variables those use stay unchanged; a changed or new query must return data.", current.number)
	writeCapped(b, lines)
	b.WriteString("\nKeep unverified panels unless the request asks to delete them.")
}

func writeConversion(b *strings.Builder, current *revision) {
	if len(current.conversion) == 0 {
		return
	}
	lines := make([]string, len(current.conversion))
	for i, a := range current.conversion {
		lines[i] = fmt.Sprintf("- %s %s %q: %s", a.Status, a.PanelID, a.Title, strings.Join(strings.Fields(a.Reason), " "))
	}
	fmt.Fprintf(b, "\nVersion %d is a Grafana import. The importer converted these panels only partly or kept them as placeholders:", current.number)
	writeCapped(b, lines)
}

func writeCapped(b *strings.Builder, lines []string) {
	size := 0
	for i, line := range lines {
		if i == maxListLines || size+len(line) > maxListBytes {
			fmt.Fprintf(b, "\n- ... and %d more", len(lines)-i)
			break
		}
		size += len(line)
		b.WriteString("\n" + line)
	}
}

func firstIssue(issues []spec.Issue) string {
	if len(issues) == 0 {
		return ""
	}
	return fmt.Sprintf("%s: %s", issues[0].Code, strings.Join(strings.Fields(issues[0].Message), " "))
}

func panelTitle(d *spec.Dashboard, id string) (string, bool) {
	entry, ok := d.FindPanel(id)
	if !ok {
		return "", false
	}
	return entry.Panel.Title, true
}
