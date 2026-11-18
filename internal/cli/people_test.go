package cli

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const testInvite = "ffffffff-0000-4000-8000-000000000009"

type peopleFake struct {
	server   *httptest.Server
	mu       sync.Mutex
	requests []recordedRequest
	invites  string
}

const inviteBody = `{"id":"` + testInvite + `","email":"new@example.com","role":"viewer","invited_by":"Ada","created_at":"2026-11-19T09:00:00Z","updated_at":"2026-11-19T09:00:00Z","sign_in_url":"https://observe.example.com/login"}`

func newPeopleFake(t *testing.T) *peopleFake {
	t.Helper()
	f := &peopleFake{invites: `{"invites":[` + inviteBody + `],"next_cursor":"next"}`}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		data, _ := io.ReadAll(r.Body)
		f.requests = append(f.requests, recordedRequest{method: r.Method, path: r.URL.Path, query: r.URL.RawQuery, body: string(data)})
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/invites":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, inviteBody)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/invites":
			_, _ = io.WriteString(w, f.invites)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/invites/"+testInvite:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":"invite not found; it was withdrawn or the person has signed in","code":"not_found"}`)
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *peopleFake) run(args ...string) (string, string, int) {
	var stdout, stderr bytes.Buffer
	code := Main(append([]string{"--server", f.server.URL, "--token", "k", "people"}, args...), IO{In: strings.NewReader(""), Out: &stdout, Err: &stderr}, "test")
	return stdout.String(), stderr.String(), code
}

func TestPeopleInviteSendsTheEmailAndRoleAndPrintsTheLink(t *testing.T) {
	f := newPeopleFake(t)
	stdout, stderr, code := f.run("invite", "new@example.com", "--role", "Viewer")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if got := f.requests[0]; got.method != http.MethodPost || got.body != `{"email":"new@example.com","role":"viewer"}` {
		t.Fatalf("request %+v", got)
	}
	if !strings.Contains(stdout, "https://observe.example.com/login") || !strings.Contains(stdout+stderr, "Invited new@example.com as viewer") {
		t.Fatalf("stdout %q stderr %q", stdout, stderr)
	}
}

func TestPeopleInviteNeedsAKnownRole(t *testing.T) {
	f := newPeopleFake(t)
	_, stderr, code := f.run("invite", "new@example.com", "--role", "owner")
	if code == 0 || !strings.Contains(stderr, "pass --role admin, editor, commenter, viewer") || len(f.requests) != 0 {
		t.Fatalf("exit %d, stderr %q, requests %d", code, stderr, len(f.requests))
	}
}

func TestPeopleInvitesPrintsATableAndTheNextPage(t *testing.T) {
	f := newPeopleFake(t)
	stdout, stderr, code := f.run("invites", "--limit", "1")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	got := spaces.ReplaceAllString(stdout, " ")
	if !strings.HasPrefix(got, "ID EMAIL ROLE INVITED BY INVITED\n"+testInvite+" new@example.com viewer Ada ") {
		t.Fatalf("stdout %q", got)
	}
	if !strings.Contains(stderr, "more results: --cursor next") || f.requests[0].query != "limit=1" {
		t.Fatalf("stderr %q, query %q", stderr, f.requests[0].query)
	}
}

func TestPeopleUninviteDeletesByID(t *testing.T) {
	f := newPeopleFake(t)
	stdout, stderr, code := f.run("uninvite", testInvite)
	if code != 0 || f.requests[0].method != http.MethodDelete {
		t.Fatalf("exit %d, stderr %q, requests %+v", code, stderr, f.requests)
	}
	if !strings.Contains(stdout+stderr, "Withdrew invite "+testInvite) {
		t.Fatalf("stdout %q stderr %q", stdout, stderr)
	}
}
