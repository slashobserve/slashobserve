package cli

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
