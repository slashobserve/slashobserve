package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type stubReply struct {
	status int
	body   string
}

func runRoutes(t *testing.T, routes map[string]stubReply, args ...string) (string, string, int) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reply, found := routes[r.Method+" "+r.URL.Path]
		if !found {
			reply = stubReply{status: http.StatusNotFound, body: `{"error":"no route","code":"not_found"}`}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(reply.status)
		_, _ = fmt.Fprint(w, reply.body)
	}))
	t.Cleanup(server.Close)
	var stdout, stderr bytes.Buffer
	full := append([]string{"--server", server.URL, "--token", "k"}, args...)
	code := Main(full, IO{In: strings.NewReader(""), Out: &stdout, Err: &stderr}, "test")
	return stdout.String(), stderr.String(), code
}

func TestAnAddressThatIsNotAnAPIIsNamedAsSuch(t *testing.T) {
	_, stderr, code := runRoutes(t, map[string]stubReply{"GET /api/v1/dashboards": {200, "<!doctype html><html></html>"}}, "dashboards", "list")
	if code != 1 || !strings.Contains(stderr, "probably not a SlashObserve server") || strings.Contains(stderr, "versions match") {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
}

func TestAnInvalidPanelNamesEveryFailingPath(t *testing.T) {
	body := `{"error":"the panel is invalid","code":"invalid_panel","issues":[{"path":"/panel/type","code":"unknown_type","message":"unknown panel type \"nope\""},{"path":"/panel/queries","message":"a data panel needs a query"}]}`
	_, stderr, code := runRoutes(t, map[string]stubReply{"GET /api/v1/dashboards": {422, body}}, "dashboards", "list")
	for _, want := range []string{"the panel is invalid (invalid_panel)", "/panel/type: unknown panel type", "/panel/queries: a data panel needs a query"} {
		if code != 1 || !strings.Contains(stderr, want) {
			t.Fatalf("exit %d, stderr %q lacks %q", code, stderr, want)
		}
	}
}

func TestIssueLinesStopAfterTen(t *testing.T) {
	issues := make([]string, 13)
	for i := range issues {
		issues[i] = fmt.Sprintf(`{"path":"/p/%d","message":"bad"}`, i)
	}
	body := `{"error":"invalid","code":"invalid_spec","issues":[` + strings.Join(issues, ",") + `]}`
	_, stderr, _ := runRoutes(t, map[string]stubReply{"GET /api/v1/dashboards": {422, body}}, "dashboards", "list")
	if !strings.Contains(stderr, "/p/9: bad") || strings.Contains(stderr, "/p/10") || !strings.Contains(stderr, "and 3 more") {
		t.Fatalf("stderr %q", stderr)
	}
}

func TestErrorHintsNameWhatToChange(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		args   []string
		want   string
	}{
		{"allowlist", 403, `{"error":"hooks.example.com is not on the outbound allowlist.","code":"outbound_domain_not_allowed"}`, []string{"webhooks", "add", "--name", "n", "--url", "https://hooks.example.com/x", "--event", "run.failed"}, "Settings, Outbound access"},
		{"cursor", 400, `{"error":"cursor is not valid","code":"invalid_request"}`, []string{"dashboards", "list", "--cursor", "zzz"}, "add --all"},
		{"thread exists", 409, `{"error":"this observation already has a discussion","code":"observation_thread_exists","thread_id":"` + stoppedID + `"}`, []string{"comments", "add", "--on", "observation:" + stoppedID, "hello"}, "observe comments reply " + stoppedID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, stderr, code := runAgainst(t, tc.status, tc.body, tc.args...)
			if code != 1 || !strings.Contains(stderr, tc.want) {
				t.Fatalf("exit %d, stderr %q lacks %q", code, stderr, tc.want)
			}
		})
	}
}

func TestAMissingVersionIsNotReportedAsAMissingDashboard(t *testing.T) {
	routes := map[string]stubReply{
		"GET /api/v1/dashboards/" + testDashboard + "/versions/9": {404, `{"error":"dashboard not found","code":"not_found"}`},
		"GET /api/v1/dashboards/" + testDashboard:                 {200, `{"dashboard":{"id":"` + testDashboard + `","title":"Shop","latest_version":3},"version":{"version":3,"spec":{"panels":[]}}}`},
	}
	_, stderr, code := runRoutes(t, routes, "dashboards", "show", testDashboard, "--version", "9")
	if code != 1 || !strings.Contains(stderr, "has no version 9; its latest version is 3") || !strings.Contains(stderr, "version_not_found") {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	_, stderr, code = runRoutes(t, map[string]stubReply{}, "dashboards", "show", testDashboard, "--version", "2")
	if code != 1 || !strings.Contains(stderr, "no route") {
		t.Fatalf("a missing dashboard changed its message: exit %d, stderr %q", code, stderr)
	}
}

func TestWithJSONAnErrorIsAnObjectWithAStableCode(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		args   []string
		code   int
		want   string
	}{
		{"api code", 404, `{"error":"dashboard not found","code":"not_found"}`, []string{"dashboards", "show", testDashboard}, 1, "not_found"},
		{"no code", 502, `bad gateway`, []string{"dashboards", "list"}, 1, "http_502"},
		{"unauthenticated", 401, `{}`, []string{"dashboards", "list"}, 1, "unauthenticated"},
		{"usage", 200, `{}`, []string{"dashboards", "show", "nope"}, 2, "usage"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, stderr, code := runAgainst(t, tc.status, tc.body, append([]string{"--json"}, tc.args...)...)
			var got struct{ Error, Code string }
			if err := json.Unmarshal([]byte(stderr), &got); err != nil {
				t.Fatalf("stderr %q is not JSON: %v", stderr, err)
			}
			if code != tc.code || got.Code != tc.want || got.Error == "" {
				t.Fatalf("exit %d, got %+v, want code %q", code, got, tc.want)
			}
		})
	}
}

func TestWithoutJSONAnErrorStaysPlainText(t *testing.T) {
	_, _, stderr, _ := runAgainst(t, 404, `{"error":"dashboard not found","code":"not_found"}`, "dashboards", "show", testDashboard)
	if strings.HasPrefix(stderr, "{") {
		t.Fatalf("stderr %q", stderr)
	}
}
