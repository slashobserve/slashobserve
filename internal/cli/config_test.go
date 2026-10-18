package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSavedKeyApplies(t *testing.T) {
	cases := []struct {
		name   string
		server string
		saved  string
		ok     bool
	}{
		{"identical", "https://observe.example.com", "https://observe.example.com", true},
		{"trailing slash", "https://observe.example.com/", "https://observe.example.com", true},
		{"saved trailing slash", "https://observe.example.com", "https://observe.example.com/", true},
		{"host case", "https://Observe.Example.com", "https://observe.example.com", true},
		{"scheme case", "HTTPS://observe.example.com", "https://observe.example.com", true},
		{"default https port", "https://observe.example.com:443", "https://observe.example.com", true},
		{"default https port saved", "https://observe.example.com", "https://observe.example.com:443/", true},
		{"default http port", "http://localhost:80", "http://localhost", true},
		{"other port", "https://observe.example.com:8443", "https://observe.example.com", false},
		{"http against https", "http://localhost", "https://localhost", false},
		{"ipv6 loopback", "http://[::1]:8080", "http://[::1]:8080", true},
		{"ipv6 default port", "http://[::1]:80", "http://[::1]", true},
		{"ipv6 other port", "http://[::1]:8081", "http://[::1]:8080", false},
		{"loopback names differ", "http://127.0.0.1:8080", "http://localhost:8080", false},
		{"other host", "https://evil.example.com", "https://observe.example.com", false},
		{"suffix host", "https://observe.example.com.evil.example", "https://observe.example.com", false},
		{"path prefix", "https://example.com/observe", "https://example.com", false},
		{"same path prefix", "https://example.com/observe/", "https://example.com/observe", true},
		{"dot path", "https://example.com/.", "https://example.com", true},
		{"dot dot path", "https://example.com/observe/..", "https://example.com", true},
		{"other path", "https://example.com/a", "https://example.com/b", false},
		{"empty saved server", "https://observe.example.com", "", false},
		{"invalid saved server", "https://observe.example.com", "not a url", false},
		{"plain http saved for remote host", "https://observe.example.com", "http://observe.example.com", false},
		{"empty server", "", "https://observe.example.com", false},
		{"invalid server", "ftp://observe.example.com", "https://observe.example.com", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := savedKeyApplies(tc.server, tc.saved)
			if tc.ok && err != nil {
				t.Fatalf("saved key refused: %v", err)
			}
			if !tc.ok && err == nil {
				t.Fatal("saved key accepted for a different server")
			}
		})
	}
}

func TestSavedKeyMismatchMessage(t *testing.T) {
	err := savedKeyApplies("https://b.example.com", "https://a.example.com")
	want := "the saved key belongs to https://a.example.com; pass --token or run observe login --server https://b.example.com"
	if err == nil || err.Error() != want {
		t.Fatalf("got %v, want %q", err, want)
	}
}

func configFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "slashobserve", "cli.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func readFields(t *testing.T, path string) map[string]json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // a test file under t.TempDir
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("the saved config is not JSON: %v\n%s", err, data)
	}
	return fields
}

func TestConfigKeepsUnknownFields(t *testing.T) {
	path := configFile(t, `{"server":"https://a.example.com","token":"k1","future":{"nested":[1,{"x":null}],"flag":true},"Other":"v"}`)
	cfg, err := loadFileConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server != "https://a.example.com" || cfg.Token != "k1" {
		t.Fatalf("known fields not read: %+v", cfg)
	}
	cfg.Server, cfg.Token = "https://b.example.com", "k2"
	if err := saveFileConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	fields := readFields(t, path)
	var future struct {
		Nested []json.RawMessage `json:"nested"`
		Flag   bool              `json:"flag"`
	}
	if err := json.Unmarshal(fields["future"], &future); err != nil || len(future.Nested) != 2 || !future.Flag {
		t.Fatalf("nested unknown field not preserved: %s (%v)", fields["future"], err)
	}
	if string(fields["Other"]) != `"v"` || string(fields["server"]) != `"https://b.example.com"` || string(fields["token"]) != `"k2"` {
		t.Fatalf("unexpected fields: %v", fields)
	}
}

func TestConfigNullAndMissingFile(t *testing.T) {
	cfg, err := loadFileConfig(filepath.Join(t.TempDir(), "absent.json"))
	if err != nil || cfg.Server != "" || cfg.Token != "" {
		t.Fatalf("missing file: %+v, %v", cfg, err)
	}
	cfg, err = loadFileConfig(configFile(t, "null\n"))
	if err != nil || cfg.Server != "" || cfg.Token != "" {
		t.Fatalf("null file: %+v, %v", cfg, err)
	}
}

func TestConfigCorruptFiles(t *testing.T) {
	cases := []struct{ name, content string }{
		{"empty", ""},
		{"truncated", `{"server": `},
		{"array", `[1]`},
		{"string", `"x"`},
		{"server type", `{"server": 5}`},
		{"key type", `{"token": {"a": 1}}`},
		{"case variant", `{"Token": 7}`},
		{"trailing data", `{"server": "https://a.example.com"} x`},
		{"two objects", `{} {}`},
	}
	for _, tc := range cases {
		content := tc.content
		t.Run(tc.name, func(t *testing.T) {
			path := configFile(t, content)
			_, err := loadFileConfig(path)
			if err == nil || !strings.Contains(err.Error(), "is not a valid CLI config") {
				t.Fatalf("got %v, want a corrupt config error", err)
			}
			data, readErr := os.ReadFile(path) //nolint:gosec // a test file under t.TempDir
			if readErr != nil || string(data) != content {
				t.Fatalf("the corrupt file was modified: %q (%v)", data, readErr)
			}
		})
	}
}

func TestConfigKeysMatchCaseInsensitively(t *testing.T) {
	path := configFile(t, `{"SERVER":"https://a.example.com","Token":"old","token":"new","TOKEN":"newest","Keep":1}`)
	cfg, err := loadFileConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server != "https://a.example.com" || cfg.Token != "newest" {
		t.Fatalf("case variants not read with the last one winning: %+v", cfg)
	}
	cfg.Token = ""
	if err := saveFileConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	fields := readFields(t, path)
	for name := range fields {
		if strings.EqualFold(name, "token") {
			t.Fatalf("logout left %q behind: %v", name, fields)
		}
	}
	if string(fields["server"]) != `"https://a.example.com"` || string(fields["Keep"]) != "1" || len(fields) != 2 {
		t.Fatalf("unexpected fields after logout: %v", fields)
	}
}

func TestConfigNullClearsAKnownField(t *testing.T) {
	cases := []struct {
		name, content, server, token string
	}{
		{"later null", `{"token":"a","Token":null}`, "", ""},
		{"earlier null", `{"Token":null,"token":"a"}`, "", "a"},
		{"null server", `{"server":"https://a.example.com","SERVER":null,"token":"a"}`, "", "a"},
		{"only null", `{"token":null}`, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := loadFileConfig(configFile(t, tc.content))
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Server != tc.server || cfg.Token != tc.token {
				t.Fatalf("got %+v, want server %q and key %q", cfg, tc.server, tc.token)
			}
		})
	}
}

func TestConfigEmptyTokenIsRemoved(t *testing.T) {
	path := configFile(t, `{"server":"https://a.example.com","token":"k"}`)
	cfg, err := loadFileConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Token = ""
	if err := saveFileConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	fields := readFields(t, path)
	if _, has := fields["token"]; has || string(fields["server"]) != `"https://a.example.com"` {
		t.Fatalf("unexpected fields: %v", fields)
	}
}

func TestSaveTightensDirectoryAndFileModes(t *testing.T) {
	path := configFile(t, "{}")
	if err := os.Chmod(filepath.Dir(path), 0o755); err != nil { //nolint:gosec // the test widens the mode on purpose
		t.Fatal(err)
	}
	if err := saveFileConfig(path, fileConfig{Server: "https://a.example.com", Token: "k"}); err != nil {
		t.Fatal(err)
	}
	for target, want := range map[string]os.FileMode{filepath.Dir(path): 0o700, path: 0o600} {
		info, err := os.Stat(target)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != want {
			t.Fatalf("%s has mode %o, want %o", target, info.Mode().Perm(), want)
		}
	}
}

func TestNormalizeServerDropsDefaultPorts(t *testing.T) {
	cases := map[string]string{
		"https://h.example.com:443":  "https://h.example.com",
		"https://h.example.com:8443": "https://h.example.com:8443",
		"http://localhost:80/":       "http://localhost",
		"http://[::1]:80":            "http://[::1]",
		"http://[::1]:8080/a/./b/":   "http://[::1]:8080/a/b",
	}
	for raw, want := range cases {
		u, err := normalizeServer(raw)
		if err != nil || u.String() != want {
			t.Fatalf("%s: got %v, %v, want %s", raw, u, err, want)
		}
	}
}
