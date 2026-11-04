package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func step(attempt int, at, message string) agentStep {
	return agentStep{Attempt: attempt, At: at, Phase: "query", Message: message}
}

func printedMessages(t *testing.T, batches ...[]agentStep) []string {
	t.Helper()
	var stderr bytes.Buffer
	out := &output{out: &bytes.Buffer{}, err: &stderr}
	var progress stepProgress
	var lines []string
	for _, batch := range batches {
		stderr.Reset()
		progress.print(out, batch)
		for _, line := range strings.Split(strings.TrimSpace(stderr.String()), "\n") {
			if line != "" {
				lines = append(lines, strings.Fields(line)[1])
			}
		}
	}
	return lines
}

func TestStepProgress(t *testing.T) {
	cases := []struct {
		name    string
		batches [][]agentStep
		want    string
	}{
		{"append", [][]agentStep{
			{step(0, "t1", "a1")},
			{step(0, "t1", "a1"), step(0, "t2", "a2")},
			{step(0, "t1", "a1"), step(0, "t2", "a2")},
		}, "a1 a2"},
		{"attempt change after appended steps", [][]agentStep{
			{step(0, "t1", "a1"), step(0, "t2", "a2")},
			{step(0, "t1", "a1"), step(0, "t2", "a2"), step(1, "t3", "b1")},
			{step(0, "t1", "a1"), step(0, "t2", "a2"), step(1, "t3", "b1"), step(1, "t4", "b2")},
		}, "a1 a2 b1 b2"},
		{"attempt change replacing the list", [][]agentStep{
			{step(0, "t1", "a1"), step(0, "t2", "a2"), step(0, "t3", "a3")},
			{step(1, "t4", "b1"), step(1, "t5", "b2"), step(1, "t6", "b3"), step(1, "t7", "b4")},
		}, "a1 a2 a3 b1 b2 b3 b4"},
		{"shrink", [][]agentStep{
			{step(0, "t1", "a1"), step(0, "t2", "a2"), step(0, "t3", "a3")},
			{step(0, "t1", "a1")},
		}, "a1 a2 a3 a1"},
		{"release then reclaim with the same attempt", [][]agentStep{
			{step(1, "t1", "a1"), step(1, "t2", "a2"), step(1, "t3", "a3")},
			{},
			{step(1, "u1", "b1"), step(1, "u2", "b2"), step(1, "u3", "b3"), step(1, "u4", "b4")},
		}, "a1 a2 a3 b1 b2 b3 b4"},
		{"reclaim then append", [][]agentStep{
			{step(1, "t1", "a1"), step(1, "t2", "a2")},
			{step(1, "u1", "b1"), step(1, "u2", "b2"), step(1, "u3", "b3")},
			{step(1, "u1", "b1"), step(1, "u2", "b2"), step(1, "u3", "b3"), step(1, "u4", "b4")},
		}, "a1 a2 b1 b2 b3 b4"},
		{"first poll after earlier attempts", [][]agentStep{
			{step(0, "t1", "a1"), step(1, "t2", "b1")},
		}, "a1 b1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.Join(printedMessages(t, tc.batches...), " ")
			if got != tc.want {
				t.Fatalf("printed %q, want %q", got, tc.want)
			}
		})
	}
}

type pollStub struct {
	statuses []int
	calls    atomic.Int32
}

func (s *pollStub) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	index := int(s.calls.Add(1)) - 1
	status := s.statuses[min(index, len(s.statuses)-1)]
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	switch status {
	case http.StatusOK:
		_, _ = fmt.Fprint(w, `{"request":{"id":"r","status":"succeeded","url":"http://x/d","steps":[]}}`)
	case http.StatusAccepted:
		_, _ = fmt.Fprint(w, `{"request":{"id":"r","status":"running","steps":[]}}`)
	default:
		_, _ = fmt.Fprint(w, `{"error":"boom","code":"x"}`)
	}
}

func pollAgainst(ctx context.Context, t *testing.T, statuses []int, timeout time.Duration) (*pollStub, agentRequest, error) {
	t.Helper()
	stub := &pollStub{statuses: statuses}
	server := httptest.NewServer(stub)
	t.Cleanup(server.Close)
	base, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	c := &client{base: base, token: "k", userAgent: "test", http: server.Client()}
	out := &output{out: &bytes.Buffer{}, err: &bytes.Buffer{}}
	_, req, err := poll(ctx, c, out, "r", timeout, 5*time.Millisecond)
	return stub, req, err
}

func TestPollRidesOutTwoServerErrors(t *testing.T) {
	stub, req, err := pollAgainst(context.Background(), t, []int{503, 503, 200}, 5*time.Second)
	if err != nil || req.Status != "succeeded" || stub.calls.Load() != 3 {
		t.Fatalf("got %v, %q after %d calls", err, req.Status, stub.calls.Load())
	}
}

func TestPollResetsTheFailureCountOnSuccess(t *testing.T) {
	stub, req, err := pollAgainst(context.Background(), t, []int{503, 503, 202, 503, 503, 200}, 5*time.Second)
	if err != nil || req.Status != "succeeded" || stub.calls.Load() != 6 {
		t.Fatalf("got %v, %q after %d calls", err, req.Status, stub.calls.Load())
	}
}

func TestPollGivesUpAfterThreeServerErrors(t *testing.T) {
	stub, _, err := pollAgainst(context.Background(), t, []int{503}, 5*time.Second)
	if err == nil || !strings.Contains(err.Error(), "observe status r") || stub.calls.Load() != 3 {
		t.Fatalf("got %v after %d calls", err, stub.calls.Load())
	}
}

func TestPollStopsAtOnceOnUnauthorized(t *testing.T) {
	stub, _, err := pollAgainst(context.Background(), t, []int{401}, 5*time.Second)
	if !errors.Is(err, errUnauthorized) || stub.calls.Load() != 1 {
		t.Fatalf("got %v after %d calls", err, stub.calls.Load())
	}
}

func TestPollStopsAtOnceOnClientErrors(t *testing.T) {
	stub, _, err := pollAgainst(context.Background(), t, []int{404}, 5*time.Second)
	if err == nil || stub.calls.Load() != 1 {
		t.Fatalf("got %v after %d calls", err, stub.calls.Load())
	}
}

func TestPollReportsTheDeadline(t *testing.T) {
	_, _, err := pollAgainst(context.Background(), t, []int{202}, 60*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "gave up after") || !strings.Contains(err.Error(), "observe status r") {
		t.Fatalf("got %v", err)
	}
}

func TestPollReportsAnInterrupt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(60*time.Millisecond, cancel)
	_, _, err := pollAgainst(ctx, t, []int{202}, 5*time.Second)
	if err == nil || !strings.Contains(err.Error(), "still running on the server: observe status r") || strings.Contains(err.Error(), "gave up") {
		t.Fatalf("got %v", err)
	}
}

func TestShowPrintsEveryPanelShape(t *testing.T) {
	const id = "11111111-2222-3333-4444-555555555555"
	body := `{"dashboard":{"id":"` + id + `","title":"Shop","latest_version":1,"updated_at":"2026-01-01T00:00:00Z","updated_by":{"name":"Ada"}},
"version":{"version":1,"summary":"s","author":{"name":"Ada"},"created_at":"2026-01-01T00:00:00Z","spec":{"specVersion":"0","title":"Shop","timeRange":{"from":"now-1h","to":"now"},"futureField":{"a":1},"panels":[
{"id":"old","type":"timeseries","title":"Legacy","gridPos":{"x":0,"y":0,"w":6,"h":4},"datasource":{"type":"prometheus","uid":"prom"},"query":"up"},
{"id":"row","type":"row","title":"Row","gridPos":{"x":0,"y":4,"w":24,"h":1},"collapsed":true,"panels":[
{"id":"child","type":"stat","title":"Child","gridPos":{"x":0,"y":5,"w":6,"h":4},"datasource":"prom","queries":[{"query":"up","datasource":"ch"},{"query":"up"}]}]},
{"id":"note","type":"text","gridPos":{"x":0,"y":9,"w":6,"h":4},"content":"hi","newThing":[1,2]}]}}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, body)
	}))
	t.Cleanup(server.Close)
	var stdout, stderr bytes.Buffer
	code := Main([]string{"--server", server.URL, "--token", "k", "dashboards", "show", id}, IO{In: strings.NewReader(""), Out: &stdout, Err: &stderr}, "test")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	for _, want := range []string{"old", "prom", "row", "child", "ch", "note"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("output lacks %q:\n%s", want, stdout.String())
		}
	}
}

const stoppedID = "11111111-2222-3333-4444-555555555555"

type controlCall struct {
	method string
	path   string
	body   string
}

func runAgainst(t *testing.T, status int, reply string, args ...string) (controlCall, string, string, int) {
	t.Helper()
	var call controlCall
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := new(bytes.Buffer)
		_, _ = body.ReadFrom(r.Body)
		call = controlCall{method: r.Method, path: r.URL.Path, body: body.String()}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = fmt.Fprint(w, reply)
	}))
	t.Cleanup(server.Close)
	var stdout, stderr bytes.Buffer
	full := append([]string{"--server", server.URL, "--token", "k"}, args...)
	code := Main(full, IO{In: strings.NewReader(""), Out: &stdout, Err: &stderr}, "test")
	return call, stdout.String(), stderr.String(), code
}

func TestCancelPostsAndPrintsTheRequest(t *testing.T) {
	reply := `{"request":{"id":"` + stoppedID + `","status":"failed","source":"observe","failure_code":"canceled","steps":[]}}`
	call, stdout, stderr, code := runAgainst(t, http.StatusOK, reply, "cancel", stoppedID)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if call.method != http.MethodPost || call.path != "/api/v1/observe/"+stoppedID+"/cancel" || call.body != "" {
		t.Fatalf("got %+v", call)
	}
	if !strings.Contains(stdout, stoppedID) || !strings.Contains(stdout, "failed") {
		t.Fatalf("output lacks the request:\n%s", stdout)
	}
}

func TestCancelAndContinueErrorLines(t *testing.T) {
	cases := []struct {
		name   string
		args   []string
		status int
		reply  string
		want   string
	}{
		{"cancel forbidden", []string{"cancel", stoppedID}, 403, `{"error":"your role does not allow this","code":"forbidden_role"}`, "only the requester can cancel or continue " + stoppedID},
		{"continue forbidden", []string{"continue", stoppedID}, 403, `{"error":"x","code":"forbidden_role"}`, "only the requester can cancel or continue " + stoppedID},
		{"cancel finished", []string{"cancel", stoppedID}, 409, `{"error":"the request already finished","code":"request_finished"}`, "request " + stoppedID + " already finished"},
		{"not continuable", []string{"continue", stoppedID}, 409, `{"error":"x","code":"not_continuable"}`, "request " + stoppedID + " has no stopped build to continue"},
		{"version conflict", []string{"continue", stoppedID}, 409, `{"error":"x","code":"version_conflict","latest_version":7}`, "newer version (7)"},
		{"version conflict without a number", []string{"continue", stoppedID}, 409, `{"error":"x","code":"version_conflict"}`, "cannot be continued: the dashboard has a newer version than"},
		{"invalid note", []string{"continue", stoppedID, "--note", " "}, 400, `{"error":"note must be 1 to 4000 characters","code":"invalid_request"}`, "note must be 1 to 4000 characters"},
		{"missing", []string{"cancel", stoppedID}, 404, `{"error":"request not found","code":"not_found"}`, "request not found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, stderr, code := runAgainst(t, tc.status, tc.reply, tc.args...)
			if code != 1 || !strings.Contains(stderr, tc.want) {
				t.Fatalf("exit %d, stderr %q, want %q", code, stderr, tc.want)
			}
		})
	}
}

func TestContinueSendsTheNoteOnlyWhenGiven(t *testing.T) {
	const newID = "99999999-2222-3333-4444-555555555555"
	const created = `{"request":{"id":"` + newID + `","status":"queued","steps":[]}}`
	cases := []struct {
		name string
		args []string
		body string
	}{
		{"no note", []string{"continue", stoppedID, "--no-wait"}, ""},
		{"note", []string{"continue", stoppedID, "--no-wait", "--note", "keep the latency row"}, `{"note":"keep the latency row"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			call, stdout, stderr, code := runAgainst(t, http.StatusCreated, created, tc.args...)
			if code != 0 {
				t.Fatalf("exit %d: %s", code, stderr)
			}
			if call.method != http.MethodPost || call.path != "/api/v1/observe/"+stoppedID+"/continue" || call.body != tc.body {
				t.Fatalf("got %+v", call)
			}
			if strings.TrimSpace(stdout) != newID {
				t.Fatalf("stdout %q", stdout)
			}
		})
	}
}

func TestControlCommandsRejectABadID(t *testing.T) {
	for _, name := range []string{"cancel", "continue"} {
		_, _, stderr, code := runAgainst(t, http.StatusOK, `{}`, name, "nope")
		if code != 2 || !strings.Contains(stderr, "not a valid id") {
			t.Fatalf("%s: exit %d, stderr %q", name, code, stderr)
		}
	}
}

func TestPrintRequestExplainsQueueAndStoppedBuilds(t *testing.T) {
	position, saved, continuedBy := 3, "2026-11-03T00:00:00Z", "99999999-2222-3333-4444-555555555555"
	build := &buildSummary{Panels: 2}
	cases := []struct {
		name string
		req  agentRequest
		want string
		none string
	}{
		{"queued with a position", agentRequest{ID: stoppedID, Status: "queued", QueuePosition: &position}, "Queued, position 3", ""},
		{"queued without a position", agentRequest{ID: stoppedID, Status: "queued"}, "", "Queued, position"},
		{"failed with a build", agentRequest{ID: stoppedID, Status: "failed", Build: build}, "Save it in the browser, or continue it: observe continue " + stoppedID, ""},
		{"failed with an empty build", agentRequest{ID: stoppedID, Status: "failed", Build: &buildSummary{}}, "", "continue"},
		{"failed with a saved build", agentRequest{ID: stoppedID, Status: "failed", Build: build, BuildSavedAt: &saved}, "", "continue"},
		{"failed and continued", agentRequest{ID: stoppedID, Status: "failed", Build: build, ContinuedBy: &continuedBy}, "Continued by " + continuedBy, "Save it in the browser"},
		{"running with a build", agentRequest{ID: stoppedID, Status: "running", Build: build}, "", "Save it in the browser"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout bytes.Buffer
			printRequest(&output{out: &stdout, err: &bytes.Buffer{}}, tc.req)
			got := stdout.String()
			if tc.want != "" && !strings.Contains(got, tc.want) {
				t.Fatalf("output lacks %q:\n%s", tc.want, got)
			}
			if tc.none != "" && strings.Contains(got, tc.none) {
				t.Fatalf("output has %q:\n%s", tc.none, got)
			}
		})
	}
}
