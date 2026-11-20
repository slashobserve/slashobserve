package cli

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const accessDashboard = "ffffffff-0000-4000-8000-000000000011"

const accessRequestBody = `{"id":"ffffffff-0000-4000-8000-000000000012","object":{"kind":"dashboard","id":"` + accessDashboard + `","title":"Checkout"},"requested_by":{"kind":"user","id":"ffffffff-0000-4000-8000-000000000013","name":"Ada"},"note":"on call","created_at":"2026-11-20T09:00:00Z"}`

func runAccess(t *testing.T, handler http.HandlerFunc, args ...string) (string, string, int) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	var stdout, stderr bytes.Buffer
	code := Main(append([]string{"--server", server.URL, "--token", "k", "access"}, args...), IO{In: strings.NewReader(""), Out: &stdout, Err: &stderr}, "test")
	return stdout.String(), stderr.String(), code
}

func TestAccessRequestSendsTheObjectAndNote(t *testing.T) {
	var got recordedRequest
	stdout, stderr, code := runAccess(t, func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		got = recordedRequest{method: r.Method, path: r.URL.Path, body: string(data)}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, accessRequestBody)
	}, "request", "dashboard:"+accessDashboard, "--note", "on call")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if got.method != http.MethodPost || got.path != "/api/v1/access-requests" || got.body != `{"note":"on call","object":{"id":"`+accessDashboard+`","kind":"dashboard"}}` {
		t.Fatalf("request %+v", got)
	}
	if !strings.Contains(stdout+stderr, `Asked for comment access to dashboard Checkout; the people who manage its folder will see it in their inbox`) {
		t.Fatalf("stdout %q stderr %q", stdout, stderr)
	}
}

func TestAccessRequestRefusesAnythingButADashboardOrObservation(t *testing.T) {
	for _, target := range []string{"folder:" + accessDashboard, accessDashboard, "dashboard:nope"} {
		called := false
		_, stderr, code := runAccess(t, func(w http.ResponseWriter, r *http.Request) { called = true }, "request", target)
		if code == 0 || called || !strings.Contains(stderr, "is not <kind>:<id> with kind dashboard or observation") {
			t.Errorf("%s: exit %d, called %v, stderr %q", target, code, called, stderr)
		}
	}
}

func TestAccessRequestSaysWhenARequestIsAlreadyOpen(t *testing.T) {
	_, stderr, code := runAccess(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(w, `{"error":"you already asked for comment access here and the request is still open","code":"request_open","requested_at":"2026-11-20T09:00:00Z"}`)
	}, "request", "observation:"+accessDashboard)
	if code == 0 || !strings.Contains(stderr, "the request is still open") {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
}

func TestAccessRequestsPrintsATableOrSaysThereAreNone(t *testing.T) {
	stdout, stderr, code := runAccess(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"items":[`+accessRequestBody+`]}`)
	}, "requests")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	for _, want := range []string{"ASKED", "WHO ID", "Ada", "user:ffffffff-0000-4000-8000-000000000013", "dashboard:" + accessDashboard, "Checkout", "on call"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout %q lacks %q", stdout, want)
		}
	}

	stdout, _, code = runAccess(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"items":[]}`)
	}, "requests")
	if code != 0 || !strings.Contains(stdout, "No open access requests on folders you manage.") {
		t.Fatalf("exit %d, stdout %q", code, stdout)
	}
}
