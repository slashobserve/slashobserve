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
	testFinding   = "dddddddd-dddd-dddd-dddd-dddddddddddd"
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

func threadJSON(id, kind, targetID, panel string, present bool, status, version string) string {
	return fmt.Sprintf(`{"id":%q,"target":{"kind":%q,"id":%q},"panel_id":%s,"panel_present":%t,"status":%q,"resolved_by_version":%s}`,
		id, kind, targetID, orNull(panel), present, status, version)
}

func messageJSON(id, threadID, label, body string) string {
	return fmt.Sprintf(`{"id":%q,"thread_id":%q,"actor":{"label":%q},"created_at":"2026-01-02T03:04:05Z","body":%q}`, id, threadID, label, body)
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
	threadsPage := `{"threads":[` + strings.Join([]string{
		threadJSON(testThread, "finding", testFinding, "", true, "open", "null"),
		threadJSON(testReply, "dashboard", testDashboard, testPanel, false, "resolved", "2"),
	}, ",") + `],"messages":[` + strings.Join([]string{
		messageJSON(testThread, testThread, "Ada", "is this real?"),
		messageJSON("eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee", testThread, "/observe for Sam", "yes, see the finding"),
		messageJSON(testReply, testReply, "Eve\u202eevil", "gone panel"),
	}, ",") + `],"next_cursor":"next","can_comment":true}`
	threadResolved := func(version string) string {
		return `{"thread":` + threadJSON(testThread, "finding", testFinding, "", true, "resolved", version) + `}`
	}
	messageEnvelope := `{"thread":` + threadJSON(testThread, "finding", testFinding, "", true, "open", "null") + `,"message":` + messageJSON(testReply, testThread, "Ada", "x") + `}`
	on := "finding:" + testFinding
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
			name:    "reply with a help flag as its whole text shows help",
			args:    []string{"comments", "reply", testDashboard, testThread, "--help"},
			wantOut: []string{"observe comments reply [flags] [<dashboard-id>] <comment-id> <text...>"},
		},
		{
			name:    "reply with -h as its whole text shows help",
			args:    []string{"comments", "reply", testDashboard, testThread, "-h"},
			wantOut: []string{"observe comments reply [flags] [<dashboard-id>] <comment-id> <text...>"},
		},
		{
			name:       "resolve with --json reports a version that was not applied",
			args:       []string{"comments", "resolve", "--json", testDashboard, testThread, "--version", "5"},
			response:   resolvedV2,
			wantCode:   1,
			wantMethod: http.MethodPatch,
			wantPath:   "/api/v1/dashboards/" + testDashboard + "/comments/" + testThread,
			wantBody:   `{"resolved_by_version":5,"status":"resolved"}`,
			wantOut:    []string{`"resolved_by_version":2`},
			wantNotOut: []string{"Resolved thread"},
			wantErr:    "thread " + testThread + " is already resolved by version 2; --version was not applied",
		},
		{
			name:       "list on a target renders messages with their thread",
			args:       []string{"comments", "list", "--on", on},
			response:   threadsPage,
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/threads",
			wantQuery:  "limit=50&status=open&target=finding%3A" + testFinding,
			wantOut: []string{
				testThread + " - finding open Ada",
				"eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee " + testThread + " finding open /observe for Sam",
				testReply + " - " + testPanel + " (removed) resolved Eve evil",
			},
			wantNotOut: []string{"\u202e"},
			wantErr:    "more results: --cursor next",
		},
		{
			name:       "list on a target sends status and panel",
			args:       []string{"comments", "list", "--on", "dashboard:" + testDashboard, "--status", "all", "--panel", testPanel},
			response:   `{"threads":[],"messages":[],"next_cursor":null,"can_comment":true}`,
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/threads",
			wantQuery:  "limit=50&panel_id=" + testPanel + "&status=all&target=dashboard%3A" + testDashboard,
		},
		{
			name:       "list on a target with --json prints the body",
			args:       []string{"comments", "list", "--json", "--on", on},
			response:   threadsPage,
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/threads",
			wantQuery:  "limit=50&status=open&target=finding%3A" + testFinding,
			wantOut:    []string{`"can_comment":true`},
		},
		{
			name:       "list on an observation sends its target",
			args:       []string{"comments", "list", "--on", "observation:" + testFinding},
			response:   threadsPage,
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/threads",
			wantQuery:  "limit=50&status=open&target=observation%3A" + testFinding,
			wantErr:    "more results: --cursor next",
		},
		{
			name:     "list takes a dashboard id or --on",
			args:     []string{"comments", "list"},
			wantCode: 2,
			wantErr:  "comments list takes a dashboard id or --on <kind>:<id>, one of them",
		},
		{
			name:     "list refuses a dashboard id with --on",
			args:     []string{"comments", "list", testDashboard, "--on", on},
			wantCode: 2,
			wantErr:  "comments list takes a dashboard id or --on <kind>:<id>, one of them",
		},
		{
			name:     "list rejects an unknown target kind",
			args:     []string{"comments", "list", "--on", "run:" + testFinding},
			wantCode: 2,
			wantErr:  `--on "run:` + testFinding + `" is not <kind>:<id>`,
		},
		{
			name:     "list rejects a target without an id",
			args:     []string{"comments", "list", "--on", "finding"},
			wantCode: 2,
			wantErr:  `--on "finding" is not <kind>:<id>`,
		},
		{
			name:     "list rejects a target with a bad id",
			args:     []string{"comments", "list", "--on", "finding:nope"},
			wantCode: 2,
			wantErr:  `--on "finding:nope" is not <kind>:<id>`,
		},
		{
			name:       "add starts a thread on a finding",
			args:       []string{"comments", "add", "--on", on, "looks", "wrong"},
			status:     http.StatusCreated,
			response:   messageEnvelope,
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/threads",
			wantBody:   `{"body":"looks wrong","target":"` + on + `"}`,
			wantOut:    []string{"Started thread " + testThread + " on finding:" + testFinding},
		},
		{
			name:       "add anchors to a panel and a version",
			args:       []string{"comments", "add", "--on", "dashboard:" + testDashboard, "--panel", "p1", "--version", "3", "check", "this"},
			status:     http.StatusCreated,
			response:   messageEnvelope,
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/threads",
			wantBody:   `{"body":"check this","panel_id":"p1","target":"dashboard:` + testDashboard + `","version":3}`,
		},
		{
			name:       "add keeps flag-like words and mentions in the text",
			args:       []string{"comments", "add", "--on", on, "@observe", "rerun", "with", "--json"},
			status:     http.StatusCreated,
			response:   messageEnvelope,
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/threads",
			wantBody:   `{"body":"@observe rerun with --json","target":"` + on + `"}`,
			wantNotOut: []string{`"thread"`},
		},
		{
			name:       "add with --json prints the body",
			args:       []string{"comments", "add", "--json", "--on", on, "ok"},
			status:     http.StatusCreated,
			response:   messageEnvelope,
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/threads",
			wantBody:   `{"body":"ok","target":"` + on + `"}`,
			wantOut:    []string{`"thread"`},
		},
		{
			name:     "add needs a target",
			args:     []string{"comments", "add", "ok"},
			wantCode: 2,
			wantErr:  "--on <kind>:<id> is required",
		},
		{
			name:     "add needs text",
			args:     []string{"comments", "add", "--on", on, "  "},
			wantCode: 2,
			wantErr:  "add needs text",
		},
		{
			name:     "add rejects version 0",
			args:     []string{"comments", "add", "--on", on, "--version", "0", "ok"},
			wantCode: 2,
			wantErr:  "--version must be 1 or higher",
		},
		{
			name:       "reply with one id uses the thread endpoint",
			args:       []string{"comments", "reply", testThread, "looks", "good"},
			status:     http.StatusCreated,
			response:   messageEnvelope,
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/threads/" + testThread + "/messages",
			wantBody:   `{"body":"looks good"}`,
			wantOut:    []string{"Replied to thread " + testThread + " as " + testReply},
		},
		{
			name:       "reply with one id with --json prints the body",
			args:       []string{"comments", "reply", "--json", testThread, "ok"},
			status:     http.StatusCreated,
			response:   messageEnvelope,
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/threads/" + testThread + "/messages",
			wantBody:   `{"body":"ok"}`,
			wantOut:    []string{`"message"`},
		},
		{
			name:     "reply with one id needs text",
			args:     []string{"comments", "reply", testThread},
			wantCode: 2,
			wantErr:  "reply needs text",
		},
		{
			name:     "reply rejects an id that is not one",
			args:     []string{"comments", "reply", "nope", "text"},
			wantCode: 2,
			wantErr:  `"nope" is not a valid id`,
		},
		{
			name:     "reply needs an id",
			args:     []string{"comments", "reply"},
			wantCode: 2,
			wantErr:  "comments reply takes a comment id and the reply text",
		},
		{
			name:       "edit replaces the text",
			args:       []string{"comments", "edit", testReply, "better", "text"},
			response:   messageEnvelope,
			wantMethod: http.MethodPatch,
			wantPath:   "/api/v1/messages/" + testReply,
			wantBody:   `{"body":"better text"}`,
			wantOut:    []string{"Edited comment " + testReply},
		},
		{
			name:       "edit with --quiet prints nothing",
			args:       []string{"-q", "comments", "edit", testReply, "ok"},
			response:   messageEnvelope,
			wantMethod: http.MethodPatch,
			wantPath:   "/api/v1/messages/" + testReply,
			wantBody:   `{"body":"ok"}`,
			wantExact:  new(string),
		},
		{
			name:       "edit with --json prints the body",
			args:       []string{"comments", "edit", "--json", testReply, "ok"},
			response:   messageEnvelope,
			wantMethod: http.MethodPatch,
			wantPath:   "/api/v1/messages/" + testReply,
			wantBody:   `{"body":"ok"}`,
			wantOut:    []string{`"message"`},
		},
		{
			name:     "edit needs text",
			args:     []string{"comments", "edit", testReply},
			wantCode: 2,
			wantErr:  "edit needs text",
		},
		{
			name:     "edit needs an id",
			args:     []string{"comments", "edit"},
			wantCode: 2,
			wantErr:  "comments edit takes a comment id and the new text",
		},
		{
			name:    "edit with -h as its whole text shows help",
			args:    []string{"comments", "edit", testReply, "-h"},
			wantOut: []string{"observe comments edit [flags] <comment-id> <text...>"},
		},
		{
			name:       "resolve with one id uses the thread endpoint",
			args:       []string{"comments", "resolve", testThread},
			response:   threadResolved("null"),
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/threads/" + testThread + "/resolve",
			wantBody:   `{}`,
			wantOut:    []string{"Resolved thread " + testThread},
			wantNotOut: []string{"by version"},
		},
		{
			name:       "resolve with one id and a version",
			args:       []string{"comments", "resolve", testThread, "--version", "3"},
			response:   threadResolved("3"),
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/threads/" + testThread + "/resolve",
			wantBody:   `{"version":3}`,
			wantOut:    []string{"Resolved thread " + testThread + " by version 3"},
		},
		{
			name:       "resolve with one id reports a version that was not applied",
			args:       []string{"comments", "resolve", testThread, "--version", "5"},
			response:   threadResolved("2"),
			wantCode:   1,
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/threads/" + testThread + "/resolve",
			wantBody:   `{"version":5}`,
			wantExact:  new(string),
			wantErr:    "thread " + testThread + " is already resolved by version 2; --version was not applied",
		},
		{
			name:       "resolve with one id and --json prints the body",
			args:       []string{"comments", "resolve", "--json", testThread},
			response:   threadResolved("null"),
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/threads/" + testThread + "/resolve",
			wantBody:   `{}`,
			wantOut:    []string{`"thread"`},
			wantNotOut: []string{"Resolved thread"},
		},
		{
			name:     "resolve takes at most two ids",
			args:     []string{"comments", "resolve", testDashboard, testThread, testReply},
			wantCode: 2,
			wantErr:  "comments resolve takes a comment id, after an optional dashboard id",
		},
		{
			name:       "reopen sends no body",
			args:       []string{"comments", "reopen", testThread},
			response:   `{"thread":` + threadJSON(testThread, "finding", testFinding, "", true, "open", "null") + `}`,
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/threads/" + testThread + "/reopen",
			wantOut:    []string{"Reopened thread " + testThread},
		},
		{
			name:       "reopen with --quiet prints nothing",
			args:       []string{"-q", "comments", "reopen", testThread},
			response:   `{"thread":` + threadJSON(testThread, "finding", testFinding, "", true, "open", "null") + `}`,
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/threads/" + testThread + "/reopen",
			wantExact:  new(string),
		},
		{
			name:       "reopen with --json prints the body",
			args:       []string{"comments", "reopen", "--json", testThread},
			response:   `{"thread":` + threadJSON(testThread, "finding", testFinding, "", true, "open", "null") + `}`,
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/threads/" + testThread + "/reopen",
			wantOut:    []string{`"thread"`},
		},
		{
			name:     "reopen needs one id",
			args:     []string{"comments", "reopen"},
			wantCode: 2,
			wantErr:  "comments reopen takes a comment id",
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
					t.Fatalf("expected no request, got %v", got)
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
