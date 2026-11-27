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
	reply := `{"requests":[{"id":"` + stoppedID + `","kind":"build","title":"Checkout latency","intent":"ask","state":"failed","dashboard_id":null,"dashboard_title":null,"created_at":"2026-01-01T00:00:00Z","allowed_actions":["retry"]}],"next_cursor":"c2","summary":{"total":7,"people":1}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(server.Close)
	stdout, stderr, code := runWith(server, "requests", "list", "--scope", "all", "--state", "failed,canceled", "--state", "queued", "--q", "checkout", "--pinned=false", "--dashboard", testDashboard, "--limit", "5")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	want := map[string]string{"scope": "all", "q": "checkout", "pinned": "false", "dashboard_id": testDashboard, "limit": "5"}
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
		return `{"id":"` + id + `","kind":"build","title":"T","intent":"ask","state":"` + state + `","dashboard_id":null,"dashboard_title":null,"created_at":"x","allowed_actions":[]}`
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
	for id, word := range map[string]string{"1": "Queued", "2": "Running", "3": "Stopping", "4": "Done", "5": "Failed", "6": "Canceled"} {
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
		return `{"id":"` + id + `","kind":"build","title":"T","intent":"ask","state":"succeeded","dashboard_id":null,"dashboard_title":"Shop","created_at":"x","allowed_actions":[]}`
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

func TestRequestsArchiveDeletesTheSavedObservationFirst(t *testing.T) {
	request := "/api/v1/observe/" + stoppedID
	observation := "/api/v1/observations/" + testObservation
	f := newManageFake(t,
		manageReply{http.MethodGet, request, http.StatusOK, `{"request":{"id":"` + stoppedID + `","observation_id":"` + testObservation + `"}}`},
		manageReply{http.MethodDelete, observation, http.StatusNoContent, ""},
		manageReply{http.MethodDelete, request, http.StatusNoContent, ""},
	)
	stdout, stderr, code := f.run("requests", "archive", stoppedID, "--delete-observation", "--yes")
	if code != 0 || stderr != "" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	f.wantRequests(t,
		recordedRequest{http.MethodGet, request, "", ""},
		recordedRequest{http.MethodDelete, observation, "", ""},
		recordedRequest{http.MethodDelete, request, "", ""},
	)
	if want := "Deleted observation " + testObservation + "\nArchived request " + stoppedID + "\n"; stdout != want {
		t.Fatalf("stdout %q, want %q", stdout, want)
	}
}

func TestRequestsArchiveKeepsTheRequestWhenTheObservationIsNotDeleted(t *testing.T) {
	request := "/api/v1/observe/" + stoppedID
	observation := "/api/v1/observations/" + testObservation
	f := newManageFake(t,
		manageReply{http.MethodGet, request, http.StatusOK, `{"request":{"id":"` + stoppedID + `","observation_id":"` + testObservation + `"}}`},
		manageReply{http.MethodDelete, observation, http.StatusForbidden, `{"error":{"code":"forbidden","message":"needs Manage on the folder"}}`},
	)
	_, stderr, code := f.run("requests", "archive", stoppedID, "--delete-observation", "--yes")
	if code == 0 || !strings.Contains(stderr, "was not archived") {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	f.wantRequests(t,
		recordedRequest{http.MethodGet, request, "", ""},
		recordedRequest{http.MethodDelete, observation, "", ""},
	)
}

func TestRequestsArchiveRefusesToDeleteWhatIsNotThere(t *testing.T) {
	request := "/api/v1/observe/" + stoppedID
	f := newManageFake(t, manageReply{http.MethodGet, request, http.StatusOK, `{"request":{"id":"` + stoppedID + `","observation_id":null}}`})
	_, stderr, code := f.run("requests", "archive", stoppedID, "--delete-observation", "--yes")
	if code == 0 || !strings.Contains(stderr, "saved no observation") {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	f.wantRequests(t, recordedRequest{http.MethodGet, request, "", ""})
}

func TestRequestsArchiveNeedsYesToDeleteTheObservation(t *testing.T) {
	f := newManageFake(t)
	_, stderr, code := f.run("requests", "archive", stoppedID, "--delete-observation")
	if code != 2 || !strings.Contains(stderr, "--yes") {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	f.wantRequests(t)
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

func TestRequestsListFiltersAnchoredAndObservationAndShowsTheSpot(t *testing.T) {
	var got url.Values
	reply := `{"requests":[{"id":"` + stoppedID + `","kind":"investigate","title":"Spot","intent":"ask","state":"succeeded","dashboard_id":null,"dashboard_title":null,"created_at":"x","allowed_actions":[],"anchor":{"kind":"row","pos":{"x":0.4,"y":0.3},"row":{"service":"payments"},"shown":"440"},"block_id":null}],"next_cursor":null,"summary":{"total":1,"people":null}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(server.Close)
	stdout, stderr, code := runWith(server, "requests", "list", "--anchored", "--observation", testDashboard)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if got.Get("anchored") != "true" || got.Get("observation_id") != testDashboard {
		t.Fatalf("query %v", got)
	}
	if !strings.Contains(stdout, "AT") || !strings.Contains(stdout, "the row payments (440)") {
		t.Fatalf("stdout %q", stdout)
	}
	jsonOut, _, code := runWith(server, "--json", "requests", "list", "--anchored")
	if code != 0 || !strings.Contains(jsonOut, `"anchor":{"kind":"row"`) {
		t.Fatalf("json output %q", jsonOut)
	}
	if _, _, code := runWith(server, "requests", "list", "--observation", "nope"); code == 0 {
		t.Fatal("a bad observation id was sent")
	}
}

func TestChatsShowAndStatusNameTheSpotAQuestionPointsAt(t *testing.T) {
	const spot = `"anchor":{"kind":"row","pos":{"x":0.4,"y":0.3},"row":{"service":"payments"},"shown":"440"},"block_id":"` + testThread + `"`
	reply := `{"chat":{"id":"` + stoppedID + `","title":"T","dashboard_id":null,"dashboard_title":null,"observation_id":null,"pinned_at":null,"created_at":"x","last_turn_at":"x","turns":1,"last_turn":null},` +
		`"turns":[{"id":"00000000-0000-4000-8000-000000000001","state":"succeeded","prompt":"first",` + spot + `},{"id":"00000000-0000-4000-8000-000000000002","state":"succeeded","prompt":"second","anchor":null,"block_id":null},` +
		`{"id":"00000000-0000-4000-8000-000000000003","state":"succeeded","prompt":"third","anchor":null,"block_id":"` + testThread + `"}]}`
	_, stdout, stderr, code := runAgainst(t, http.StatusOK, reply, "chats", "show", stoppedID)
	if code != 0 || !strings.Contains(stdout, "Request 00000000-0000-4000-8000-000000000001 points at the row payments (440)") || strings.Contains(stdout, "000000000002 points") || !strings.Contains(stdout, "Request 00000000-0000-4000-8000-000000000003 is about block "+testThread) {
		t.Fatalf("chats show: exit %d, stdout:\n%s\nstderr %s", code, stdout, stderr)
	}
	status := `{"request":{"id":"` + stoppedID + `","status":"succeeded","state":"succeeded","intent":"ask",` + spot + `}}`
	_, stdout, stderr, code = runAgainst(t, http.StatusOK, status, "status", stoppedID)
	if code != 0 || !strings.Contains(stdout, "Points at      the row payments (440)") || !strings.Contains(stdout, "On block       "+testThread) {
		t.Fatalf("status: exit %d, stdout:\n%s\nstderr %s", code, stdout, stderr)
	}
	whole := `{"request":{"id":"` + stoppedID + `","status":"succeeded","state":"succeeded","intent":"ask","anchor":null,"block_id":"` + testThread + `"}}`
	_, stdout, stderr, code = runAgainst(t, http.StatusOK, whole, "status", stoppedID)
	if code != 0 || strings.Contains(stdout, "Points at") || !strings.Contains(stdout, "On block") {
		t.Fatalf("status of a whole-block question: exit %d, stdout:\n%s\nstderr %s", code, stdout, stderr)
	}
}
