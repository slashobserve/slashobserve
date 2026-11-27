package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestVersionsAndShowPrintTheActorLabel(t *testing.T) {
	const versions = `{"versions":[
{"version":2,"parent_version":1,"summary":"added latency","actor":{"label":"Observe Agent for Ada"},"created_at":"2026-01-02T00:00:00Z"},
{"version":1,"parent_version":null,"summary":"first","actor":{"label":"Ada (via Claude Code)"},"created_at":"2026-01-01T00:00:00Z"}],"next_cursor":null}`
	const shown = `{"dashboard":{"id":"` + testDashboard + `","title":"Shop","latest_version":2,"updated_at":"2026-01-02T00:00:00Z","updated_by":{"name":"Ada"}},
"version":{"version":2,"parent_version":1,"summary":"added latency","actor":{"label":"Observe Agent for Ada"},"created_at":"2026-01-02T00:00:00Z","spec":{"panels":[]}}}`
	cases := []struct {
		name     string
		args     []string
		response string
		want     []string
	}{
		{"versions", []string{"dashboards", "versions", testDashboard}, versions, []string{"Observe Agent for Ada", "Ada (via Claude Code)"}},
		{"show", []string{"dashboards", "show", testDashboard}, shown, []string{"Observe Agent for Ada"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, tc.response)
			}))
			t.Cleanup(server.Close)
			var stdout, stderr bytes.Buffer
			args := append([]string{"--server", server.URL, "--token", "k"}, tc.args...)
			if code := Main(args, IO{In: strings.NewReader(""), Out: &stdout, Err: &stderr}, "test"); code != 0 {
				t.Fatalf("exit %d: %s", code, stderr.String())
			}
			for _, want := range tc.want {
				if !strings.Contains(stdout.String(), want) {
					t.Fatalf("output lacks %q:\n%s", want, stdout.String())
				}
			}
		})
	}
}

func TestDashboardsDelete(t *testing.T) {
	deletePath := "/api/v1/dashboards/" + testDashboard
	cases := []struct {
		name       string
		args       []string
		status     int
		response   string
		wantCode   int
		wantNoCall bool
		wantOut    string
		wantErr    string
	}{
		{
			name:       "refuses without --yes and sends nothing",
			args:       []string{"dashboards", "delete", testDashboard},
			wantCode:   2,
			wantNoCall: true,
			wantErr:    "rerun with --yes",
		},
		{
			name:    "deletes with --yes",
			args:    []string{"dashboards", "delete", testDashboard, "--yes"},
			status:  http.StatusNoContent,
			wantOut: "Deleted dashboard " + testDashboard + "\n",
		},
		{
			name:   "quiet hides the confirmation",
			args:   []string{"-q", "dashboards", "delete", testDashboard, "--yes"},
			status: http.StatusNoContent,
		},
		{
			name:   "json prints nothing for the empty response",
			args:   []string{"--json", "dashboards", "delete", testDashboard, "--yes"},
			status: http.StatusNoContent,
		},
		{
			name:     "reports a dashboard still being created",
			args:     []string{"dashboards", "delete", testDashboard, "--yes"},
			status:   http.StatusConflict,
			response: `{"error":"the agent is still creating this dashboard; try again when it finishes","code":"dashboard_being_created"}`,
			wantCode: 1,
			wantErr:  "dashboard_being_created",
		},
		{
			name:     "reports a missing dashboard",
			args:     []string{"dashboards", "delete", testDashboard, "--yes"},
			status:   http.StatusNotFound,
			response: `{"error":"dashboard not found","code":"not_found"}`,
			wantCode: 1,
			wantErr:  "dashboard not found",
		},
		{
			name:       "rejects a malformed id and sends nothing",
			args:       []string{"dashboards", "delete", "not-an-id", "--yes"},
			wantCode:   2,
			wantNoCall: true,
			wantErr:    "not a valid id",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []recordedRequest
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				got = append(got, recordedRequest{r.Method, r.URL.Path, r.URL.RawQuery, string(body)})
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.response)
			}))
			t.Cleanup(server.Close)

			var stdout, stderr bytes.Buffer
			args := append([]string{"--server", server.URL, "--token", "k"}, tc.args...)
			code := Main(args, IO{In: strings.NewReader(""), Out: &stdout, Err: &stderr}, "test")
			if code != tc.wantCode {
				t.Fatalf("exit %d, want %d (stdout %q, stderr %q)", code, tc.wantCode, stdout.String(), stderr.String())
			}
			if tc.wantNoCall {
				if len(got) != 0 {
					t.Fatalf("expected no request, got %v", got)
				}
			} else {
				want := recordedRequest{http.MethodDelete, deletePath, "", ""}
				if len(got) != 1 || got[0] != want {
					t.Fatalf("requests %+v, want %+v", got, want)
				}
			}
			if stdout.String() != tc.wantOut {
				t.Fatalf("stdout %q, want %q", stdout.String(), tc.wantOut)
			}
			if tc.wantErr == "" && stderr.Len() > 0 {
				t.Fatalf("unexpected stderr %q", stderr.String())
			}
			if !strings.Contains(stderr.String(), tc.wantErr) {
				t.Fatalf("stderr %q lacks %q", stderr.String(), tc.wantErr)
			}
		})
	}
}

func runDashboards(t *testing.T, handler http.HandlerFunc, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	var out, errOut bytes.Buffer
	full := append([]string{"--server", server.URL, "--token", "k"}, args...)
	code = Main(full, IO{In: strings.NewReader(""), Out: &out, Err: &errOut}, "test")
	return out.String(), errOut.String(), code
}

func TestDashboardsListShowsViewsAndSortsByThem(t *testing.T) {
	var query string
	out, errOut, code := runDashboards(t, func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"dashboards":[{"id":"`+testDashboard+`","title":"Shop","latest_version":2,"view_count":14,"updated_at":"2026-01-02T00:00:00Z","updated_by":{"name":"Ada"}}],"next_cursor":null}`)
	}, "dashboards", "list", "--order", "views")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(query, "order=views") {
		t.Errorf("query %q does not ask for order=views", query)
	}
	lines := strings.Split(out, "\n")
	if !strings.Contains(lines[0], "VIEWS") || !strings.Contains(lines[1], "14") {
		t.Errorf("table lacks the views column:\n%s", out)
	}
}

func TestDashboardsListRejectsAnUnknownOrderBeforeAnyRequest(t *testing.T) {
	called := false
	_, errOut, code := runDashboards(t, func(http.ResponseWriter, *http.Request) { called = true }, "dashboards", "list", "--order", "title")
	if code != 2 || called || !strings.Contains(errOut, "--order must be one of updated, views, name") {
		t.Fatalf("exit %d, called %t, stderr %q", code, called, errOut)
	}
}

func TestDashboardsShowPrintsViewsAndAddsThemToJSON(t *testing.T) {
	const shown = `{"dashboard":{"id":"` + testDashboard + `","title":"Shop","latest_version":2,"updated_at":"2026-01-02T00:00:00Z","updated_by":{"name":"Ada"}},"version":{"version":2,"summary":"s","actor":{"label":"Ada"},"created_at":"2026-01-02T00:00:00Z","spec":{"panels":[]}}}`
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/views") {
			_, _ = fmt.Fprint(w, `{"view_count":9,"window_days":30}`)
			return
		}
		_, _ = fmt.Fprint(w, shown)
	}
	out, errOut, code := runDashboards(t, handler, "dashboards", "show", testDashboard)
	if code != 0 || !strings.Contains(out, "9 in the last 30 days") {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	if strings.Contains(out, "Managed by") {
		t.Fatalf("stdout %q names what manages a dashboard the server said nothing about", out)
	}
	out, errOut, code = runDashboards(t, handler, "--json", "dashboards", "show", testDashboard)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	var body struct {
		ViewCount  int             `json:"view_count"`
		WindowDays int             `json:"window_days"`
		Dashboard  json.RawMessage `json:"dashboard"`
		Version    json.RawMessage `json:"version"`
	}
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatal(err)
	}
	if body.ViewCount != 9 || body.WindowDays != 30 || len(body.Dashboard) == 0 || len(body.Version) == 0 {
		t.Errorf("json = %s", out)
	}
}

func TestDashboardsListSendsExcludeAuditOnlyWhenAsked(t *testing.T) {
	for _, tt := range []struct {
		args []string
		want bool
	}{
		{[]string{"dashboards", "list"}, false},
		{[]string{"dashboards", "list", "--exclude-audit"}, true},
	} {
		var query string
		_, errOut, code := runDashboards(t, func(w http.ResponseWriter, r *http.Request) {
			query = r.URL.RawQuery
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"dashboards":[],"next_cursor":null}`)
		}, tt.args...)
		if code != 0 {
			t.Fatalf("%v: exit %d: %s", tt.args, code, errOut)
		}
		if got := strings.Contains(query, "exclude_audit=true"); got != tt.want {
			t.Errorf("%v: query %q, sends exclude_audit=true is %t, want %t", tt.args, query, got, tt.want)
		}
	}
}

func TestDashboardsShowNamesWhatManagesTheDashboard(t *testing.T) {
	const shown = `{"dashboard":{"id":"` + testDashboard + `","title":"Shop","latest_version":2,"managed_by":"apply","updated_at":"2026-01-02T00:00:00Z","updated_by":{"name":"Ada"}},"version":{"version":2,"summary":"s","actor":{"label":"Ada"},"created_at":"2026-01-02T00:00:00Z","spec":{"panels":[]}}}`
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/views") {
			_, _ = fmt.Fprint(w, `{"view_count":9,"window_days":30}`)
			return
		}
		_, _ = fmt.Fprint(w, shown)
	}
	out, errOut, code := runDashboards(t, handler, "dashboards", "show", testDashboard)
	if code != 0 || !regexp.MustCompile(`(?m)^Managed by\s+apply$`).MatchString(out) {
		t.Fatalf("exit %d, stdout %q, stderr %q, want a Managed by line that says apply", code, out, errOut)
	}
}
