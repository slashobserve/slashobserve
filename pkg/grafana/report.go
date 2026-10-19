package grafana

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const codeQueryEmpty spec.IssueCode = "query_empty"

var queryIssuePath = regexp.MustCompile(`^/queries/([0-9]+)/query$`)

func (r *Report) recount() {
	r.Counts = Counts{Total: len(r.Panels)}
	for _, p := range r.Panels {
		switch p.Status {
		case statusImported:
			r.Counts.Imported++
		case statusPartial:
			r.Counts.Partial++
		case statusPlaceholder:
			r.Counts.Placeholder++
		case statusUnverified:
			r.Counts.Unverified++
		}
	}
}

func (r *Result) Verify(issues []spec.Issue) {
	r.Report.Verified = true
	if r.Spec == nil {
		return
	}
	entries := r.Spec.AllPanels()
	for _, issue := range issues {
		if rest, ok := strings.CutPrefix(issue.Path, "/variables/"); ok {
			r.verifyVariable(rest, issue)
			continue
		}
		best := -1
		for i, e := range entries {
			if issue.Path != e.Pointer && !strings.HasPrefix(issue.Path, e.Pointer+"/") {
				continue
			}
			if best < 0 || len(e.Pointer) > len(entries[best].Pointer) {
				best = i
			}
		}
		if best < 0 {
			r.dashboardNote(issue)
			continue
		}
		entry := entries[best]
		line := r.panelLine(entry.Panel.ID)
		if line == nil {
			r.dashboardNote(issue)
			continue
		}
		if line.Status == statusPlaceholder {
			continue
		}
		line.Status = statusUnverified
		line.Reasons = append(line.Reasons, verifyReason(entry, strings.TrimPrefix(issue.Path, entry.Pointer), issue))
	}
	r.Report.recount()
}

func (r *Result) dashboardNote(issue spec.Issue) {
	r.Report.Notes = append(r.Report.Notes, Note{Subject: "dashboard", Status: statusUnverified, Reason: issue.Message, Paths: []string{}})
}

func verifyReason(entry spec.PanelEntry, suffix string, issue spec.Issue) string {
	panel := entry.Panel
	if m := queryIssuePath.FindStringSubmatch(suffix); m != nil {
		if j, err := strconv.Atoi(m[1]); err == nil && j < len(panel.Queries) {
			return fmt.Sprintf("Query %s failed on %s: %s", panel.QueryRef(j), panel.QueryDatasource(j), issue.Message)
		}
	}
	switch {
	case suffix == "/queries" && issue.Code == codeQueryEmpty:
		return "The queries returned no data for the dashboard's default time range."
	case suffix == "/datasource":
		return fmt.Sprintf("Datasource %s is not available.", panel.Datasource)
	}
	return issue.Message
}

func (r *Result) verifyVariable(rest string, issue spec.Issue) {
	index, _, _ := strings.Cut(rest, "/")
	i, err := strconv.Atoi(index)
	if err != nil || i < 0 || i >= len(r.Spec.Variables) {
		return
	}
	note := Note{
		Subject: "variable " + r.Spec.Variables[i].Name,
		Status:  statusUnverified,
		Reason:  issue.Message,
		Paths:   []string{},
	}
	if i < len(r.variablePath) {
		note.Paths = []string{r.variablePath[i]}
	}
	r.Report.Notes = append(r.Report.Notes, note)
}

func (r *Result) panelLine(id string) *PanelLine {
	for i := range r.Report.Panels {
		if r.Report.Panels[i].PanelID == id {
			return &r.Report.Panels[i]
		}
	}
	return nil
}
