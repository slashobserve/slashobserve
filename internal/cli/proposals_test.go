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

const testProposal = "eeeeeeee-2222-4222-8222-eeeeeeeeeeee"

func proposalJSON(state string, revision int, merged string, behind bool) string {
	if merged == "" {
		merged = "null"
	}
	return fmt.Sprintf(`{"id":%q,"dashboard_id":%q,"state":%q,"title":"Rename the errors panel","revision":%d,"base_version":3,"summary":"Rename it","author":{"kind":"user","id":"u1","name":"Bo"},"origin":"mcp","origin_client":"Claude Code","run_id":null,"created_at":"2026-01-02T03:04:05Z","updated_at":"2026-01-02T03:04:05Z","closed_at":null,"closed_by":null,"merged_version":%s,"behind":%t,"latest_version":5,"allowed_actions":["update","merge","discard"]}`,
		testProposal, testDashboard, state, revision, merged, behind)
}

func TestProposalsCommands(t *testing.T) {
	listBody := `{"proposals":[` + proposalJSON("open", 2, "", true) + `,` + strings.Replace(proposalJSON("merged", 1, "4", false), "Rename the errors panel", "Older change", 1) + `],"next_cursor":"c1"}`
	detail := strings.TrimSuffix(proposalJSON("open", 2, "", true), "}") +
		`,"spec":{},"changes":[{"kind":"panel","id":"p4","change":"added"},{"kind":"setting","id":"title","change":"changed"}],"conflicts":[{"kind":"panel","id":"p1","reason":"both_changed","message":"panel p1 was changed in both"}]}`
	closed := strings.TrimSuffix(proposalJSON("merged", 2, "4", false), "}") + `,"spec":{},"changes":[],"conflicts":[]}`
	mergeTwo := `{"revision":2,"choices":[{"kind":"panel","id":"p1","take":"theirs"},{"kind":"setting","id":"title","take":"ours"}]}`
	dashboardPath := "/api/v1/dashboards/" + testDashboard + "/proposals"
	proposalPath := "/api/v1/proposals/" + testProposal

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
			name:       "list prints a row for each proposal",
			args:       []string{"proposals", "list", testDashboard},
			response:   listBody,
			wantMethod: http.MethodGet,
			wantPath:   dashboardPath,
			wantQuery:  "limit=50",
			wantOut: []string{
				"ID STATE REVISION BEHIND TITLE",
				testProposal + " open 2 behind main Rename the errors panel",
				testProposal + " merged 1 - Older change",
			},
			wantErr: "more results: --cursor c1",
		},
		{
			name:       "list takes a state and a page",
			args:       []string{"proposals", "list", testDashboard, "--state", "all", "--limit", "5", "--cursor", "c0"},
			response:   `{"proposals":[],"next_cursor":null}`,
			wantMethod: http.MethodGet,
			wantPath:   dashboardPath,
			wantQuery:  "cursor=c0&limit=5&state=all",
		},
		{
			name:       "list with --json prints the body",
			args:       []string{"proposals", "list", testDashboard, "--json"},
			response:   listBody,
			wantMethod: http.MethodGet,
			wantPath:   dashboardPath,
			wantQuery:  "limit=50",
			wantOut:    []string{`"proposals"`, `"next_cursor":"c1"`},
		},
		{name: "list refuses an unknown state", args: []string{"proposals", "list", testDashboard, "--state", "stale"}, wantCode: 2, wantErr: "--state must be one of open, merged, discarded, all"},
		{name: "list refuses a dashboard that is not an id", args: []string{"proposals", "list", "nope"}, wantCode: 2, wantErr: `"nope" is not a valid id`},
		{name: "list needs a dashboard", args: []string{"proposals", "list"}, wantCode: 2, wantErr: "proposals list takes one dashboard id"},
		{
			name:       "show prints the summary, the changes, the conflicts and the allowed actions",
			args:       []string{"proposals", "show", testProposal},
			response:   detail,
			wantMethod: http.MethodGet,
			wantPath:   proposalPath,
			wantOut: []string{
				"Title Rename the errors panel",
				"State open",
				"Revision 2",
				"Based on version 3 (latest 5, behind main)",
				"Author Bo",
				"Via Claude Code",
				"Changes",
				"added panel p4",
				"changed setting title",
				"Conflicts",
				"panel p1: panel p1 was changed in both",
				"Allowed actions: update, merge, discard",
			},
		},
		{
			name:       "show of a closed proposal names the version it became",
			args:       []string{"proposals", "show", testProposal},
			response:   closed,
			wantMethod: http.MethodGet,
			wantPath:   proposalPath,
			wantOut:    []string{"State merged", "Merged as version 4", "Based on version 3"},
		},
		{
			name:       "show with --json prints the body",
			args:       []string{"proposals", "show", testProposal, "--json"},
			response:   detail,
			wantMethod: http.MethodGet,
			wantPath:   proposalPath,
			wantOut:    []string{`"conflicts"`},
		},
		{name: "show needs a proposal id", args: []string{"proposals", "show"}, wantCode: 2, wantErr: "proposals show takes one proposal id"},
		{
			name:       "merge sends the revision and each choice",
			args:       []string{"proposals", "merge", testProposal, "--revision", "2", "--take", "panel:p1=theirs", "--take", "setting:title=ours"},
			response:   proposalJSON("merged", 2, "6", false),
			wantMethod: http.MethodPost,
			wantPath:   proposalPath + "/merge",
			wantBody:   mergeTwo,
			wantOut:    []string{"Merged as version 6"},
		},
		{
			name:       "merge of a change already in the dashboard says so",
			args:       []string{"proposals", "merge", testProposal, "--revision", "2"},
			response:   proposalJSON("merged", 2, "", false),
			wantMethod: http.MethodPost,
			wantPath:   proposalPath + "/merge",
			wantBody:   `{"revision":2}`,
			wantOut:    []string{"Already in the dashboard"},
		},
		{
			name:       "merge with --quiet prints nothing",
			args:       []string{"-q", "proposals", "merge", testProposal, "--revision", "2"},
			response:   proposalJSON("merged", 2, "6", false),
			wantMethod: http.MethodPost,
			wantPath:   proposalPath + "/merge",
			wantBody:   `{"revision":2}`,
			wantExact:  new(string),
		},
		{
			name:       "merge with --json prints the body",
			args:       []string{"proposals", "merge", testProposal, "--revision", "2", "--json"},
			response:   proposalJSON("merged", 2, "6", false),
			wantMethod: http.MethodPost,
			wantPath:   proposalPath + "/merge",
			wantBody:   `{"revision":2}`,
			wantOut:    []string{`"merged_version":6`},
		},
		{name: "merge needs a revision", args: []string{"proposals", "merge", testProposal}, wantCode: 2, wantErr: "--revision <n> is required"},
		{name: "merge needs a proposal id", args: []string{"proposals", "merge", "--revision", "2"}, wantCode: 2, wantErr: "proposals merge takes one proposal id"},
		{
			name:       "discard posts and confirms",
			args:       []string{"proposals", "discard", testProposal},
			response:   proposalJSON("discarded", 2, "", false),
			wantMethod: http.MethodPost,
			wantPath:   proposalPath + "/discard",
			wantOut:    []string{"Discarded"},
		},
		{
			name:       "discard with --json prints the body",
			args:       []string{"proposals", "discard", testProposal, "--json"},
			response:   proposalJSON("discarded", 2, "", false),
			wantMethod: http.MethodPost,
			wantPath:   proposalPath + "/discard",
			wantOut:    []string{`"state":"discarded"`},
		},
		{name: "discard refuses an id that is not one", args: []string{"proposals", "discard", "nope"}, wantCode: 2, wantErr: `"nope" is not a valid id`},
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
			if tc.wantErr == "" && stderr.Len() > 0 {
				t.Fatalf("unexpected stderr %q", stderr.String())
			}
			if tc.wantErr != "" && !strings.Contains(stderr.String(), tc.wantErr) {
				t.Fatalf("stderr %q lacks %q", stderr.String(), tc.wantErr)
			}
		})
	}
}

func TestMergeTakeMustBeKindIdAndSide(t *testing.T) {
	for raw, want := range map[string]mergeChoice{
		"panel:p1=theirs":      {Kind: "panel", ID: "p1", Take: "theirs"},
		"variable:env=ours":    {Kind: "variable", ID: "env", Take: "ours"},
		"setting:title=theirs": {Kind: "setting", ID: "title", Take: "theirs"},
		"panel:a=b=ours":       {Kind: "panel", ID: "a=b", Take: "ours"},
	} {
		got, err := parseTake(raw)
		if err != nil || got != want {
			t.Errorf("parseTake(%q) = %+v, %v; want %+v", raw, got, err, want)
		}
	}
	for _, raw := range []string{"", "p1=theirs", "panel:p1", "panel:p1=", "panel:p1=mine", "panel:=ours", "folder:f1=ours", "panel=p1:ours"} {
		if _, err := parseTake(raw); err == nil || !strings.Contains(err.Error(), "is not <kind>:<id>=ours|theirs") {
			t.Errorf("parseTake(%q) err = %v, want a usage error", raw, err)
		}
	}
}

func TestMergeRefusalsShowTheCodeTheServerSent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = fmt.Fprint(w, `{"error":"the proposal has a newer revision","code":"proposal_changed","revision":3}`)
	}))
	t.Cleanup(server.Close)

	var stdout, stderr bytes.Buffer
	code := Main([]string{"--server", server.URL, "--token", "k", "proposals", "merge", testProposal, "--revision", "2"}, IO{In: strings.NewReader(""), Out: &stdout, Err: &stderr}, "test")
	if code != 1 || !strings.Contains(stderr.String(), "the proposal has a newer revision (proposal_changed)") {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
}
