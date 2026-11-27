package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func mcpSourcesFixture(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mcp-sources.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestMCPSourcesFileReadsEverySource(t *testing.T) {
	secretFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(secretFile, []byte("file-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := mcpSourcesFixture(t, `mcpSources:
  - uid: docs
    name: Company docs
    description: Runbooks and design notes
    url: https://mcp.example.com/mcp
    audience: admins
    secretHeader: Authorization
    secretEnv: SLASHOBSERVE_DOCS_TOKEN
  - uid: wiki
    url: https://wiki.example.com/mcp
    secretHeader: X-Api-Key
    secretFile: `+secretFile+`
  - uid: open
    url: https://open.example.com/mcp
  - uid: code
    url: http://codeserver.demo.internal:8080/mcp
    allowPrivate: true
    approve: [list_dir, " grep "]
`)
	lookup := func(name string) (string, bool) { return "env-secret", name == "SLASHOBSERVE_DOCS_TOKEN" }
	got, err := loadMCPSources(path, lookup)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("got %d sources, want 4", len(got))
	}
	if d := got[0]; d.UID != "docs" || d.Name != "Company docs" || d.Description != "Runbooks and design notes" || d.Audience != datasource.AudienceAdmins || d.SecretHeader != "Authorization" || d.Secret.Reveal() != "env-secret" {
		t.Errorf("docs = %+v", d)
	}
	if w := got[1]; w.Name != "wiki" || w.Description != "" || w.Audience != datasource.AudienceEveryone || w.Secret.Reveal() != "file-secret" {
		t.Errorf("wiki = %+v", w)
	}
	if o := got[2]; !o.Secret.IsZero() || o.SecretHeader != "" || o.AllowPrivate {
		t.Errorf("open = %+v", o)
	}
	if c := got[3]; !c.AllowPrivate || c.URL != "http://codeserver.demo.internal:8080/mcp" || got[0].AllowPrivate || strings.Join(c.Approve, ",") != "list_dir,grep" || len(got[0].Approve) != 0 {
		t.Errorf("code = %+v", c)
	}
}

func TestMCPSourcesFileRefusals(t *testing.T) {
	cases := map[string]struct{ body, want string }{
		"no list":               {"other: 1\n", "field"},
		"missing list":          {"{}\n", "mcpSources: required"},
		"bad uid":               {"mcpSources:\n  - uid: \"-x\"\n    url: https://a.example.com\n", "mcpSources[0].uid"},
		"duplicate uid":         {"mcpSources:\n  - uid: a\n    url: https://a.example.com\n  - uid: a\n    url: https://b.example.com\n", "already used by mcpSources[0]"},
		"no url":                {"mcpSources:\n  - uid: a\n", "mcpSources[0].url: required"},
		"bad audience":          {"mcpSources:\n  - uid: a\n    url: https://a.example.com\n    audience: nobody\n", "mcpSources[0].audience"},
		"both secret fields":    {"mcpSources:\n  - uid: a\n    url: https://a.example.com\n    secretHeader: X\n    secretEnv: SLASHOBSERVE_A\n    secretFile: /x\n", "mutually exclusive"},
		"secret no header":      {"mcpSources:\n  - uid: a\n    url: https://a.example.com\n    secretEnv: SLASHOBSERVE_A\n", "secretHeader: required"},
		"header no secret":      {"mcpSources:\n  - uid: a\n    url: https://a.example.com\n    secretHeader: X\n", "needs secretEnv or secretFile"},
		"env prefix":            {"mcpSources:\n  - uid: a\n    url: https://a.example.com\n    secretHeader: X\n    secretEnv: HOME\n", "must start with SLASHOBSERVE_"},
		"format character":      {"mcpSources:\n  - uid: a\n    url: https://a.example.com\n    name: \"Do\\u200Bcs\"\n", "mcpSources[0].name"},
		"multiline description": {"mcpSources:\n  - uid: a\n    url: https://a.example.com\n    description: \"one\\ntwo\"\n", "mcpSources[0].description"},
		"long description":      {"mcpSources:\n  - uid: a\n    url: https://a.example.com\n    description: " + strings.Repeat("x", 301) + "\n", "mcpSources[0].description"},
		"empty tool name":       {"mcpSources:\n  - uid: a\n    url: https://a.example.com\n    approve: [\"\"]\n", "mcpSources[0].approve"},
		"bad tool name":         {"mcpSources:\n  - uid: a\n    url: https://a.example.com\n    approve: [\"a b\"]\n", "is not a tool name"},
		"duplicate tool name":   {"mcpSources:\n  - uid: a\n    url: https://a.example.com\n    approve: [x, x]\n", "is listed twice"},
		"two documents":         {"mcpSources: []\n---\nmcpSources: []\n", "multiple YAML documents"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := loadMCPSources(mcpSourcesFixture(t, c.body), func(string) (string, bool) { return "v", true })
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want it to contain %q", err, c.want)
			}
		})
	}
}

func TestMCPSourcesFileNeverShowsASecretInAnError(t *testing.T) {
	_, err := loadMCPSources(mcpSourcesFixture(t, "mcpSources:\n  - uid: a\n    url: https://a.example.com\n    audience: bad\n    secretHeader: X\n    secretEnv: SLASHOBSERVE_A\n"), func(string) (string, bool) { return "very-secret-value", true })
	if err == nil || strings.Contains(err.Error(), "very-secret-value") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadReadsTheMCPSourcesFile(t *testing.T) {
	path := mcpSourcesFixture(t, "mcpSources:\n  - uid: a\n    url: https://a.example.com/mcp\n")
	env := llmEnv(nil)
	env[envMCPSourcesFile] = path
	cfg, err := Load(envOf(env))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MCPSourcesFile != path || len(cfg.MCPSources) != 1 || cfg.MCPSources[0].UID != "a" {
		t.Fatalf("got %q and %+v", cfg.MCPSourcesFile, cfg.MCPSources)
	}
	env[envMCPSourcesFile] = filepath.Join(t.TempDir(), "absent.yaml")
	_, err = Load(envOf(env))
	if err == nil || !strings.Contains(err.Error(), envMCPSourcesFile) {
		t.Fatalf("err = %v, want it to name %s", err, envMCPSourcesFile)
	}
}

func TestValidMCPSourceNameRefusesControlAndFormatCharacters(t *testing.T) {
	for name, want := range map[string]bool{"Docs": true, "": false, "a\nb": false, "a\u200bb": false, strings.Repeat("é", 100): true, strings.Repeat("é", 101): false} {
		if got := ValidMCPSourceName(name); got != want {
			t.Errorf("ValidMCPSourceName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestTheDemoCodeLayerProvisionsOnePrivateSourceOnItsOwnHost(t *testing.T) {
	sources, err := loadMCPSources(filepath.Join("..", "..", "deploy", "astronomy", "mcp-sources.yaml"), func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 {
		t.Fatalf("got %d sources, want 1", len(sources))
	}
	got := sources[0]
	if got.UID != "shop-code" || !got.AllowPrivate || got.URL != "http://codeserver.demo.internal:8080/mcp" || got.Description == "" || strings.Join(got.Approve, ",") != "list_dir,read_file,grep" {
		t.Fatalf("got %+v", got)
	}
	compose, err := os.ReadFile(filepath.Join("..", "..", "deploy", "astronomy", "compose.code.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(compose), "- codeserver.demo.internal\n") || !strings.Contains(string(compose), "SLASHOBSERVE_OUTBOUND_DOMAINS: codeserver.demo.internal\n") {
		t.Fatal("compose.code.yaml must give the code server the host the source names and allow only that host")
	}
}
