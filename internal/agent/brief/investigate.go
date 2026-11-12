package brief

import (
	"fmt"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func investigateContext(statuses []datasource.Status, task Task, current *Revision, sd seed) string {
	var b strings.Builder
	writeDatasources(&b, statuses)
	b.WriteString("\nInvestigate the question and save one finding.")
	if task.From != nil && task.To != nil {
		fmt.Fprintf(&b, "\nWindow: %s to %s", task.From.UTC().Format(time.RFC3339), task.To.UTC().Format(time.RFC3339))
	} else {
		b.WriteString("\nWindow: the last hour")
	}
	if current != nil {
		fmt.Fprintf(&b, "\nThe question is about dashboard %s, version %d.\n", current.DashboardID, current.Number)
		writeOutline(&b, current.Spec, outlineTargets(current.Spec, task.PanelID, task.Prompt))
		if title, ok := panelTitle(current.Spec, task.PanelID); ok {
			fmt.Fprintf(&b, "\nThe question is about panel %s titled %q, shown in full above.", task.PanelID, title)
		}
	}
	writeSeedNote(&b, sd)
	return b.String()
}
