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

func TestSearchCommand(t *testing.T) {
	page := `{"results":[` +
		`{"kind":"dashboard","id":"d1","title":"Checkout latency","snippet":[{"text":"Slow ","match":false},{"text":"checkout","match":true},{"text":" requests","match":false}],"url":"https://observe.example.com/d/d1","folder":{"id":"f1","title":"Payments"},"target":null,"thread_id":null,"updated_at":"2026-01-02T03:04:05Z"},` +
		`{"kind":"annotation","id":"a1","title":"Deploy","snippet":[],"url":null,"folder":null,"target":null,"thread_id":null,"updated_at":"2026-01-02T03:04:05Z"}` +
		`],"next_cursor":"more"}`
	empty := `{"results":[],"next_cursor":null}`
	noResults := "No results.\n"

	cases := []struct {
		name      string
		args      []string
		response  string
		wantCode  int
		wantQuery string
		wantOut   []string
		wantExact *string
		wantErr   string
	}{
		{
			name:      "the words are joined into one query",
			args:      []string{"search", "checkout", "latency"},
			response:  empty,
			wantQuery: "limit=20&q=checkout+latency",
		},
		{
			name:      "a quoted phrase stays one argument",
			args:      []string{"search", `"connection reset"`, "--", "-staging"},
			response:  empty,
			wantQuery: "limit=20&q=%22connection+reset%22+-staging",
		},
		{
			name:      "kind repeats",
			args:      []string{"search", "checkout", "--kind", "comment", "--kind", "saved_search", "--limit", "5", "--cursor", "c1"},
			response:  empty,
			wantQuery: "cursor=c1&kind=comment&kind=saved_search&limit=5&q=checkout",
		},
		{
			name:      "observation is a kind",
			args:      []string{"search", "heron", "--kind", "observation"},
			response:  empty,
			wantQuery: "kind=observation&limit=20&q=heron",
		},
		{
			name:      "finding is still accepted as a kind",
			args:      []string{"search", "heron", "--kind", "finding"},
			response:  empty,
			wantQuery: "kind=finding&limit=20&q=heron",
		},
		{
			name:      "results print one line each with the snippet below",
			args:      []string{"search", "checkout"},
			response:  page,
			wantQuery: "limit=20&q=checkout",
			wantOut: []string{
				"dashboard Checkout latency Payments https://observe.example.com/d/d1",
				"Slow checkout requests",
				"annotation Deploy no folder no page",
			},
			wantErr: "more results: --cursor more",
		},
		{
			name:      "an empty answer says so",
			args:      []string{"search", "nothing"},
			response:  empty,
			wantQuery: "limit=20&q=nothing",
			wantExact: &noResults,
		},
		{
			name:      "json prints the body",
			args:      []string{"search", "checkout", "--json"},
			response:  page,
			wantQuery: "limit=20&q=checkout",
			wantOut:   []string{`"results"`, `"next_cursor":"more"`},
		},
		{name: "no words", args: []string{"search"}, wantCode: 1, wantErr: "requires at least 1 arg"},
		{name: "limit 0", args: []string{"search", "a", "--limit", "0"}, wantCode: 2, wantErr: "--limit must be 1 to 50"},
		{name: "limit 51", args: []string{"search", "a", "--limit", "51"}, wantCode: 2, wantErr: "--limit must be 1 to 50"},
		{name: "unknown kind", args: []string{"search", "a", "--kind", "folder"}, wantCode: 2, wantErr: `--kind must be one of dashboard, annotation, observation, comment, saved_search, finding, not "folder"`},
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
			if tc.wantQuery == "" {
				if len(got) != 0 {
					t.Fatalf("expected no request, got %v", got)
				}
			} else if want := (recordedRequest{http.MethodGet, "/api/v1/search", tc.wantQuery, ""}); len(got) != 1 || got[0] != want {
				t.Fatalf("requests %+v, want one %+v", got, want)
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
