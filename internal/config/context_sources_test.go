package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func contextSourcesFixture(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "context-sources.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestContextSourcesFileReadsEverySource(t *testing.T) {
	secretFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(secretFile, []byte("file-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := contextSourcesFixture(t, `contextSources:
  - uid: docs
    name: Company docs
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
`)
	lookup := func(name string) (string, bool) { return "env-secret", name == "SLASHOBSERVE_DOCS_TOKEN" }
	got, err := loadContextSources(path, lookup)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d sources, want 3", len(got))
	}
	if d := got[0]; d.UID != "docs" || d.Name != "Company docs" || d.Audience != datasource.AudienceAdmins || d.SecretHeader != "Authorization" || d.Secret.Reveal() != "env-secret" {
		t.Errorf("docs = %+v", d)
	}
	if w := got[1]; w.Name != "wiki" || w.Audience != datasource.AudienceEveryone || w.Secret.Reveal() != "file-secret" {
		t.Errorf("wiki = %+v", w)
	}
	if o := got[2]; !o.Secret.IsZero() || o.SecretHeader != "" {
		t.Errorf("open = %+v", o)
	}
}

func TestContextSourcesFileRefusals(t *testing.T) {
	cases := map[string]struct{ body, want string }{
		"no list":            {"other: 1\n", "field"},
		"missing list":       {"{}\n", "contextSources: required"},
		"bad uid":            {"contextSources:\n  - uid: \"-x\"\n    url: https://a.example.com\n", "contextSources[0].uid"},
		"duplicate uid":      {"contextSources:\n  - uid: a\n    url: https://a.example.com\n  - uid: a\n    url: https://b.example.com\n", "already used by contextSources[0]"},
		"no url":             {"contextSources:\n  - uid: a\n", "contextSources[0].url: required"},
		"bad audience":       {"contextSources:\n  - uid: a\n    url: https://a.example.com\n    audience: nobody\n", "contextSources[0].audience"},
		"both secret fields": {"contextSources:\n  - uid: a\n    url: https://a.example.com\n    secretHeader: X\n    secretEnv: SLASHOBSERVE_A\n    secretFile: /x\n", "mutually exclusive"},
		"secret no header":   {"contextSources:\n  - uid: a\n    url: https://a.example.com\n    secretEnv: SLASHOBSERVE_A\n", "secretHeader: required"},
		"header no secret":   {"contextSources:\n  - uid: a\n    url: https://a.example.com\n    secretHeader: X\n", "needs secretEnv or secretFile"},
		"env prefix":         {"contextSources:\n  - uid: a\n    url: https://a.example.com\n    secretHeader: X\n    secretEnv: HOME\n", "must start with SLASHOBSERVE_"},
		"format character":   {"contextSources:\n  - uid: a\n    url: https://a.example.com\n    name: \"Do\\u200Bcs\"\n", "contextSources[0].name"},
		"two documents":      {"contextSources: []\n---\ncontextSources: []\n", "multiple YAML documents"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := loadContextSources(contextSourcesFixture(t, c.body), func(string) (string, bool) { return "v", true })
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want it to contain %q", err, c.want)
			}
		})
	}
}

func TestContextSourcesFileNeverShowsASecretInAnError(t *testing.T) {
	_, err := loadContextSources(contextSourcesFixture(t, "contextSources:\n  - uid: a\n    url: https://a.example.com\n    audience: bad\n    secretHeader: X\n    secretEnv: SLASHOBSERVE_A\n"), func(string) (string, bool) { return "very-secret-value", true })
	if err == nil || strings.Contains(err.Error(), "very-secret-value") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadReadsTheContextSourcesFile(t *testing.T) {
	path := contextSourcesFixture(t, "contextSources:\n  - uid: a\n    url: https://a.example.com/mcp\n")
	env := llmEnv(nil)
	env[envContextSourcesFile] = path
	cfg, err := Load(envOf(env))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ContextSourcesFile != path || len(cfg.ContextSources) != 1 || cfg.ContextSources[0].UID != "a" {
		t.Fatalf("got %q and %+v", cfg.ContextSourcesFile, cfg.ContextSources)
	}
	env[envContextSourcesFile] = filepath.Join(t.TempDir(), "absent.yaml")
	_, err = Load(envOf(env))
	if err == nil || !strings.Contains(err.Error(), envContextSourcesFile) {
		t.Fatalf("err = %v, want it to name %s", err, envContextSourcesFile)
	}
}

func TestValidContextSourceNameRefusesControlAndFormatCharacters(t *testing.T) {
	for name, want := range map[string]bool{"Docs": true, "": false, "a\nb": false, "a\u200bb": false, strings.Repeat("é", 100): true, strings.Repeat("é", 101): false} {
		if got := ValidContextSourceName(name); got != want {
			t.Errorf("ValidContextSourceName(%q) = %v, want %v", name, got, want)
		}
	}
}
