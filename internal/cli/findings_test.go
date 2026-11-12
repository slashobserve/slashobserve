package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
)

const testFolderTarget = "eeeeeeee-1111-4111-8111-eeeeeeeeeeee"

func findingRowJSON(id, title, dashboard, folder, label string) string {
	return fmt.Sprintf(`{"id":%q,"title":%q,"dashboard_id":%s,"folder_id":%s,"actor":{"label":%q},"created_at":"2026-01-02T03:04:05Z"}`,
		id, title, orNull(dashboard), orNull(folder), label)
}

func TestFindingsCommands(t *testing.T) {
	listBody := `{"findings":[` + strings.Join([]string{
		findingRowJSON(testFinding, "Slow pod", "", testFolderTarget, "Ada"),
		findingRowJSON("ffffffff-ffff-ffff-ffff-ffffffffffff", "Disk full", testDashboard, "", "Grace"),
		findingRowJSON("99999999-9999-4999-8999-999999999999", "Old note", "", "", "ci"),
	}, ",") + `],"next_cursor":"more"}`
	moved := `{"finding":` + findingRowJSON(testFinding, "Slow pod", "", testFolderTarget, "Ada") + `,"folder":{"id":"` + testFolderTarget + `","title":"Payments"},"permissions":{}}`

	cases := []struct {
		name       string
		args       []string
		response   string
		wantCode   int
		wantMethod string
		wantPath   string
		wantQuery  string
		wantBody   string
		wantOut    []string
		wantExact  *string
		wantErr    string
	}{
		{
			name:       "list prints one row per finding and where it lives",
			args:       []string{"findings", "list"},
			response:   listBody,
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/findings",
			wantQuery:  "limit=50",
			wantOut: []string{
				"ID TITLE WHERE SAVED BY CREATED",
				testFinding + " Slow pod folder " + testFolderTarget + " Ada",
				"Disk full dashboard " + testDashboard + " Grace",
				"Old note - ci",
			},
			wantErr: "more results: --cursor more",
		},
		{
			name:       "list by folder",
			args:       []string{"findings", "list", "--folder", testFolderTarget, "--limit", "5"},
			response:   `{"findings":[],"next_cursor":null}`,
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/findings",
			wantQuery:  "folder_id=" + testFolderTarget + "&limit=5",
		},
		{
			name:       "list by dashboard",
			args:       []string{"findings", "list", "--dashboard", testDashboard},
			response:   `{"findings":[],"next_cursor":null}`,
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/findings",
			wantQuery:  "dashboard_id=" + testDashboard + "&limit=50",
		},
		{
			name:       "list with --json prints the body",
			args:       []string{"findings", "list", "--json"},
			response:   listBody,
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/findings",
			wantQuery:  "limit=50",
			wantOut:    []string{`"findings"`},
		},
		{
			name:     "list refuses a dashboard and a folder together",
			args:     []string{"findings", "list", "--dashboard", testDashboard, "--folder", testFolderTarget},
			wantCode: 2,
			wantErr:  "--dashboard and --folder cannot be used together",
		},
		{
			name:     "list refuses a folder that is not an id",
			args:     []string{"findings", "list", "--folder", "nope"},
			wantCode: 2,
			wantErr:  `"nope" is not a valid id`,
		},
		{
			name:       "move names the finding and the folder",
			args:       []string{"findings", "move", testFinding, "--folder", testFolderTarget},
			response:   moved,
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/findings/" + testFinding + "/move",
			wantBody:   `{"folder_id":"` + testFolderTarget + `"}`,
			wantOut:    []string{"Moved Slow pod to Payments"},
		},
		{
			name:       "move with --quiet prints nothing",
			args:       []string{"-q", "findings", "move", testFinding, "--folder", testFolderTarget},
			response:   moved,
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/findings/" + testFinding + "/move",
			wantBody:   `{"folder_id":"` + testFolderTarget + `"}`,
			wantExact:  new(string),
		},
		{
			name:       "move with --json prints the body",
			args:       []string{"findings", "move", "--json", testFinding, "--folder", testFolderTarget},
			response:   moved,
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/findings/" + testFinding + "/move",
			wantBody:   `{"folder_id":"` + testFolderTarget + `"}`,
			wantOut:    []string{`"folder"`},
		},
		{
			name:     "move needs a folder",
			args:     []string{"findings", "move", testFinding},
			wantCode: 2,
			wantErr:  "--folder <folder-id> is required",
		},
		{
			name:     "move needs a finding id",
			args:     []string{"findings", "move", "--folder", testFolderTarget},
			wantCode: 2,
			wantErr:  "findings move takes one finding id",
		},
		{
			name:     "move refuses an id that is not one",
			args:     []string{"findings", "move", "nope", "--folder", testFolderTarget},
			wantCode: 2,
			wantErr:  `"nope" is not a valid id`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []recordedRequest
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				got = append(got, recordedRequest{r.Method, r.URL.Path, r.URL.RawQuery, string(body)})
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, tc.response)
			}))
			t.Cleanup(server.Close)

			var stdout, stderr bytes.Buffer
			args := append([]string{"--server", server.URL, "--token", "k"}, tc.args...)
			code := Main(args, IO{In: strings.NewReader(""), Out: &stdout, Err: &stderr}, "test")
			if code != tc.wantCode {
				t.Fatalf("exit %d, want %d (stdout %q, stderr %q)", code, tc.wantCode, stdout.String(), stderr.String())
			}
			if tc.wantMethod == "" {
				if len(got) != 0 {
					t.Fatalf("expected no request, got %v", got)
				}
			} else {
				want := recordedRequest{tc.wantMethod, tc.wantPath, tc.wantQuery, tc.wantBody}
				if len(got) != 1 || got[0] != want {
					t.Fatalf("requests %+v, want one %+v", got, want)
				}
			}
			squeezed := spaces.ReplaceAllString(stdout.String(), " ")
			for _, want := range tc.wantOut {
				if !strings.Contains(squeezed, want) {
					t.Fatalf("stdout lacks %q:\n%s", want, stdout.String())
				}
			}
			if tc.wantExact != nil && stdout.String() != *tc.wantExact {
				t.Fatalf("stdout %q, want %q", stdout.String(), *tc.wantExact)
			}
			if tc.wantErr == "" && stderr.String() != findingsDeprecation(tc.args) {
				t.Fatalf("stderr %q, want only the deprecation line", stderr.String())
			}
			if tc.wantErr != "" && !strings.Contains(stderr.String(), tc.wantErr) {
				t.Fatalf("stderr %q lacks %q", stderr.String(), tc.wantErr)
			}
		})
	}
}

func findingsDeprecation(args []string) string {
	instead := "use observe o list"
	if slices.Contains(args, "move") {
		instead = "move the observation in the web app"
	}
	return "observe findings is deprecated and is removed in the next release; " + instead + "\n"
}

func TestFindingsCommandsPrintTheDeprecationFirstAndKeepJSONOutput(t *testing.T) {
	body := `{"findings":[],"next_cursor":null}`
	moved := `{"finding":` + findingRowJSON(testFinding, "Slow pod", "", testFolderTarget, "Ada") + `,"folder":null,"permissions":{}}`
	cases := []struct {
		name     string
		args     []string
		response string
	}{
		{"list", []string{"findings", "list", "--json"}, body},
		{"list quiet", []string{"-q", "findings", "list", "--json"}, body},
		{"move", []string{"findings", "move", "--json", testFinding, "--folder", testFolderTarget}, moved},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, tc.response)
			}))
			t.Cleanup(server.Close)

			var stdout, stderr bytes.Buffer
			args := append([]string{"--server", server.URL, "--token", "k"}, tc.args...)
			if code := Main(args, IO{In: strings.NewReader(""), Out: &stdout, Err: &stderr}, "test"); code != 0 {
				t.Fatalf("exit %d (stdout %q, stderr %q)", code, stdout.String(), stderr.String())
			}
			if stderr.String() != findingsDeprecation(tc.args) {
				t.Errorf("stderr %q, want the deprecation line", stderr.String())
			}
			var got, want any
			if err := json.Unmarshal(stdout.Bytes(), &got); err != nil || json.Unmarshal([]byte(tc.response), &want) != nil || !reflect.DeepEqual(got, want) {
				t.Errorf("stdout %q is not the response body %s: %v", stdout.String(), tc.response, err)
			}
		})
	}
}
