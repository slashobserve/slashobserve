package cli

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const auditEventID = "019a8f3c-2a1e-7d6f-9b7e-9d395a7f0f0c"

func auditEventJSON(id, action, outcome string) string {
	return fmt.Sprintf(`{"id":%q,"occurred_at":"2026-11-21T10:04:05.123Z","action":%q,"outcome":%q,"reason":null,"status":403,"actor_kind":"user","actor_name":"Ada","initiator":"agent","agent_name":"Claude","origin":"mcp","tool":"update_panels","route":null,"target_kind":"dashboard","target_id":"d1","target_name":"Checkout","request_id":null,"run_id":null,"client_ip":"10.0.0.1"}`, id, action, outcome)
}

type auditCall struct {
	path   string
	query  url.Values
	accept string
}

func runAudit(t *testing.T, status int, header http.Header, reply string, args ...string) (auditCall, string, string, int) {
	t.Helper()
	var call auditCall
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call = auditCall{path: r.URL.Path, query: r.URL.Query(), accept: r.Header.Get("Accept")}
		for k, v := range header {
			w.Header()[k] = v
		}
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "application/json")
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, reply)
	}))
	t.Cleanup(server.Close)
	var stdout, stderr bytes.Buffer
	code := Main(append([]string{"--server", server.URL, "--token", "k"}, args...), IO{In: strings.NewReader(""), Out: &stdout, Err: &stderr}, "test")
	return call, stdout.String(), stderr.String(), code
}

func TestAuditSearchSendsTheWindowAndPrintsATable(t *testing.T) {
	reply := `{"events":[` + auditEventJSON(auditEventID, "dashboard.update", "denied") + `],"next_cursor":"c2","since":"x","until":"y","form":"kql"}`
	call, stdout, stderr, code := runAudit(t, http.StatusOK, nil, reply, "audit", "search", "--query", "outcome:denied", "--from", "now-7d", "--to", "now-1d", "--limit", "5")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if call.path != "/api/v1/audit/events" || call.query.Get("q") != "outcome:denied" || call.query.Get("since") != "now-7d" || call.query.Get("until") != "now-1d" || call.query.Get("limit") != "5" {
		t.Fatalf("call = %+v", call)
	}
	for _, want := range []string{"WHEN", "2026-11-21T10:04:05Z", "Claude for Ada", "mcp:update_panels", "dashboard.update", "denied", "dashboard Checkout", auditEventID, "c2"} {
		if !strings.Contains(stdout+stderr, want) {
			t.Errorf("output lacks %q:\n%s%s", want, stdout, stderr)
		}
	}
}

func TestAuditSearchPrintsTheBodyWithJSON(t *testing.T) {
	reply := `{"events":[],"next_cursor":null}`
	call, stdout, _, code := runAudit(t, http.StatusOK, nil, reply, "--json", "audit", "search")
	if code != 0 || strings.TrimSpace(stdout) != reply || call.query.Has("q") || call.query.Has("since") {
		t.Fatalf("exit %d, call %+v, stdout %q", code, call, stdout)
	}
}

func TestAuditSearchSaysWhenNothingMatches(t *testing.T) {
	_, stdout, stderr, code := runAudit(t, http.StatusOK, nil, `{"events":[],"next_cursor":null}`, "audit", "search")
	if code != 0 || !strings.Contains(stdout+stderr, "No audit events match in the window.") {
		t.Fatalf("exit %d: %s%s", code, stdout, stderr)
	}
}

func TestAuditShowFacetsHistogramAndFieldsReadTheirRoutes(t *testing.T) {
	for _, tc := range []struct {
		args  []string
		reply string
		path  string
		query url.Values
		want  []string
	}{
		{[]string{"audit", "show", auditEventID}, `{"event":` + auditEventJSON(auditEventID, "auth.login", "success") + `,"same_request":[],"same_run":[]}`,
			"/api/v1/audit/events/" + auditEventID, url.Values{}, []string{"auth.login", "10.0.0.1"}},
		{[]string{"audit", "facets", "actor_name", "--query", "outcome:denied", "--limit", "3"}, `{"field":"actor_name","values":[{"value":"Ada","count":7}]}`,
			"/api/v1/audit/facets", url.Values{"field": {"actor_name"}, "q": {"outcome:denied"}, "limit": {"3"}}, []string{"ACTOR_NAME", "Ada", "7"}},
		{[]string{"audit", "histogram", "--buckets", "2"}, `{"step_ms":3600000,"total":3,"buckets":[{"time":"2026-11-21T10:00:00Z","count":1},{"time":"2026-11-21T11:00:00Z","count":2}]}`,
			"/api/v1/audit/histogram", url.Values{"buckets": {"2"}}, []string{"2026-11-21T11:00:00Z", "3 events, one bucket per 1h0m0s."}},
		{[]string{"audit", "aggregate", "--query", `{"aggs":{"o":{"terms":{"field":"origin"}}}}`}, `{"levels":[{"name":"o"}],"metrics":[],"rows":[{"keys":["mcp"],"doc_count":4,"values":[]}],"truncated":false}`,
			"/api/v1/audit/aggregate", url.Values{"q": {`{"aggs":{"o":{"terms":{"field":"origin"}}}}`}}, []string{"DOC_COUNT", "mcp", "4"}},
		{[]string{"audit", "fields"}, `{"fields":[{"name":"actor_name","alias":"actor","type":"text","description":"The actor's name","values":null}]}`,
			"/api/v1/audit/fields", url.Values{}, []string{"actor_name", "actor", "The actor's name"}},
	} {
		call, stdout, stderr, code := runAudit(t, http.StatusOK, nil, tc.reply, tc.args...)
		if code != 0 {
			t.Errorf("%v: exit %d: %s", tc.args, code, stderr)
			continue
		}
		if call.path != tc.path || call.query.Encode() != tc.query.Encode() {
			t.Errorf("%v: call %+v", tc.args, call)
		}
		for _, want := range tc.want {
			if !strings.Contains(stdout+stderr, want) {
				t.Errorf("%v: output lacks %q:\n%s%s", tc.args, want, stdout, stderr)
			}
		}
	}
}

func TestAuditUsageErrorsExitTwo(t *testing.T) {
	for _, args := range [][]string{
		{"audit", "aggregate", "--query", "outcome:denied"},
		{"audit", "facets"},
		{"audit", "facets", "actor_name", "--limit", "101"},
		{"audit", "histogram", "--buckets", "0"},
		{"audit", "export", "--format", "xml"},
		{"audit", "search", "--limit", "1001"},
		{"audit", "show", "not-an-id"},
	} {
		if _, _, stderr, code := runAudit(t, http.StatusOK, nil, `{}`, args...); code != 2 {
			t.Errorf("%v: exit %d: %s", args, code, stderr)
		}
	}
}

func TestAuditShowsTheServersParseError(t *testing.T) {
	reply := `{"error":"expected a value after \"status:\"","code":"query_invalid","start":19,"end":27}`
	_, _, stderr, code := runAudit(t, http.StatusUnprocessableEntity, nil, reply, "audit", "search", "--query", "outcome:denied and status:")
	if code != 1 || !strings.Contains(stderr, `expected a value after "status:"`) {
		t.Fatalf("exit %d: %s", code, stderr)
	}
}

func TestAuditExportStreamsToStandardOutput(t *testing.T) {
	body := `{"id":"a"}` + "\n" + `{"id":"b"}` + "\n"
	header := http.Header{"Content-Type": {"application/x-ndjson"}, exportRowLimitKey: {"1000000"}}
	call, stdout, stderr, code := runAudit(t, http.StatusOK, header, body, "audit", "export", "--query", "origin:cli")
	if code != 0 || stdout != body || strings.Contains(stderr, "limit") {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if call.path != "/api/v1/audit/export" || call.query.Get("format") != "ndjson" || call.query.Get("q") != "origin:cli" || call.accept != "application/x-ndjson" {
		t.Fatalf("call = %+v", call)
	}
}

func TestAuditExportWritesAFileAndWarnsAtTheLimit(t *testing.T) {
	body := "id,action\n1,auth.login\n2,\"dashboard.update\nwith a newline\"\n"
	header := http.Header{"Content-Type": {"text/csv"}, exportRowLimitKey: {"2"}}
	file := filepath.Join(t.TempDir(), "audit.csv")
	call, stdout, stderr, code := runAudit(t, http.StatusOK, header, body, "audit", "export", "--format", "csv", "--file", file)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	written, err := os.ReadFile(file) //nolint:gosec // a test file under t.TempDir
	if err != nil || string(written) != body {
		t.Fatalf("file %q, %v", written, err)
	}
	if call.accept != "text/csv" || !strings.Contains(stdout+stderr, "Wrote 2 events to "+file) || !strings.Contains(stderr, "limit of 2 rows") {
		t.Fatalf("call %+v, stdout %q, stderr %q", call, stdout, stderr)
	}
}
