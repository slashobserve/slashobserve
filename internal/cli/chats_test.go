package cli

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestChatsListSendsFiltersAndPrintsEachChat(t *testing.T) {
	var got url.Values
	var path string
	reply := `{"chats":[{"id":"` + stoppedID + `","title":"Checkout latency","dashboard_id":null,"observation_id":null,"pinned_at":null,"created_at":"2026-01-01T00:00:00Z","last_turn_at":"2026-01-01T00:05:00Z","turns":3,"last_turn":{"id":"` + stoppedID + `","state":"succeeded","prompt":"and now?"}}],"next_cursor":"c2"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, path = r.URL.Query(), r.URL.Path
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(server.Close)
	stdout, stderr, code := runWith(server, "chats", "list", "--q", "checkout", "--pinned", "--dashboard", testDashboard, "--observation", stoppedID, "--limit", "5")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	want := map[string]string{"q": "checkout", "pinned": "true", "dashboard_id": testDashboard, "observation_id": stoppedID, "limit": "5"}
	for key, value := range want {
		if got.Get(key) != value {
			t.Fatalf("%s = %q, query %v", key, got.Get(key), got)
		}
	}
	if path != "/api/v1/chats" {
		t.Fatalf("path = %q", path)
	}
	line := strings.Split(strings.TrimSpace(stdout), "\n")[1]
	for _, part := range []string{stoppedID, "Done", "3", "2026-01-01T00:05Z", "Checkout latency"} {
		if !strings.Contains(line, part) {
			t.Fatalf("row %q lacks %q", line, part)
		}
	}
	if !strings.Contains(stderr, "--cursor c2") {
		t.Fatalf("stderr %q", stderr)
	}
}

func TestChatsListForEveryoneNamesEachOwner(t *testing.T) {
	reply := `{"chats":[{"id":"` + stoppedID + `","title":"Checkout latency","dashboard_id":null,"observation_id":null,"pinned_at":null,"created_at":"2026-01-01T00:00:00Z","last_turn_at":"2026-01-01T00:05:00Z","turns":1,"last_turn":null,"owner":{"kind":"user","name":"Grace"},"read_only":true}],"next_cursor":null}`
	var got url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(server.Close)
	stdout, stderr, code := runWith(server, "chats", "list", "--scope", "all")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if got.Get("scope") != "all" {
		t.Fatalf("query %v lacks scope=all", got)
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if !strings.Contains(lines[0], "OWNER") || !strings.Contains(lines[1], "Grace") {
		t.Fatalf("output %q does not name the owner", stdout)
	}
}

func TestChatsListRejectsABadIDAndALimitOverFifty(t *testing.T) {
	for _, args := range [][]string{{"--dashboard", "nope"}, {"--observation", "nope"}, {"--limit", "51"}, {"--scope", "everyone"}} {
		_, _, stderr, code := runAgainst(t, http.StatusOK, `{}`, append([]string{"chats", "list"}, args...)...)
		if code != 2 {
			t.Fatalf("%v: exit %d, stderr %q", args, code, stderr)
		}
	}
}

func TestChatsShowPrintsTheChatAndItsTurnsInOrder(t *testing.T) {
	reply := `{"chat":{"id":"` + stoppedID + `","title":"Checkout latency","dashboard_id":"` + testDashboard + `","observation_id":null,"pinned_at":null,"created_at":"x","last_turn_at":"x","turns":2,"last_turn":null},` +
		`"turns":[{"id":"00000000-0000-4000-8000-000000000001","state":"succeeded","prompt":"first question\nmore"},{"id":"00000000-0000-4000-8000-000000000002","state":"running","prompt":"second question"}]}`
	call, stdout, stderr, code := runAgainst(t, http.StatusOK, reply, "chats", "show", stoppedID)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if call.method != http.MethodGet || call.path != "/api/v1/chats/"+stoppedID {
		t.Fatalf("got %+v", call)
	}
	first, second := strings.Index(stdout, "first question"), strings.Index(stdout, "second question")
	if first < 0 || second < first || strings.Contains(stdout, "more") || !strings.Contains(stdout, testDashboard) || !strings.Contains(stdout, "Running") {
		t.Fatalf("stdout:\n%s", stdout)
	}
}

func TestChatsArchiveDeletesTheChat(t *testing.T) {
	call, stdout, stderr, code := runAgainst(t, http.StatusNoContent, ``, "chats", "archive", stoppedID)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if call.method != http.MethodDelete || call.path != "/api/v1/chats/"+stoppedID || !strings.Contains(stdout+stderr, "Archived chat "+stoppedID) {
		t.Fatalf("got %+v, stdout %q, stderr %q", call, stdout, stderr)
	}
	_, _, stderr, code = runAgainst(t, http.StatusNoContent, ``, "chats", "archive", "nope")
	if code != 2 || !strings.Contains(stderr, "not a valid id") {
		t.Fatalf("bad id: exit %d, stderr %q", code, stderr)
	}
}

func TestAPromptInAChatSendsChatID(t *testing.T) {
	const created = `{"request":{"id":"` + stoppedID + `","status":"queued","state":"queued","steps":[]}}`
	call, _, stderr, code := runAgainst(t, http.StatusAccepted, created, "--no-wait", "--chat", stoppedID, "--dashboard", testDashboard, "and the day before?")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if call.body != `{"chat_id":"`+stoppedID+`","dashboard_id":"`+testDashboard+`","prompt":"and the day before?"}` {
		t.Fatalf("got %+v", call)
	}
	_, _, stderr, code = runAgainst(t, http.StatusAccepted, created, "--chat", stoppedID, "--follows", stoppedID, "why is it slow")
	if code != 2 || !strings.Contains(stderr, "exclude each other") {
		t.Fatalf("both flags: exit %d, stderr %q", code, stderr)
	}
}
