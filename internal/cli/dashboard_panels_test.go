package cli

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDashboardPanels(t *testing.T) {
	saved := dashboardVersionJSON(testDashboard, "Checkout", 6)
	panelsPath := dashboardPrefix + "/panels"
	t.Run("removes panels on top of the latest version", func(t *testing.T) {
		f := newManageFake(t,
			manageReply{http.MethodGet, dashboardPrefix, 200, dashboardVersionJSON(testDashboard, "Checkout", 5)},
			manageReply{http.MethodPost, panelsPath, 201, saved})
		stdout, stderr, code := f.run("dashboards", "panels", testDashboard, "--remove", "p1", "--remove", "p2", "--summary", "Drop two panels")
		if code != 0 || stderr != "" {
			t.Fatalf("exit %d, stderr %q", code, stderr)
		}
		f.wantRequests(t,
			recordedRequest{http.MethodGet, dashboardPrefix, "", ""},
			recordedRequest{http.MethodPost, panelsPath, "", `{"delete":["p1","p2"],"summary":"Drop two panels","parent_version":5}`})
		if want := "Saved version 6 of Checkout\n"; stdout != want {
			t.Fatalf("stdout %q, want %q", stdout, want)
		}
	})
	t.Run("prints the warning of a save to a dashboard that a file writes", func(t *testing.T) {
		warned := strings.TrimSuffix(saved, "}") + `,"warning":"A change to its file replaces this edit."}`
		f := newManageFake(t, manageReply{http.MethodPost, panelsPath, 201, warned})
		stdout, stderr, code := f.run("dashboards", "panels", testDashboard, "--remove", "p1", "--parent", "5", "--summary", "Drop a panel")
		if code != 0 || stdout != "Saved version 6 of Checkout\n" || stderr != "A change to its file replaces this edit.\n" {
			t.Fatalf("exit %d, stdout %q, stderr %q, want the save line and the warning on stderr", code, stdout, stderr)
		}
	})
	t.Run("sends the patch file with the given parent", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "patch.json")
		patch := `{"upsert":[{"id":"p1_copy","type":"text","title":"Notes copy"}],"layout":[{"id":"p1_copy","gridPos":{"x":0,"y":8,"w":12,"h":4}}]}`
		if err := os.WriteFile(file, []byte(patch), 0o600); err != nil {
			t.Fatal(err)
		}
		f := newManageFake(t, manageReply{http.MethodPost, panelsPath, 201, saved})
		if _, stderr, code := f.run("dashboards", "panels", testDashboard, "--file", file, "--parent", "4", "--summary", "Copy notes"); code != 0 {
			t.Fatalf("exit %d, stderr %q", code, stderr)
		}
		f.wantRequests(t, recordedRequest{http.MethodPost, panelsPath, "", `{"upsert":[{"id":"p1_copy","type":"text","title":"Notes copy"}],"layout":[{"id":"p1_copy","gridPos":{"x":0,"y":8,"w":12,"h":4}}],"summary":"Copy notes","parent_version":4}`})
	})
	t.Run("refuses a patch file with an unknown field", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "patch.json")
		if err := os.WriteFile(file, []byte(`{"remove":["p1"]}`), 0o600); err != nil {
			t.Fatal(err)
		}
		f := newManageFake(t)
		_, stderr, code := f.run("dashboards", "panels", testDashboard, "--file", file, "--summary", "x")
		if code != 2 || !strings.Contains(stderr, `unknown field "remove"`) || len(f.got) != 0 {
			t.Fatalf("exit %d, stderr %q, requests %+v", code, stderr, f.got)
		}
	})
	t.Run("asks for a change and a summary before calling the server", func(t *testing.T) {
		f := newManageFake(t)
		for _, tc := range []struct {
			args []string
			want string
		}{
			{[]string{"--summary", "x"}, "nothing to change"},
			{[]string{"--remove", "p1"}, "--summary <text> is required"},
		} {
			_, stderr, code := f.run(append([]string{"dashboards", "panels", testDashboard}, tc.args...)...)
			if code != 2 || !strings.Contains(stderr, tc.want) {
				t.Fatalf("args %v: exit %d, stderr %q, want %q", tc.args, code, stderr, tc.want)
			}
		}
		if len(f.got) != 0 {
			t.Fatalf("requests %+v, want none", f.got)
		}
	})
	t.Run("prints the server's invalid spec issues", func(t *testing.T) {
		f := newManageFake(t, manageReply{http.MethodPost, panelsPath, 422, `{"error":"patch is invalid","code":"invalid_spec","issues":[{"path":"/panels/3","code":"schema","message":"no panel has id p9"}]}`})
		_, stderr, code := f.run("dashboards", "panels", testDashboard, "--remove", "p9", "--parent", "5", "--summary", "x")
		if code != 1 || !strings.Contains(stderr, "invalid_spec") {
			t.Fatalf("exit %d, stderr %q", code, stderr)
		}
	})
}
