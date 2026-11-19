package cli

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	testTeam   = "ffffffff-0000-4000-8000-00000000000a"
	testPerson = "ffffffff-0000-4000-8000-00000000000b"
	teamBody   = `{"id":"` + testTeam + `","name":"sre","created_by":"Ada","created_at":"2026-11-19T09:00:00Z","members":[{"id":"` + testPerson + `","name":"","email":"bob@example.com"}]}`
)

func runTeams(t *testing.T, args ...string) (string, string, int, []recordedRequest) {
	t.Helper()
	var requests []recordedRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		requests = append(requests, recordedRequest{method: r.Method, path: r.URL.Path, body: string(data)})
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/teams":
			_, _ = io.WriteString(w, `{"teams":[`+teamBody+`]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/teams":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, strings.Replace(teamBody, `"members":[{"id":"`+testPerson+`","name":"","email":"bob@example.com"}]`, `"members":[]`, 1))
		case r.URL.Path == "/api/v1/teams/"+testTeam+"/members/"+testPerson:
			_, _ = io.WriteString(w, teamBody)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/teams/"+testTeam:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":"team not found","code":"not_found"}`)
		}
	}))
	defer server.Close()
	var stdout, stderr bytes.Buffer
	code := Main(append([]string{"--server", server.URL, "--token", "k", "teams"}, args...), IO{In: strings.NewReader(""), Out: &stdout, Err: &stderr}, "test")
	return stdout.String(), stderr.String(), code, requests
}

func TestTeamsListNamesTheMembers(t *testing.T) {
	stdout, stderr, code, _ := runTeams(t, "list")
	got := spaces.ReplaceAllString(stdout, " ")
	if code != 0 || !strings.Contains(got, testTeam+" sre 1 bob@example.com") {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, got, stderr)
	}
}

func TestTeamsCreateSendsTheName(t *testing.T) {
	stdout, stderr, code, requests := runTeams(t, "create", "sre")
	if code != 0 || requests[0].body != `{"name":"sre"}` || !strings.Contains(stdout+stderr, "Created team sre") {
		t.Fatalf("exit %d, requests %+v, stdout %q, stderr %q", code, requests, stdout, stderr)
	}
}

func TestTeamsAddAndRemoveUseTheMemberRoute(t *testing.T) {
	for verb, method := range map[string]string{"add": http.MethodPut, "remove": http.MethodDelete} {
		stdout, stderr, code, requests := runTeams(t, verb, testTeam, testPerson)
		if code != 0 || requests[0].method != method {
			t.Fatalf("%s: exit %d, requests %+v, stderr %q", verb, code, requests, stderr)
		}
		if !strings.Contains(stdout+stderr, "team sre") {
			t.Fatalf("%s: stdout %q stderr %q", verb, stdout, stderr)
		}
	}
}

func TestTeamsDeleteRefusesABadID(t *testing.T) {
	_, stderr, code, requests := runTeams(t, "delete", "nope")
	if code == 0 || len(requests) != 0 || !strings.Contains(stderr, "not a valid id") {
		t.Fatalf("exit %d, requests %d, stderr %q", code, len(requests), stderr)
	}
}
