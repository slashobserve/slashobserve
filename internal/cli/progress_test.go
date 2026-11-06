package cli

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

const progressDashboardID = "aaaaaaaa-2222-3333-4444-555555555555"

type pollReply struct {
	saves int
	body  string
}

type growingBuild struct {
	mu          sync.Mutex
	polls       []pollReply
	saves       int
	builds      map[int]string
	baseVersion string
	baseSpec    string
	final       string
	buildCalls  int
	versionHits int
	served      int
}

func (g *growingBuild) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/api/v1/observe/r":
		reply := g.polls[min(g.served, len(g.polls)-1)]
		g.served++
		g.saves = reply.saves
		_, _ = fmt.Fprint(w, reply.body)
	case "/api/v1/observe/r/build":
		g.buildCalls++
		base := "null"
		if g.baseVersion != "" {
			base = g.baseVersion
		}
		_, _ = fmt.Fprintf(w, `{"build":{"request_id":"r","dashboard_id":"%s","base_version":%s,"saves":%d,"panels":0,"planned_panels":2,"spec":%s,"updated_at":"2026-01-01T00:00:00Z"}}`, progressDashboardID, base, g.saves, g.builds[g.saves])
	case "/api/v1/dashboards/" + progressDashboardID + "/versions/" + g.baseVersion:
		g.versionHits++
		if g.baseSpec == "" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprint(w, `{"error":"version not found","code":"not_found"}`)
			return
		}
		_, _ = fmt.Fprintf(w, `{"dashboard":{"id":"%s","title":"Shop"},"version":{"version":%s,"spec":%s}}`, progressDashboardID, g.baseVersion, g.baseSpec)
	case "/api/v1/dashboards/" + progressDashboardID:
		_, _ = fmt.Fprintf(w, `{"dashboard":{"id":"%s","title":"Checkout health"},"version":{"version":1,"spec":%s}}`, progressDashboardID, g.final)
	default:
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `{"error":"not found","code":"not_found"}`)
	}
}

func running(saves int, steps string) pollReply {
	build := "null"
	if saves > 0 {
		build = fmt.Sprintf(`{"saves":%d,"panels":%d,"planned_panels":2,"title":"t","updated_at":"2026-01-01T00:00:00Z"}`, saves, saves)
	}
	return pollReply{saves: saves, body: fmt.Sprintf(`{"request":{"id":"r","status":"running","steps":[%s],"build":%s}}`, steps, build)}
}

func succeeded() pollReply {
	return pollReply{body: fmt.Sprintf(`{"request":{"id":"r","status":"succeeded","dashboard_id":"%s","result_version":1,"url":"http://x/d/%s","steps":[],"build":null}}`, progressDashboardID, progressDashboardID)}
}

const (
	requestsPanel = `{"id":"rps","type":"timeseries","title":"Requests"}`
	latencyPanel  = `{"id":"p99","type":"timeseries","title":""}`
	rowWithChild  = `{"id":"row","type":"row","title":"Pods","panels":[{"id":"pods","type":"stat","title":"Pods up"}]}`
)

func spec(panels ...string) string {
	return `{"panels":[` + strings.Join(panels, ",") + `]}`
}

func progressAgainst(t *testing.T, stub *growingBuild, out *output) (*requestProgress, agentRequest) {
	t.Helper()
	server := httptest.NewServer(stub)
	t.Cleanup(server.Close)
	base, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	c := &client{base: base, token: "k", userAgent: "test", http: server.Client()}
	progress := newRequestProgress(c, out, "r")
	_, req, err := poll(context.Background(), c, progress, "r", 5*time.Second, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	return progress, req
}

func lines(s string) []string {
	var kept []string
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			kept = append(kept, line)
		}
	}
	return kept
}

func TestProgressStreamsPanelsAsTheBuildGrows(t *testing.T) {
	stub := &growingBuild{
		polls: []pollReply{
			running(0, `{"at":"t1","attempt":1,"phase":"plan","message":"planning"}`),
			running(1, `{"at":"t1","attempt":1,"phase":"plan","message":"planning"},{"at":"t2","attempt":1,"phase":"save","message":"saved Requests"}`),
			running(1, `{"at":"t1","attempt":1,"phase":"plan","message":"planning"},{"at":"t2","attempt":1,"phase":"save","message":"saved Requests"}`),
			running(2, `{"at":"t1","attempt":1,"phase":"plan","message":"planning"},{"at":"t2","attempt":1,"phase":"save","message":"saved Requests"}`),
			succeeded(),
		},
		builds: map[int]string{1: spec(requestsPanel), 2: spec(requestsPanel, latencyPanel)},
		final:  spec(requestsPanel, latencyPanel, rowWithChild),
	}
	var stdout, stderr bytes.Buffer
	progress, req := progressAgainst(t, stub, &output{out: &stdout, err: &stderr})
	title, ok := progress.finish(context.Background(), *req.DashboardID)
	if !ok || title != "Checkout health" {
		t.Fatalf("finish = %q, %v", title, ok)
	}
	want := []string{
		"plan  planning",
		"save  saved Requests",
		"Panel 1 of 2: Requests (timeseries)",
		"Panel 2 of 2: p99 (timeseries)",
		"Panel 3: Pods up (stat)",
	}
	if got := lines(stderr.String()); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("stderr lines\n%q\nwant\n%q", got, want)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout %q, want nothing before the address", stdout.String())
	}
	if stub.buildCalls != 2 {
		t.Fatalf("build fetched %d times, want once per new save count", stub.buildCalls)
	}
}

func TestProgressRevisionSkipsPanelsOfTheBaseVersion(t *testing.T) {
	cases := []struct {
		name     string
		baseSpec string
		want     []string
	}{
		{"base version read", spec(requestsPanel), []string{"Panel 2 of 2: p99 (timeseries)"}},
		{"base version unreadable", "", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := &growingBuild{
				polls:       []pollReply{running(1, ""), succeeded()},
				builds:      map[int]string{1: spec(requestsPanel, latencyPanel)},
				baseVersion: "3",
				baseSpec:    tc.baseSpec,
			}
			var stderr bytes.Buffer
			progressAgainst(t, stub, &output{out: &bytes.Buffer{}, err: &stderr})
			if got := lines(stderr.String()); strings.Join(got, "|") != strings.Join(tc.want, "|") {
				t.Fatalf("stderr lines %q, want %q", got, tc.want)
			}
			if stub.versionHits != 1 {
				t.Fatalf("base version read %d times, want once", stub.versionHits)
			}
		})
	}
}

func TestProgressLiveStatusLineRewritesInPlace(t *testing.T) {
	stub := &growingBuild{
		polls: []pollReply{
			running(0, `{"at":"t1","attempt":1,"phase":"plan","message":"planning"}`),
			running(1, `{"at":"t1","attempt":1,"phase":"plan","message":"planning"},{"at":"t2","attempt":1,"phase":"save","message":"saved Requests"}`),
			succeeded(),
		},
		builds: map[int]string{1: spec(requestsPanel)},
	}
	var stdout, stderr bytes.Buffer
	progressAgainst(t, stub, &output{out: &stdout, err: &stderr, terminal: true})
	got := stdout.String()
	if stderr.Len() != 0 {
		t.Fatalf("stderr %q, want the live view on stdout only", stderr.String())
	}
	for _, want := range []string{clearLine + "plan  planning", clearLine + "Panel 1 of 2: Requests (timeseries)\n", clearLine + "save  saved Requests"} {
		if !strings.Contains(got, want) {
			t.Fatalf("stdout %q lacks %q", got, want)
		}
	}
	if !strings.HasSuffix(got, clearLine) {
		t.Fatalf("stdout %q does not end by clearing the status line", got)
	}
	if strings.Contains(got, "\x1b[2m") {
		t.Fatalf("stdout %q is coloured although colour is off", got)
	}
}

func TestProgressQuietPrintsNothing(t *testing.T) {
	stub := &growingBuild{
		polls:  []pollReply{running(1, `{"at":"t1","attempt":1,"phase":"plan","message":"planning"}`), succeeded()},
		builds: map[int]string{1: spec(requestsPanel)},
	}
	for _, terminal := range []bool{false, true} {
		var stdout, stderr bytes.Buffer
		progressAgainst(t, stub, &output{out: &stdout, err: &stderr, quiet: true, terminal: terminal})
		if stdout.Len() != 0 || stderr.Len() != 0 {
			t.Fatalf("terminal %v: stdout %q, stderr %q", terminal, stdout.String(), stderr.String())
		}
	}
}

func TestProgressReportSaved(t *testing.T) {
	version := 4
	cases := []struct {
		name           string
		out            output
		version        *int
		stdout, stderr string
	}{
		{"terminal", output{terminal: true}, &version, "Saved version 4  http://x/d/1/shop\n", ""},
		{"terminal with colour", output{terminal: true, color: palette{on: true}}, &version, "\x1b[32mSaved version 4\x1b[0m  http://x/d/1/shop\n", ""},
		{"pipe", output{}, &version, "http://x/d/1/shop\n", "Saved version 4\n"},
		{"quiet terminal", output{terminal: true, quiet: true}, &version, "http://x/d/1/shop\n", ""},
		{"no version", output{terminal: true}, nil, "http://x/d/1/shop\n", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			out := tc.out
			out.out, out.err = &stdout, &stderr
			reportSaved(&out, tc.version, "http://x/d/1/shop")
			if stdout.String() != tc.stdout || stderr.String() != tc.stderr {
				t.Fatalf("stdout %q, stderr %q; want %q, %q", stdout.String(), stderr.String(), tc.stdout, tc.stderr)
			}
		})
	}
}

func TestProgressTitledAddress(t *testing.T) {
	cases := []struct{ address, title, want string }{
		{"https://o.example.com/d/" + progressDashboardID, "Checkout health (prod)", "https://o.example.com/d/" + progressDashboardID + "/checkout-health-prod"},
		{"https://o.example.com/base/d/" + progressDashboardID, "Shop", "https://o.example.com/base/d/" + progressDashboardID + "/shop"},
		{"https://o.example.com/d/" + progressDashboardID, "???", "https://o.example.com/d/" + progressDashboardID},
		{"https://o.example.com/d/" + progressDashboardID, "Versions", "https://o.example.com/d/" + progressDashboardID},
		{"https://o.example.com/d/" + progressDashboardID + "/old", "Shop", "https://o.example.com/d/" + progressDashboardID + "/old"},
		{"/d/" + progressDashboardID, "Shop", "/d/" + progressDashboardID},
	}
	for _, tc := range cases {
		if got := titled(tc.address, progressDashboardID, tc.title); got != tc.want {
			t.Errorf("titled(%q, %q) = %q, want %q", tc.address, tc.title, got, tc.want)
		}
	}
}

func TestProgressTruncate(t *testing.T) {
	cases := []struct {
		in    string
		width int
		want  string
	}{
		{"short", 10, "short"},
		{"exactly ten", 11, "exactly ten"},
		{"a longer status line", 8, "a longe…"},
		{"größer als", 4, "grö…"},
		{"anything", 0, "anything"},
	}
	for _, tc := range cases {
		if got := truncate(tc.in, tc.width); got != tc.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tc.in, tc.width, got, tc.want)
		}
	}
}

func TestProgressColourIsOffWithoutATerminal(t *testing.T) {
	t.Setenv(noColorEnv, "")
	a := &app{std: IO{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}}
	out := a.output()
	if out.terminal || out.color.on {
		t.Fatalf("a buffer is treated as a terminal: %+v", out)
	}
	if got := (palette{}).green("x"); got != "x" {
		t.Fatalf("an off palette painted %q", got)
	}
}
