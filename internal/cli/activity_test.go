package cli

import (
	"context"
	"errors"
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

const testFolder = "99999999-8888-7777-6666-555555555555"

func activityJSON(seq int, kind, label, at, data string) string {
	return fmt.Sprintf(`{"seq":%d,"type":%q,"target":{"kind":"dashboard","id":%q},"dashboard_id":%q,"folder_id":null,"actor":{"kind":"user","id":"u1","name":"Sam","origin":"web","client":null,"run_id":null,"label":%q},"data":%s,"created_at":%q}`,
		seq, kind, testDashboard, testDashboard, label, data, at)
}

type activityFake struct {
	server *httptest.Server
	mu     sync.Mutex

	page     string
	streams  []func(w http.ResponseWriter, r *http.Request)
	list     []recordedRequest
	opened   []string
	accepted []string
}

func newActivityFake(t *testing.T, page string, streams ...func(w http.ResponseWriter, r *http.Request)) *activityFake {
	t.Helper()
	f := &activityFake{page: page, streams: streams}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer k" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/v1/activity":
			f.list = append(f.list, recordedRequest{method: r.Method, path: r.URL.Path, query: r.URL.RawQuery})
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, f.page)
		case "/api/v1/activity/stream":
			n := len(f.opened)
			f.opened = append(f.opened, r.Header.Get("Last-Event-ID"))
			f.accepted = append(f.accepted, r.Header.Get("Accept"))
			if n < len(f.streams) {
				f.streams[n](w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"error":"stop","code":"stop_here"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *activityFake) run(args ...string) (string, string, int) {
	return runCLI(append([]string{"--server", f.server.URL, "--token", "k"}, args...)...)
}

func sendFrames(frames string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, frames)
	}
}

func status(code int) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_, _ = io.WriteString(w, `{"error":"try later","code":"too_many_streams"}`)
	}
}

func activityFrame(seq int, kind, label, at, data string) string {
	return fmt.Sprintf("id: %d\nevent: activity\ndata: %s\n\n", seq, activityJSON(seq, kind, label, at, data))
}

func twoEventPage() string {
	newest := activityJSON(12, "dashboard.version_created", "Sam", "2026-11-01T10:05:09Z", `{"version":12,"title":"Checkout","summary":"x"}`)
	oldest := activityJSON(11, "run.failed", "/observe for Sam", "2026-11-01T10:00:00Z", `{"source":"prompt","failure_code":"no_progress"}`)
	return `{"activity":[` + newest + `,` + oldest + `],"next_cursor":null}`
}

func TestActivityPrintsTheNewestEventsOldestFirst(t *testing.T) {
	f := newActivityFake(t, twoEventPage())
	stdout, stderr, code := f.run("activity", "--dashboard", testDashboard, "--folder", testFolder, "--type", "run.failed", "--type", "dashboard.moved", "--since", "2026-11-01T00:00:00Z", "--limit", "5")
	if code != 0 || stderr != "" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	want := "2026-11-01T10:00:00Z  /observe for Sam  run failed (no_progress)\n" +
		"2026-11-01T10:05:09Z  Sam  saved version 12 of \"Checkout\"\n"
	if stdout != want {
		t.Errorf("stdout %q, want %q", stdout, want)
	}
	if len(f.list) != 1 || f.list[0].method != http.MethodGet {
		t.Fatalf("requests %+v", f.list)
	}
	got, err := url.ParseQuery(f.list[0].query)
	if err != nil {
		t.Fatal(err)
	}
	wantQuery := url.Values{
		"dashboard_id": {testDashboard}, "folder_id": {testFolder}, "type": {"run.failed", "dashboard.moved"},
		"since": {"2026-11-01T00:00:00Z"}, "limit": {"5"},
	}
	if got.Encode() != wantQuery.Encode() {
		t.Errorf("query %v, want %v", got, wantQuery)
	}
}

func TestActivityAsksForTwentyEventsByDefault(t *testing.T) {
	f := newActivityFake(t, `{"activity":[],"next_cursor":null}`)
	f.run("activity")
	if len(f.list) != 1 || f.list[0].query != "limit=20" {
		t.Fatalf("requests %+v", f.list)
	}
}

func TestActivityJSONPrintsTheServerBody(t *testing.T) {
	f := newActivityFake(t, twoEventPage())
	stdout, _, code := f.run("activity", "--json")
	if code != 0 || stdout != twoEventPage()+"\n" {
		t.Fatalf("exit %d, stdout %q", code, stdout)
	}
}

func TestActivityNamesAnEmptyLog(t *testing.T) {
	f := newActivityFake(t, `{"activity":[],"next_cursor":null}`)
	stdout, stderr, code := f.run("activity")
	if code != 0 || stdout != "" || stderr != "No activity yet\n" {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestActivityRefusesBadFlagsBeforeAskingTheServer(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"limit zero", []string{"--limit", "0"}, "--limit must be 1 to 100"},
		{"limit too high", []string{"--limit", "101"}, "--limit must be 1 to 100"},
		{"dashboard", []string{"--dashboard", "nope"}, `"nope" is not a valid id`},
		{"folder", []string{"--folder", "nope"}, `"nope" is not a valid id`},
		{"since word", []string{"--since", "yesterday"}, `--since "yesterday" is not a duration`},
		{"since negative", []string{"--since", "-1h"}, `--since "-1h" is not a duration`},
		{"since zero", []string{"--since", "0s"}, `--since "0s" is not a duration`},
		{"argument", []string{"extra"}, `unexpected argument "extra"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newActivityFake(t, twoEventPage())
			stdout, stderr, code := f.run(append([]string{"activity"}, tc.args...)...)
			if code != 2 || stdout != "" || !strings.Contains(stderr, tc.want) {
				t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
			}
			if len(f.list) != 0 {
				t.Errorf("asked the server: %+v", f.list)
			}
		})
	}
}

func TestParseSince(t *testing.T) {
	now := time.Date(2026, 11, 1, 10, 0, 0, 0, time.FixedZone("IST", 5*3600+1800))
	cases := []struct{ raw, want string }{
		{"", ""},
		{"24h", "2026-10-31T04:30:00Z"},
		{"90m", "2026-11-01T03:00:00Z"},
		{"2026-11-01T12:00:00+02:00", "2026-11-01T12:00:00+02:00"},
	}
	for _, tc := range cases {
		got, err := parseSince(tc.raw, now)
		if err != nil || got != tc.want {
			t.Errorf("parseSince(%q) = %q, %v, want %q", tc.raw, got, err, tc.want)
		}
	}
}

func TestDescribeCoversEveryEventType(t *testing.T) {
	cases := []struct {
		kind string
		data string
		want string
	}{
		{"dashboard.created", `{"version":1,"title":"Checkout"}`, `created "Checkout"`},
		{"dashboard.version_created", `{"version":12,"title":"Checkout"}`, `saved version 12 of "Checkout"`},
		{"dashboard.restored", `{"version":13,"restored_from":9,"title":"Checkout"}`, `restored version 9 of "Checkout" as version 13`},
		{"dashboard.moved", `{"title":"Checkout"}`, `moved "Checkout"`},
		{"dashboard.deleted", `{"title":"Checkout"}`, `deleted "Checkout"`},
		{"dashboard.deleted", `{}`, `deleted a dashboard`},
		{"annotation.created", `{"tags":["deploy","checkout"]}`, `added an annotation (deploy, checkout)`},
		{"annotation.created", `{"tags":[]}`, `added an annotation`},
		{"annotation.deleted", `{}`, `deleted an annotation`},
		{"finding.created", `{"title":"Slow pod"}`, `saved the finding "Slow pod"`},
		{"finding.pinned", `{"annotation_id":"a"}`, `pinned a finding`},
		{"finding.moved", `{"title":"Slow pod","from_folder_id":null,"to_folder_id":"f"}`, `moved the finding "Slow pod"`},
		{"saved_search.created", `{"title":"Errors","kind":"logs"}`, `saved the logs search "Errors"`},
		{"saved_search.created", `{"title":"Slow","kind":"traces"}`, `saved the trace search "Slow"`},
		{"saved_search.updated", `{"title":"Errors v2","kind":"logs"}`, `renamed the saved search to "Errors v2"`},
		{"saved_search.moved", `{"title":"Errors","kind":"logs","from_folder_id":"a","to_folder_id":"b"}`, `moved the saved search "Errors"`},
		{"saved_search.deleted", `{"title":"Errors","kind":"logs"}`, `deleted the saved search "Errors"`},
		{"run.queued", `{"source":"prompt"}`, `run queued`},
		{"run.started", `{"source":"prompt","attempt":1}`, `run started`},
		{"run.succeeded", `{"source":"prompt","result_version":4}`, `run succeeded (saved version 4)`},
		{"run.succeeded", `{"source":"investigate"}`, `run succeeded`},
		{"run.failed", `{"source":"prompt","failure_code":"no_progress"}`, `run failed (no_progress)`},
		{"run.failed", `{"source":"prompt"}`, `run failed`},
		{"run.canceled", `{"source":"prompt"}`, `run canceled`},
		{"run.canceling", `{"source":"prompt"}`, `run canceling`},
		{"run.released", `{"source":"prompt","attempt":2}`, `run released back to the queue`},
		{"run.kept", `{"source":"prompt","result_version":4}`, `kept the build of a stopped run as version 4`},
		{"run.kept", `{"source":"prompt"}`, `kept the build of a stopped run`},
		{"thread.opened", `{"object":{"kind":"finding","id":"f"}}`, `commented on a finding`},
		{"thread.opened", `{"object":{"kind":"dashboard","id":"d"},"version":3,"panel_id":"p1"}`, `commented on a dashboard`},
		{"thread.opened", `{"object":{"kind":"annotation","id":"a"}}`, `commented on an annotation`},
		{"thread.opened", `{"object":{"kind":"proposal","id":"p"}}`, `commented on a proposal`},
		{"thread.opened", `{}`, `commented on an object`},
		{"thread.replied", `{"object":{"kind":"finding","id":"f"},"message_id":"m"}`, `replied to a thread`},
		{"thread.edited", `{"object":{"kind":"finding","id":"f"},"message_id":"m"}`, `edited a comment`},
		{"thread.resolved", `{"object":{"kind":"dashboard","id":"d"}}`, `resolved a thread`},
		{"thread.resolved", `{"object":{"kind":"dashboard","id":"d"},"resolved_by_version":4}`, `resolved a thread (fixed by version 4)`},
		{"thread.reopened", `{"object":{"kind":"dashboard","id":"d"}}`, `reopened a thread`},
		{"thread.mentioned", `{"mentioned":{"kind":"user","id":"u","name":"Ada"}}`, `mentioned Ada`},
		{"thread.mentioned", `{"mentioned":{"kind":"user","id":"u","name":"Ada\u202eevil"}}`, `mentioned Ada evil`},
		{"thread.mentioned", `{"mentioned":{"kind":"user","id":"u","name":""}}`, `mentioned someone`},
		{"thread.mentioned", `{"mentioned":{"kind":"agent","id":"observe","name":"observe"}}`, `mentioned @observe`},
		{"thread.deleted", `{}`, `thread.deleted`},
		{"proposal.opened", `{"title":"Rename the errors panel","revision":1,"base_version":3}`, `opened the proposal "Rename the errors panel"`},
		{"proposal.opened", `{}`, `opened a proposal`},
		{"proposal.updated", `{"title":"Rename the errors panel","revision":2}`, `updated the proposal "Rename the errors panel" (revision 2)`},
		{"proposal.merged", `{"title":"Rename the errors panel","revision":2,"version":6}`, `merged the proposal "Rename the errors panel" as version 6`},
		{"proposal.merged", `{"title":"Rename the errors panel","revision":2,"version":null}`, `merged the proposal "Rename the errors panel", already in the dashboard`},
		{"proposal.discarded", `{"title":"Evil\u001b[31m","revision":1}`, `discarded the proposal "Evil [31m"`},
		{"dashboard.created", `{"title":"Evil\u001b[31m"}`, `created "Evil [31m"`},
	}
	for _, tc := range cases {
		event, err := decode[activityEvent]([]byte(activityJSON(1, tc.kind, "Sam", "2026-11-01T10:00:00Z", tc.data)))
		if err != nil {
			t.Fatal(err)
		}
		if got := describe(event); got != tc.want {
			t.Errorf("describe(%s %s) = %q, want %q", tc.kind, tc.data, got, tc.want)
		}
	}
}

func TestReadFrames(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  []sseFrame
	}{
		{"one frame", "id: 4\nevent: activity\ndata: {\"a\":1}\n\n", []sseFrame{{id: "4", event: "activity", data: `{"a":1}`}}},
		{"comments and pings are skipped", ": ping\n\nid: 4\n: inside\nevent: activity\ndata: x\n\n", []sseFrame{{id: "4", event: "activity", data: "x"}}},
		{"data lines join with a newline", "data: a\ndata: b\ndata:\ndata: c\n\n", []sseFrame{{data: "a\nb\n\nc"}}},
		{"retry shares the block of ready", "retry: 3000\nevent: ready\ndata: {}\n\n", []sseFrame{{event: "ready", data: "{}", retry: 3 * time.Second}}},
		{"a bad retry is ignored", "retry: soon\nevent: ready\ndata: {}\n\n", []sseFrame{{event: "ready", data: "{}"}}},
		{"carriage returns are dropped", "event: reset\r\ndata: {}\r\n\r\n", []sseFrame{{event: "reset", data: "{}"}}},
		{"no space after the colon", "event:activity\ndata:x\n\n", []sseFrame{{event: "activity", data: "x"}}},
		{"unknown fields are ignored", "x: 1\n\n", nil},
		{"an unfinished frame is dropped", "event: activity\ndata: x\n", nil},
		{"two frames", "data: 1\n\ndata: 2\n\n", []sseFrame{{data: "1"}, {data: "2"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []sseFrame
			err := readFrames(strings.NewReader(tc.input), func(f sseFrame) error {
				got = append(got, f)
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Errorf("frames %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestReadFramesStopsWhenTheHandlerFails(t *testing.T) {
	stop := fmt.Errorf("stop")
	calls := 0
	err := readFrames(strings.NewReader("data: 1\n\ndata: 2\n\n"), func(sseFrame) error {
		calls++
		return stop
	})
	if !errors.Is(err, stop) || calls != 1 {
		t.Fatalf("err %v after %d calls", err, calls)
	}
}

func TestActivityFollowPrintsFramesAndResumesFromTheLastOne(t *testing.T) {
	ready := "retry: 10\nevent: ready\ndata: {}\n\n"
	first := ready + ": ping\n\n" + activityFrame(13, "dashboard.moved", "Sam", "2026-11-01T10:06:00Z", `{"title":"Checkout"}`)
	second := ready + "event: reset\ndata: {}\n\n" + activityFrame(20, "run.canceled", "/observe for Sam", "2026-11-01T10:07:00Z", `{"source":"prompt"}`)
	f := newActivityFake(t, twoEventPage(), sendFrames(first), status(http.StatusTooManyRequests), sendFrames(second))
	stdout, stderr, code := f.run("activity", "--follow", "--type", "run.canceled", "--limit", "7")
	if code != 1 || !strings.Contains(stderr, "stop (stop_here)") {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	want := "2026-11-01T10:00:00Z  /observe for Sam  run failed (no_progress)\n" +
		"2026-11-01T10:05:09Z  Sam  saved version 12 of \"Checkout\"\n" +
		"2026-11-01T10:06:00Z  Sam  moved \"Checkout\"\n" +
		"2026-11-01T10:07:00Z  /observe for Sam  run canceled\n"
	if stdout != want {
		t.Errorf("stdout %q, want %q", stdout, want)
	}
	if !strings.Contains(stderr, "Some activity may have been missed.\n") {
		t.Errorf("stderr %q has no reset notice", stderr)
	}
	if fmt.Sprint(f.opened) != "[12 13 13 20]" {
		t.Errorf("Last-Event-ID sent %q", f.opened)
	}
	for _, accept := range f.accepted {
		if accept != "text/event-stream" {
			t.Errorf("Accept %q", accept)
		}
	}
	if len(f.list) != 1 || !strings.Contains(f.list[0].query, "limit=7") {
		t.Errorf("page requests %+v", f.list)
	}
}

func TestActivityFollowStartsFromTheBeginningWhenThePageIsEmpty(t *testing.T) {
	frames := "retry: 10\nevent: ready\ndata: {}\n\n" + activityFrame(5, "annotation.created", "Sam", "2026-11-01T10:00:00Z", `{"tags":["deploy"]}`)
	f := newActivityFake(t, `{"activity":[],"next_cursor":null}`, sendFrames(frames))
	stdout, _, code := f.run("activity", "--follow", "--json", "--dashboard", testDashboard, "--limit", "9")
	if code != 1 {
		t.Fatalf("exit %d", code)
	}
	if want := activityJSON(5, "annotation.created", "Sam", "2026-11-01T10:00:00Z", `{"tags":["deploy"]}`) + "\n"; stdout != want {
		t.Errorf("stdout %q, want %q", stdout, want)
	}
	if fmt.Sprint(f.opened) != "[0 5]" {
		t.Errorf("Last-Event-ID sent %q", f.opened)
	}
}

func newFollower(t *testing.T, f *activityFake, stdout io.Writer) *activityFollower {
	t.Helper()
	base, err := url.Parse(f.server.URL)
	if err != nil {
		t.Fatal(err)
	}
	c, err := (&app{version: "test"}).clientFor(base, "k")
	if err != nil {
		t.Fatal(err)
	}
	return &activityFollower{c: c, query: url.Values{}, out: &output{out: stdout, err: io.Discard}, retry: time.Millisecond, idle: streamIdleTimeout}
}

func TestActivityFollowGivesUpAfterRepeatedServerFailures(t *testing.T) {
	f := newActivityFake(t, "", status(http.StatusServiceUnavailable), status(http.StatusServiceUnavailable), status(http.StatusServiceUnavailable), status(http.StatusServiceUnavailable))
	err := newFollower(t, f, io.Discard).run(t.Context())
	if err == nil || !isTransient(err) {
		t.Fatalf("run: %v", err)
	}
	if len(f.opened) != maxPollFailures {
		t.Errorf("opened %d streams, want %d", len(f.opened), maxPollFailures)
	}
}

func TestActivityFollowTreatsAnEmptyStreamAsAFailure(t *testing.T) {
	f := newActivityFake(t, "", sendFrames(""), sendFrames(""), sendFrames(""), sendFrames(""))
	err := newFollower(t, f, io.Discard).run(t.Context())
	if err == nil || !strings.Contains(err.Error(), "without sending anything") || len(f.opened) != maxPollFailures {
		t.Fatalf("run: %v after %d streams", err, len(f.opened))
	}
}

func TestActivityFollowReconnectsFromTheLastEventWhenTheStreamGoesSilent(t *testing.T) {
	silent := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "retry: 10\nevent: ready\ndata: {}\n\n"+activityFrame(7, "annotation.created", "Sam", "2026-11-01T10:00:00Z", `{"tags":["deploy"]}`))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}
	f := newActivityFake(t, "", silent)
	var stdout strings.Builder
	follower := newFollower(t, f, &stdout)
	follower.idle = 50 * time.Millisecond

	err := follower.run(t.Context())

	if err == nil || !strings.Contains(err.Error(), "stop (stop_here)") {
		t.Fatalf("run: %v", err)
	}
	if fmt.Sprint(f.opened) != "[0 7]" {
		t.Errorf("Last-Event-ID sent %q", f.opened)
	}
	if want := "2026-11-01T10:00:00Z  Sam  added an annotation (deploy)\n"; stdout.String() != want {
		t.Errorf("stdout %q, want %q", stdout.String(), want)
	}
}

func TestActivityFollowReportsASilentStreamThatNeverSentAnything(t *testing.T) {
	silent := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}
	f := newActivityFake(t, "", silent, silent, silent, silent)
	follower := newFollower(t, f, io.Discard)
	follower.idle = 20 * time.Millisecond

	err := follower.run(t.Context())

	if err == nil || !isTransient(err) || !strings.Contains(err.Error(), "sent nothing for 20ms") || len(f.opened) != maxPollFailures {
		t.Fatalf("run: %v after %d streams", err, len(f.opened))
	}
}

func TestActivityFollowStopsCleanlyOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	opened := make(chan struct{})
	hold := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "retry: 10\nevent: ready\ndata: {}\n\n")
		w.(http.Flusher).Flush()
		close(opened)
		<-r.Context().Done()
	}
	f := newActivityFake(t, "", hold)
	done := make(chan error, 1)
	go func() { done <- newFollower(t, f, io.Discard).run(ctx) }()
	<-opened
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestStreamRefusesAnUnauthorizedKey(t *testing.T) {
	f := newActivityFake(t, "")
	base, err := url.Parse(f.server.URL)
	if err != nil {
		t.Fatal(err)
	}
	c, err := (&app{version: "test"}).clientFor(base, "wrong")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.stream(t.Context(), "activity/stream", nil, ""); !errors.Is(err, errUnauthorized) {
		t.Fatalf("stream: %v", err)
	}
}
