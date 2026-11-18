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

func dashboardPageServer(t *testing.T, pages map[string]string, seen *[]string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cursor := r.URL.Query().Get("cursor")
		*seen = append(*seen, cursor)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, pages[cursor])
	}))
	t.Cleanup(server.Close)
	return server
}

func dashboardRow(id, title string) string {
	return `{"id":"` + id + `","title":"` + title + `","latest_version":1,"updated_at":"2026-01-01T00:00:00Z","updated_by":{"name":"Ada"}}`
}

func runWith(server *httptest.Server, args ...string) (string, string, int) {
	var stdout, stderr bytes.Buffer
	full := append([]string{"--server", server.URL, "--token", "k"}, args...)
	code := Main(full, IO{In: strings.NewReader(""), Out: &stdout, Err: &stderr}, "test")
	return stdout.String(), stderr.String(), code
}

func TestAllReadsEveryPageAndKeepsTheTotal(t *testing.T) {
	var seen []string
	server := dashboardPageServer(t, map[string]string{
		"":   `{"dashboards":[` + dashboardRow(testDashboard, "First") + `],"next_cursor":"p2","total_count":3}`,
		"p2": `{"dashboards":[` + dashboardRow(testAnnotation, "Second") + `],"next_cursor":"p3","total_count":3}`,
		"p3": `{"dashboards":[` + dashboardRow(stoppedID, "Third") + `],"next_cursor":null,"total_count":3}`,
	}, &seen)

	stdout, stderr, code := runWith(server, "dashboards", "list", "--all")
	if code != 0 || !strings.Contains(stdout, "First") || !strings.Contains(stdout, "Second") || !strings.Contains(stdout, "Third") || stderr != "" {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if strings.Join(seen, ",") != ",p2,p3" {
		t.Fatalf("pages asked for: %q", seen)
	}

	stdout, _, code = runWith(server, "--json", "dashboards", "list", "--all")
	var merged struct {
		Dashboards []map[string]any `json:"dashboards"`
		NextCursor *string          `json:"next_cursor"`
		TotalCount int              `json:"total_count"`
	}
	if err := json.Unmarshal([]byte(stdout), &merged); err != nil || code != 0 {
		t.Fatalf("exit %d, %v: %s", code, err, stdout)
	}
	if len(merged.Dashboards) != 3 || merged.NextCursor != nil || merged.TotalCount != 3 {
		t.Fatalf("merged %+v", merged)
	}
}

func TestAPageThatHasMoreSaysSoWithTheTotalAndTheWayOn(t *testing.T) {
	var seen []string
	server := dashboardPageServer(t, map[string]string{
		"": `{"dashboards":[` + dashboardRow(testDashboard, "First") + `],"next_cursor":"p2","total_count":7}`,
	}, &seen)
	_, stderr, code := runWith(server, "dashboards", "list", "--limit", "1")
	if code != 0 || !strings.Contains(stderr, "Showing 1 of 7. More: --cursor p2, or --all for every page.") {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
}

func TestAPageLoopThatNeverEndsStops(t *testing.T) {
	var seen []string
	server := dashboardPageServer(t, map[string]string{
		"":      `{"dashboards":[],"next_cursor":"again"}`,
		"again": `{"dashboards":[],"next_cursor":"again"}`,
	}, &seen)
	_, stderr, code := runWith(server, "dashboards", "list", "--all")
	if code != 1 || !strings.Contains(stderr, "kept answering with more pages") {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
}

func TestPagingFlagsAreCheckedBeforeAnyRequest(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"dashboards", "list", "--limit", "0"}, "--limit must be 1 to 200"},
		{[]string{"dashboards", "list", "--limit", "201"}, "--limit must be 1 to 200"},
		{[]string{"o", "list", "--limit", "101"}, "--limit must be 1 to 100"},
		{[]string{"dashboards", "list", "--all", "--cursor", "x"}, "cannot be combined with --cursor"},
		{[]string{"search", "--all", "--cursor", "x", "checkout"}, "cannot be combined with --cursor"},
	}
	for _, tc := range cases {
		call, _, stderr, code := runAgainst(t, 200, `{}`, tc.args...)
		if code != 2 || !strings.Contains(stderr, tc.want) || call.method != "" {
			t.Errorf("%v: exit %d, stderr %q, call %+v", tc.args, code, stderr, call)
		}
	}
}

func TestActivityAndDeliveriesPageLikeEveryList(t *testing.T) {
	event := func(seq int) string {
		return activityJSON(seq, "dashboard.created", "Sam", "2026-01-01T00:00:00Z", `{}`)
	}
	delivery := func(id string) string {
		return `{"id":"` + id + `","delivery_id":"d` + id + `","event_type":"run.failed","activity_seq":1,"attempted_at":"2026-11-11T09:30:00Z","duration_ms":42,"status_code":500,"outcome":"failed","error_code":"status"}`
	}
	cases := []struct {
		name  string
		args  []string
		pages map[string]string
		want  []string
	}{
		{"activity", []string{"activity"}, map[string]string{
			"":   `{"activity":[` + event(2) + `],"next_cursor":"p2"}`,
			"p2": `{"activity":[` + event(1) + `],"next_cursor":null}`,
		}, []string{"created a dashboard"}},
		{"deliveries", []string{"webhooks", "deliveries", testWebhook}, map[string]string{
			"":   `{"deliveries":[` + delivery("2") + `],"next_cursor":"p2"}`,
			"p2": `{"deliveries":[` + delivery("1") + `],"next_cursor":null}`,
		}, []string{"run.failed"}},
	}
	for _, tc := range cases {
		var seen []string
		server := dashboardPageServer(t, tc.pages, &seen)
		_, stderr, code := runWith(server, tc.args...)
		if code != 0 || !strings.Contains(stderr, "Showing 1, and more exist. More: --cursor p2, or --all") {
			t.Fatalf("%s: exit %d, stderr %q", tc.name, code, stderr)
		}
		stdout, stderr, code := runWith(server, append(tc.args, "--all")...)
		if code != 0 || stderr != "" || strings.Count(stdout, tc.want[0]) != 2 {
			t.Fatalf("%s --all: exit %d, stdout %q, stderr %q", tc.name, code, stdout, stderr)
		}
		stdout, _, code = runWith(server, append(tc.args, "--cursor", "p2")...)
		if code != 0 || strings.Count(stdout, tc.want[0]) != 1 {
			t.Fatalf("%s --cursor: exit %d, stdout %q", tc.name, code, stdout)
		}
	}
}

func TestActivityFollowCannotStartFromACursorOrAllPages(t *testing.T) {
	for _, flag := range [][]string{{"--all"}, {"--cursor", "p2"}} {
		_, _, stderr, code := runAgainst(t, 200, `{"activity":[],"next_cursor":null}`, append([]string{"activity", "--follow"}, flag...)...)
		if code != 2 || !strings.Contains(stderr, "--follow") {
			t.Fatalf("%v: exit %d, stderr %q", flag, code, stderr)
		}
	}
}
