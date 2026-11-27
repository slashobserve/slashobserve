package dashboards

import (
	"encoding/json"
	"testing"

	"github.com/slashobserve/slashobserve/internal/folders"
)

func TestVersionActorLabels(t *testing.T) {
	author := Author{Kind: "user", ID: "11111111-1111-4111-8111-111111111111", Name: "Ada", Email: "ada@example.com"}
	cases := []struct {
		name       string
		meta       VersionMeta
		wantLabel  string
		wantClient string
	}{
		{"saved by the agent", VersionMeta{Author: author, Origin: "web", AgentRequestID: "22222222-2222-4222-8222-222222222222"}, "Observe Agent for Ada", ""},
		{"saved over mcp", VersionMeta{Author: author, Origin: "mcp", OriginClient: "claude-code"}, "Ada (via Claude Code)", "Claude Code"},
		{"saved on the web", VersionMeta{Author: author, Origin: "web"}, "Ada", ""},
		{"saved by an author with only an email", VersionMeta{Author: Author{Kind: "user", ID: author.ID, Email: "ada@example.com"}, Origin: "web"}, "ada@example.com", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := newVersionMetaJSON(tc.meta).Actor
			if got.Label != tc.wantLabel {
				t.Fatalf("label = %q, want %q", got.Label, tc.wantLabel)
			}
			switch {
			case tc.wantClient == "" && got.Client != nil:
				t.Fatalf("client = %q, want null", *got.Client)
			case tc.wantClient != "" && (got.Client == nil || *got.Client != tc.wantClient):
				t.Fatalf("client = %v, want %q", got.Client, tc.wantClient)
			}
		})
	}
}

func TestCreatorActorLabels(t *testing.T) {
	author := Author{Kind: "service_account", ID: "11111111-1111-4111-8111-111111111111", Name: "ci-deploys"}
	cases := []struct {
		name    string
		creator Creator
		want    string
	}{
		{"created by the agent", Creator{Author: author, Origin: "api", RunID: "22222222-2222-4222-8222-222222222222"}, "Observe Agent for ci-deploys"},
		{"created over mcp", Creator{Author: author, Origin: "mcp", OriginClient: "cursor"}, "ci-deploys (via Cursor)"},
		{"created through the api", Creator{Author: author, Origin: "api"}, "ci-deploys"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := newCreatorJSON(tc.creator).Actor.Label; got != tc.want {
				t.Fatalf("label = %q, want %q", got, tc.want)
			}
		})
	}
}

func managedByOf(t *testing.T, d Dashboard) string {
	t.Helper()
	raw, err := json.Marshal(NewDashboardVersionJSON(d, Version{}))
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Dashboard struct {
			ManagedBy   *string `json:"managed_by"`
			Provisioned bool    `json:"provisioned"`
		} `json:"dashboard"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Dashboard.ManagedBy == nil {
		t.Fatalf("the dashboard read has no managed_by: %s", raw)
	}
	if got.Dashboard.Provisioned != d.Provisioned {
		t.Fatalf("provisioned = %v, want %v as before", got.Dashboard.Provisioned, d.Provisioned)
	}
	return *got.Dashboard.ManagedBy
}

func TestTheDashboardReadNamesWhatManagesTheDashboard(t *testing.T) {
	if got := managedByOf(t, Dashboard{}); got != "workspace" {
		t.Fatalf("managed_by = %q, want workspace for a dashboard made in the app", got)
	}
	cases := []struct {
		access folders.DashboardAccess
		want   string
	}{
		{folders.DashboardAccess{}, "workspace"},
		{folders.DashboardAccess{Provisioned: true}, "starter"},
		{folders.DashboardAccess{FromFile: true}, "file"},
		{folders.DashboardAccess{Applied: true}, "apply"},
	}
	for _, tc := range cases {
		if got := managedByOf(t, Dashboard{}.WithAccess(tc.access)); got != tc.want {
			t.Errorf("managed_by = %q, want %q", got, tc.want)
		}
	}
}

func TestAnAnswerToAnEditCarriesTheWarningOfWhatManagesTheDashboard(t *testing.T) {
	for _, managedBy := range []folders.ManagedBy{folders.ManagedByWorkspace, folders.ManagedByStarter, folders.ManagedByFile, folders.ManagedByApply} {
		raw, err := json.Marshal(editedJSON(Dashboard{ManagedBy: managedBy}, Version{}))
		if err != nil {
			t.Fatal(err)
		}
		var got struct {
			Warning *string `json:"warning"`
		}
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatal(err)
		}
		want := managedBy.EditWarning()
		switch {
		case want == "" && got.Warning != nil:
			t.Errorf("an edit to a dashboard managed by %s answers warning %q, want no warning key", managedBy, *got.Warning)
		case want != "" && (got.Warning == nil || *got.Warning != want):
			t.Errorf("an edit to a dashboard managed by %s answers %s, want warning %q", managedBy, raw, want)
		}
	}
}

func TestAVersionEventNamesTheVersionItReplacedOnlyWhenThereIsOne(t *testing.T) {
	parent := 4
	meta := VersionMeta{Number: 5, ParentVersion: &parent, Summary: "Provisioned from a.json"}
	fields := func(replaced int) map[string]any {
		raw, err := json.Marshal(versionActivityData("A", meta, replaced))
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got := fields(4); got["replaced_version"] != float64(4) || got["version"] != float64(5) || got["parent_version"] != float64(4) {
		t.Errorf("event data = %v, want replaced_version 4 beside version 5", got)
	}
	if got := fields(0); len(got) != 4 {
		t.Errorf("event data = %v, want only version, parent_version, title and summary", got)
	}
}

func TestADashboardInAListNamesWhatManagesIt(t *testing.T) {
	for _, managedBy := range []folders.ManagedBy{folders.ManagedByWorkspace, folders.ManagedByStarter, folders.ManagedByFile, folders.ManagedByApply} {
		raw, err := json.Marshal(newDashboardListItemJSON(ListItem{Dashboard: Dashboard{ManagedBy: managedBy}}))
		if err != nil {
			t.Fatal(err)
		}
		var got struct {
			ManagedBy *string `json:"managed_by"`
		}
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatal(err)
		}
		if got.ManagedBy == nil || *got.ManagedBy != managedBy.String() {
			t.Errorf("the list item of a dashboard managed by %s reads %s, want managed_by %q", managedBy, raw, managedBy)
		}
	}
}
