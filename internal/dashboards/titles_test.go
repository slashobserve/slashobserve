package dashboards

import (
	"strings"
	"testing"

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
	issues := cleanTitles(d)
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
		issues := cleanTitles(&spec.Dashboard{Title: title})
		if len(issues) != 1 || issues[0].Path != "/title" {
			t.Errorf("%q: %v", title, issues)
		}
	}
}
