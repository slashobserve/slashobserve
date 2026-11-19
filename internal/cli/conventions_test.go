package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestMissingArgumentsExitTwo(t *testing.T) {
	for _, args := range [][]string{
		{"search"},
		{"comments", "list", testDashboard, testDashboard},
		{"o", "edit", testDashboard, "a", "new", "title"},
		{"o", "resolve", testDashboard, "all", "good"},
	} {
		call, _, stderr, code := runAgainst(t, 200, `{}`, args...)
		if code != 2 || call.method != "" || stderr == "" {
			t.Errorf("%v: exit %d, stderr %q, call %+v", args, code, stderr, call)
		}
	}
	_, _, stderr, _ := runAgainst(t, 200, `{}`, "o", "edit", testDashboard, "a", "new", "title")
	if !strings.Contains(stderr, "goes in quotes after --title") {
		t.Fatalf("stderr %q", stderr)
	}
}

func TestCommentsListTakesATargetWrittenAsKindAndID(t *testing.T) {
	call, _, stderr, code := runAgainst(t, 200, `{"threads":[],"messages":[],"next_cursor":null}`, "comments", "list", "dashboard:"+testDashboard)
	if code != 0 || call.path != "/api/v1/threads" {
		t.Fatalf("exit %d, stderr %q, call %+v", code, stderr, call)
	}
}

func TestApplyOfAnEmptyOrUnreadableDirectory(t *testing.T) {
	empty := t.TempDir()
	call, _, stderr, code := runAgainst(t, 200, `{}`, "dashboards", "apply", "--dir", empty)
	if code != 0 || call.method != "" || !strings.Contains(stderr, "No .json file found under") {
		t.Fatalf("empty: exit %d, stderr %q, call %+v", code, stderr, call)
	}
	_, _, stderr, code = runAgainst(t, 200, `{}`, "dashboards", "apply", "--dir", filepath.Join(empty, "missing"))
	if code != 2 || !strings.Contains(stderr, "cannot read") {
		t.Fatalf("missing: exit %d, stderr %q", code, stderr)
	}
}
