package cli

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEditBlockSendsTheChangesWithTheVersion(t *testing.T) {
	panelFile := filepath.Join(t.TempDir(), "panel.json")
	if err := os.WriteFile(panelFile, []byte(`{"type":"timeseries"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	reply := `{"block":{"id":"` + testBlock + `","kind":"text"}}`
	cases := []struct {
		name string
		args []string
		body string
	}{
		{"text and width", []string{"--text", "new words", "--half"}, `{"body":{"markdown":"new words"},"version":3,"width":"half"}`},
		{"position only", []string{"--position", "0", "--full"}, `{"position":0,"version":3,"width":"full"}`},
		{"panel", []string{"--panel-file", panelFile, "--from", "2026-01-01T00:00:00Z", "--to", "2026-01-02T00:00:00Z"}, `{"body":{"from":"2026-01-01T00:00:00Z","panel":{"type":"timeseries"},"to":"2026-01-02T00:00:00Z"},"version":3}`},
	}
	for _, tc := range cases {
		server, seen := recordingServer(t, reply)
		args := append([]string{"o", "edit-block", testBlock, "--version", "3"}, tc.args...)
		stdout, stderr, code := runWith(server, args...)
		if code != 0 || seen.method != http.MethodPatch || seen.path != "/api/v1/blocks/"+testBlock || !sameJSON(seen.body, tc.body) {
			t.Fatalf("%s: exit %d, call %+v, stderr %q", tc.name, code, seen, stderr)
		}
		if !strings.Contains(stdout, "Changed text block "+testBlock) {
			t.Fatalf("%s: stdout %q", tc.name, stdout)
		}
	}
}

func TestEditBlockRefusesAnEmptyOrUnversionedChange(t *testing.T) {
	for _, args := range [][]string{
		{"o", "edit-block", testBlock, "--text", "x"},
		{"o", "edit-block", testBlock, "--version", "2"},
		{"o", "edit-block", testBlock, "--version", "2", "--text", "x", "--panel-file", "p.json"},
		{"o", "edit-block", testBlock, "--version", "2", "--half", "--full"},
		{"o", "edit-block", testBlock, "--version", "2", "--from", "2026-01-01T00:00:00Z", "--text", "x"},
	} {
		_, _, stderr, code := runAgainst(t, http.StatusOK, `{}`, args...)
		if code != 2 || stderr == "" {
			t.Fatalf("%v: exit %d, stderr %q", args, code, stderr)
		}
	}
}

func TestRemoveBlockDeletesWithTheVersion(t *testing.T) {
	server, seen := recordingServer(t, ``)
	stdout, stderr, code := runWith(server, "o", "remove-block", testBlock, "--version", "4")
	if code != 0 || seen.method != http.MethodDelete || seen.path != "/api/v1/blocks/"+testBlock || seen.query != "version=4" {
		t.Fatalf("exit %d, call %+v, stderr %q", code, seen, stderr)
	}
	if !strings.Contains(stdout, "Removed block "+testBlock) {
		t.Fatalf("stdout %q", stdout)
	}
	if _, _, stderr, code := runAgainst(t, http.StatusNoContent, ``, "o", "remove-block", testBlock); code != 2 || !strings.Contains(stderr, "--version") {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
}
