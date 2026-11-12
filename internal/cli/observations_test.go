package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const (
	testObservation = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	testBlock       = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	testPanelBlock  = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	testObservedURL = "https://observe.example.com/o/" + testObservation
)

func observationJSON(blocks string) string {
	return fmt.Sprintf(`{"id":%q,"title":"Checkout 5xx","state":"open","summary":null,"url":%q,"updated_at":"2026-01-02T03:04:05Z","blocks":[%s]}`, testObservation, testObservedURL, blocks)
}

func blockJSON(id, kind, state string, position int, body string) string {
	return fmt.Sprintf(`{"id":%q,"kind":%q,"state":%q,"position":%d,"width":"full","version":3,"body":%s}`, id, kind, state, position, body)
}

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestObservationCommands(t *testing.T) {
	envelope := `{"observation":` + observationJSON("") + `}`
	shown := `{"observation":` + observationJSON(strings.Join([]string{
		blockJSON(testBlock, "text", "requested", 0, `{"markdown":"Errors started at 09:12\nthen a second line"}`),
		blockJSON(testPanelBlock, "panel", "live", 1, `{"panel":{"title":"Error rate","type":"timeseries"}}`),
	}, ",")) + `}`
	listed := `{"observations":[` + observationJSON("") + `],"next_cursor":"more"}`
	added := `{"block":` + blockJSON(testBlock, "text", "live", 0, `{"markdown":"x"}`) + `}`
	requested := `{"block":` + blockJSON(testBlock, "text", "requested", 0, `{"markdown":"x"}`) + `}`
	published := `{"rca":{"id":"dddddddd-dddd-4ddd-8ddd-dddddddddddd","observation_id":"` + testObservation + `","revision":2,"title":"Checkout 5xx","summary":null,"observation_state":"open","from":null,"to":null,"blocks":[],"created_by":{},"created_at":"2026-01-02T03:04:05Z"}}`
	textFile := writeTemp(t, "note.md", "From a file")
	panelFile := writeTemp(t, "panel.json", `{"type":"timeseries","title":"Errors"}`)
	notAnObject := writeTemp(t, "panel.json", `[1]`)

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
			name:       "new posts the title and prints the address",
			args:       []string{"o", "new", "Checkout", "5xx"},
			response:   envelope,
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/observations",
			wantBody:   `{"title":"Checkout 5xx"}`,
			wantOut:    []string{"Created Checkout 5xx " + testObservedURL},
		},
		{
			name:       "new with a folder, a range and text sends a first block",
			args:       []string{"o", "new", "Checkout", "--folder", testFolderTarget, "--from", "2026-01-01T00:00:00Z", "--to", "2026-01-01T01:00:00Z", "--text", "Look at checkout"},
			response:   envelope,
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/observations",
			wantBody:   `{"first_block":{"body":{"markdown":"Look at checkout"},"kind":"text"},"folder_id":"` + testFolderTarget + `","from":"2026-01-01T00:00:00Z","title":"Checkout","to":"2026-01-01T01:00:00Z"}`,
			wantOut:    []string{"Created Checkout"},
		},
		{
			name:       "new reads the first block from a file",
			args:       []string{"o", "new", "Checkout", "--text-file", textFile},
			response:   envelope,
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/observations",
			wantBody:   `{"first_block":{"body":{"markdown":"From a file"},"kind":"text"},"title":"Checkout"}`,
			wantOut:    []string{"Created Checkout"},
		},
		{
			name:       "new still prints the address with --quiet",
			args:       []string{"-q", "o", "new", "Checkout"},
			response:   envelope,
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/observations",
			wantBody:   `{"title":"Checkout"}`,
			wantOut:    []string{testObservedURL},
		},
		{
			name:     "new refuses --text with --text-file",
			args:     []string{"o", "new", "Checkout", "--text", "a", "--text-file", textFile},
			wantCode: 2,
			wantErr:  "--text and --text-file cannot be used together",
		},
		{
			name:     "new refuses a range with one end",
			args:     []string{"o", "new", "Checkout", "--from", "2026-01-01T00:00:00Z"},
			wantCode: 2,
			wantErr:  "--from and --to are given together",
		},
		{
			name:     "new refuses a missing text file",
			args:     []string{"o", "new", "Checkout", "--text-file", filepath.Join(t.TempDir(), "missing.md")},
			wantCode: 2,
			wantErr:  "cannot read",
		},
		{
			name:     "new needs a title",
			args:     []string{"o", "new"},
			wantCode: 2,
			wantErr:  "o new takes a title",
		},
		{
			name:       "show prints the observation and marks a requested block",
			args:       []string{"o", "show", testObservation},
			response:   shown,
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/observations/" + testObservation,
			wantOut: []string{
				"Title Checkout 5xx",
				"State open",
				"Summary -",
				"URL " + testObservedURL,
				"POS ID KIND STATE VERSION WIDTH CONTENT",
				"0 " + testBlock + " text requested 3 full Errors started at 09:12",
				"1 " + testPanelBlock + " panel live 3 full Error rate",
			},
		},
		{
			name:       "show with --json prints the body",
			args:       []string{"o", "show", "--json", testObservation},
			response:   shown,
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/observations/" + testObservation,
			wantExact:  new(shown + "\n"),
		},
		{
			name:     "show refuses an id that is not one",
			args:     []string{"o", "show", "nope"},
			wantCode: 2,
			wantErr:  `"nope" is not a valid id`,
		},
		{
			name:       "add text posts a text block",
			args:       []string{"o", "add", testObservation, "--text", "Hello"},
			response:   added,
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/observations/" + testObservation + "/blocks",
			wantBody:   `{"body":{"markdown":"Hello"},"kind":"text"}`,
			wantOut:    []string{"Added text block " + testBlock + " (live)"},
		},
		{
			name:       "add says when the block is requested",
			args:       []string{"o", "add", testObservation, "--text-file", textFile, "--half", "--position", "0"},
			response:   requested,
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/observations/" + testObservation + "/blocks",
			wantBody:   `{"body":{"markdown":"From a file"},"kind":"text","position":0,"width":"half"}`,
			wantOut:    []string{"Added text block " + testBlock + " (requested)"},
		},
		{
			name:       "add a panel sends the file and its range",
			args:       []string{"o", "add", testObservation, "--panel-file", panelFile, "--from", "2026-01-01T00:00:00Z", "--to", "2026-01-01T01:00:00Z"},
			response:   strings.Replace(added, `"text"`, `"panel"`, 1),
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/observations/" + testObservation + "/blocks",
			wantBody:   `{"body":{"from":"2026-01-01T00:00:00Z","panel":{"type":"timeseries","title":"Errors"},"to":"2026-01-01T01:00:00Z"},"kind":"panel"}`,
			wantOut:    []string{"Added panel block " + testBlock + " (live)"},
		},
		{
			name:       "add a snapshot names the panel block",
			args:       []string{"o", "add", testObservation, "--snapshot-of", testPanelBlock},
			response:   strings.Replace(added, `"text"`, `"snapshot"`, 1),
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/observations/" + testObservation + "/blocks",
			wantBody:   `{"from_block_id":"` + testPanelBlock + `","kind":"snapshot"}`,
			wantOut:    []string{"Added snapshot block " + testBlock + " (live)"},
		},
		{
			name:       "add with --json prints the body",
			args:       []string{"o", "add", testObservation, "--text", "Hello", "--json"},
			response:   added,
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/observations/" + testObservation + "/blocks",
			wantBody:   `{"body":{"markdown":"Hello"},"kind":"text"}`,
			wantExact:  new(added + "\n"),
		},
		{
			name:     "add needs exactly one source",
			args:     []string{"o", "add", testObservation},
			wantCode: 2,
			wantErr:  "give exactly one of --text, --text-file, --panel-file and --snapshot-of",
		},
		{
			name:     "add refuses two sources",
			args:     []string{"o", "add", testObservation, "--text", "a", "--snapshot-of", testPanelBlock},
			wantCode: 2,
			wantErr:  "give exactly one of",
		},
		{
			name:     "add refuses --text with --text-file",
			args:     []string{"o", "add", testObservation, "--text", "a", "--text-file", textFile},
			wantCode: 2,
			wantErr:  "--text and --text-file cannot be used together",
		},
		{
			name:     "add refuses a range on a text block",
			args:     []string{"o", "add", testObservation, "--text", "a", "--from", "2026-01-01T00:00:00Z"},
			wantCode: 2,
			wantErr:  "--from and --to go with --panel-file",
		},
		{
			name:     "add refuses a panel file that is not an object",
			args:     []string{"o", "add", testObservation, "--panel-file", notAnObject},
			wantCode: 2,
			wantErr:  "must hold one JSON object",
		},
		{
			name:       "resolve posts the summary",
			args:       []string{"o", "resolve", testObservation, "--summary", "A bad deploy"},
			response:   envelope,
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/observations/" + testObservation + "/resolve",
			wantBody:   `{"summary":"A bad deploy"}`,
			wantOut:    []string{"Resolved Checkout 5xx"},
		},
		{
			name:     "resolve needs a summary",
			args:     []string{"o", "resolve", testObservation},
			wantCode: 2,
			wantErr:  "--summary <line> is required",
		},
		{
			name:       "list prints one row per observation",
			args:       []string{"o", "list"},
			response:   listed,
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/observations",
			wantQuery:  "limit=50",
			wantOut:    []string{"ID TITLE STATE UPDATED", testObservation + " Checkout 5xx open 2026-01-02T03:04Z"},
			wantErr:    "more results: --cursor more",
		},
		{
			name:       "list filters by folder and state",
			args:       []string{"o", "list", "--folder", testFolderTarget, "--state", "resolved", "--limit", "5"},
			response:   `{"observations":[],"next_cursor":null}`,
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/observations",
			wantQuery:  "folder_id=" + testFolderTarget + "&limit=5&state=resolved",
		},
		{
			name:       "list with --json prints the body",
			args:       []string{"o", "list", "--json"},
			response:   listed,
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/observations",
			wantQuery:  "limit=50",
			wantExact:  new(listed + "\n"),
			wantErr:    "",
		},
		{
			name:     "list refuses a state that is neither",
			args:     []string{"o", "list", "--state", "closed"},
			wantCode: 2,
			wantErr:  "--state must be open or resolved",
		},
		{
			name:       "keep posts the version",
			args:       []string{"o", "keep", testBlock, "--version", "3"},
			response:   added,
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/blocks/" + testBlock + "/keep",
			wantBody:   `{"version":3}`,
			wantOut:    []string{"Kept text block " + testBlock},
		},
		{
			name:     "keep needs a version",
			args:     []string{"o", "keep", testBlock},
			wantCode: 2,
			wantErr:  "--version <n> is required",
		},
		{
			name:       "discard posts to the block",
			args:       []string{"o", "discard", testBlock},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/blocks/" + testBlock + "/discard",
			wantOut:    []string{"Discarded block " + testBlock},
		},
		{
			name:       "discard with --json prints the empty response",
			args:       []string{"o", "discard", testBlock, "--json"},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/blocks/" + testBlock + "/discard",
			wantExact:  new("\n"),
		},
		{
			name:       "publish posts to the RCAs of the observation and names the revision",
			args:       []string{"o", "publish", testObservation},
			response:   published,
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/observations/" + testObservation + "/rcas",
			wantOut:    []string{"Published revision 2 of Checkout 5xx"},
		},
		{
			name:       "publish with --json prints the response unchanged",
			args:       []string{"o", "publish", testObservation, "--json"},
			response:   published,
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/observations/" + testObservation + "/rcas",
			wantExact:  new(published + "\n"),
		},
		{
			name:     "publish needs one observation id",
			args:     []string{"o", "publish"},
			wantCode: 2,
			wantErr:  "o publish takes one observation id",
		},
		{
			name:     "publish refuses an id that is not a UUID",
			args:     []string{"o", "publish", "checkout"},
			wantCode: 2,
			wantErr:  `"checkout" is not a valid id`,
		},
		{
			name:     "an unknown subcommand is a usage error",
			args:     []string{"o", "frobnicate"},
			wantCode: 2,
			wantErr:  `unknown command "frobnicate" for observe o`,
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
				if len(got) != 1 || got[0].method != want.method || got[0].path != want.path || got[0].query != want.query || !sameJSON(got[0].body, want.body) {
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

func sameJSON(got, want string) bool {
	if got == want {
		return true
	}
	var a, b any
	return json.Unmarshal([]byte(got), &a) == nil && json.Unmarshal([]byte(want), &b) == nil && reflect.DeepEqual(a, b)
}

func TestObservationErrorsPrintTheServersMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = fmt.Fprint(w, `{"error":"the block changed since you read it","code":"version_conflict"}`)
	}))
	t.Cleanup(server.Close)
	var stdout, stderr bytes.Buffer
	code := Main([]string{"--server", server.URL, "--token", "k", "o", "keep", testBlock, "--version", "2"}, IO{In: strings.NewReader(""), Out: &stdout, Err: &stderr}, "test")
	if code != 1 || !strings.Contains(stderr.String(), "the block changed since you read it") || stdout.Len() != 0 {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
}
