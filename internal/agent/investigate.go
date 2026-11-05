package agent

import (
	_ "embed"
	"fmt"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/agent/prompts"
	"github.com/slashobserve/slashobserve/internal/datasource"
)

//go:embed prompts/investigate.md
var investigatePrompt string

func buildInvestigateSystem() string {
	return strings.TrimRight(investigatePrompt, "\n") + "\n\n# Authoring guide\n" + strings.TrimRight(prompts.SpecGuide(), "\n") + "\n"
}

func investigateContext(statuses []datasource.Status, task Task, current *revision, sd seed) string {
	var b strings.Builder
	writeDatasources(&b, statuses)
	b.WriteString("\nInvestigate the question and save one finding.")
	if task.From != nil && task.To != nil {
		fmt.Fprintf(&b, "\nWindow: %s to %s", task.From.UTC().Format(time.RFC3339), task.To.UTC().Format(time.RFC3339))
	} else {
		b.WriteString("\nWindow: the last hour")
	}
	if current != nil {
		fmt.Fprintf(&b, "\nThe question is about dashboard %s, version %d.\n", current.dashboardID, current.number)
		writeOutline(&b, current.spec, outlineTargets(current.spec, task.PanelID, task.Prompt))
		if title, ok := panelTitle(current.spec, task.PanelID); ok {
			fmt.Fprintf(&b, "\nThe question is about panel %s titled %q, shown in full above.", task.PanelID, title)
		}
	}
	writeSeedNote(&b, sd)
	return b.String()
}
