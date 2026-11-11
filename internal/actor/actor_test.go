package actor

import (
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/origin"
)

const (
	userID = "11111111-1111-1111-1111-111111111111"
	runID  = "22222222-2222-2222-2222-222222222222"
)

func TestLabel(t *testing.T) {
	cases := []struct {
		name string
		in   Actor
		want string
	}{
		{"user on the web", Actor{Kind: auth.KindUser, Name: "Ishan", Origin: origin.Origin{Kind: origin.Web}}, "Ishan"},
		{"user on the cli", Actor{Kind: auth.KindUser, Name: "Ishan", Origin: origin.Origin{Kind: origin.CLI}}, "Ishan"},
		{"user over mcp with a known client", Actor{Kind: auth.KindUser, Name: "Ishan", Origin: origin.Origin{Kind: origin.MCP, Client: "claude-code"}}, "Ishan (via Claude Code)"},
		{"user over mcp with no client", Actor{Kind: auth.KindUser, Name: "Ishan", Origin: origin.Origin{Kind: origin.MCP}}, "Ishan (via MCP)"},
		{"user over mcp with an unknown client", Actor{Kind: auth.KindUser, Name: "Ishan", Origin: origin.Origin{Kind: origin.MCP, Client: "My Bot "}}, "Ishan (via My Bot)"},
		{"service account on the api", Actor{Kind: auth.KindServiceAccount, Name: "ci-deploys", Origin: origin.Origin{Kind: origin.API}}, "ci-deploys"},
		{"service account over mcp", Actor{Kind: auth.KindServiceAccount, Name: "ci-deploys", Origin: origin.Origin{Kind: origin.MCP, Client: "cursor"}}, "ci-deploys (via Cursor)"},
		{"user in a run", Actor{Kind: auth.KindUser, Name: "Ishan", Origin: origin.Origin{Kind: origin.Web}, RunID: runID}, "Observe Agent for Ishan"},
		{"service account in a run", Actor{Kind: auth.KindServiceAccount, Name: "ci-deploys", Origin: origin.Origin{Kind: origin.CLI}, RunID: runID}, "Observe Agent for ci-deploys"},
		{"mcp user in a run", Actor{Kind: auth.KindUser, Name: "Ishan", Origin: origin.Origin{Kind: origin.MCP, Client: "claude-code"}, RunID: runID}, "Observe Agent for Ishan"},
		{"no name", Actor{Kind: auth.KindUser, Origin: origin.Origin{Kind: origin.Web}}, "Unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.in.Label(); got != tc.want {
				t.Fatalf("Label() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestClientName(t *testing.T) {
	cases := map[string]string{
		"claude-code":        "Claude Code",
		"claude code":        "Claude Code",
		"Claude Code":        "Claude Code",
		"claude-ai":          "Claude",
		"cursor":             "Cursor",
		"cursor-vscode":      "Cursor",
		"opencode":           "OpenCode",
		"codex":              "Codex",
		"codex-mcp-client":   "Codex",
		"windsurf":           "Windsurf",
		"zed":                "Zed",
		"vscode":             "VS Code",
		"visual studio code": "VS Code",
		"Visual Studio Code": "VS Code",
		"  Cursor  ":         "Cursor",
		"my-agent":           "my-agent",
		" my-agent ":         "my-agent",
		"":                   "",
		"   ":                "",
	}
	for raw, want := range cases {
		if got := ClientName(raw); got != want {
			t.Errorf("ClientName(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestOfFallsBackToEmail(t *testing.T) {
	a := Of(auth.Principal{Kind: auth.KindUser, ID: userID, Email: "i@example.com"}, origin.Origin{Kind: origin.Web}, "")
	if a.Name != "i@example.com" {
		t.Fatalf("Name = %q, want the email", a.Name)
	}
	a = Of(auth.Principal{Kind: auth.KindUser, ID: userID, Name: "Ishan", Email: "i@example.com"}, origin.Origin{Kind: origin.Web}, "")
	if a.Name != "Ishan" {
		t.Fatalf("Name = %q, want the name", a.Name)
	}
}

func TestColumnsRoundTrip(t *testing.T) {
	in := Actor{Kind: auth.KindServiceAccount, ID: userID, Name: "ci", Origin: origin.Origin{Kind: origin.MCP, Client: "claude-code"}, RunID: runID}
	cols, err := in.Columns()
	if err != nil {
		t.Fatalf("Columns: %v", err)
	}
	if got := FromColumns(cols); got != in {
		t.Fatalf("round trip = %+v, want %+v", got, in)
	}

	web := Actor{Kind: auth.KindUser, ID: userID, Name: "Ishan", Origin: origin.Origin{Kind: origin.Web, Client: "claude-code"}}
	cols, err = web.Columns()
	if err != nil {
		t.Fatalf("Columns: %v", err)
	}
	got := FromColumns(cols)
	if got.Origin.Client != "" || got.Origin.Kind != origin.Web || got.RunID != "" {
		t.Fatalf("web round trip = %+v, want the client dropped", got)
	}
}

func TestColumnsRejectsBadUUIDs(t *testing.T) {
	cases := map[string]Actor{
		"id":     {Kind: auth.KindUser, ID: "nope", Name: "n"},
		"run id": {Kind: auth.KindUser, ID: userID, Name: "n", RunID: "nope"},
	}
	for name, a := range cases {
		if _, err := a.Columns(); err == nil {
			t.Errorf("%s: Columns returned no error", name)
		}
	}
}

func TestJSON(t *testing.T) {
	web := Actor{Kind: auth.KindUser, ID: userID, Name: "Ishan", Origin: origin.Origin{Kind: origin.Web}}.JSON()
	if web.Origin == nil || *web.Origin != "web" || web.Client != nil || web.RunID != nil || web.Label != "Ishan" {
		t.Fatalf("web JSON = %+v", web)
	}

	mcp := Actor{Kind: auth.KindUser, ID: userID, Name: "Ishan", Origin: origin.Origin{Kind: origin.MCP, Client: "claude-code"}}.JSON()
	if mcp.Client == nil || *mcp.Client != "Claude Code" || mcp.Label != "Ishan (via Claude Code)" {
		t.Fatalf("mcp JSON = %+v", mcp)
	}

	none := Actor{Kind: auth.KindUser, ID: userID, Name: "Ishan"}.JSON()
	if none.Origin != nil {
		t.Fatalf("empty origin JSON = %+v, want null origin", none)
	}

	run := Actor{Kind: auth.KindUser, ID: userID, Name: "Ishan", RunID: runID}.JSON()
	if run.RunID == nil || *run.RunID != runID {
		t.Fatalf("run JSON = %+v", run)
	}
}
