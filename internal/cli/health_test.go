package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func strPtr(s string) *string { return &s }

func TestHealthRowsListOneRowPerProblem(t *testing.T) {
	rows := healthRows([]healthDashboard{
		{
			ID: "d1", Title: "Checkout", Folder: healthFolder{Title: "Shop"}, CheckedAt: strPtr("2026-11-06T03:00:12Z"),
			Problems: []healthProblem{
				{Kind: "no_data", PanelID: strPtr("p_rps"), PanelTitle: strPtr("Requests")},
				{Kind: "variable_failed", Variable: strPtr("pod")},
				{Kind: "query_failed", PanelID: strPtr("p_err")},
			},
		},
		{ID: "d2", Title: "Old\x1b[31m", Folder: healthFolder{Title: "General"}, Problems: []healthProblem{{Kind: "unopened"}}},
	})
	want := [][]string{
		{"d1", "Checkout", "Shop", "no_data", "Requests (p_rps)", "2026-11-06T03:00Z"},
		{"d1", "Checkout", "Shop", "variable_failed", "$pod", "2026-11-06T03:00Z"},
		{"d1", "Checkout", "Shop", "query_failed", "p_err", "2026-11-06T03:00Z"},
		{"d2", "Old [31m", "General", "unopened", "-", "-"},
	}
	if !slices.EqualFunc(rows, want, slices.Equal[[]string]) {
		t.Errorf("rows = %q\nwant %q", rows, want)
	}
}

func TestJobLineSaysWhatTheLastCheckDid(t *testing.T) {
	cases := []struct {
		job  healthJob
		want string
	}{
		{healthJob{}, "No health check has run yet."},
		{healthJob{Running: true, LastStartedAt: strPtr("2026-11-06T03:00:00Z")}, "A health check is running now."},
		{healthJob{LastStartedAt: strPtr("2026-11-06T03:00:00Z"), LastFinishedAt: strPtr("2026-11-06T03:04:31Z"), LastOutcome: strPtr("succeeded")}, "Last check finished 2026-11-06T03:04Z: succeeded."},
		{healthJob{LastStartedAt: strPtr("2026-11-06T03:00:00Z")}, "Last check started 2026-11-06T03:00Z and did not finish."},
	}
	for _, c := range cases {
		if got := jobLine(c.job); got != c.want {
			t.Errorf("jobLine(%+v) = %q, want %q", c.job, got, c.want)
		}
	}
}

func TestRepairRowsFollowTheIDsAsked(t *testing.T) {
	var outcome repairOutcome
	outcome.Queued = append(outcome.Queued, struct {
		DashboardID string `json:"dashboard_id"`
		RequestID   string `json:"request_id"`
	}{"b", "r1"})
	outcome.Skipped = append(outcome.Skipped, struct {
		DashboardID string `json:"dashboard_id"`
		Reason      string `json:"reason"`
	}{"a", "nothing_to_repair"})
	rows := repairRows([]string{"a", "b", "a", "c"}, outcome)
	want := [][]string{{"a", "nothing_to_repair", "-"}, {"b", "queued", "r1"}, {"c", "-", "-"}}
	if !slices.EqualFunc(rows, want, slices.Equal[[]string]) {
		t.Errorf("rows = %q, want %q", rows, want)
	}
}

const (
	repairID      = "aaaaaaaa-0000-4000-8000-000000000001"
	otherRepairID = "aaaaaaaa-0000-4000-8000-000000000002"
)

func healthServer(t *testing.T, answer string) (*httptest.Server, *[]recordedRequest) {
	t.Helper()
	var requests []recordedRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		requests = append(requests, recordedRequest{method: r.Method, path: r.URL.Path, query: r.URL.RawQuery, body: string(data)})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, answer)
	}))
	t.Cleanup(server.Close)
	return server, &requests
}

func TestHealthRepairExitsOneWhenNothingIsQueued(t *testing.T) {
	cases := []struct {
		name, answer string
		code         int
		stdout       string
	}{
		{"one queued", `{"queued":[{"dashboard_id":"` + repairID + `","request_id":"r1"}],"skipped":[{"dashboard_id":"` + otherRepairID + `","reason":"active"}]}`, 0, "queued"},
		{"none queued", `{"queued":[],"skipped":[{"dashboard_id":"` + repairID + `","reason":"nothing_to_repair"},{"dashboard_id":"` + otherRepairID + `","reason":"forbidden_role"}]}`, 1, "forbidden_role"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			server, requests := healthServer(t, c.answer)
			stdout, stderr, code := runCLI("--server", server.URL, "--token", "k", "health", "repair", repairID, otherRepairID)
			if code != c.code || !strings.Contains(stdout, c.stdout) {
				t.Fatalf("code %d stdout %q stderr %q", code, stdout, stderr)
			}
			if c.code == 1 && !strings.Contains(stderr, "no repair was queued") {
				t.Errorf("stderr = %q", stderr)
			}
			if got := (*requests)[0]; got.method != http.MethodPost || got.path != "/api/v1/health/repairs" || got.body != `{"dashboard_ids":["`+repairID+`","`+otherRepairID+`"]}` {
				t.Errorf("request = %+v", got)
			}
		})
	}
}

func TestHealthRepairRefusesBadIDsBeforeCallingTheServer(t *testing.T) {
	ids := make([]string, maxRepairIDs+1)
	for i := range ids {
		ids[i] = repairID
	}
	for name, args := range map[string][]string{"none": nil, "not an id": {"checkout"}, "too many": ids} {
		server, requests := healthServer(t, `{}`)
		_, stderr, code := runCLI(append([]string{"--server", server.URL, "--token", "k", "health", "repair"}, args...)...)
		if code != 2 || len(*requests) != 0 {
			t.Errorf("%s: code %d, %d requests, stderr %q", name, code, len(*requests), stderr)
		}
	}
}
