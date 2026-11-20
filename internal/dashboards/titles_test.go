package dashboards

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestCleanTitlesTrimsAndReportsEachPath(t *testing.T) {
	d := &spec.Dashboard{
		Title: "  QA lead  ",
		Panels: []spec.Panel{
			{ID: "a", Title: " Rate "},
			{ID: "b"},
			{ID: "row", Title: "Row", Panels: []spec.Panel{{ID: "c", Title: "\u200b"}}},
			{ID: "d", Title: "x\u202ey"},
		},
	}
	issues := cleanTitles(d, nil)
	if d.Title != "QA lead" || d.Panels[0].Title != "Rate" || d.Panels[1].Title != "" {
		t.Fatalf("titles = %q, %q, %q", d.Title, d.Panels[0].Title, d.Panels[1].Title)
	}
	want := map[string]spec.IssueCode{"/panels/2/panels/0/title": "text_invisible_only", "/panels/3/title": "text_bidi_override"}
	if len(issues) != len(want) {
		t.Fatalf("issues = %v", issues)
	}
	for _, issue := range issues {
		if want[issue.Path] != issue.Code || !strings.HasPrefix(issue.Message, "title ") {
			t.Errorf("issue = %+v", issue)
		}
	}
}

func TestCleanTitlesRefusesWhatSchemaMinLengthLetsThrough(t *testing.T) {
	for _, title := range []string{"  ", "\u200b", "a\nb", "a\x07b", "x\u202ey"} {
		issues := cleanTitles(&spec.Dashboard{Title: title}, nil)
		if len(issues) != 1 || issues[0].Path != "/title" {
			t.Errorf("%q: %v", title, issues)
		}
	}
}

func TestCleanTitlesKeepsATitleThePreviousVersionAlreadyHeld(t *testing.T) {
	stored := &spec.Dashboard{Title: "Shop", Panels: []spec.Panel{{ID: "a", Title: "CPU\nusage"}, {ID: "b", Title: "Memory"}}}
	next := &spec.Dashboard{Title: "Shop", Panels: []spec.Panel{
		{ID: "a", Title: "CPU\nusage"},
		{ID: "b", Title: "Mem\u200bory"},
		{ID: "c", Title: "new\tline"},
	}}
	issues := cleanTitles(next, stored)
	if next.Panels[0].Title != "CPU\nusage" {
		t.Errorf("a stored title changed to %q", next.Panels[0].Title)
	}
	paths := make([]string, len(issues))
	for i, issue := range issues {
		paths[i] = issue.Path
	}
	if want := []string{"/panels/1/title", "/panels/2/title"}; !slices.Equal(paths, want) {
		t.Errorf("issues at %v, want %v", paths, want)
	}
}

func TestAStoredPanelTitleThatBreaksTheRuleDoesNotBlockLaterSavesOrCopies(t *testing.T) {
	r := newRig(t, false)
	dash := r.createSpec(t, specDef{panels: []panelDef{{id: "a", query: "m1"}, {id: "b", query: "m2"}}})
	r.db.Exec(t, `UPDATE dashboard_versions SET spec = jsonb_set(spec, '{panels,0,title}', to_jsonb($2::text)) WHERE dashboard_id = $1`, dash, "CPU\nusage\u00ad")

	ver, err := r.saveChanged(t, dash, 1, upsert(1, panelDef{id: "b", query: "m3"}))
	if err != nil {
		t.Fatalf("a save that leaves the stored title alone: %v", err)
	}
	saved, err := r.service.VersionSpec(t.Context(), dash, ver.Number)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Panels[0].Title != "CPU\nusage\u00ad" {
		t.Errorf("stored title became %q", saved.Panels[0].Title)
	}
	if _, _, err := r.service.Duplicate(t.Context(), testUser, dash, pgtype.UUID{}, "Copy"); err != nil {
		t.Fatalf("duplicate: %v", err)
	}
	raw := r.mergedPatch(t, dash, ver.Number, upsert(1, panelDef{id: "b", query: "m3"}))
	raw = bytes.Replace(raw, []byte(`"title":"b"`), []byte(`"title":"b\tc"`), 1)
	_, _, _, err = r.saveRequest(t, dash, ver.Number, raw, "typed", pgtype.UUID{})
	if err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("a new title with a tab was accepted: %v", err)
	}
}
