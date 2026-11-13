package cli

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	manageFolder    = "eeeeeeee-2222-4222-8222-eeeeeeeeeeee"
	manageCopy      = "ffffffff-3333-4333-8333-ffffffffffff"
	dashboardPrefix = "/api/v1/dashboards/" + testDashboard
)

type manageReply struct {
	method string
	path   string
	status int
	body   string
}

type manageFake struct {
	server *httptest.Server
	got    []recordedRequest
}

func newManageFake(t *testing.T, replies ...manageReply) *manageFake {
	t.Helper()
	f := &manageFake{}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.got = append(f.got, recordedRequest{r.Method, r.URL.Path, r.URL.RawQuery, string(body)})
		w.Header().Set("Content-Type", "application/json")
		for _, reply := range replies {
			if reply.method == r.Method && reply.path == r.URL.Path {
				w.WriteHeader(reply.status)
				_, _ = io.WriteString(w, reply.body)
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":"no such route in the test","code":"not_found"}`)
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *manageFake) run(args ...string) (string, string, int) {
	stdout, stderr, code := runCLI(append([]string{"--server", f.server.URL, "--token", "k"}, args...)...)
	return spaces.ReplaceAllString(stdout, " "), stderr, code
}

func (f *manageFake) wantRequests(t *testing.T, want ...recordedRequest) {
	t.Helper()
	if len(f.got) != len(want) {
		t.Fatalf("requests %+v, want %+v", f.got, want)
	}
	for i, w := range want {
		g := f.got[i]
		if g.method != w.method || g.path != w.path || g.query != w.query || !sameJSON(g.body, w.body) {
			t.Fatalf("request %d is %+v, want %+v", i, g, w)
		}
	}
}

func dashboardVersionJSON(id, title string, version int) string {
	return fmt.Sprintf(`{"dashboard":{"id":%q,"title":%q,"tags":[],"latest_version":%d,"created_at":"2026-01-02T03:04:05Z","updated_at":"2026-01-02T03:04:05Z","updated_by":{"name":"Ada"}},"version":{"version":%d,"spec":{"panels":[]}}}`, id, title, version, version)
}

func TestDashboardMove(t *testing.T) {
	f := newManageFake(t, manageReply{http.MethodPatch, dashboardPrefix, 200, dashboardVersionJSON(testDashboard, "Checkout", 3)})
	stdout, stderr, code := f.run("dashboards", "move", testDashboard, "--folder", manageFolder)
	if code != 0 || stderr != "" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	f.wantRequests(t, recordedRequest{http.MethodPatch, dashboardPrefix, "", `{"folder_id":"` + manageFolder + `"}`})
	if want := "Moved Checkout to folder " + manageFolder + "\n"; stdout != want {
		t.Fatalf("stdout %q, want %q", stdout, want)
	}
}

func TestDashboardMoveWithoutAFolderIsAUsageError(t *testing.T) {
	f := newManageFake(t)
	_, stderr, code := f.run("dashboards", "move", testDashboard)
	if code != 2 || !strings.Contains(stderr, "--folder <folder-id> is required") || len(f.got) != 0 {
		t.Fatalf("exit %d, stderr %q, requests %v", code, stderr, f.got)
	}
}

func TestDashboardDuplicate(t *testing.T) {
	copied := dashboardVersionJSON(manageCopy, "Checkout copy", 1)
	cases := []struct {
		name    string
		args    []string
		body    string
		wantErr string
	}{
		{"with a title", []string{"--title", "Checkout copy"}, `{"title":"Checkout copy"}`, ""},
		{"with a folder", []string{"--title", "Checkout copy", "--folder", manageFolder}, `{"title":"Checkout copy","folder_id":"` + manageFolder + `"}`, ""},
		{"without a title", nil, "", "--title <title> is required"},
		{"with a bad folder", []string{"--title", "x", "--folder", "payments"}, "", `"payments" is not a valid id`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newManageFake(t, manageReply{http.MethodPost, dashboardPrefix + "/duplicate", 201, copied})
			stdout, stderr, code := f.run(append([]string{"dashboards", "duplicate", testDashboard}, tc.args...)...)
			if tc.wantErr != "" {
				if code != 2 || !strings.Contains(stderr, tc.wantErr) || len(f.got) != 0 {
					t.Fatalf("exit %d, stderr %q, requests %v", code, stderr, f.got)
				}
				return
			}
			if code != 0 || stderr != "" {
				t.Fatalf("exit %d, stderr %q", code, stderr)
			}
			f.wantRequests(t, recordedRequest{http.MethodPost, dashboardPrefix + "/duplicate", "", tc.body})
			if want := "Duplicated as " + manageCopy + "\n"; stdout != want {
				t.Fatalf("stdout %q, want %q", stdout, want)
			}
		})
	}
}

func TestDashboardRestore(t *testing.T) {
	restored := dashboardVersionJSON(testDashboard, "Checkout", 6)
	restorePath := dashboardPrefix + "/versions/2/restore"
	t.Run("reads the latest version for the parent when none is given", func(t *testing.T) {
		f := newManageFake(t,
			manageReply{http.MethodGet, dashboardPrefix, 200, dashboardVersionJSON(testDashboard, "Checkout", 5)},
			manageReply{http.MethodPost, restorePath, 201, restored})
		stdout, stderr, code := f.run("dashboards", "restore", testDashboard, "--version", "2")
		if code != 0 || stderr != "" {
			t.Fatalf("exit %d, stderr %q", code, stderr)
		}
		f.wantRequests(t,
			recordedRequest{http.MethodGet, dashboardPrefix, "", ""},
			recordedRequest{http.MethodPost, restorePath, "", `{"parent_version":5}`})
		if want := "Restored version 2 as version 6\n"; stdout != want {
			t.Fatalf("stdout %q, want %q", stdout, want)
		}
	})
	t.Run("sends the given parent and summary without reading the dashboard", func(t *testing.T) {
		f := newManageFake(t, manageReply{http.MethodPost, restorePath, 201, restored})
		if _, stderr, code := f.run("dashboards", "restore", testDashboard, "--version", "2", "--parent", "4", "--summary", "back to good"); code != 0 {
			t.Fatalf("exit %d, stderr %q", code, stderr)
		}
		f.wantRequests(t, recordedRequest{http.MethodPost, restorePath, "", `{"parent_version":4,"summary":"back to good"}`})
	})
	t.Run("prints the server's version conflict", func(t *testing.T) {
		f := newManageFake(t, manageReply{http.MethodPost, restorePath, 409, `{"error":"the dashboard changed since version 4","code":"version_conflict","latest_version":5}`})
		stdout, stderr, code := f.run("dashboards", "restore", testDashboard, "--version", "2", "--parent", "4")
		if code != 1 || stdout != "" || !strings.Contains(stderr, "the dashboard changed since version 4 (version_conflict)") {
			t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
		}
	})
	for name, args := range map[string][]string{
		"without a version":    {"dashboards", "restore", testDashboard},
		"with a zero parent":   {"dashboards", "restore", testDashboard, "--version", "2", "--parent", "0"},
		"with a bad dashboard": {"dashboards", "restore", "checkout", "--version", "2"},
	} {
		t.Run(name+" is a usage error", func(t *testing.T) {
			f := newManageFake(t)
			if _, _, code := f.run(args...); code != 2 || len(f.got) != 0 {
				t.Fatalf("exit %d, requests %v", code, f.got)
			}
		})
	}
}

func TestDashboardDiff(t *testing.T) {
	changes := `{"from":2,"to":3,"changes":[{"op":"replace","path":"/panels/0/title","from":"Errors","to":"Error rate"},{"op":"add","path":"/panels/1","to":{"id":"p2","type":"stat"}},{"op":"remove","path":"/description","from":"` + strings.Repeat("long ", 30) + `"}]}`
	diffPath := dashboardPrefix + "/diff"
	t.Run("prints one row per change with clipped values", func(t *testing.T) {
		f := newManageFake(t, manageReply{http.MethodGet, diffPath, 200, changes})
		stdout, stderr, code := f.run("dashboards", "diff", testDashboard)
		if code != 0 || stderr != "" {
			t.Fatalf("exit %d, stderr %q", code, stderr)
		}
		f.wantRequests(t, recordedRequest{http.MethodGet, diffPath, "", ""})
		for _, want := range []string{
			"OP PATH FROM TO",
			`replace /panels/0/title "Errors" "Error rate"`,
			`add /panels/1 - {"id":"p2","type":"stat"}`,
			"remove /description \"long long",
			"…",
		} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("stdout lacks %q:\n%s", want, stdout)
			}
		}
	})
	t.Run("sends only the flags that were set", func(t *testing.T) {
		f := newManageFake(t, manageReply{http.MethodGet, diffPath, 200, changes})
		if _, _, code := f.run("dashboards", "diff", testDashboard, "--from", "0", "--to", "3"); code != 0 {
			t.Fatalf("exit %d", code)
		}
		f.wantRequests(t, recordedRequest{http.MethodGet, diffPath, "from=0&to=3", ""})
	})
	t.Run("says when nothing changed", func(t *testing.T) {
		f := newManageFake(t, manageReply{http.MethodGet, diffPath, 200, `{"from":2,"to":3,"changes":[]}`})
		stdout, _, code := f.run("dashboards", "diff", testDashboard)
		if want := "No changes between version 2 and 3\n"; code != 0 || stdout != want {
			t.Fatalf("exit %d, stdout %q, want %q", code, stdout, want)
		}
	})
	t.Run("prints the response body with --json", func(t *testing.T) {
		f := newManageFake(t, manageReply{http.MethodGet, diffPath, 200, changes})
		stdout, _, code := f.run("--json", "dashboards", "diff", testDashboard)
		if code != 0 || !sameJSON(stdout, changes) {
			t.Fatalf("exit %d, stdout %q", code, stdout)
		}
	})
	t.Run("refuses a to below one", func(t *testing.T) {
		f := newManageFake(t)
		if _, _, code := f.run("dashboards", "diff", testDashboard, "--to", "0"); code != 2 || len(f.got) != 0 {
			t.Fatalf("exit %d, requests %v", code, f.got)
		}
	})
}
