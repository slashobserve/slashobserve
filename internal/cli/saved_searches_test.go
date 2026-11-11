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

const testSavedSearch = "abababab-abab-4bab-8bab-abababababab"

func savedSearchJSON(id, kind, title, folder, datasource, url string) string {
	return fmt.Sprintf(`{"id":%q,"kind":%q,"title":%q,"datasource_uid":%q,"query":null,"filters":{},"from":"now-1h","to":"now","folder":{"id":%q,"title":%q,"general":false,"personal":false},"permissions":{},"actor":{"label":"Ada"},"created_at":"2026-01-02T03:04:05Z","updated_at":"2026-01-02T03:04:05Z","url":%q}`,
		id, kind, title, datasource, testFolderTarget, folder, url)
}

func TestSavedSearchesCommands(t *testing.T) {
	listBody := `{"saved_searches":[` + strings.Join([]string{
		savedSearchJSON(testSavedSearch, "logs", "Checkout errors", "Payments", "loki", "/logs?ds=loki"),
		savedSearchJSON("cdcdcdcd-cdcd-4dcd-8dcd-cdcdcdcdcdcd", "traces", "Slow checkout", "My Dashboards", "tempo", "/traces?ds=tempo"),
	}, ",") + `],"next_cursor":"more"}`
	saved := savedSearchJSON(testSavedSearch, "logs", "Checkout errors", "Payments", "loki", "https://observe.example.com/logs?ds=loki&q=x")
	const savedURL = "https://observe.example.com/logs?ds=loki&q=x"

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
			name:       "list prints one row per saved search",
			args:       []string{"saved-searches", "list"},
			response:   listBody,
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/saved-searches",
			wantQuery:  "limit=50",
			wantOut: []string{
				"ID KIND TITLE FOLDER DATASOURCE UPDATED",
				testSavedSearch + " logs Checkout errors Payments loki 2026-01-02T03:04Z",
				"traces Slow checkout My Dashboards tempo",
			},
			wantErr: "more results: --cursor more",
		},
		{
			name:       "list by kind and folder",
			args:       []string{"saved-searches", "list", "--kind", "traces", "--folder", testFolderTarget, "--limit", "5"},
			response:   `{"saved_searches":[],"next_cursor":null}`,
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/saved-searches",
			wantQuery:  "folder_id=" + testFolderTarget + "&kind=traces&limit=5",
		},
		{
			name:       "list with --json prints the body",
			args:       []string{"saved-searches", "list", "--json"},
			response:   listBody,
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/saved-searches",
			wantQuery:  "limit=50",
			wantOut:    []string{`"saved_searches"`},
		},
		{
			name:     "list refuses an unknown kind",
			args:     []string{"saved-searches", "list", "--kind", "metrics"},
			wantCode: 2,
			wantErr:  `--kind must be logs or traces, not "metrics"`,
		},
		{
			name:     "list refuses a folder that is not an id",
			args:     []string{"saved-searches", "list", "--folder", "nope"},
			wantCode: 2,
			wantErr:  `"nope" is not a valid id`,
		},
		{
			name:       "save a logs search prints the address",
			args:       []string{"saved-searches", "save", "--kind", "logs", "--title", "Checkout errors", "--datasource", "loki", "--query", `{app="checkout"}`, "--columns", "timestamp,line", "--folder", testFolderTarget},
			response:   saved,
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/saved-searches",
			wantBody:   `{"datasource_uid":"loki","filters":{"columns":["timestamp","line"]},"folder_id":"` + testFolderTarget + `","from":"now-1h","kind":"logs","query":"{app=\"checkout\"}","title":"Checkout errors","to":"now"}`,
			wantExact:  new(savedURL + "\n"),
		},
		{
			name:       "save a trace search sends its filters",
			args:       []string{"saved-searches", "save", "--kind", "traces", "--title", "Slow", "--datasource", "tempo", "--service", "checkout", "--span", "POST /pay", "--min", "250ms", "--max", "2s", "--errors", "--from", "now-6h", "--to", "now-1h"},
			response:   saved,
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/saved-searches",
			wantBody:   `{"datasource_uid":"tempo","filters":{"errors":true,"max":"2s","min":"250ms","service":"checkout","span":"POST /pay"},"from":"now-6h","kind":"traces","title":"Slow","to":"now-1h"}`,
			wantExact:  new(savedURL + "\n"),
		},
		{
			name:       "save with --quiet still prints the address",
			args:       []string{"-q", "saved-searches", "save", "--kind", "traces", "--title", "Slow", "--datasource", "tempo"},
			response:   saved,
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/saved-searches",
			wantBody:   `{"datasource_uid":"tempo","from":"now-1h","kind":"traces","title":"Slow","to":"now"}`,
			wantExact:  new(savedURL + "\n"),
		},
		{
			name:       "save with --json prints the body",
			args:       []string{"saved-searches", "save", "--json", "--kind", "traces", "--title", "Slow", "--datasource", "tempo"},
			response:   saved,
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/saved-searches",
			wantBody:   `{"datasource_uid":"tempo","from":"now-1h","kind":"traces","title":"Slow","to":"now"}`,
			wantOut:    []string{`"datasource_uid":"loki"`},
		},
		{
			name:     "save refuses trace flags on a logs search",
			args:     []string{"saved-searches", "save", "--kind", "logs", "--title", "T", "--datasource", "loki", "--query", "x", "--errors"},
			wantCode: 2,
			wantErr:  "--errors applies to traces searches, not logs",
		},
		{
			name:     "save refuses logs flags on a trace search",
			args:     []string{"saved-searches", "save", "--kind", "traces", "--title", "T", "--datasource", "tempo", "--columns", "a"},
			wantCode: 2,
			wantErr:  "--columns applies to logs searches, not traces",
		},
		{
			name:     "save needs a kind",
			args:     []string{"saved-searches", "save", "--title", "T", "--datasource", "loki"},
			wantCode: 2,
			wantErr:  "--kind logs|traces is required",
		},
		{
			name:     "save refuses an unknown kind",
			args:     []string{"saved-searches", "save", "--kind", "metrics", "--title", "T", "--datasource", "loki"},
			wantCode: 2,
			wantErr:  `--kind must be logs or traces, not "metrics"`,
		},
		{
			name:     "save needs a title",
			args:     []string{"saved-searches", "save", "--kind", "logs", "--datasource", "loki", "--query", "x"},
			wantCode: 2,
			wantErr:  "--title <title> is required",
		},
		{
			name:     "save needs a datasource",
			args:     []string{"saved-searches", "save", "--kind", "logs", "--title", "T", "--query", "x"},
			wantCode: 2,
			wantErr:  "--datasource <uid> is required",
		},
		{
			name:     "save refuses a folder that is not an id",
			args:     []string{"saved-searches", "save", "--kind", "logs", "--title", "T", "--datasource", "loki", "--folder", "nope"},
			wantCode: 2,
			wantErr:  `"nope" is not a valid id`,
		},
		{
			name:       "rename joins the words of the title",
			args:       []string{"saved-searches", "rename", testSavedSearch, "Checkout", "errors", "v2"},
			response:   savedSearchJSON(testSavedSearch, "logs", "Checkout errors v2", "Payments", "loki", "/logs"),
			wantMethod: http.MethodPatch,
			wantPath:   "/api/v1/saved-searches/" + testSavedSearch,
			wantBody:   `{"title":"Checkout errors v2"}`,
			wantOut:    []string{"Renamed saved search " + testSavedSearch + " to Checkout errors v2"},
		},
		{
			name:       "rename with --quiet prints nothing",
			args:       []string{"-q", "saved-searches", "rename", testSavedSearch, "New"},
			response:   saved,
			wantMethod: http.MethodPatch,
			wantPath:   "/api/v1/saved-searches/" + testSavedSearch,
			wantBody:   `{"title":"New"}`,
			wantExact:  new(string),
		},
		{
			name:       "rename with --json prints the body",
			args:       []string{"saved-searches", "rename", "--json", testSavedSearch, "New"},
			response:   saved,
			wantMethod: http.MethodPatch,
			wantPath:   "/api/v1/saved-searches/" + testSavedSearch,
			wantBody:   `{"title":"New"}`,
			wantOut:    []string{`"kind":"logs"`},
		},
		{
			name:     "rename needs a title",
			args:     []string{"saved-searches", "rename", testSavedSearch},
			wantCode: 2,
			wantErr:  "rename needs a title",
		},
		{
			name:     "rename refuses an id that is not one",
			args:     []string{"saved-searches", "rename", "nope", "New"},
			wantCode: 2,
			wantErr:  `"nope" is not a valid id`,
		},
		{
			name:       "move names the saved search and the folder",
			args:       []string{"saved-searches", "move", testSavedSearch, "--folder", testFolderTarget},
			response:   saved,
			wantMethod: http.MethodPatch,
			wantPath:   "/api/v1/saved-searches/" + testSavedSearch,
			wantBody:   `{"folder_id":"` + testFolderTarget + `"}`,
			wantOut:    []string{"Moved saved search Checkout errors to Payments"},
		},
		{
			name:       "move with --quiet prints nothing",
			args:       []string{"-q", "saved-searches", "move", testSavedSearch, "--folder", testFolderTarget},
			response:   saved,
			wantMethod: http.MethodPatch,
			wantPath:   "/api/v1/saved-searches/" + testSavedSearch,
			wantBody:   `{"folder_id":"` + testFolderTarget + `"}`,
			wantExact:  new(string),
		},
		{
			name:       "move with --json prints the body",
			args:       []string{"saved-searches", "move", "--json", testSavedSearch, "--folder", testFolderTarget},
			response:   saved,
			wantMethod: http.MethodPatch,
			wantPath:   "/api/v1/saved-searches/" + testSavedSearch,
			wantBody:   `{"folder_id":"` + testFolderTarget + `"}`,
			wantOut:    []string{`"folder"`},
		},
		{
			name:     "move needs a folder",
			args:     []string{"saved-searches", "move", testSavedSearch},
			wantCode: 2,
			wantErr:  "--folder <folder-id> is required",
		},
		{
			name:     "move needs a saved search id",
			args:     []string{"saved-searches", "move", "--folder", testFolderTarget},
			wantCode: 2,
			wantErr:  "saved-searches move takes one saved search id",
		},
		{
			name:       "delete says what it did",
			args:       []string{"saved-searches", "delete", testSavedSearch},
			wantMethod: http.MethodDelete,
			wantPath:   "/api/v1/saved-searches/" + testSavedSearch,
			wantOut:    []string{"Deleted saved search " + testSavedSearch},
		},
		{
			name:       "delete with --quiet prints nothing",
			args:       []string{"-q", "saved-searches", "delete", testSavedSearch},
			wantMethod: http.MethodDelete,
			wantPath:   "/api/v1/saved-searches/" + testSavedSearch,
			wantExact:  new(string),
		},
		{
			name:       "delete with --json prints nothing",
			args:       []string{"saved-searches", "delete", "--json", testSavedSearch},
			wantMethod: http.MethodDelete,
			wantPath:   "/api/v1/saved-searches/" + testSavedSearch,
			wantExact:  new(string),
		},
		{
			name:     "delete refuses an id that is not one",
			args:     []string{"saved-searches", "delete", "nope"},
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
				if r.Method == http.MethodDelete {
					w.WriteHeader(http.StatusNoContent)
					return
				}
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
