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
		{"width and height", []string{"--width", "two_thirds", "--height", "360"}, `{"height":360,"version":3,"width":"two_thirds"}`},
		{"automatic height", []string{"--height", "0"}, `{"height":null,"version":3}`},
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
		{"o", "edit-block", testBlock, "--version", "2", "--half", "--width", "third"},
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

func TestArrangeSendsTheOrderWithWidthsAndHeights(t *testing.T) {
	second := "22222222-2222-4222-8222-222222222222"
	third := "33333333-3333-4333-8333-333333333333"
	server, seen := recordingServer(t, `{"blocks":[{"id":"`+testBlock+`"},{"id":"`+second+`"},{"id":"`+third+`"}]}`)
	stdout, stderr, code := runWith(server, "o", "arrange", testObservation, second+":third:auto", testBlock+"::300", third)
	want := `{"blocks":[{"id":"` + second + `","width":"third","height":null},{"id":"` + testBlock + `","height":300},{"id":"` + third + `"}]}`
	if code != 0 || seen.method != http.MethodPut || seen.path != "/api/v1/observations/"+testObservation+"/layout" || !sameJSON(seen.body, want) {
		t.Fatalf("exit %d, call %+v, stderr %q", code, seen, stderr)
	}
	if !strings.Contains(stdout, "Arranged 3 blocks") {
		t.Fatalf("stdout %q", stdout)
	}
}

func TestArrangeCountsTheBlocksItPlaced(t *testing.T) {
	second := "22222222-2222-4222-8222-222222222222"
	server, _ := recordingServer(t, `{"blocks":[{"id":"`+testBlock+`"},{"id":"`+second+`"}]}`)
	stdout, stderr, code := runWith(server, "o", "arrange", testObservation, second)
	if code != 0 || !strings.Contains(stdout, "Arranged 1 block\n") {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestArrangeRefusesWhatIsNotALayout(t *testing.T) {
	for _, args := range [][]string{
		{"o", "arrange", testObservation},
		{"o", "arrange", testObservation, "nope"},
		{"o", "arrange", testObservation, testBlock + ":half:tall"},
		{"o", "arrange", testObservation, testBlock + ":half:300:x"},
	} {
		_, _, stderr, code := runAgainst(t, http.StatusOK, `{}`, args...)
		if code != 2 || stderr == "" {
			t.Fatalf("%v: exit %d, stderr %q", args, code, stderr)
		}
	}
}
