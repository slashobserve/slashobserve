package agent

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/internal/agent/prompts"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/pkg/spec"
	"github.com/slashobserve/slashobserve/spec/examples"
)

type revision struct {
	dashboardID string
	number      int
	spec        *spec.Dashboard
}

func buildSystem() string {
	var b strings.Builder
	b.WriteString(strings.TrimRight(prompts.System(), "\n"))
	b.WriteString("\n\n# Authoring guide\n")
	b.WriteString(strings.TrimRight(prompts.SpecGuide(), "\n"))
	b.WriteString("\n\n# ObserveSpec v0 JSON Schema\n")
	writeJSONBlock(&b, spec.Schema())
	b.WriteString("\n\n# Example Spec (format only)\n")
	writeJSONBlock(&b, examples.CheckoutServiceHealth)
	b.WriteString("\n")
	return b.String()
}

func writeJSONBlock(b *strings.Builder, body []byte) {
	b.WriteString("```json\n")
	b.WriteString(strings.TrimRight(string(body), "\n"))
	b.WriteString("\n```")
}

func buildContext(statuses []datasource.Status, mode tools.Mode, source revisions.Source, panelID string, current *revision) (string, error) {
	var b strings.Builder
	b.WriteString("Configured datasources:\n")
	sorted := slices.Clone(statuses)
	slices.SortFunc(sorted, func(a, b datasource.Status) int { return strings.Compare(a.UID, b.UID) })
	for _, s := range sorted {
		state := "unhealthy"
		if s.Healthy {
			state = "healthy"
		}
		fmt.Fprintf(&b, "- %s (%s): %s\n", s.UID, s.Type, state)
	}
	if mode == tools.ModeCreate {
		b.WriteString("\nCreate a new dashboard for the request.")
		return b.String(), nil
	}
	body, err := json.Marshal(current.spec)
	if err != nil {
		return "", fmt.Errorf("marshal current spec: %w", err)
	}
	if len(body) > tools.MaxModelSpecBytes {
		return "", ErrTooLarge
	}
	fmt.Fprintf(&b, "\nYou are revising dashboard %s, currently version %d.\n```json\n%s\n```", current.dashboardID, current.number, body)
	if panelID != "" {
		fromComment := source == revisions.SourceComment
		if title, ok := panelTitle(current.spec, panelID); ok {
			if fromComment {
				fmt.Fprintf(&b, "\nThe request is a comment on panel %s titled %q.", panelID, title)
			} else {
				fmt.Fprintf(&b, "\nThe request is about panel %s titled %q.", panelID, title)
			}
		} else if fromComment {
			fmt.Fprintf(&b, "\nPanel %s from the comment is not in the latest version.", panelID)
		} else {
			fmt.Fprintf(&b, "\nPanel %s from the request is not in the latest version.", panelID)
		}
	}
	return b.String(), nil
}

func panelTitle(d *spec.Dashboard, id string) (string, bool) {
	entry, ok := d.FindPanel(id)
	if !ok {
		return "", false
	}
	return entry.Panel.Title, true
}
