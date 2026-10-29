package cli

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestPromptName(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"unquoted words", []string{"Checkout", "health"}, "Checkout health"},
		{"padded words", []string{"  a  ", "b"}, "a b"},
		{"one quoted argument", []string{"Checkout   health"}, "Checkout health"},
		{"nothing", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := promptName(tc.args); got != tc.want {
				t.Fatalf("promptName(%q) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}

type promptsServer struct {
	mu       sync.Mutex
	requests []string
	posted   string
	listBody string
}

func (s *promptsServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, r.Method+" "+r.URL.RequestURI())
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/prompts":
		_, _ = fmt.Fprint(w, s.listBody)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/observe":
		body, _ := io.ReadAll(r.Body)
		s.posted = string(body)
		w.WriteHeader(http.StatusAccepted)
		_, _ = fmt.Fprint(w, `{"request":{"id":"req-1","status":"queued","steps":[]}}`)
	default:
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `{"error":"not found","code":"not_found"}`)
	}
}

func runObserve(t *testing.T, s *promptsServer, args ...string) (int, string, string) {
	t.Helper()
	server := httptest.NewServer(s)
	t.Cleanup(server.Close)
	var stdout, stderr bytes.Buffer
	full := append([]string{"--server", server.URL, "--token", "k"}, args...)
	code := Main(full, IO{In: strings.NewReader(""), Out: &stdout, Err: &stderr}, "test")
	return code, stdout.String(), stderr.String()
}

const twoPrompts = `{"prompts":[
{"id":"a","name":"Checkout health","body":"checkout health by pod","shared":true,"owner":{"id":"u1","name":"Ada"}},
{"id":"b","name":"Deploy check","body":"deploys in the last day","shared":true,"owner":{"id":"u2","name":"bob@example.com"}}]}`

func TestRunWithoutANameListsThePrompts(t *testing.T) {
	s := &promptsServer{listBody: twoPrompts}
	code, stdout, stderr := runObserve(t, s, "run")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	for _, want := range []string{"NAME", "SHARED BY", "Checkout health", "Ada", "Deploy check", "bob@example.com"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("output lacks %q:\n%s", want, stdout)
		}
	}
	if len(s.requests) != 1 || s.requests[0] != "GET /api/v1/prompts" {
		t.Fatalf("requests = %v, want one plain list", s.requests)
	}
}

func TestRunWithoutANameAndNoPromptsSaysSo(t *testing.T) {
	s := &promptsServer{listBody: `{"prompts":[]}`}
	code, stdout, stderr := runObserve(t, s, "run")
	if code != 0 || !strings.Contains(stdout, "no shared prompts yet") {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestRunWithJSONPrintsTheListBody(t *testing.T) {
	s := &promptsServer{listBody: twoPrompts}
	code, stdout, stderr := runObserve(t, s, "--json", "run")
	if code != 0 || strings.TrimSpace(stdout) != strings.TrimSpace(twoPrompts) {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestRunSendsTheSavedBodyAsThePrompt(t *testing.T) {
	s := &promptsServer{listBody: `{"prompts":[{"id":"a","name":"Checkout health","body":"checkout health by pod","shared":true,"owner":{"id":"u1","name":"Ada"}}]}`}
	code, stdout, stderr := runObserve(t, s, "run", "--no-wait", "checkout", "health")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if strings.TrimSpace(stdout) != "req-1" {
		t.Fatalf("stdout = %q, want the request id", stdout)
	}
	wantRequests := []string{"GET /api/v1/prompts?name=checkout+health", "POST /api/v1/observe"}
	if strings.Join(s.requests, "|") != strings.Join(wantRequests, "|") {
		t.Fatalf("requests = %v, want %v", s.requests, wantRequests)
	}
	if s.posted != `{"prompt":"checkout health by pod"}` {
		t.Fatalf("posted %s", s.posted)
	}
	if !strings.Contains(stderr, `Running "Checkout health"`) {
		t.Fatalf("stderr lacks the matched name: %q", stderr)
	}
}

func TestRunQuietHidesTheMatchedName(t *testing.T) {
	s := &promptsServer{listBody: `{"prompts":[{"id":"a","name":"Checkout health","body":"b","shared":true,"owner":{"id":"u1","name":"Ada"}}]}`}
	code, _, stderr := runObserve(t, s, "--quiet", "run", "--no-wait", "Checkout health")
	if code != 0 || stderr != "" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
}

func TestRunUnknownNameExitsOne(t *testing.T) {
	s := &promptsServer{listBody: `{"prompts":[]}`}
	code, _, stderr := runObserve(t, s, "run", "nothing", "here")
	if code != 1 || !strings.Contains(stderr, `no shared prompt named "nothing here"`) {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if len(s.requests) != 1 {
		t.Fatalf("requests = %v, want only the lookup", s.requests)
	}
}

func TestRunRefusesANonPositiveTimeout(t *testing.T) {
	s := &promptsServer{listBody: twoPrompts}
	code, _, stderr := runObserve(t, s, "run", "--timeout", "0", "Checkout health")
	if code != 2 || !strings.Contains(stderr, "--timeout must be positive") {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if len(s.requests) != 0 {
		t.Fatalf("requests = %v, want none", s.requests)
	}
}
