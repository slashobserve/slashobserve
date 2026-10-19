package grafana

import "testing"

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

func TestLegacyCrosshairSettingIsSilent(t *testing.T) {
	res := convertDoc(t, dashboardJSON(`"sharedCrosshair":true,"graphTooltip":1`, panelJSON(1, 0, 0, 12, 4, "")))
	if notes := notesFor(res, "dashboard options"); len(notes) != 0 {
		t.Fatalf("notes = %+v, want none", notes)
	}
	res = convertDoc(t, dashboardJSON(`"somethingNew":true`, panelJSON(1, 0, 0, 12, 4, "")))
	if notes := notesFor(res, "dashboard options"); len(notes) != 1 || !hasPath(notes, "somethingNew") {
		t.Fatalf("notes = %+v, want the unknown key reported", notes)
	}
}
