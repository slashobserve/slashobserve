package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type askFake struct {
	server      *httptest.Server
	kind        string
	draftStatus int
	draft       string
	frames      string
	mu          sync.Mutex
	drafts      []map[string]any
	queries     []map[string]any
}

func newAskFake(t *testing.T, kind, draft, frames string) *askFake {
	t.Helper()
	f := &askFake{kind: kind, draftStatus: http.StatusOK, draft: draft, frames: frames}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		var body map[string]any
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &body)
		switch r.URL.Path {
		case "/api/v1/datasources":
			_, _ = fmt.Fprintf(w, `{"datasources":[{"uid":"ds","type":%q,"name":"D","healthy":true,"checked_at":"x","dashboard_count":0}]}`, f.kind)
		case "/api/v1/datasources/ds/draft-query":
			f.drafts = append(f.drafts, body)
			w.WriteHeader(f.draftStatus)
			_, _ = io.WriteString(w, f.draft)
		case "/api/v1/datasources/ds/query":
			f.queries = append(f.queries, body)
			_, _ = io.WriteString(w, f.frames)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":"not found","code":"not_found"}`)
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *askFake) run(command string, args ...string) (string, string, int) {
	return runCLI(append([]string{"--server", f.server.URL, "--token", "k", command, "--datasource", "ds"}, args...)...)
}

func TestPrintDraftWritesTheQueryExplanationAndWarnings(t *testing.T) {
	var stdout, stderr bytes.Buffer
	out := &output{out: &stdout, err: &stderr}
	printDraft(out, draftAnswer{Query: "{service_name=\"checkout\"}", Language: "logql", Explanation: "Checkout lines.", Warnings: []string{"No matches in the last 15 minutes.", "two"}})
	want := "Query: {service_name=\"checkout\"}\nCheckout lines.\nWarning: No matches in the last 15 minutes.\nWarning: two\n"
	if stderr.String() != want || stdout.Len() != 0 {
		t.Fatalf("stderr %q, stdout %q", stderr.String(), stdout.String())
	}
}

func TestPrintDraftWritesOnlyTheSetFilters(t *testing.T) {
	cases := []struct {
		filters traceFilters
		want    string
	}{
		{traceFilters{Service: "checkout", Min: "500ms", Errors: true}, "Filters: service=checkout min=500ms errors\n"},
		{traceFilters{Span: "GET /cart", Max: "2s"}, "Filters: span=GET /cart max=2s\n"},
		{traceFilters{}, "Filters: \n"},
	}
	for _, c := range cases {
		var stderr bytes.Buffer
		printDraft(&output{out: io.Discard, err: &stderr}, draftAnswer{Language: "filters", TraceFilters: &c.filters, Explanation: "E."})
		if got := stderr.String(); got != c.want+"E.\n" {
			t.Errorf("%+v: %q", c.filters, got)
		}
	}
}

func TestPrintDraftReplacesControlCharacters(t *testing.T) {
	var stderr bytes.Buffer
	printDraft(&output{out: io.Discard, err: &stderr}, draftAnswer{Query: "SELECT 1\nFROM t", Explanation: "a\x1b[31mb"})
	if got := stderr.String(); got != "Query: SELECT 1 FROM t\na [31mb\n" {
		t.Fatalf("stderr %q", got)
	}
}

func TestQuietHidesTheDraft(t *testing.T) {
	var stderr bytes.Buffer
	printDraft(&output{out: io.Discard, err: &stderr, quiet: true}, draftAnswer{Query: "q", Explanation: "E.", Warnings: []string{"w"}})
	if stderr.Len() != 0 {
		t.Fatalf("stderr %q", stderr.String())
	}
}

func TestAskFailuresExitWithTheServerMessage(t *testing.T) {
	cases := []struct {
		status int
		body   string
		want   string
	}{
		{http.StatusUnprocessableEntity, `{"error":"This Loki datasource holds logs, not CPU metrics.","code":"cannot_translate","reason":"This Loki datasource holds logs, not CPU metrics."}`, "This Loki datasource holds logs, not CPU metrics."},
		{http.StatusServiceUnavailable, `{"error":"No model is configured on this server, so Ask is off.","code":"agent_unavailable"}`, "No model is configured on this server, so Ask is off."},
	}
	for _, command := range []string{"logs", "traces"} {
		for _, c := range cases {
			f := newAskFake(t, "clickhouse", c.body, `{"frames":[]}`)
			f.draftStatus = c.status
			_, stderr, code := f.run(command, "--ask", "cpu of the payments pod")
			if code != 1 || !strings.Contains(stderr, c.want) || len(f.queries) != 0 {
				t.Errorf("%s %d: exit %d, stderr %q, queries %d", command, c.status, code, stderr, len(f.queries))
			}
		}
	}
}
