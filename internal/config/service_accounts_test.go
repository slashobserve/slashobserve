package config

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func newServiceAccountKey(t *testing.T) string {
	t.Helper()
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return "sok_" + base64.RawURLEncoding.EncodeToString(b)
}

func accountsFile(t *testing.T, entries ...string) string {
	t.Helper()
	return writeFile(t, "service-accounts.yaml", "serviceAccounts:\n"+strings.Join(entries, ""))
}

func accountEntry(name, role, description, keyFile string) string {
	return fmt.Sprintf("  - name: %s\n    role: %s\n    description: %q\n    keyFile: %s\n", name, role, description, keyFile)
}

func TestServiceAccountsFileReadsEveryAccount(t *testing.T) {
	loadgenKey, ciKey := newServiceAccountKey(t), newServiceAccountKey(t)
	path := accountsFile(t,
		accountEntry("demo-loadgen", "editor", "Posts the demo's deploy markers", writeFile(t, "loadgen.key", loadgenKey+"\n")),
		accountEntry("ci", "viewer", "", writeFile(t, "ci.key", "  "+ciKey+"\r\n")),
	)

	accounts, err := loadServiceAccounts(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct{ name, role, description, key string }{
		{"demo-loadgen", "editor", "Posts the demo's deploy markers", loadgenKey},
		{"ci", "viewer", "", ciKey},
	}
	if len(accounts) != len(want) {
		t.Fatalf("got %d accounts, want %d", len(accounts), len(want))
	}
	for i, w := range want {
		got := accounts[i]
		if got.Name != w.name || got.Role != w.role || got.Description != w.description || got.Key.Reveal() != w.key {
			t.Errorf("account %d = %s %s %q, want %s %s %q with its key", i, got.Name, got.Role, got.Description, w.name, w.role, w.description)
		}
	}
}

func TestServiceAccountsFileAcceptsAnEmptyList(t *testing.T) {
	accounts, err := loadServiceAccounts(writeFile(t, "service-accounts.yaml", "serviceAccounts: []\n"))
	if err != nil || len(accounts) != 0 {
		t.Fatalf("got %d accounts and %v, want none and no error", len(accounts), err)
	}
}

func TestServiceAccountsFileRefusals(t *testing.T) {
	key := newServiceAccountKey(t)
	cases := []struct {
		name    string
		content func(t *testing.T) string
		want    string
	}{
		{"missing list", func(t *testing.T) string { return "{}\n" }, "serviceAccounts: required"},
		{"unknown field", func(t *testing.T) string {
			return "serviceAccounts:\n  - name: a\n    role: editor\n    keyFile: /k\n    token: x\n"
		}, "invalid service accounts file"},
		{"two documents", func(t *testing.T) string { return "serviceAccounts: []\n---\nserviceAccounts: []\n" }, "multiple YAML documents"},
		{"bad name", func(t *testing.T) string {
			return "serviceAccounts:\n" + accountEntry("Demo_Bot", "editor", "", writeFile(t, "k", key))
		}, "serviceAccounts[0].name: must match"},
		{"commenter role", func(t *testing.T) string {
			return "serviceAccounts:\n" + accountEntry("bot", "commenter", "", writeFile(t, "k", key))
		}, `serviceAccounts[0].role: must be viewer, editor or admin, got "commenter"`},
		{"long description", func(t *testing.T) string {
			return "serviceAccounts:\n" + accountEntry("bot", "editor", strings.Repeat("d", 257), writeFile(t, "k", key))
		}, "serviceAccounts[0].description: must be at most 256 characters"},
		{"control character in description", func(t *testing.T) string {
			return "serviceAccounts:\n" + accountEntry("bot", "editor", "a\tb", writeFile(t, "k", key))
		}, "serviceAccounts[0].description"},
		{"no key file", func(t *testing.T) string { return "serviceAccounts:\n  - name: bot\n    role: editor\n" }, "serviceAccounts[0].keyFile: required"},
		{"missing key file", func(t *testing.T) string {
			return "serviceAccounts:\n" + accountEntry("bot", "editor", "", filepath.Join(t.TempDir(), "absent.key"))
		}, "serviceAccounts[0].keyFile:"},
		{"duplicate name", func(t *testing.T) string {
			return "serviceAccounts:\n" + accountEntry("bot", "editor", "", writeFile(t, "a", key)) +
				accountEntry("bot", "viewer", "", writeFile(t, "b", newServiceAccountKey(t)))
		}, `serviceAccounts[1].name: "bot" is already used by serviceAccounts[0]`},
		{"shared key", func(t *testing.T) string {
			return "serviceAccounts:\n" + accountEntry("bot", "editor", "", writeFile(t, "a", key)) +
				accountEntry("other", "viewer", "", writeFile(t, "b", key))
		}, "serviceAccounts[1].keyFile: holds the same key as serviceAccounts[0]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := loadServiceAccounts(writeFile(t, "service-accounts.yaml", tc.content(t)))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want an error containing %q", err, tc.want)
			}
			if strings.Contains(err.Error(), key) {
				t.Fatalf("the error holds the key: %v", err)
			}
		})
	}
}

func TestServiceAccountKeyFileRefusesMalformedKeysWithoutEchoingThem(t *testing.T) {
	valid := newServiceAccountKey(t)
	cases := map[string]string{
		"empty":           "",
		"wrong prefix":    "sak_" + valid[4:],
		"too short":       valid[:len(valid)-1],
		"too long":        valid + "A",
		"bad character":   valid[:len(valid)-1] + "!",
		"two keys":        valid + "\n" + newServiceAccountKey(t),
		"space in middle": valid[:20] + " " + valid[20:],
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			keyFile := writeFile(t, "bot.key", content)
			_, err := loadServiceAccounts(accountsFile(t, accountEntry("bot", "editor", "", keyFile)))
			if err == nil || !strings.Contains(err.Error(), "does not hold a key") {
				t.Fatalf("got %v, want a refusal of the key file", err)
			}
			if content != "" && strings.Contains(err.Error(), strings.TrimSpace(content)) || strings.Contains(err.Error(), valid[4:]) {
				t.Fatalf("the error holds the file's content: %v", err)
			}
		})
	}
}

func TestLoadReadsTheServiceAccountsFile(t *testing.T) {
	path := accountsFile(t, accountEntry("bot", "editor", "", writeFile(t, "bot.key", newServiceAccountKey(t))))
	env := llmEnv(nil)
	cfg, err := Load(envOf(env))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ServiceAccountsFile != "" || len(cfg.ServiceAccounts) != 0 {
		t.Fatalf("without the variable got %q and %d accounts, want none", cfg.ServiceAccountsFile, len(cfg.ServiceAccounts))
	}

	env[envServiceAccountsFile] = path
	cfg, err = Load(envOf(env))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ServiceAccountsFile != path || len(cfg.ServiceAccounts) != 1 || cfg.ServiceAccounts[0].Name != "bot" {
		t.Fatalf("got %q and %+v, want the file's one account", cfg.ServiceAccountsFile, cfg.ServiceAccounts)
	}

	env[envServiceAccountsFile] = filepath.Join(t.TempDir(), "absent.yaml")
	if _, err := Load(envOf(env)); err == nil || !strings.Contains(err.Error(), envServiceAccountsFile) {
		t.Fatalf("got %v, want an error naming the variable", err)
	}
}
