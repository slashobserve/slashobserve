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

func originOf(t *testing.T, d Dashboard) string {
	t.Helper()
	raw, err := json.Marshal(NewDashboardVersionJSON(d, Version{}))
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Dashboard struct {
			Origin      *string `json:"origin"`
			Provisioned bool    `json:"provisioned"`
		} `json:"dashboard"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Dashboard.Origin == nil {
		t.Fatalf("the dashboard read has no origin: %s", raw)
	}
	if got.Dashboard.Provisioned != d.Provisioned {
		t.Fatalf("provisioned = %v, want %v as before", got.Dashboard.Provisioned, d.Provisioned)
	}
	return *got.Dashboard.Origin
}

func TestTheDashboardReadNamesWhereTheDashboardComesFrom(t *testing.T) {
	if got := originOf(t, Dashboard{}); got != "editable" {
		t.Fatalf("origin = %q, want editable for a dashboard made in the app", got)
	}
	cases := []struct {
		access folders.DashboardAccess
		want   string
	}{
		{folders.DashboardAccess{}, "editable"},
		{folders.DashboardAccess{Provisioned: true}, "starter"},
		{folders.DashboardAccess{FromFile: true}, "file"},
		{folders.DashboardAccess{Applied: true}, "applied"},
	}
	for _, tc := range cases {
		if got := originOf(t, withAccess(Dashboard{}, tc.access)); got != tc.want {
			t.Errorf("origin = %q, want %q", got, tc.want)
		}
	}
}

func TestOnlyASaveToAFileDashboardCarriesTheWarning(t *testing.T) {
	for _, access := range []folders.DashboardAccess{{}, {Provisioned: true}, {Applied: true}} {
		if got := saveWarning(access); got != "" {
			t.Errorf("a save to a dashboard with origin %s warns %q, want no warning", access.Origin(), got)
		}
	}
	warning := saveWarning(folders.DashboardAccess{FromFile: true})
	if warning != folders.FileOwnedWarning {
		t.Fatalf("warning = %q, want the file warning", warning)
	}

	fields := func(body versionSavedJSON) map[string]json.RawMessage {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]json.RawMessage
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got := fields(versionSavedJSON{Warning: warning}); string(got["warning"]) != `"`+warning+`"` {
		t.Errorf("the save answer carries warning %s, want the file warning", got["warning"])
	}
	if _, ok := fields(versionSavedJSON{})["warning"]; ok {
		t.Error("a save with nothing to warn about has a warning key")
	}
}
