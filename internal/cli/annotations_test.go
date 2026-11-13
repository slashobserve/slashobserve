package cli

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

const testAnnotation = "12121212-3434-4343-8787-565656565656"

func annotationJSON(id string, dashboard *string, at, body string, tags ...string) string {
	dash := "null"
	if dashboard != nil {
		dash = fmt.Sprintf("%q", *dashboard)
	}
	return fmt.Sprintf(`{"id":%q,"dashboard_id":%s,"panel_id":null,"time_from":%q,"time_to":null,"text":%q,"tags":[%s],"created_by":null,"created_by_name":null,"actor":null,"created_at":%q}`,
		id, dash, at, body, quoteAll(tags), at)
}

func quoteAll(items []string) string {
	quoted := make([]string, len(items))
	for i, item := range items {
		quoted[i] = fmt.Sprintf("%q", item)
	}
	return strings.Join(quoted, ",")
}

func TestAnnotationsList(t *testing.T) {
	dashboard := testDashboard
	listed := `{"annotations":[` +
		annotationJSON(testAnnotation, &dashboard, "2026-01-02T03:04:05Z", "Deploy checkout v1.4.2", "deploy", "checkout") + "," +
		annotationJSON(manageCopy, nil, "2026-01-01T00:00:00Z", strings.Repeat("long ", 30)) + `],"next_cursor":"page2"}`
	path := "/api/v1/annotations"

	t.Run("prints one row per annotation and the next cursor", func(t *testing.T) {
		f := newManageFake(t, manageReply{http.MethodGet, path, 200, listed})
		stdout, stderr, code := f.run("annotations", "list")
		if code != 0 {
			t.Fatalf("exit %d, stderr %q", code, stderr)
		}
		f.wantRequests(t, recordedRequest{http.MethodGet, path, "limit=50", ""})
		for _, want := range []string{
			"ID TIME DASHBOARD TAGS TEXT",
			testAnnotation + " 2026-01-02T03:04Z " + testDashboard + " deploy,checkout Deploy checkout v1.4.2",
			manageCopy + " 2026-01-01T00:00Z - - long long",
			"…",
		} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("stdout lacks %q:\n%s", want, stdout)
			}
		}
		if !strings.Contains(stderr, "more results: --cursor page2") {
			t.Fatalf("stderr %q lacks the cursor hint", stderr)
		}
	})
	t.Run("sends the filters, with the tags joined by commas", func(t *testing.T) {
		f := newManageFake(t, manageReply{http.MethodGet, path, 200, listed})
		_, _, code := f.run("annotations", "list", "--dashboard", testDashboard, "--from", "2026-01-01T00:00:00Z", "--to", "2026-01-02T00:00:00Z", "--tag", "deploy", "--tag", "checkout", "--limit", "5", "--cursor", "page1")
		if code != 0 {
			t.Fatalf("exit %d", code)
		}
		f.wantRequests(t, recordedRequest{http.MethodGet, path, "cursor=page1&dashboard_id=" + testDashboard + "&from=2026-01-01T00%3A00%3A00Z&limit=5&tags=deploy%2Ccheckout&to=2026-01-02T00%3A00%3A00Z", ""})
	})
	t.Run("prints the body with --json", func(t *testing.T) {
		f := newManageFake(t, manageReply{http.MethodGet, path, 200, listed})
		stdout, _, code := f.run("--json", "annotations", "list")
		if code != 0 || !sameJSON(stdout, listed) {
			t.Fatalf("exit %d, stdout %q", code, stdout)
		}
	})
	t.Run("refuses a dashboard that is not an id", func(t *testing.T) {
		f := newManageFake(t)
		if _, stderr, code := f.run("annotations", "list", "--dashboard", "checkout"); code != 2 || !strings.Contains(stderr, `"checkout" is not a valid id`) || len(f.got) != 0 {
			t.Fatalf("exit %d, stderr %q, requests %v", code, stderr, f.got)
		}
	})
}

func TestAnnotationsDelete(t *testing.T) {
	path := "/api/v1/annotations/" + testAnnotation
	f := newManageFake(t, manageReply{http.MethodDelete, path, 204, ""})
	stdout, stderr, code := f.run("annotations", "delete", testAnnotation)
	if code != 0 || stderr != "" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	f.wantRequests(t, recordedRequest{http.MethodDelete, path, "", ""})
	if want := "Deleted annotation " + testAnnotation + "\n"; stdout != want {
		t.Fatalf("stdout %q, want %q", stdout, want)
	}
	f = newManageFake(t)
	if _, _, code := f.run("annotations", "delete", "deploy"); code != 2 || len(f.got) != 0 {
		t.Fatalf("exit %d, requests %v", code, f.got)
	}
}

func TestFoldersAndAnnotationsAreReservedPromptWords(t *testing.T) {
	stdout, _, code := runCLI("--help")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	var note string
	for line := range strings.SplitSeq(stdout, "\n") {
		if strings.Contains(line, "must be quoted") {
			note = line
		}
	}
	for _, word := range []string{" annotations,", " folders,"} {
		if !strings.Contains(note, word) {
			t.Fatalf("the quoted-command note %q lacks %q", note, word)
		}
	}
}
