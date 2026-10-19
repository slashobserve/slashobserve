package cli

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

const (
	testDashboard = "11111111-2222-3333-4444-555555555555"
	testThread    = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	testReply     = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	testPanel     = "cccccccc-cccc-cccc-cccc-cccccccccccc"
)

var spaces = regexp.MustCompile(` {2,}`)

type recordedRequest struct {
	method string
	path   string
	query  string
	body   string
}

func commentJSON(id, parent, panel string, present bool, author, body string, version string) string {
	return fmt.Sprintf(`{"id":%q,"parent_id":%s,"panel_id":%s,"panel_present":%t,"status":"open","author":{"name":%q},"created_at":"2026-01-02T03:04:05Z","body":%q,"resolved_by_version":%s}`,
		id, orNull(parent), orNull(panel), present, author, body, version)
}

func orNull(s string) string {
	if s == "" {
		return "null"
	}
	return fmt.Sprintf("%q", s)
}

func TestCommentsCommands(t *testing.T) {
	longBody := strings.Repeat("é", 81)
	listBody := `{"comments":[` + strings.Join([]string{
		commentJSON("c1", "", "", true, "Ada", "whole dashboard note", "null"),
		commentJSON("c2", "", testPanel, false, "Grace", "gone panel", "null"),
		commentJSON("c3", "c1", "", true, "Linus", "a reply", "null"),
		commentJSON("c4", "", "", true, "Ken", "first line\nsecond line", "null"),
		commentJSON("c5", "", "", true, "Rob", longBody, "null"),
		commentJSON("c6", "", "", true, "Eve\u202eevil", "bidi author", "null"),
	}, ",") + `],"next_cursor":null}`
	envelope := func(c string) string { return `{"comment":` + c + `}` }
	replyBody := envelope(commentJSON(testReply, testThread, "", true, "Ada", "x", "null"))
	resolvedPlain := envelope(commentJSON(testThread, "", "", true, "Ada", "x", "null"))
	resolvedV3 := envelope(commentJSON(testThread, "", "", true, "Ada", "x", "3"))
	resolvedV2 := envelope(commentJSON(testThread, "", "", true, "Ada", "x", "2"))

	cases := []struct {
		name       string
		args       []string
		status     int
		response   string
		wantCode   int
		wantMethod string
		wantPath   string
		wantQuery  string
		wantBody   string
		wantOut    []string
		wantNotOut []string
		wantExact  *string
		wantErr    string
	}{
		{
			name:       "list renders every row shape",
			args:       []string{"comments", "list", testDashboard},
			response:   listBody,
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/dashboards/" + testDashboard + "/comments",
			wantQuery:  "limit=50&status=open",
			wantOut: []string{
				"ID", "THREAD", "COMMENT",
				"c1 - dashboard",
				"c2 - " + testPanel + " (removed)",
				"c3 c1 dashboard",
				"first line",
				strings.Repeat("é", 80) + "...",
				"Eve evil",
			},
			wantNotOut: []string{"second line", strings.Repeat("é", 81), "\u202e"},
		},
		{
			name:       "list sends status and panel",
			args:       []string{"comments", "list", testDashboard, "--status", "all", "--panel", testPanel},
			response:   `{"comments":[],"next_cursor":null}`,
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/dashboards/" + testDashboard + "/comments",
			wantQuery:  "limit=50&panel_id=" + testPanel + "&status=all",
		},
		{
			name:     "list rejects an unknown status",
			args:     []string{"comments", "list", testDashboard, "--status", "bogus"},
			wantCode: 2,
			wantErr:  "--status must be open, resolved or all",
		},
		{
			name:       "resolve without a version",
			args:       []string{"comments", "resolve", testDashboard, testThread},
			response:   resolvedPlain,
			wantMethod: http.MethodPatch,
			wantPath:   "/api/v1/dashboards/" + testDashboard + "/comments/" + testThread,
			wantBody:   `{"status":"resolved"}`,
			wantOut:    []string{"Resolved thread " + testThread},
			wantNotOut: []string{"by version"},
		},
		{
			name:       "resolve with a version",
			args:       []string{"comments", "resolve", testDashboard, testThread, "--version", "3"},
			response:   resolvedV3,
			wantMethod: http.MethodPatch,
			wantPath:   "/api/v1/dashboards/" + testDashboard + "/comments/" + testThread,
			wantBody:   `{"resolved_by_version":3,"status":"resolved"}`,
			wantOut:    []string{"Resolved thread " + testThread + " by version 3"},
		},
		{
			name:       "resolve with --quiet prints nothing",
			args:       []string{"-q", "comments", "resolve", testDashboard, testThread},
			response:   resolvedPlain,
			wantMethod: http.MethodPatch,
			wantPath:   "/api/v1/dashboards/" + testDashboard + "/comments/" + testThread,
			wantBody:   `{"status":"resolved"}`,
			wantExact:  new(string),
		},
		{
			name:       "resolve reports a version that was not applied",
			args:       []string{"comments", "resolve", testDashboard, testThread, "--version", "5"},
			response:   resolvedV2,
			wantCode:   1,
			wantMethod: http.MethodPatch,
			wantPath:   "/api/v1/dashboards/" + testDashboard + "/comments/" + testThread,
			wantBody:   `{"resolved_by_version":5,"status":"resolved"}`,
			wantExact:  new(string),
			wantErr:    "thread " + testThread + " is already resolved by version 2; --version was not applied",
		},
		{
			name:       "resolve reports a version that was not applied to a thread resolved without one",
			args:       []string{"comments", "resolve", testDashboard, testThread, "--version", "5"},
			response:   resolvedPlain,
			wantCode:   1,
			wantMethod: http.MethodPatch,
			wantPath:   "/api/v1/dashboards/" + testDashboard + "/comments/" + testThread,
			wantBody:   `{"resolved_by_version":5,"status":"resolved"}`,
			wantExact:  new(string),
			wantErr:    "thread " + testThread + " is already resolved; --version was not applied",
		},
		{
			name:     "resolve rejects version 0",
			args:     []string{"comments", "resolve", testDashboard, testThread, "--version", "0"},
			wantCode: 2,
			wantErr:  "--version must be 1 or higher",
		},
		{
			name:       "reply prints the thread and the new id",
			args:       []string{"comments", "reply", testDashboard, testThread, "looks", "good"},
			status:     http.StatusCreated,
			response:   replyBody,
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/dashboards/" + testDashboard + "/comments",
			wantBody:   `{"body":"looks good","parent_id":"` + testThread + `"}`,
			wantOut:    []string{"Replied to thread " + testThread + " as " + testReply},
		},
		{
			name:       "reply still prints with --quiet",
			args:       []string{"-q", "comments", "reply", testDashboard, testThread, "ok"},
			status:     http.StatusCreated,
			response:   replyBody,
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/dashboards/" + testDashboard + "/comments",
			wantBody:   `{"body":"ok","parent_id":"` + testThread + `"}`,
			wantOut:    []string{"Replied to thread " + testThread + " as " + testReply},
		},
		{
			name:       "reply keeps flag-like words in the text",
			args:       []string{"comments", "reply", testDashboard, testThread, "rerun", "with", "-q", "and", "--json", "to", "check"},
			status:     http.StatusCreated,
			response:   replyBody,
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/dashboards/" + testDashboard + "/comments",
			wantBody:   `{"body":"rerun with -q and --json to check","parent_id":"` + testThread + `"}`,
			wantOut:    []string{"Replied to thread " + testThread + " as " + testReply},
			wantNotOut: []string{`"comment"`},
		},
		{
			name:       "reply takes global flags before the ids",
			args:       []string{"comments", "reply", "--json", testDashboard, testThread, "ok"},
			status:     http.StatusCreated,
			response:   replyBody,
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/dashboards/" + testDashboard + "/comments",
			wantBody:   `{"body":"ok","parent_id":"` + testThread + `"}`,
			wantOut:    []string{`"comment"`},
		},
		{
			name:     "reply needs text",
			args:     []string{"comments", "reply", testDashboard, testThread, "  "},
			wantCode: 2,
			wantErr:  "reply needs text",
		},
		{
			name:       "a prompt keeps flag-like words",
			args:       []string{"--no-wait", "show", "me", "-q", "latency", "--json"},
			status:     http.StatusAccepted,
			response:   `{"request":{"id":"r","status":"queued","steps":[]}}`,
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/observe",
			wantBody:   `{"prompt":"show me -q latency --json"}`,
			wantOut:    []string{"r"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []recordedRequest
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				got = append(got, recordedRequest{r.Method, r.URL.Path, r.URL.RawQuery, string(body)})
				w.Header().Set("Content-Type", "application/json")
				status := tc.status
				if status == 0 {
					status = http.StatusOK
				}
				w.WriteHeader(status)
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
					t.Fatalf("a usage error should send no request, got %v", got)
				}
			} else {
				if len(got) != 1 {
					t.Fatalf("want one request, got %v", got)
				}
				want := recordedRequest{tc.wantMethod, tc.wantPath, tc.wantQuery, tc.wantBody}
				if got[0] != want {
					t.Fatalf("request %+v, want %+v", got[0], want)
				}
			}
			squeezed := spaces.ReplaceAllString(stdout.String(), " ")
			for _, want := range tc.wantOut {
				if !strings.Contains(squeezed, want) {
					t.Fatalf("stdout lacks %q:\n%s", want, stdout.String())
				}
			}
			for _, unwanted := range tc.wantNotOut {
				if strings.Contains(stdout.String(), unwanted) {
					t.Fatalf("stdout has %q:\n%s", unwanted, stdout.String())
				}
			}
			if tc.wantExact != nil && stdout.String() != *tc.wantExact {
				t.Fatalf("stdout %q, want %q", stdout.String(), *tc.wantExact)
			}
			if tc.wantErr == "" && stderr.Len() > 0 {
				t.Fatalf("unexpected stderr %q", stderr.String())
			}
			if tc.wantErr != "" && !strings.Contains(stderr.String(), tc.wantErr) {
				t.Fatalf("stderr %q lacks %q", stderr.String(), tc.wantErr)
			}
		})
	}
}
