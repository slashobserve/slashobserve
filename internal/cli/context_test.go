package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const contextSourceReply = `{"uid":"docs","name":"Docs","url":"https://docs.example.com/mcp","audience":"admins","secret_header":"Authorization","secret_set":true,"provisioned":false,"state":"ready","failure":null,"checked_at":"2026-11-25T10:00:00Z","tools":{"offered":2,"approved":1,"changed":1,"gone":0,"refused":1},"created_at":"2026-11-25T09:00:00Z","updated_at":"2026-11-25T10:00:00Z"}`

const contextToolsReply = `{"tools":[
{"name":"search","state":"offered","refusal":null,"description":"Search\nthe docs","hash":"h-search","seen":null},
{"name":"fetch page","state":"changed","refusal":null,"description":"Fetch","hash":"h-old","seen":{"hash":"h-new"}},
{"name":"wipe","state":"offered","refusal":"the server marks it destructive","description":"Wipe","hash":"h-wipe","seen":null}]}`

type contextFake struct {
	server   *httptest.Server
	mu       sync.Mutex
	requests []recordedRequest
	replies  map[string]string
	status   map[string]int
}

func newContextFake(t *testing.T) *contextFake {
	t.Helper()
	f := &contextFake{replies: map[string]string{}, status: map[string]int{}}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		data, _ := io.ReadAll(r.Body)
		f.requests = append(f.requests, recordedRequest{method: r.Method, path: r.URL.EscapedPath(), body: string(data)})
		key := r.Method + " " + r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		status := f.status[key]
		if status == 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
		if status != http.StatusNoContent {
			_, _ = io.WriteString(w, f.replies[key])
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *contextFake) run(stdin string, args ...string) (string, string, int) {
	var stdout, stderr strings.Builder
	code := Main(append([]string{"--server", f.server.URL, "--token", "k"}, args...), IO{In: strings.NewReader(stdin), Out: &stdout, Err: &stderr}, "test")
	return stdout.String(), stderr.String(), code
}

func (f *contextFake) last() recordedRequest { return f.requests[len(f.requests)-1] }

func TestContextAddSendsTheSecretFromStandardInput(t *testing.T) {
	f := newContextFake(t)
	f.status["POST /api/v1/context-sources"] = http.StatusCreated
	f.replies["POST /api/v1/context-sources"] = contextSourceReply

	stdout, stderr, code := f.run("s3cret\n", "context", "add", "docs", "--url", "https://docs.example.com/mcp", "--secret-header", "Authorization", "--secret-stdin", "--audience", "admins")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, "Registered context source docs") || strings.Contains(stdout+stderr, "s3cret") {
		t.Errorf("output = %q %q", stdout, stderr)
	}
	var body map[string]string
	if err := json.Unmarshal([]byte(f.last().body), &body); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"uid": "docs", "url": "https://docs.example.com/mcp", "secret_header": "Authorization", "secret": "s3cret", "audience": "admins"}
	for k, v := range want {
		if body[k] != v {
			t.Errorf("body[%s] = %q, want %q", k, body[k], v)
		}
	}
}

func TestContextAddNeedsAnAddressAndAMatchedSecretPair(t *testing.T) {
	f := newContextFake(t)
	for _, args := range [][]string{
		{"context", "add", "docs"},
		{"context", "add", "docs", "--url", "https://docs.example.com/mcp", "--secret-stdin"},
		{"context", "add", "docs", "--url", "https://docs.example.com/mcp", "--secret-header", "Authorization"},
	} {
		if _, _, code := f.run("x", args...); code != 2 {
			t.Errorf("%v exit = %d, want 2", args, code)
		}
	}
	if len(f.requests) != 0 {
		t.Errorf("sent %d requests", len(f.requests))
	}
}

func TestContextEditSendsOnlyWhatChanged(t *testing.T) {
	f := newContextFake(t)
	f.replies["PATCH /api/v1/context-sources/docs"] = contextSourceReply

	if _, stderr, code := f.run("", "context", "edit", "docs", "--audience", "everyone", "--clear-secret"); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(f.last().body), &body); err != nil {
		t.Fatal(err)
	}
	if len(body) != 2 || body["audience"] != "everyone" || body["clear_secret"] != true {
		t.Errorf("body = %v", body)
	}
	if _, _, code := f.run("", "context", "edit", "docs"); code != 2 {
		t.Errorf("edit without flags exit = %d, want 2", code)
	}
	if _, _, code := f.run("x", "context", "edit", "docs", "--clear-secret", "--secret-stdin", "--secret-header", "X-Key"); code != 2 {
		t.Errorf("clear with new secret exit = %d, want 2", code)
	}
}

func TestContextListPrintsAStateAndToolSummaryPerSource(t *testing.T) {
	f := newContextFake(t)
	f.replies["GET /api/v1/context-sources"] = `{"context_sources":[` + contextSourceReply + `]}`

	stdout, stderr, code := f.run("", "context", "list")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	for _, want := range []string{"docs", "ready", "admins", "1 approved, 2 offered, 1 changed, 1 refused", "https://docs.example.com/mcp"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("output lacks %q:\n%s", want, stdout)
		}
	}
}

func TestContextListOfOneSourcePrintsTheSecretHeaderNeverAValue(t *testing.T) {
	f := newContextFake(t)
	f.replies["GET /api/v1/context-sources/docs"] = contextSourceReply

	stdout, _, code := f.run("", "context", "list", "docs")
	if code != 0 || !strings.Contains(stdout, "set in Authorization") {
		t.Fatalf("exit %d, output = %q", code, stdout)
	}
}

func TestContextDiscoverReportsTheTallyAndFailsOnAStoredFailure(t *testing.T) {
	f := newContextFake(t)
	f.replies["POST /api/v1/context-sources/docs/discover"] = `{"source":` + contextSourceReply + `,"tally":{"found":3,"new":1,"changed":1,"gone":0,"ignored":0}}`

	stdout, stderr, code := f.run("", "context", "discover", "docs")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, "Found 3 tools: 1 new, 1 changed, 0 gone, 0 ignored") || !strings.Contains(stdout+stderr, "stopped being served") {
		t.Errorf("output = %q %q", stdout, stderr)
	}

	failed := strings.Replace(contextSourceReply, `"failure":null`, `"failure":"unauthorized"`, 1)
	f.replies["POST /api/v1/context-sources/docs/discover"] = `{"source":` + failed + `,"tally":{"found":0,"new":0,"changed":0,"gone":0,"ignored":0}}`
	_, stderr, code = f.run("", "context", "discover", "docs")
	if code != 1 || !strings.Contains(stderr, "unauthorized") {
		t.Errorf("exit %d, stderr = %q", code, stderr)
	}
}

func TestContextToolsListsEachToolWithItsRefusal(t *testing.T) {
	f := newContextFake(t)
	f.replies["GET /api/v1/context-sources/docs/tools"] = contextToolsReply

	stdout, _, code := f.run("", "context", "tools", "docs")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	for _, want := range []string{"search", "offered", "Search", "fetch page", "changed", "wipe", "refused: the server marks it destructive"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("output lacks %q:\n%s", want, stdout)
		}
	}
}

func TestContextApprovePinsTheHashOfWhatWasListed(t *testing.T) {
	f := newContextFake(t)
	f.replies["GET /api/v1/context-sources/docs/tools"] = contextToolsReply
	f.replies["POST /api/v1/context-sources/docs/tools/search/approve"] = `{}`
	f.replies["POST /api/v1/context-sources/docs/tools/fetch%20page/approve"] = `{}`

	if _, stderr, code := f.run("", "context", "approve", "docs", "search"); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if got := f.last(); got.path != "/api/v1/context-sources/docs/tools/search/approve" || !strings.Contains(got.body, `"h-search"`) {
		t.Errorf("request = %+v", got)
	}
	if _, stderr, code := f.run("", "context", "approve", "docs", "fetch page"); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if got := f.last(); got.path != "/api/v1/context-sources/docs/tools/fetch%20page/approve" || !strings.Contains(got.body, `"h-new"`) {
		t.Errorf("a changed tool must be approved at its new hash: %+v", got)
	}
}

func TestContextApproveExplainsARefusedTool(t *testing.T) {
	f := newContextFake(t)
	f.replies["GET /api/v1/context-sources/docs/tools"] = contextToolsReply
	f.status["POST /api/v1/context-sources/docs/tools/wipe/approve"] = http.StatusConflict
	f.replies["POST /api/v1/context-sources/docs/tools/wipe/approve"] = `{"error":"the server marks it destructive","code":"tool_refused","refusal":"the server marks it destructive"}`

	_, stderr, code := f.run("", "context", "approve", "docs", "wipe")
	if code != 1 || !strings.Contains(stderr, "destructive") {
		t.Fatalf("exit %d, stderr = %q", code, stderr)
	}
}

func TestContextApproveNeedsAToolOrTheReadOnlyFlag(t *testing.T) {
	f := newContextFake(t)
	f.replies["GET /api/v1/context-sources/docs/tools"] = contextToolsReply
	for _, args := range [][]string{
		{"context", "approve", "docs"},
		{"context", "approve", "docs", "search", "--all-read-only"},
	} {
		if _, _, code := f.run("", args...); code != 2 {
			t.Errorf("%v exit = %d, want 2", args, code)
		}
	}
	if _, stderr, code := f.run("", "context", "approve", "docs", "missing"); code != 1 || !strings.Contains(stderr, "has no tool missing") {
		t.Errorf("exit %d, stderr = %q", code, stderr)
	}
}

func TestContextApproveAllReadOnlyPostsOnceAndCounts(t *testing.T) {
	f := newContextFake(t)
	f.replies["POST /api/v1/context-sources/docs/tools/approve-read-only"] = `{"approved":[{"name":"search"},{"name":"list"}]}`

	stdout, _, code := f.run("", "context", "approve", "docs", "--all-read-only")
	if code != 0 || !strings.Contains(stdout, "Approved 2 tools") || len(f.requests) != 1 {
		t.Fatalf("exit %d, output = %q, requests = %d", code, stdout, len(f.requests))
	}
}

func TestContextRevokeAndRemoveCallTheirRoutes(t *testing.T) {
	f := newContextFake(t)
	f.replies["POST /api/v1/context-sources/docs/tools/search/revoke"] = `{}`
	f.status["DELETE /api/v1/context-sources/docs"] = http.StatusNoContent

	if _, _, code := f.run("", "context", "revoke", "docs", "search"); code != 0 {
		t.Fatalf("revoke exit %d", code)
	}
	stdout, _, code := f.run("", "context", "remove", "docs")
	if code != 0 || f.last().method != http.MethodDelete || !strings.Contains(stdout, "Removed context source docs") {
		t.Fatalf("remove exit %d, output = %q", code, stdout)
	}
}

func TestContextDescriptionIsSentOnAddAndEditAndShownInTheFullView(t *testing.T) {
	f := newContextFake(t)
	f.status["POST /api/v1/context-sources"] = http.StatusCreated
	f.replies["POST /api/v1/context-sources"] = contextSourceReply
	f.replies["PATCH /api/v1/context-sources/docs"] = contextSourceReply

	if _, stderr, code := f.run("", "context", "add", "docs", "--url", "https://docs.example.com/mcp", "--description", "Runbooks and design notes"); code != 0 {
		t.Fatalf("add exit %d: %s", code, stderr)
	}
	var added map[string]any
	if err := json.Unmarshal([]byte(f.last().body), &added); err != nil || added["description"] != "Runbooks and design notes" {
		t.Errorf("add body = %v, %v", added, err)
	}

	if _, stderr, code := f.run("", "context", "edit", "docs", "--description", ""); code != 0 {
		t.Fatalf("edit exit %d: %s", code, stderr)
	}
	var edited map[string]any
	if err := json.Unmarshal([]byte(f.last().body), &edited); err != nil {
		t.Fatal(err)
	}
	if value, sent := edited["description"]; !sent || value != "" {
		t.Errorf("an empty --description must be sent to remove it, body = %v", edited)
	}

	f.replies["GET /api/v1/context-sources/docs"] = strings.Replace(contextSourceReply, `"name":`, `"description":"Runbooks and design notes","name":`, 1)
	stdout, _, code := f.run("", "context", "list", "docs")
	if code != 0 || !strings.Contains(stdout, "Runbooks and design notes") {
		t.Fatalf("exit %d, output = %q", code, stdout)
	}
}
