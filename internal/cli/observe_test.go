package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
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
	_, req, err := poll(ctx, c, newRequestProgress(c, out, "r"), "r", timeout, 5*time.Millisecond)
	return stub, req, err
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
	reply := `{"request":{"id":"` + stoppedID + `","status":"failed","intent":"ask","failure_code":"canceled","steps":[]}}`
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
		{"cancel forbidden", []string{"cancel", stoppedID}, 403, `{"error":"your role does not allow this","code":"forbidden_role"}`, "only the requester can cancel or continue " + stoppedID + " (forbidden_role)"},
		{"continue forbidden", []string{"continue", stoppedID}, 403, `{"error":"x","code":"forbidden_role"}`, "only the requester can cancel or continue " + stoppedID + " (forbidden_role)"},
		{"forbidden without a code", []string{"cancel", stoppedID}, 403, `{"error":"x"}`, "only the requester can cancel or continue " + stoppedID},
		{"cancel finished", []string{"cancel", stoppedID}, 409, `{"error":"the request already finished","code":"request_finished"}`, "request " + stoppedID + " already finished"},
		{"not continuable", []string{"continue", stoppedID}, 409, `{"error":"x","code":"not_continuable"}`, "request " + stoppedID + " has no stopped build to continue"},
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

func TestRetryPostsAndPrintsTheNewRequest(t *testing.T) {
	const newID = "99999999-2222-3333-4444-555555555555"
	const created = `{"request":{"id":"` + newID + `","status":"queued","state":"queued","steps":[]}}`
	call, stdout, stderr, code := runAgainst(t, http.StatusAccepted, created, "retry", stoppedID, "--no-wait")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if call.method != http.MethodPost || call.path != "/api/v1/observe/"+stoppedID+"/retry" || call.body != "" {
		t.Fatalf("got %+v", call)
	}
	if strings.TrimSpace(stdout) != newID {
		t.Fatalf("stdout %q", stdout)
	}
}

func TestRetryErrorLines(t *testing.T) {
	cases := []struct {
		name   string
		status int
		reply  string
		want   string
	}{
		{"forbidden", 403, `{"error":"your role does not allow this","code":"forbidden_role"}`, "you may not start this kind of request on the dashboard of " + stoppedID},
		{"not finished", 409, `{"error":"retry is not allowed while the run is running","code":"invalid_transition","state":"running","allowed_actions":["cancel","pin","archive"]}`, "request " + stoppedID + " is running and cannot be retried"},
		{"conflict without a state", 409, `{"error":"x","code":"invalid_transition"}`, "x"},
		{"hidden", 404, `{"error":"agent request not found","code":"not_found"}`, "agent request not found"},
		{"agent unavailable", 503, `{"error":"no agent is configured","code":"agent_unavailable"}`, "no agent is configured"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, stderr, code := runAgainst(t, tc.status, tc.reply, "retry", stoppedID)
			if code != 1 || !strings.Contains(stderr, tc.want) {
				t.Fatalf("exit %d, stderr %q, want %q", code, stderr, tc.want)
			}
		})
	}
}

func TestSaveAnswerPostsAndPrintsTheObservationAddress(t *testing.T) {
	const saved = `{"observation_id":"` + observationID + `","url":"https://observe.example.com/o/x","replayed":false}`
	call, stdout, stderr, code := runAgainst(t, http.StatusCreated, saved, "save-answer", stoppedID)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if call.method != http.MethodPost || call.path != "/api/v1/observe/"+stoppedID+"/observation" || call.body != "" {
		t.Fatalf("got %+v", call)
	}
	if strings.TrimSpace(stdout) != "Saved as an observation: https://observe.example.com/o/x" {
		t.Fatalf("stdout %q", stdout)
	}
	_, stdout, _, _ = runAgainst(t, http.StatusOK, strings.Replace(saved, "false", "true", 1), "save-answer", stoppedID)
	if strings.TrimSpace(stdout) != "Already an observation: https://observe.example.com/o/x" {
		t.Fatalf("repeat stdout %q", stdout)
	}
}

func TestSaveAnswerErrorLines(t *testing.T) {
	cases := []struct {
		name   string
		status int
		reply  string
		want   string
	}{
		{"no answer", 409, `{"error":"x","code":"no_answer"}`, "request " + stoppedID + " has no answer to save"},
		{"not yours", 404, `{"error":"agent request not found","code":"not_found"}`, "only the person who made it can save its answer"},
		{"no folder", 403, `{"error":"x","code":"forbidden_role"}`, "your role or your folders do not allow it"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, stderr, code := runAgainst(t, tc.status, tc.reply, "save-answer", stoppedID)
			if code != 1 || !strings.Contains(stderr, tc.want) {
				t.Fatalf("exit %d, stderr %q, want %q", code, stderr, tc.want)
			}
		})
	}
}

func TestControlCommandsRejectABadID(t *testing.T) {
	for _, name := range []string{"cancel", "continue", "retry", "save-answer"} {
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
		{"failed with a build", agentRequest{ID: stoppedID, Status: "failed", State: "failed", Build: build, AllowedActions: []string{"keep_build", "continue", "retry", "pin", "archive"}}, "Save it in the browser, or continue it: observe continue " + stoppedID, "observe retry"},
		{"failed with an empty build", agentRequest{ID: stoppedID, Status: "failed", State: "failed", Build: &buildSummary{}, AllowedActions: []string{"retry", "pin", "archive"}}, "Run it again: observe retry " + stoppedID, "continue"},
		{"failed with a saved build", agentRequest{ID: stoppedID, Status: "failed", State: "failed", Build: build, BuildSavedAt: &saved, AllowedActions: []string{"retry", "pin", "archive"}}, "Run it again: observe retry " + stoppedID, "continue"},
		{"failed and continued", agentRequest{ID: stoppedID, Status: "failed", State: "failed", Build: build, ContinuedBy: &continuedBy, AllowedActions: []string{"pin", "archive"}}, "Continued by " + continuedBy, "Save it in the browser"},
		{"running with a build", agentRequest{ID: stoppedID, Status: "running", State: "running", Build: build, AllowedActions: []string{"cancel", "pin", "archive"}}, "State", "Save it in the browser"},
		{"canceled with a build", agentRequest{ID: stoppedID, Status: "failed", State: "canceled", Build: build, AllowedActions: []string{"keep_build", "continue", "retry", "pin", "archive"}}, "State          Canceled", "observe retry"},
		{"succeeded and retryable", agentRequest{ID: stoppedID, Status: "succeeded", State: "succeeded", AllowedActions: []string{"retry", "pin", "archive"}}, "", "Run it again"},
		{"an older server sends no actions", agentRequest{ID: stoppedID, Status: "failed", Build: build}, "", "Save it in the browser"},
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

func TestPrintRequestShowsWhatStartedTheRun(t *testing.T) {
	var stdout bytes.Buffer
	printRequest(&output{out: &stdout, err: &bytes.Buffer{}}, agentRequest{ID: stoppedID, Status: "running", Intent: "apply", Trigger: "person"})
	lines := strings.Split(stdout.String(), "\n")
	if len(lines) < 6 || strings.Fields(lines[3])[0] != "Intent" || strings.Fields(lines[3])[1] != "apply" || strings.Fields(lines[5])[0] != "Trigger" || strings.Fields(lines[5])[1] != "person" {
		t.Fatalf("output lacks the trigger after the intent:\n%s", stdout.String())
	}
}

func TestPrintRequestSaysWhetherTheRunIsAnInvestigation(t *testing.T) {
	for _, tc := range []struct {
		investigation bool
		want          string
	}{{false, "no"}, {true, "yes"}} {
		var stdout bytes.Buffer
		printRequest(&output{out: &stdout, err: &bytes.Buffer{}}, agentRequest{ID: stoppedID, Status: "succeeded", Intent: "ask", Investigation: tc.investigation})
		fields := strings.Fields(strings.Split(stdout.String(), "\n")[4])
		if len(fields) != 2 || fields[0] != "Investigation" || fields[1] != tc.want {
			t.Fatalf("investigation %v: line = %q, want Investigation %s:\n%s", tc.investigation, fields, tc.want, stdout.String())
		}
	}
}

func TestPrintRequestSaysTheWordsOfARedactedRunAreHidden(t *testing.T) {
	var stdout bytes.Buffer
	printRequest(&output{out: &stdout, err: &bytes.Buffer{}}, agentRequest{ID: stoppedID, Status: "succeeded", Intent: "apply", Redacted: true})
	hidden := map[string]bool{}
	for _, line := range strings.Split(stdout.String(), "\n") {
		if label, rest, ok := strings.Cut(line, " "); ok && strings.TrimSpace(rest) == privateChatWords {
			hidden[label] = true
		}
	}
	if !hidden["Title"] || !hidden["Prompt"] || len(hidden) != 2 {
		t.Fatalf("hidden lines = %v, want Title and Prompt:\n%s", hidden, stdout.String())
	}
}

func TestPrintRequestNamesTheObservationAnAskIsAbout(t *testing.T) {
	about := "0b1d2c3e-4f50-4617-8899-aabbccddeeff"
	for _, tc := range []struct {
		subject *string
		want    bool
	}{{&about, true}, {nil, false}} {
		var stdout bytes.Buffer
		printRequest(&output{out: &stdout, err: &bytes.Buffer{}}, agentRequest{ID: stoppedID, Status: "succeeded", Intent: "ask", SubjectID: tc.subject})
		shown := false
		for _, line := range strings.Split(stdout.String(), "\n") {
			if fields := strings.Fields(line); len(fields) == 3 && fields[0] == "About" && fields[1] == "observation" && fields[2] == about {
				shown = true
			}
		}
		if shown != tc.want {
			t.Fatalf("subject %v: About observation shown %v, want %v:\n%s", tc.subject, shown, tc.want, stdout.String())
		}
	}
}

func TestPrintRequestShowsTheLiveTextOfARunningRun(t *testing.T) {
	live := "Checking the p99 of checkout.\n\nNow reading the error rate.\x1b[31m"
	var stdout bytes.Buffer
	printRequest(&output{out: &stdout, err: &bytes.Buffer{}}, agentRequest{ID: stoppedID, Status: "running", LiveText: &live})
	want := "\nLive text\n  Checking the p99 of checkout.\n  \n  Now reading the error rate. [31m\n"
	if !strings.HasSuffix(stdout.String(), want) {
		t.Fatalf("output does not end with the live text:\n%q", stdout.String())
	}
	var without bytes.Buffer
	printRequest(&output{out: &without, err: &bytes.Buffer{}}, agentRequest{ID: stoppedID, Status: "running"})
	if strings.Contains(without.String(), "Live text") {
		t.Fatalf("a run with no live text prints a Live text section:\n%s", without.String())
	}
}

func TestPrintRequestShowsTheRunCost(t *testing.T) {
	cost := func(v float64) *requestUsage { return &requestUsage{CostUSD: &v} }
	cases := []struct {
		name  string
		usage *requestUsage
		want  string
	}{
		{"no usage", nil, "-"},
		{"an unpriced model", &requestUsage{}, "-"},
		{"free", cost(0), "$0.00"},
		{"a fraction of a cent", cost(0.0042), "under $0.01"},
		{"one cent", cost(0.01), "about $0.01"},
		{"normal", cost(0.1234), "about $0.12"},
		{"large", cost(12.5), "about $12.50"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout bytes.Buffer
			printRequest(&output{out: &stdout, err: &bytes.Buffer{}}, agentRequest{ID: stoppedID, Status: "succeeded", Trigger: "person", Usage: tc.usage})
			lines := strings.Split(stdout.String(), "\n")
			fields := strings.Fields(lines[6])
			if len(fields) < 2 || fields[0] != "Cost" || strings.Join(fields[1:], " ") != tc.want {
				t.Fatalf("Cost line = %q, want %q after the trigger:\n%s", lines[6], tc.want, stdout.String())
			}
		})
	}
}

func TestAPromptThatOnlyAsksAQuestionPrintsTheFinding(t *testing.T) {
	var posted string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			data, _ := io.ReadAll(r.Body)
			posted = string(data)
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"request":{"id":"`+investigationID+`","kind":"build","intent":"ask","status":"queued","steps":[]}}`)
			return
		}
		_, _ = io.WriteString(w, `{"request":{"id":"`+investigationID+`","kind":"build","intent":"ask","status":"succeeded","dashboard_id":null,"result_version":null,"url":"https://observe.example.com/findings/f1","steps":[],"finding":`+findingJSON+`}}`)
	}))
	t.Cleanup(server.Close)

	stdout, stderr, code := runCLI("--server", server.URL, "--token", "k", "-q", "what", "logs", "are", "available?")
	if code != 0 {
		t.Fatalf("code %d stderr %q", code, stderr)
	}
	want := "Checkout errors rose after the deploy\n\nErrors rose at 10:02.\nThe deploy at 10:00 added a retry.\n\nhttps://observe.example.com/findings/f1\n"
	if stdout != want || posted != `{"prompt":"what logs are available?"}` {
		t.Errorf("stdout %q posted %q", stdout, posted)
	}
}

const askOnDashboardID = "2f0f2c1e-6a53-4a35-8a0e-1d3b0d1f7a11"

func TestAFindingFromHomeSaysWhichDashboardToAskOn(t *testing.T) {
	finding := `{"id":"f1","title":"Add a stat panel","markdown":"The change goes on Checkout.","url":"https://observe.example.com/o/f1","ask_on":{"dashboard_id":"` + askOnDashboardID + `","title":"Checkout"}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"request":{"id":"`+investigationID+`","kind":"build","intent":"ask","status":"queued","steps":[]}}`)
			return
		}
		_, _ = io.WriteString(w, `{"request":{"id":"`+investigationID+`","kind":"build","intent":"ask","status":"succeeded","prompt":"on Checkout add Ada's stat panel","dashboard_id":null,"result_version":null,"steps":[],"finding":`+finding+`}}`)
	}))
	t.Cleanup(server.Close)

	stdout, stderr, code := runCLI("--server", server.URL, "--token", "k", "-q", "on", "Checkout", "add", "Ada's", "stat", "panel")
	if code != 0 {
		t.Fatalf("code %d stderr %q", code, stderr)
	}
	want := "Add a stat panel\n\nThe change goes on Checkout.\n\nhttps://observe.example.com/o/f1\n\nThis change is made on the dashboard Checkout. Ask there:\n  observe --revise --dashboard " + askOnDashboardID + ` 'on Checkout add Ada'\''s stat panel'` + "\n"
	if stdout != want {
		t.Errorf("stdout %q\nwant %q", stdout, want)
	}
}

func TestPrintRequestShowsTheTitleAndPrompt(t *testing.T) {
	var stdout bytes.Buffer
	printRequest(&output{out: &stdout, err: &bytes.Buffer{}}, agentRequest{ID: stoppedID, Status: "failed", Title: "Checkout latency", Prompt: "checkout p99\nlast 6h"})
	for _, want := range []string{"Title", "Checkout latency", "Prompt", "checkout p99 last 6h"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("output lacks %q:\n%s", want, stdout.String())
		}
	}
}

func TestApplyPostsTheAnswerInPlaceOfAPrompt(t *testing.T) {
	var posted string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		posted = string(data)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, `{"request":{"id":"`+investigationID+`","kind":"build","intent":"apply","status":"queued","steps":[]}}`)
	}))
	t.Cleanup(server.Close)

	_, stderr, code := runCLI("--server", server.URL, "--token", "k", "--no-wait", "--revise", "--dashboard", askOnDashboardID, "--apply", investigationID)
	if code != 0 {
		t.Fatalf("code %d stderr %q", code, stderr)
	}
	if want := `{"applies_finding_id":"` + investigationID + `","dashboard_id":"` + askOnDashboardID + `","kind":"revise"}`; posted != want {
		t.Errorf("posted %s, want %s", posted, want)
	}
	for _, args := range [][]string{
		{"--revise", "--dashboard", askOnDashboardID, "--apply", investigationID, "add", "a", "panel"},
		{"--dashboard", askOnDashboardID, "--apply", investigationID},
		{"--revise", "--apply", investigationID},
	} {
		if _, _, code := runCLI(append([]string{"--server", server.URL, "--token", "k", "--no-wait"}, args...)...); code == 0 {
			t.Errorf("observe %v succeeded", args)
		}
	}
}
