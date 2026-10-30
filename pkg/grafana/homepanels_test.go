package grafana

import (
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestHomePanelsImportAsTextPanels(t *testing.T) {
	cases := []struct{ typ, contains string }{
		{"welcome", "Welcome to Grafana. This panel was Grafana's home page welcome panel."},
		{"gettingstarted", docsURL},
	}
	for _, tc := range cases {
		t.Run(tc.typ, func(t *testing.T) {
			panel, line := convertedPanel(t, panelDoc(tc.typ, `"title":"Home"`))
			if panel.Type != spec.Text || panel.Content == nil || !strings.Contains(*panel.Content, tc.contains) {
				t.Fatalf("panel = %+v, want a text panel containing %q", panel, tc.contains)
			}
			if line.Status != statusImported || line.Type != "text" || line.SourceType != tc.typ {
				t.Fatalf("line = %+v, want imported text from %s", line, tc.typ)
			}
		})
	}
}

func TestDebugPanelWithTargetsImportsAsTable(t *testing.T) {
	panel, line := convertedPanel(t, panelDoc("debug", `"title":"D",`+upTarget))
	if panel.Type != spec.Table || len(panel.Queries) != 1 || panel.Queries[0].Query != "up" {
		t.Fatalf("panel = %+v, want a table over the target", panel)
	}
	if line.Status != statusPartial || !slices.Contains(line.Reasons, debugPanelReason) || line.SourceType != "debug" {
		t.Fatalf("line = %+v, want partial with the debug reason", line)
	}
}

func TestDebugPanelWithoutTargetsImportsAsText(t *testing.T) {
	panel, line := convertedPanel(t, panelDoc("debug", `"title":"D"`))
	if panel.Type != spec.Text || panel.Content == nil || *panel.Content != debugPanelReason {
		t.Fatalf("panel = %+v, want a text panel saying %q", panel, debugPanelReason)
	}
	if line.Status != statusPartial || !slices.Contains(line.Reasons, debugPanelReason) {
		t.Fatalf("line = %+v, want partial with the debug reason", line)
	}
}

func TestTransparentPanelsKeepTheFlagSilently(t *testing.T) {
	cases := []struct {
		name, typ, members string
		want, reported     bool
	}{
		{"data", "stat", upTarget + `,"transparent":true`, true, false},
		{"text", "text", `"options":{"content":"hi"},"transparent":true`, true, false},
		{"false", "stat", upTarget + `,"transparent":false`, false, false},
		{"absent", "stat", upTarget, false, false},
		{"dashlist", "dashlist", `"transparent":true`, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := convertedPanel(t, panelDoc(tc.typ, tc.members))
			if panel.Transparent != tc.want {
				t.Fatalf("transparent = %v, want %v", panel.Transparent, tc.want)
			}
			if got := slices.Contains(line.Paths, "transparent"); got != tc.reported {
				t.Fatalf("paths = %v, reported = %v, want %v", line.Paths, got, tc.reported)
			}
		})
	}
}

func TestHideTimeOverrideIsReportedOnlyWhereAnOverrideIsShown(t *testing.T) {
	cases := []struct {
		name, members string
		reported      bool
	}{
		{"time from", `"timeFrom":"1h","hideTimeOverride":true`, true},
		{"time shift", `"timeShift":"1d","hideTimeOverride":true`, true},
		{"no override", `"hideTimeOverride":true`, false},
		{"not hidden", `"timeFrom":"1h","hideTimeOverride":false`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, line := convertedPanel(t, panelDoc("stat", upTarget+","+tc.members))
			got := slices.Contains(line.Paths, "hideTimeOverride")
			if got != tc.reported || got && !slices.Contains(line.Reasons, hideTimeOverrideReason) {
				t.Fatalf("paths = %v, reasons = %v, want reported = %v", line.Paths, line.Reasons, tc.reported)
			}
		})
	}
}
