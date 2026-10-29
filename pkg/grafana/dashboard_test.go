package grafana

import (
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestLinkOptionsAreReported(t *testing.T) {
	links := `"links":[
		{"type":"link","title":"Docs","url":"https://example.com","keepTime":true,"includeVars":true},
		{"type":"link","title":"Plain","url":"https://example.com/plain","keepTime":false,"includeVars":false},
		{"type":"link","title":"Vars","url":"https://example.com/vars","includeVars":true}
	]`
	res := convertDoc(t, dashboardJSON(links, panelJSON(1, 0, 0, 12, 4, "")))
	if got := len(res.Spec.Links); got != 3 {
		t.Fatalf("links = %d, want 3", got)
	}
	notes := notesFor(res, "dashboard links")
	if len(notes) != 2 {
		t.Fatalf("notes = %+v, want one for keepTime and one for includeVars", notes)
	}
	if !hasPath(notes, "links[0].keepTime") || hasPath(notes, "links[1].keepTime") || hasPath(notes, "links[2].keepTime") {
		t.Fatalf("keepTime paths in %+v", notes)
	}
	for _, path := range []string{"links[0].includeVars", "links[2].includeVars"} {
		if !hasPath(notes, path) {
			t.Fatalf("no note for %s in %+v", path, notes)
		}
	}
	if hasPath(notes, "links[1].includeVars") {
		t.Fatalf("links[1] has no options but is reported: %+v", notes)
	}
}

func TestCursorSync(t *testing.T) {
	tests := []struct {
		name     string
		extra    string
		want     spec.CursorSync
		wantNote bool
	}{
		{name: "no setting", extra: ``, want: ""},
		{name: "tooltip 0", extra: `"graphTooltip":0`, want: ""},
		{name: "tooltip 1", extra: `"graphTooltip":1`, want: spec.CursorSyncCrosshair},
		{name: "tooltip 2", extra: `"graphTooltip":2`, want: spec.CursorSyncTooltip},
		{name: "legacy crosshair true", extra: `"sharedCrosshair":true`, want: spec.CursorSyncCrosshair},
		{name: "legacy crosshair false", extra: `"sharedCrosshair":false`, want: ""},
		{name: "tooltip wins over legacy crosshair", extra: `"sharedCrosshair":false,"graphTooltip":2`, want: spec.CursorSyncTooltip},
		{name: "unknown tooltip number", extra: `"graphTooltip":3`, want: "", wantNote: true},
		{name: "tooltip as string", extra: `"graphTooltip":"1"`, want: "", wantNote: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := convertDoc(t, dashboardJSON(tt.extra, panelJSON(1, 0, 0, 12, 4, "")))
			if res.Spec.CursorSync != tt.want {
				t.Fatalf("cursorSync = %q, want %q", res.Spec.CursorSync, tt.want)
			}
			notes := notesFor(res, "shared crosshair")
			if tt.wantNote != (len(notes) == 1 && hasPath(notes, "graphTooltip")) || len(notes) > 1 {
				t.Fatalf("shared crosshair notes = %+v, want note: %v", notes, tt.wantNote)
			}
			if notes := notesFor(res, "dashboard options"); len(notes) != 0 {
				t.Fatalf("dashboard options notes = %+v, want none", notes)
			}
		})
	}
}

func TestUnknownDashboardOptionIsReported(t *testing.T) {
	res := convertDoc(t, dashboardJSON(`"somethingNew":true`, panelJSON(1, 0, 0, 12, 4, "")))
	if notes := notesFor(res, "dashboard options"); len(notes) != 1 || !hasPath(notes, "somethingNew") {
		t.Fatalf("notes = %+v, want the unknown key reported", notes)
	}
}

func TestLinksWithForeignSchemesAreNotKept(t *testing.T) {
	urls := []struct {
		url  string
		kept bool
	}{
		{"https://example.com/a", true},
		{"HTTP://example.com/a", true},
		{"mailto:ops@example.com", true},
		{"/d/abc/other?orgId=1", true},
		{"${__url_time_range}", true},
		{"https://example.com/?q=a:b", true},
		{"javascript:alert(1)", false},
		{"JaVaScRiPt:alert(1)", false},
		{"java\\tscript:alert(1)", false},
		{" data:text/html,<script>alert(1)</script>", false},
		{"vbscript:msgbox(1)", false},
	}
	var dashLinks, panelLinks []string
	for _, u := range urls {
		dashLinks = append(dashLinks, `{"type":"link","title":"L","url":"`+u.url+`"}`)
		panelLinks = append(panelLinks, `{"title":"L","url":"`+u.url+`"}`)
	}
	res := convertDoc(t, dashboardJSON(`"links":[`+strings.Join(dashLinks, ",")+`]`, `{"id":1,"type":"timeseries","title":"P1","gridPos":{"x":0,"y":0,"w":12,"h":4},"datasource":{"type":"prometheus","uid":"prom"},`+upTarget+`,"links":[`+strings.Join(panelLinks, ",")+`]}`))
	wantKept := 0
	for _, u := range urls {
		if u.kept {
			wantKept++
		}
	}
	if got := len(res.Spec.Links); got != wantKept {
		t.Errorf("dashboard links kept = %d, want %d: %+v", got, wantKept, res.Spec.Links)
	}
	panel := res.Spec.AllPanels()[0].Panel
	if got := len(panel.Links); got != wantKept {
		t.Errorf("panel links kept = %d, want %d: %+v", got, wantKept, panel.Links)
	}
	for _, link := range append(slices.Clone(res.Spec.Links), panel.Links...) {
		if strings.Contains(strings.ToLower(link.URL), "script:") || strings.Contains(link.URL, "data:") {
			t.Errorf("link %q was kept", link.URL)
		}
	}
	if !hasPath(notesFor(res, "dashboard links"), "links[6]") {
		t.Errorf("the dropped dashboard links are not reported: %+v", res.Report.Notes)
	}
	if !slices.Contains(panelLine(t, res, "p1").Paths, "links[6]") {
		t.Errorf("the dropped panel links are not reported: %+v", panelLine(t, res, "p1"))
	}
}
