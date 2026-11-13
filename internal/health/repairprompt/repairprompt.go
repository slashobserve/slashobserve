package repairprompt

import (
	"fmt"
	"strings"
)

const Prefix = "Repair the panels and variables the health check found failing so each query runs and returns data for the last 7 days; keep every other panel unchanged and never delete a panel."

const panelLine = "- panel "

func Is(prompt string) bool {
	return strings.HasPrefix(prompt, Prefix)
}

func PanelLine(id, title, what string) string {
	if title == "" {
		return fmt.Sprintf("%s%s %s", panelLine, id, what)
	}
	return fmt.Sprintf("%s%s (%q) %s", panelLine, id, title, what)
}

func PanelIDs(prompt string) []string {
	var ids []string
	seen := map[string]bool{}
	for line := range strings.SplitSeq(prompt, "\n") {
		rest, ok := strings.CutPrefix(line, panelLine)
		if !ok {
			continue
		}
		id, _, _ := strings.Cut(rest, " ")
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids
}
