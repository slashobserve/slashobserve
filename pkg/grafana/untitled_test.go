package grafana

import (
	"fmt"
	"slices"
	"testing"
)

func titledDoc(titleMember string) string {
	return fmt.Sprintf(`{"title":"T","schemaVersion":39,"panels":[{%s"id":1,"gridPos":{"x":0,"y":0,"w":12,"h":8},"datasource":{"type":"prometheus","uid":"prom"},"type":"stat",%s}]}`, titleMember, upTarget)
}

func TestUntitledPanelKeepsAHiddenTitle(t *testing.T) {
	cases := []struct {
		name, titleMember string
		wantTitle         string
		wantHidden        bool
	}{
		{"missing", ``, "Panel 1", true},
		{"empty", `"title":"",`, "Panel 1", true},
		{"blank", `"title":"   ",`, "Panel 1", true},
		{"titled", `"title":"Requests",`, "Requests", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := convertedPanel(t, titledDoc(tc.titleMember))
			if panel.Title != tc.wantTitle || panel.TitleHidden != tc.wantHidden {
				t.Fatalf("title = %q hidden = %v, want %q hidden = %v", panel.Title, panel.TitleHidden, tc.wantTitle, tc.wantHidden)
			}
			if line.Status != statusImported || slices.Contains(line.Paths, "title") {
				t.Fatalf("line = %+v, want imported with no title report", line)
			}
		})
	}
}
