package grafana

import (
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const verifySpec = `{
	"specVersion": "0",
	"title": "Verify",
	"time": {"from": "now-6h", "to": "now"},
	"panels": [
		{"id": "r1", "type": "row", "title": "Row", "gridPos": {"x": 0, "y": 0, "w": 24, "h": 1}, "panels": [
			{"id": "p1", "type": "stat", "title": "Child", "gridPos": {"x": 0, "y": 1, "w": 6, "h": 4}, "datasource": "prom", "queries": [{"query": "up"}]}
		]},
		{"id": "p2", "type": "placeholder", "gridPos": {"x": 0, "y": 5, "w": 6, "h": 4}, "reason": "Not supported."}
	]
}`

func verifiable(t *testing.T) *Result {
	t.Helper()
	parsed, err := spec.Parse([]byte(verifySpec))
	if err != nil {
		t.Fatal(err)
	}
	return &Result{
		Spec: parsed,
		Report: Report{Panels: []PanelLine{
			{PanelID: "p1", Status: statusImported, Reasons: []string{}},
			{PanelID: "p2", Status: statusPlaceholder, Reasons: []string{}},
		}},
	}
}

func TestVerifyFallsBackToADashboardNoteForRowIssues(t *testing.T) {
	res := verifiable(t)
	res.Verify([]spec.Issue{{Path: "/panels/0/title", Message: "the row is broken"}})
	if !res.Report.Verified {
		t.Fatal("Verified is not set")
	}
	notes := notesFor(res, "dashboard")
	if len(notes) != 1 || notes[0].Status != statusUnverified || notes[0].Reason != "the row is broken" {
		t.Fatalf("notes = %+v", res.Report.Notes)
	}
	if res.Report.Counts.Unverified != 0 {
		t.Fatalf("counts = %+v, want no unverified panel", res.Report.Counts)
	}
}

func TestVerifyMarksPanelsAndSkipsPlaceholders(t *testing.T) {
	res := verifiable(t)
	res.Verify([]spec.Issue{
		{Path: "/panels/0/panels/0/queries/0/query", Message: "bad syntax"},
		{Path: "/panels/0/panels/0/queries", Code: queryproxy.CodeQueryEmpty, Message: "no data"},
		{Path: "/panels/1/queries", Code: queryproxy.CodeQueryEmpty, Message: "no data"},
	})
	line := panelLine(t, res, "p1")
	wantReasons := []string{"Query A failed on prom: bad syntax", "The queries returned no data for the dashboard's default time range."}
	if line.Status != statusUnverified || !slices.Equal(line.Reasons, wantReasons) {
		t.Fatalf("p1 = %+v", line)
	}
	if got := panelLine(t, res, "p2").Status; got != statusPlaceholder {
		t.Fatalf("p2 status = %q, placeholders are never unverified", got)
	}
	if res.Report.Counts.Unverified != 1 || res.Report.Counts.Placeholder != 1 {
		t.Fatalf("counts = %+v", res.Report.Counts)
	}
}

func TestQueryEmptyCodeMatchesTheQueryProxy(t *testing.T) {
	if codeQueryEmpty != queryproxy.CodeQueryEmpty {
		t.Fatalf("codeQueryEmpty = %q, queryproxy.CodeQueryEmpty = %q", codeQueryEmpty, queryproxy.CodeQueryEmpty)
	}
}
