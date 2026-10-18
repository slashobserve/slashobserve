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
