package dashboards

import "testing"

func TestVersionActorLabels(t *testing.T) {
	author := Author{Kind: "user", ID: "11111111-1111-4111-8111-111111111111", Name: "Ada", Email: "ada@example.com"}
	cases := []struct {
		name       string
		meta       VersionMeta
		wantLabel  string
		wantClient string
	}{
		{"saved by the agent", VersionMeta{Author: author, Origin: "web", AgentRequestID: "22222222-2222-4222-8222-222222222222"}, "/observe for Ada", ""},
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
		{"created by the agent", Creator{Author: author, Origin: "api", RunID: "22222222-2222-4222-8222-222222222222"}, "/observe for ci-deploys"},
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
