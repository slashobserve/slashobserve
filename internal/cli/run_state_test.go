package cli

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRequestStateWordsAreTheSixOfTheWeb(t *testing.T) {
	for state, word := range map[string]string{
		"queued":    "Queued",
		"running":   "Running",
		"canceling": "Stopping",
		"succeeded": "Done",
		"failed":    "Failed",
		"canceled":  "Canceled",
		"paused":    "paused",
	} {
		if got := requestStateWord(state); got != word {
			t.Errorf("%s reads %q, want %q", state, got, word)
		}
	}
}

func requestReplies(t *testing.T, replies ...string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		at := min(int(calls.Add(1)), len(replies)) - 1
		_, _ = fmt.Fprintf(w, `{"request":%s}`, replies[at])
	}))
	t.Cleanup(server.Close)
	return server, &calls
}

func TestWaitingOnACanceledRequestSaysCanceledAndExitsOne(t *testing.T) {
	canceled := `{"id":"` + stoppedID + `","status":"failed","state":"canceled","failure_code":"canceled","error":"the run was canceled","steps":[],"allowed_actions":["keep_build","continue","retry","pin","archive"]}`
	server, _ := requestReplies(t, canceled)
	for _, args := range [][]string{{"status", stoppedID, "--wait"}, {"--json", "status", stoppedID, "--wait"}} {
		_, stderr, code := runWith(server, args...)
		if code != 1 || !strings.Contains(stderr, "observe canceled") || strings.Contains(stderr, "observe failed") {
			t.Fatalf("%v: exit %d, stderr %q", args, code, stderr)
		}
		if !strings.Contains(stderr, "observe continue "+stoppedID) {
			t.Fatalf("%v: stderr lacks the next step: %q", args, stderr)
		}
	}
}

func TestWaitingOnAFailedRequestGivesTheReasonAndTheNextStep(t *testing.T) {
	failed := `{"id":"` + stoppedID + `","status":"failed","state":"failed","error":"boom","steps":[],"allowed_actions":["retry","pin","archive"]}`
	server, _ := requestReplies(t, failed)
	stdout, stderr, code := runWith(server, "status", stoppedID, "--wait")
	if code != 1 || stdout != "" || strings.TrimSpace(stderr) != "observe failed: boom\nRun it again: observe retry "+stoppedID {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestWaitingOnAFailedRequestWithNoMoveLeftGivesOnlyTheReason(t *testing.T) {
	failed := `{"id":"` + stoppedID + `","status":"failed","state":"failed","error":null,"steps":[],"allowed_actions":[]}`
	server, _ := requestReplies(t, failed)
	_, stderr, code := runWith(server, "status", stoppedID, "--wait")
	if code != 1 || strings.TrimSpace(stderr) != "observe failed: the agent gave no reason" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
}

func TestWaitingFollowsTheStateUntilItSettles(t *testing.T) {
	turn := func(state string) string {
		return `{"id":"` + stoppedID + `","state":"` + state + `","steps":[],"allowed_actions":[]}`
	}
	server, calls := requestReplies(t, turn("queued"), turn("running"), turn("canceling"), turn("canceled"))
	base, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	c := &client{base: base, token: "k", userAgent: "test", http: server.Client()}
	out := &output{out: &bytes.Buffer{}, err: &bytes.Buffer{}}
	_, req, err := poll(t.Context(), c, newRequestProgress(c, out, stoppedID), stoppedID, 5*time.Second, 5*time.Millisecond)
	if err != nil || req.State != "canceled" || calls.Load() != 4 {
		t.Fatalf("got %v, state %q after %d reads", err, req.State, calls.Load())
	}
}

func TestStatusPrintsTheStateWordOfTheLists(t *testing.T) {
	for state, word := range map[string]string{"canceling": "Stopping", "canceled": "Canceled", "succeeded": "Done"} {
		var stdout bytes.Buffer
		printRequest(&output{out: &stdout, err: &bytes.Buffer{}}, agentRequest{ID: stoppedID, Status: "failed", State: state})
		if !strings.Contains(spaces.ReplaceAllString(stdout.String(), " "), "State "+word+"\n") {
			t.Errorf("%s: output lacks State %s:\n%s", state, word, stdout.String())
		}
	}
}

func TestPrintRequestTakesItsNextStepFromTheAllowedActions(t *testing.T) {
	continuedBy := "99999999-2222-3333-4444-555555555555"
	position := 2
	cases := []struct {
		name string
		req  agentRequest
		want []string
		none []string
	}{
		{"keep and continue", agentRequest{State: "failed", AllowedActions: []string{"keep_build", "continue", "retry"}}, []string{"Save it in the browser, or continue it: observe continue " + stoppedID}, []string{"observe retry"}},
		{"continue without keep", agentRequest{State: "canceled", AllowedActions: []string{"continue", "retry"}}, []string{"Continue it: observe continue " + stoppedID}, []string{"Save it in the browser", "observe retry"}},
		{"retry only", agentRequest{State: "canceled", AllowedActions: []string{"retry"}}, []string{"Run it again: observe retry " + stoppedID}, []string{"observe continue"}},
		{"no move left", agentRequest{State: "failed", AllowedActions: []string{"pin", "archive"}}, nil, []string{"observe retry", "observe continue"}},
		{"continued whatever the status says", agentRequest{State: "canceled", ContinuedBy: &continuedBy, AllowedActions: []string{"pin"}}, []string{"Continued by " + continuedBy}, []string{"observe retry"}},
		{"a queue position needs no status", agentRequest{State: "queued", QueuePosition: &position, AllowedActions: []string{"cancel"}}, []string{"Queued, position 2"}, []string{"observe retry"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.req.ID = stoppedID
			var stdout bytes.Buffer
			printRequest(&output{out: &stdout, err: &bytes.Buffer{}}, tc.req)
			got := stdout.String()
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Fatalf("output lacks %q:\n%s", want, got)
				}
			}
			for _, none := range tc.none {
				if strings.Contains(got, none) {
					t.Fatalf("output has %q:\n%s", none, got)
				}
			}
		})
	}
}
