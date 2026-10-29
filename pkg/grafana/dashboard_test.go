package grafana

import (
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
