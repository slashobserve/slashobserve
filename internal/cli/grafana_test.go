package cli

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	testConnection = "dddddddd-0000-4000-8000-000000000001"
	testSync       = "eeeeeeee-0000-4000-8000-000000000002"
)

type grafanaFake struct {
	server   *httptest.Server
	mu       sync.Mutex
	requests []recordedRequest
	polls    []string
	pollAt   int
}

func importJSON(status string, done, failed int, results string) string {
	return fmt.Sprintf(`{"id":%q,"connection_id":%q,"status":%q,"total":3,"done":%d,"failed":%d,"skipped":1,"error":null,"started_at":"2026-11-01T10:00:00Z","finished_at":null,"results":[%s]}`, testSync, testConnection, status, done, failed, results)
}

const failedResult = `{"uid":"pay","title":"Payments","outcome":"failed","dashboard_id":null,"unverified_panels":0,"placeholder_panels":0,"error":"Grafana answered with an error"},{"uid":"shop","title":"Shop","outcome":"imported","dashboard_id":null,"unverified_panels":2,"placeholder_panels":0,"error":null}`

func newGrafanaFake(t *testing.T) *grafanaFake {
	t.Helper()
	f := &grafanaFake{polls: []string{importJSON("finished", 2, 0, "")}}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		data, _ := io.ReadAll(r.Body)
		f.requests = append(f.requests, recordedRequest{method: r.Method, path: r.URL.Path, body: string(data)})
		w.Header().Set("Content-Type", "application/json")
		prefix := "/api/v1/grafana/" + testConnection
		switch {
		case r.URL.Path == prefix+"/folders":
			_, _ = io.WriteString(w, `[{"uid":"general","title":"General","dashboards":2},{"uid":"plat","title":"Platform / Checkout","dashboards":5}]`)
		case r.Method == http.MethodPost && r.URL.Path == prefix+"/imports":
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, importJSON("running", 0, 0, ""))
		case r.URL.Path == prefix+"/imports/"+testSync:
			if f.pollAt >= len(f.polls) {
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `{"error":"import not found","code":"not_found"}`)
				return
			}
			_, _ = io.WriteString(w, f.polls[f.pollAt])
			f.pollAt++
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":"not found","code":"not_found"}`)
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *grafanaFake) run(stdin string, args ...string) (string, string, int) {
	var stdout, stderr bytes.Buffer
	code := Main(append([]string{"--server", f.server.URL, "--token", "k", "grafana"}, args...), IO{In: strings.NewReader(stdin), Out: &stdout, Err: &stderr}, "test")
	return stdout.String(), stderr.String(), code
}

func TestGrafanaImportWithoutAChoiceListsFolders(t *testing.T) {
	f := newGrafanaFake(t)
	stdout, stderr, code := f.run("", "import", testConnection)
	if code != 2 || !strings.Contains(stderr, "choose folders with --folders <uid,...> or import everything with --all") {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, "UID") || !strings.Contains(stdout, "Platform / Checkout") || !strings.Contains(spaces.ReplaceAllString(stdout, " "), "plat Platform / Checkout 5") {
		t.Fatalf("stdout %q", stdout)
	}
}

func TestGrafanaImportRefusesBothFlagsAndBadIDs(t *testing.T) {
	for _, args := range [][]string{{"import", testConnection, "--all", "--folders", "a"}, {"import", "nope", "--all"}} {
		f := newGrafanaFake(t)
		_, stderr, code := f.run("", args...)
		if code != 2 || stderr == "" || len(f.requests) != 0 {
			t.Errorf("%v: exit %d, stderr %q, requests %d", args, code, stderr, len(f.requests))
		}
	}
}

func TestGrafanaImportSendsTheChosenFolders(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"--all"}, `{"folders":"all"}`},
		{[]string{"--folders", "a, b"}, `{"folders":["a","b"]}`},
	}
	for _, tc := range cases {
		f := newGrafanaFake(t)
		stdout, stderr, code := f.run("", append([]string{"import", testConnection}, tc.args...)...)
		if code != 0 || f.requests[0].body != tc.want {
			t.Errorf("%v: exit %d, body %s, stderr %q", tc.args, code, f.requests[0].body, stderr)
		}
		if stdout != "2 done, 1 skipped, 0 failed of 3\n" {
			t.Errorf("stdout %q", stdout)
		}
	}
}

func TestGrafanaImportReportsFailedDashboardsAndExitsOne(t *testing.T) {
	f := newGrafanaFake(t)
	f.polls = []string{importJSON("running", 1, 0, ""), importJSON("finished", 2, 1, failedResult)}
	stdout, stderr, code := f.run("", "import", testConnection, "--all")
	if code != 1 || !strings.Contains(stderr, "1 dashboard failed to import") || !strings.Contains(stderr, "1 dashboard has panels that did not verify") {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	want := "2 done, 1 skipped, 1 failed of 3\nUID TITLE ERROR\npay Payments Grafana answered with an error\n"
	if got := spaces.ReplaceAllString(stdout, " "); got != want {
		t.Fatalf("stdout %q, want %q", got, want)
	}
}

func TestPollGrafanaImportWaitsUntilItFinishes(t *testing.T) {
	f := newGrafanaFake(t)
	f.polls = []string{importJSON("running", 0, 0, ""), importJSON("running", 1, 0, ""), importJSON("finished", 2, 1, failedResult)}
	base, err := url.Parse(f.server.URL)
	if err != nil {
		t.Fatal(err)
	}
	c, err := (&app{version: "test"}).clientFor(base, "k")
	if err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	_, final, err := pollGrafanaImport(t.Context(), c, &output{out: &stdout, err: io.Discard}, testConnection, testSync, 10*time.Millisecond)
	if err != nil || final.Status != "finished" || final.Failed != 1 || f.pollAt != 3 || stdout.Len() != 0 {
		t.Fatalf("final %+v, err %v, polls %d, stdout %q", final, err, f.pollAt, stdout.String())
	}
}

func TestGrafanaImportFollowsTheStateTheServerSends(t *testing.T) {
	stateOnly := func(state string, done int) string {
		return strings.Replace(importJSON(state, done, 0, ""), `"status"`, `"state"`, 1)
	}
	f := newGrafanaFake(t)
	f.polls = []string{stateOnly("running", 1), stateOnly("canceled", 1)}
	_, stderr, code := f.run("", "import", testConnection, "--all")
	if code != 1 || f.pollAt != 2 || !strings.Contains(stderr, "the import canceled") {
		t.Fatalf("exit %d after %d reads, stderr %q", code, f.pollAt, stderr)
	}
}

func TestGrafanaImportPollsOnlyARunningImport(t *testing.T) {
	withState := func(state, status string) string {
		return strings.Replace(importJSON(status, 1, 0, ""), `"status"`, `"state":"`+state+`","status"`, 1)
	}
	f := newGrafanaFake(t)
	f.polls = []string{withState("paused", "running"), withState("paused", "finished")}
	if _, stderr, code := f.run("", "import", testConnection, "--all"); code != 0 || f.pollAt != 2 {
		t.Fatalf("an unknown state with a legacy status: exit %d after %d reads, stderr %q", code, f.pollAt, stderr)
	}
	stuck := importJSON("paused", 1, 0, "")
	f = newGrafanaFake(t)
	f.polls = []string{stuck}
	stdout, stderr, code := f.run("", "--json", "import", testConnection, "--all")
	if code != 1 || f.pollAt != 1 || strings.TrimSpace(stdout) != stuck || !strings.Contains(stderr, "the import stopped in a state this CLI does not know: paused") {
		t.Fatalf("an unknown status: exit %d after %d reads, stdout %q, stderr %q", code, f.pollAt, stdout, stderr)
	}
}

func TestGrafanaImportGoneWhilePollingExitsOne(t *testing.T) {
	f := newGrafanaFake(t)
	f.polls = nil
	_, stderr, code := f.run("", "import", testConnection, "--all")
	if code != 1 || !strings.Contains(stderr, "the server no longer has import "+testSync+"; a connection keeps only its last 20 imports") {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
}

func TestGrafanaImportJSONPrintsTheFinalImport(t *testing.T) {
	f := newGrafanaFake(t)
	stdout, _, code := f.run("", "--json", "import", testConnection, "--all")
	if code != 0 || strings.TrimSpace(stdout) != importJSON("finished", 2, 0, "") {
		t.Fatalf("exit %d, stdout %q", code, stdout)
	}
}
