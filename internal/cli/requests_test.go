package cli

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestRequestsListSendsFiltersAndPrintsTheTotal(t *testing.T) {
	var got url.Values
	reply := `{"requests":[{"id":"` + stoppedID + `","kind":"build","title":"Checkout latency","source":"observe","state":"failed","dashboard_id":null,"dashboard_title":null,"created_at":"2026-01-01T00:00:00Z","allowed_actions":["retry"]}],"next_cursor":"c2","summary":{"total":7,"people":1}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(server.Close)
	stdout, stderr, code := runWith(server, "requests", "list", "--scope", "all", "--source", "observe", "--state", "failed,canceled", "--state", "queued", "--q", "checkout", "--pinned=false", "--dashboard", testDashboard, "--limit", "5")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	want := map[string]string{"scope": "all", "source": "observe", "q": "checkout", "pinned": "false", "dashboard_id": testDashboard, "limit": "5"}
	for key, value := range want {
		if got.Get(key) != value {
			t.Fatalf("%s = %q, query %v", key, got.Get(key), got)
		}
	}
	if strings.Join(got["state"], ",") != "failed,canceled,queued" {
		t.Fatalf("states %v", got["state"])
	}
	if !strings.Contains(stdout, "Checkout latency") || !strings.Contains(stderr, "Showing 1 of 7. More: --cursor c2") {
		t.Fatalf("stdout %q, stderr %q", stdout, stderr)
	}
}

func TestRequestsListPrintsTheStateWordsOfTheWebAndKeepsJSON(t *testing.T) {
	row := func(id, state string) string {
		return `{"id":"` + id + `","kind":"build","title":"T","source":"observe","state":"` + state + `","dashboard_id":null,"dashboard_title":null,"created_at":"x","allowed_actions":[]}`
	}
	reply := `{"requests":[` + strings.Join([]string{
		row("00000000-0000-4000-8000-000000000001", "queued"),
		row("00000000-0000-4000-8000-000000000002", "running"),
		row("00000000-0000-4000-8000-000000000003", "canceling"),
		row("00000000-0000-4000-8000-000000000004", "succeeded"),
		row("00000000-0000-4000-8000-000000000005", "failed"),
		row("00000000-0000-4000-8000-000000000006", "canceled"),
	}, ",") + `],"next_cursor":null,"summary":{"total":6,"people":null}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(server.Close)
	stdout, stderr, code := runWith(server, "requests", "list")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	for id, word := range map[string]string{"1": "Running", "2": "Running", "3": "Running", "4": "Done", "5": "Stopped", "6": "Stopped"} {
		found := false
		for _, line := range strings.Split(stdout, "\n") {
			if strings.Contains(line, "00000000-0000-4000-8000-00000000000"+id) && strings.Contains(line, word) {
				found = true
			}
		}
		if !found {
			t.Fatalf("request %s should read %s in %q", id, word, stdout)
		}
	}
	if strings.Contains(stdout, "succeeded") || strings.Contains(stdout, "canceled") {
		t.Fatalf("raw states in %q", stdout)
	}
	jsonOut, _, code := runWith(server, "--json", "requests", "list")
	if code != 0 || !strings.Contains(jsonOut, "succeeded") || !strings.Contains(jsonOut, "canceled") {
		t.Fatalf("json output %q", jsonOut)
	}
}

func TestRequestsListRefusesUnknownChoices(t *testing.T) {
	for _, args := range [][]string{
		{"requests", "list", "--scope", "everyone"},
		{"requests", "list", "--source", "chat"},
		{"requests", "list", "--state", "done"},
		{"requests", "list", "--limit", "101"},
		{"requests", "list", "--dashboard", "nope"},
	} {
		_, _, stderr, code := runAgainst(t, http.StatusOK, `{}`, args...)
		if code != 2 || stderr == "" {
			t.Fatalf("%v: exit %d, stderr %q", args, code, stderr)
		}
	}
}

func TestRequestsListAllJoinsPages(t *testing.T) {
	var seen []string
	row := func(id string) string {
		return `{"id":"` + id + `","kind":"build","title":"T","source":"observe","state":"succeeded","dashboard_id":null,"dashboard_title":"Shop","created_at":"x","allowed_actions":[]}`
	}
	server := dashboardPageServer(t, map[string]string{
		"":   `{"requests":[` + row(testDashboard) + `],"next_cursor":"p2","summary":{"total":2,"people":1}}`,
		"p2": `{"requests":[` + row(stoppedID) + `],"next_cursor":null,"summary":null}`,
	}, &seen)
	stdout, stderr, code := runWith(server, "requests", "list", "--all")
	if code != 0 || !strings.Contains(stdout, testDashboard) || !strings.Contains(stdout, stoppedID) || stderr != "" {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestRequestsArchiveDeletesTheRequest(t *testing.T) {
	call, stdout, stderr, code := runAgainst(t, http.StatusNoContent, "", "requests", "archive", stoppedID)
	if code != 0 || call.method != http.MethodDelete || call.path != "/api/v1/observe/"+stoppedID {
		t.Fatalf("exit %d, call %+v, stderr %q", code, call, stderr)
	}
	if !strings.Contains(stdout, "Archived request "+stoppedID) {
		t.Fatalf("stdout %q", stdout)
	}
}

func TestRequestsPinRenameSendTheChange(t *testing.T) {
	reply := `{"request":{"id":"` + stoppedID + `"}}`
	cases := []struct {
		args []string
		body string
		out  string
	}{
		{[]string{"requests", "pin", stoppedID}, `{"pinned":true}`, "pinned"},
		{[]string{"requests", "unpin", stoppedID}, `{"pinned":false}`, "unpinned"},
		{[]string{"requests", "rename", stoppedID, "Checkout", "p99"}, `{"title":"Checkout p99"}`, "renamed"},
		{[]string{"requests", "rename", stoppedID, "--clear"}, `{"title":null}`, "renamed"},
	}
	for _, tc := range cases {
		call, stdout, stderr, code := runAgainst(t, http.StatusOK, reply, tc.args...)
		if code != 0 || call.method != http.MethodPatch || call.path != "/api/v1/observe/"+stoppedID || call.body != tc.body {
			t.Fatalf("%v: exit %d, call %+v, stderr %q", tc.args, code, call, stderr)
		}
		if !strings.Contains(stdout, tc.out) {
			t.Fatalf("%v: stdout %q", tc.args, stdout)
		}
	}
}

func TestRequestsRenameNeedsATitleOrClear(t *testing.T) {
	for _, args := range [][]string{{"requests", "rename", stoppedID}, {"requests", "rename", stoppedID, "x", "--clear"}} {
		_, _, stderr, code := runAgainst(t, http.StatusOK, `{}`, args...)
		if code != 2 || !strings.Contains(stderr, "takes a request id and a title") {
			t.Fatalf("%v: exit %d, stderr %q", args, code, stderr)
		}
	}
}

func TestArchiveAndRemoveBlockPrintOnlyAnEmptyBodyWithJSON(t *testing.T) {
	for _, args := range [][]string{
		{"--json", "requests", "archive", stoppedID},
		{"--json", "o", "remove-block", testBlock, "--version", "2"},
	} {
		_, stdout, stderr, code := runAgainst(t, http.StatusNoContent, "", args...)
		if code != 0 || strings.TrimSpace(stdout) != "" || stderr != "" {
			t.Fatalf("%v: exit %d, stdout %q, stderr %q", args, code, stdout, stderr)
		}
	}
}

func TestRequestsRenameSaysWhatHappenedToTheObservationTitle(t *testing.T) {
	cases := map[string]string{
		"renamed": "Request " + stoppedID + " renamed, and the observation made from it\n",
		"kept":    "Request " + stoppedID + " renamed; the observation made from it keeps its title because you may not edit it\n",
		"":        "Request " + stoppedID + " renamed\n",
	}
	for outcome, want := range cases {
		reply := `{"request":{"id":"` + stoppedID + `","observation_rename":"` + outcome + `"}}`
		_, stdout, stderr, code := runAgainst(t, http.StatusOK, reply, "requests", "rename", stoppedID, "Routes")
		if code != 0 || stdout != want {
			t.Fatalf("%q: exit %d, stdout %q, stderr %q", outcome, code, stdout, stderr)
		}
	}
}
