package tailhttp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

const (
	testPoll    = 10 * time.Millisecond
	waitTimeout = 5 * time.Second
	fromParam   = "2026-01-01T00:00:00.000000007Z"
)

var (
	fromTime = time.Date(2026, 1, 1, 0, 0, 0, 7, time.UTC)
	baseNs   = fromTime.UnixNano()
)

type response struct {
	frames []datasource.Frame
	err    error
}

type fakeLines struct {
	typ string

	mu        sync.Mutex
	responses []response
	requests  []datasource.LinesRequest
}

func (f *fakeLines) Type() string { return f.typ }

func (f *fakeLines) CheckHealth(context.Context) error { return nil }
func (f *fakeLines) Close() error                      { return nil }
func (f *fakeLines) Discover(context.Context, datasource.DiscoverRequest) (*datasource.Catalog, error) {
	return &datasource.Catalog{}, nil
}
func (f *fakeLines) Lint(context.Context, string) ([]datasource.Diagnostic, error) { return nil, nil }
func (f *fakeLines) VariableValues(context.Context, datasource.Query, datasource.TimeRange) ([]string, error) {
	return nil, nil
}
func (f *fakeLines) QueryRange(context.Context, datasource.Query, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
	return nil, nil
}
func (f *fakeLines) QueryInstant(context.Context, datasource.Query, datasource.TimeRange) ([]datasource.Frame, error) {
	return nil, nil
}

func (f *fakeLines) Lines(_ context.Context, req datasource.LinesRequest) ([]datasource.Frame, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, req)
	if len(f.responses) == 0 {
		return nil, nil
	}
	next := f.responses[0]
	f.responses = f.responses[1:]
	return next.frames, next.err
}

func (f *fakeLines) seen() []datasource.LinesRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]datasource.LinesRequest(nil), f.requests...)
}

type withoutLines struct{ datasource.Datasource }

func linesFrame(truncated bool, offsets ...int64) datasource.Frame {
	f := datasource.Frame{Labels: map[string]string{"service_name": "checkout"}, Fields: []datasource.Field{
		{Name: "Time", Type: datasource.FieldTime},
		{Name: "Body", Type: datasource.FieldString},
		{Name: "TimeNs", Type: datasource.FieldString},
	}}
	for _, o := range offsets {
		f.Fields[0].Values = append(f.Fields[0].Values, time.Unix(0, baseNs+o).UTC())
		f.Fields[1].Values = append(f.Fields[1].Values, "line "+strconv.FormatInt(o, 10))
		f.Fields[2].Values = append(f.Fields[2].Values, strconv.FormatInt(baseNs+o, 10))
	}
	if truncated {
		f.Meta = &datasource.FrameMeta{Truncated: true}
	}
	return f
}

func lines(offsets ...int64) response {
	return response{frames: []datasource.Frame{linesFrame(false, offsets...)}}
}

type fakeSessions struct {
	valid atomic.Bool
	calls atomic.Int32
	role  atomic.Value
}

func (f *fakeSessions) RequestPrincipal(r *http.Request) (auth.Principal, bool, error) {
	f.calls.Add(1)
	p, _ := auth.FromContext(r.Context())
	if role, ok := f.role.Load().(auth.Role); ok {
		p.Role = role
	}
	return p, f.valid.Load(), nil
}

type fixture struct {
	t        *testing.T
	h        *Handler
	engine   *gin.Engine
	logs     *fakeLines
	sessions *fakeSessions
	server   *httptest.Server
}

func newFixture(t *testing.T, responses ...response) *fixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	logger := slog.New(slog.DiscardHandler)
	logs := &fakeLines{typ: "loki", responses: responses}
	registry, err := datasource.NewRegistry(logger,
		datasource.Instance{UID: "logs", Datasource: logs},
		datasource.Instance{UID: "prom", Datasource: withoutLines{&fakeLines{typ: "prometheus"}}},
	)
	if err != nil {
		t.Fatal(err)
	}
	sessions := &fakeSessions{}
	sessions.valid.Store(true)
	h := NewHandler(registry, queryproxy.NewRunner(registry, logger), sessions, logger)
	h.pollInterval = testPoll
	engine := gin.New()
	api := engine.Group("/api/v1", func(c *gin.Context) {
		id := c.GetHeader("X-Principal")
		if id == "" {
			id = "u1"
		}
		role := auth.Role(c.GetHeader("X-Role"))
		if role == "" {
			role = auth.RoleViewer
		}
		p := auth.Principal{Kind: auth.KindUser, ID: id, Role: role}
		c.Request = c.Request.WithContext(datasource.WithAccess(auth.WithPrincipal(c.Request.Context(), p), datasource.Access{By: p}))
	})
	h.Register(auth.NewRoutes(api))
	server := httptest.NewServer(engine)
	t.Cleanup(server.Close)
	t.Cleanup(server.CloseClientConnections)
	return &fixture{t: t, h: h, engine: engine, logs: logs, sessions: sessions, server: server}
}

type event struct {
	name string
	data string
}

type openStream struct {
	resp   *http.Response
	events chan event
	done   chan struct{}
}

func (f *fixture) open(principal, query string) *openStream {
	f.t.Helper()
	return f.openOn("logs", principal, "", query)
}

func (f *fixture) openOn(uid, principal string, role auth.Role, query string) *openStream {
	f.t.Helper()
	req, err := http.NewRequest(http.MethodGet, f.server.URL+"/api/v1/datasources/"+uid+"/tail?"+query, nil)
	if err != nil {
		f.t.Fatal(err)
	}
	req.Header.Set("X-Principal", principal)
	req.Header.Set("X-Role", string(role))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		f.t.Fatal(err)
	}
	f.t.Cleanup(func() { _ = resp.Body.Close() })
	s := &openStream{resp: resp, events: make(chan event, 1024), done: make(chan struct{})}
	go func() {
		defer close(s.done)
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 1<<20), 1<<20)
		var current event
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case strings.HasPrefix(line, "event: "):
				current.name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				current.data = strings.TrimPrefix(line, "data: ")
			case line == "" && current.name != "":
				s.events <- current
				current = event{}
			}
		}
	}()
	return s
}

func (s *openStream) next(t *testing.T) event {
	t.Helper()
	select {
	case ev := <-s.events:
		return ev
	case <-s.done:
		t.Fatal("stream ended before the next event")
	case <-time.After(waitTimeout):
		t.Fatal("no event within the wait timeout")
	}
	return event{}
}

func (s *openStream) closedWithin(d time.Duration) bool {
	select {
	case <-s.done:
		return true
	case <-time.After(d):
		return false
	}
}

func decode[T any](t *testing.T, ev event, name string) T {
	t.Helper()
	if ev.name != name {
		t.Fatalf("event = %q %s, want %s", ev.name, ev.data, name)
	}
	var out T
	if err := json.Unmarshal([]byte(ev.data), &out); err != nil {
		t.Fatalf("decode %s %q: %v", name, ev.data, err)
	}
	return out
}

func waitForRequests(t *testing.T, logs *fakeLines, n int) []datasource.LinesRequest {
	t.Helper()
	deadline := time.Now().Add(waitTimeout)
	for time.Now().Before(deadline) {
		if seen := logs.seen(); len(seen) >= n {
			return seen
		}
		time.Sleep(testPoll)
	}
	t.Fatalf("fewer than %d poll requests reached the datasource: %d", n, len(logs.seen()))
	return nil
}

func TestTailStreamsReadyThenLinesAndPollsFromTheNewestLinePlusOneNanosecond(t *testing.T) {
	f := newFixture(t, lines(1, 2), response{frames: []datasource.Frame{linesFrame(true, 5)}})
	s := f.open("u1", "query="+`{service_name="checkout"}`+"&from="+fromParam+"&limit=7")
	if s.resp.StatusCode != http.StatusOK || !strings.HasPrefix(s.resp.Header.Get("Content-Type"), eventStreamContent) {
		t.Fatalf("status %d, content type %q", s.resp.StatusCode, s.resp.Header.Get("Content-Type"))
	}
	ready := decode[readyData](t, s.next(t), "ready")
	if ready.DatasourceUID != "logs" || ready.From != fromParam {
		t.Fatalf("ready = %+v", ready)
	}
	first := decode[linesData](t, s.next(t), "lines")
	if first.Newest != time.Unix(0, baseNs+2).UTC().Format(time.RFC3339Nano) || first.Truncated {
		t.Fatalf("first lines = %+v", first)
	}
	var frames []struct {
		Ref    string            `json:"ref"`
		Labels map[string]string `json:"labels"`
		Fields []struct {
			Name   string `json:"name"`
			Type   string `json:"type"`
			Values []any  `json:"values"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(first.Frames, &frames); err != nil {
		t.Fatal(err)
	}
	if len(frames) != 1 || frames[0].Ref != "A" || frames[0].Labels["service_name"] != "checkout" || len(frames[0].Fields) != 3 {
		t.Fatalf("frames = %+v", frames)
	}
	if ns := frames[0].Fields[2]; ns.Name != "TimeNs" || ns.Type != "string" || ns.Values[1] != strconv.FormatInt(baseNs+2, 10) {
		t.Fatalf("TimeNs field = %+v, want nanosecond strings", ns)
	}
	if at, ok := frames[0].Fields[0].Values[0].(float64); !ok || int64(at) != time.Unix(0, baseNs+1).UnixMilli() {
		t.Fatalf("Time values = %v, want epoch milliseconds", frames[0].Fields[0].Values)
	}
	second := decode[linesData](t, s.next(t), "lines")
	if second.Newest != time.Unix(0, baseNs+5).UTC().Format(time.RFC3339Nano) || !second.Truncated {
		t.Fatalf("second lines = %+v", second)
	}
	seen := waitForRequests(t, f.logs, 3)
	if got := seen[0]; !got.Range.From.Equal(fromTime) || got.Direction != datasource.Forward || got.Limit != 7 || got.Query.Text != `{service_name="checkout"}` || got.Query.Limits != queryproxy.DraftLimits || !got.Range.To.After(got.Range.From) {
		t.Fatalf("first poll = %+v", got)
	}
	if got := seen[1].Range.From; !got.Equal(fromTime) {
		t.Fatalf("second poll from = %v, want newest plus one nanosecond minus the lag, held at the start %v", got, fromTime)
	}
	if got := seen[2].Range.From; !got.Equal(time.Unix(0, baseNs+6)) {
		t.Fatalf("third poll from = %v, want %v", got, time.Unix(0, baseNs+6).UTC())
	}
}

func TestTailAnswersTooManyStreamsForASixthTailOfOnePrincipal(t *testing.T) {
	f := newFixture(t)
	for range maxTailsPerUser {
		s := f.open("u1", "query=%7Bapp%3D%22shop%22%7D")
		decode[readyData](t, s.next(t), "ready")
	}
	sixth := f.open("u1", "query=%7Bapp%3D%22shop%22%7D")
	if sixth.resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("sixth tail status = %d, want 429", sixth.resp.StatusCode)
	}
	if !sixth.closedWithin(waitTimeout) {
		t.Fatal("the refused tail stayed open")
	}
	other := f.open("u2", "query=%7Bapp%3D%22shop%22%7D")
	if other.resp.StatusCode != http.StatusOK {
		t.Fatalf("another principal's tail status = %d, want 200", other.resp.StatusCode)
	}
}

func TestTailReportsANonTransientErrorAndEnds(t *testing.T) {
	f := newFixture(t, response{}, response{err: datasource.Errorf(datasource.ErrInvalidQuery, "parse error")})
	s := f.open("u1", "query=%7Bapp%3D%22shop%22%7D")
	decode[readyData](t, s.next(t), "ready")
	failure := decode[errorData](t, s.next(t), "error")
	if failure.Code != "query_invalid" || failure.Error != "invalid query: parse error" {
		t.Fatalf("error event = %+v", failure)
	}
	if !s.closedWithin(waitTimeout) {
		t.Fatal("the stream stayed open after the error event")
	}
}

func TestTailRetriesTransientErrorsAndReportsAfterFiveInARow(t *testing.T) {
	down := response{err: datasource.Errorf(datasource.ErrUnavailable, "connection refused")}
	f := newFixture(t, response{}, down, down, lines(9))
	s := f.open("u1", "query=%7Bapp%3D%22shop%22%7D")
	decode[readyData](t, s.next(t), "ready")
	if got := decode[linesData](t, s.next(t), "lines"); got.Newest != time.Unix(0, baseNs+9).UTC().Format(time.RFC3339Nano) {
		t.Fatalf("lines after two transient errors = %+v", got)
	}

	f = newFixture(t, response{}, down, down, down, down, down)
	s = f.open("u1", "query=%7Bapp%3D%22shop%22%7D")
	decode[readyData](t, s.next(t), "ready")
	if got := decode[errorData](t, s.next(t), "error"); got.Code != "datasource_unavailable" {
		t.Fatalf("error after five transient errors = %+v", got)
	}
	if !s.closedWithin(waitTimeout) {
		t.Fatal("the stream stayed open after the error event")
	}
	if seen := f.logs.seen(); len(seen) != 1+maxTransientErrors {
		t.Fatalf("%d polls, want the first plus %d retried", len(seen), maxTransientErrors)
	}
}

func TestTailEndsWhenTheCredentialStopsBeingValid(t *testing.T) {
	f := newFixture(t)
	f.h.pingInterval = testPoll
	s := f.open("u1", "query=%7Bapp%3D%22shop%22%7D")
	decode[readyData](t, s.next(t), "ready")
	f.sessions.valid.Store(false)
	if !s.closedWithin(waitTimeout) {
		t.Fatal("the stream stayed open after the credential became invalid")
	}
}

func (f *fixture) get(uid, query, principal string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/datasources/"+uid+"/tail?"+query, nil)
	req.Header.Set("X-Principal", principal)
	rec := httptest.NewRecorder()
	f.engine.ServeHTTP(rec, req)
	return rec
}

func expectError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d (body %s)", rec.Code, status, rec.Body.String())
	}
	if !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("content type = %q, want JSON", rec.Header().Get("Content-Type"))
	}
	var body struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
	if body.Code != code {
		t.Fatalf("code = %q, want %q (body %s)", body.Code, code, rec.Body.String())
	}
}

func TestTailRejectsABadQueryAsJSONBeforeAnyStreamBytes(t *testing.T) {
	f := newFixture(t, response{err: datasource.Errorf(datasource.ErrInvalidQuery, "lines need a log query")})
	rec := f.get("logs", "query=rate%28%7Ba%3D%22b%22%7D%5B1m%5D%29", "u1")
	expectError(t, rec, http.StatusUnprocessableEntity, "query_invalid")
	if strings.Contains(rec.Body.String(), "event:") || strings.Contains(rec.Body.String(), "retry:") {
		t.Fatalf("body carries stream bytes: %s", rec.Body.String())
	}
	if f.h.total != 0 {
		t.Fatalf("%d tails still counted after the refusal", f.h.total)
	}
}

func TestTailRejectsInvalidRequests(t *testing.T) {
	f := newFixture(t)
	query := "query=%7Bapp%3D%22shop%22%7D"
	tests := []struct {
		name   string
		uid    string
		query  string
		status int
		code   string
	}{
		{"missing query", "logs", "", http.StatusBadRequest, "invalid_request"},
		{"query over 64 KiB", "logs", "query=" + strings.Repeat("x", 64<<10+1), http.StatusBadRequest, "query_too_long"},
		{"query twice", "logs", query + "&" + query, http.StatusBadRequest, "invalid_request"},
		{"limit zero", "logs", query + "&limit=0", http.StatusBadRequest, "invalid_request"},
		{"limit over 1000", "logs", query + "&limit=1001", http.StatusBadRequest, "invalid_request"},
		{"limit not an integer", "logs", query + "&limit=ten", http.StatusBadRequest, "invalid_request"},
		{"from not a time", "logs", query + "&from=yesterday", http.StatusBadRequest, "invalid_request"},
		{"from in the future", "logs", query + "&from=now%2B1h", http.StatusBadRequest, "invalid_request"},
		{"unknown time zone", "logs", query + "&timezone=Mars%2FOlympus", http.StatusBadRequest, "invalid_request"},
		{"unknown parameter", "logs", query + "&to=now", http.StatusBadRequest, "invalid_request"},
		{"unknown datasource", "nope", query, http.StatusNotFound, "not_found"},
		{"not a line source", "prom", query, http.StatusBadRequest, "invalid_request"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expectError(t, f.get(tt.uid, tt.query, "u1"), tt.status, tt.code)
		})
	}
	if seen := f.logs.seen(); len(seen) != 0 {
		t.Fatalf("%d polls for invalid requests", len(seen))
	}
	rec := f.get("prom", query, "u1")
	if !strings.Contains(rec.Body.String(), notLineSource) {
		t.Fatalf("body = %s, want %q", rec.Body.String(), notLineSource)
	}
}

func TestParseFromDefaultsToTenSecondsAgoAndReadsEpochMilliseconds(t *testing.T) {
	now := time.Date(2026, 11, 6, 12, 0, 0, 500, time.UTC)
	from, err := parseFrom(nil, now)
	if err != nil || !from.Equal(now.Add(-defaultLookback)) {
		t.Fatalf("default from = %v, %v", from, err)
	}
	from, err = parseFrom(map[string][]string{"from": {"1762300000000"}}, now)
	if err != nil || !from.Equal(time.UnixMilli(1762300000000)) {
		t.Fatalf("epoch from = %v, %v", from, err)
	}
	from, err = parseFrom(map[string][]string{"from": {fromParam}}, now)
	if err != nil || !from.Equal(fromTime) {
		t.Fatalf("RFC 3339 from = %v, %v; nanoseconds must survive", from, err)
	}
	if _, err := parseFrom(map[string][]string{"from": {"now-1h"}, "timezone": {"Europe/Paris"}}, now); err != nil {
		t.Fatalf("relative from with a time zone: %v", err)
	}
}

func TestNewestOfPrefersTimeNsAndFallsBackToTheTimeField(t *testing.T) {
	seconds := datasource.Frame{Fields: []datasource.Field{{Name: "Timestamp", Type: datasource.FieldTime, Values: []any{time.Unix(10, 0).UTC(), time.Unix(20, 0).UTC()}}}}
	newest, ok := newestOf([]datasource.Frame{seconds})
	if !ok || !newest.Equal(time.Unix(20, 0)) {
		t.Fatalf("newest without TimeNs = %v, %v", newest, ok)
	}
	newest, ok = newestOf([]datasource.Frame{linesFrame(false, 3, 8, 5), seconds})
	if !ok || !newest.Equal(time.Unix(0, baseNs+8)) {
		t.Fatalf("newest across frames = %v, %v", newest, ok)
	}
	if _, ok := newestOf(nil); ok {
		t.Fatal("newest of no frames reported a time")
	}
	if !errors.Is(context.Canceled, context.Canceled) {
		t.Fatal("unreachable")
	}
}

func atSeconds(seconds ...int64) []int64 {
	offsets := make([]int64, len(seconds))
	for i, s := range seconds {
		offsets[i] = s * int64(time.Second)
	}
	return offsets
}

func withIDs(f datasource.Frame) datasource.Frame {
	ids := datasource.Field{Name: "ID", Type: datasource.FieldString}
	for _, v := range f.Fields[2].Values {
		ids.Values = append(ids.Values, "doc-"+v.(string))
	}
	f.Fields = append(f.Fields, ids, datasource.Field{Name: "Index", Type: datasource.FieldString, Values: make([]any, len(ids.Values))})
	return f
}

func bodiesOf(t *testing.T, ev event) []string {
	t.Helper()
	var frames []struct {
		Fields []struct {
			Name   string `json:"name"`
			Values []any  `json:"values"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(decode[linesData](t, ev, "lines").Frames, &frames); err != nil {
		t.Fatal(err)
	}
	var bodies []string
	for _, f := range frames {
		for _, field := range f.Fields {
			if field.Name != "Body" {
				continue
			}
			for _, v := range field.Values {
				bodies = append(bodies, v.(string))
			}
		}
	}
	return bodies
}

func TestTailSendsEachLineOnceAcrossTheLag(t *testing.T) {
	for _, tc := range []struct {
		name  string
		frame func(truncated bool, offsets ...int64) datasource.Frame
	}{
		{"keyed by the document id", func(truncated bool, offsets ...int64) datasource.Frame {
			return withIDs(linesFrame(truncated, offsets...))
		}},
		{"keyed by the time and the line", linesFrame},
	} {
		t.Run(tc.name, func(t *testing.T) {
			poll := func(seconds ...int64) response {
				return response{frames: []datasource.Frame{tc.frame(false, atSeconds(seconds...)...)}}
			}
			f := newFixture(t, poll(10, 11), poll(11, 12), poll(12), poll(13))
			s := f.open("u1", "query=%7Bapp%3D%22shop%22%7D&from="+fromParam)
			decode[readyData](t, s.next(t), "ready")
			for _, want := range [][]string{{"line 10000000000", "line 11000000000"}, {"line 12000000000"}, {"line 13000000000"}} {
				if got := bodiesOf(t, s.next(t)); !slices.Equal(got, want) {
					t.Fatalf("lines = %v, want %v", got, want)
				}
			}
			seen := waitForRequests(t, f.logs, 4)
			if got := seen[0].Range.From; !got.Equal(fromTime) {
				t.Fatalf("first poll from = %v, want the requested start %v", got, fromTime)
			}
			if got, want := seen[1].Range.From, time.Unix(0, baseNs+11*int64(time.Second)+1).Add(-datasource.DefaultTailLag); !got.Equal(want) {
				t.Fatalf("second poll from = %v, want the newest line plus one nanosecond minus the lag %v", got, want)
			}
		})
	}
}

func TestTailCatchesUpWithoutTheOverlap(t *testing.T) {
	f := newFixture(t,
		response{frames: []datasource.Frame{withIDs(linesFrame(true, atSeconds(20, 21)...))}},
		response{frames: []datasource.Frame{withIDs(linesFrame(false, atSeconds(22)...))}},
		response{frames: []datasource.Frame{withIDs(linesFrame(false, atSeconds(23)...))}},
	)
	s := f.open("u1", "query=%7Bapp%3D%22shop%22%7D&from="+fromParam)
	decode[readyData](t, s.next(t), "ready")
	for range 3 {
		s.next(t)
	}
	seen := waitForRequests(t, f.logs, 3)
	if got, want := seen[1].Range.From, time.Unix(0, baseNs+21*int64(time.Second)+1); !got.Equal(want) {
		t.Fatalf("poll after a full page from = %v, want the newest line plus one nanosecond %v", got, want)
	}
	if got, want := seen[2].Range.From, time.Unix(0, baseNs+22*int64(time.Second)+1).Add(-datasource.DefaultTailLag); !got.Equal(want) {
		t.Fatalf("poll after a short page from = %v, want the overlap %v", got, want)
	}
}

func TestSeenRowsForgetByCountAndByTime(t *testing.T) {
	rows := func(seconds ...int64) []datasource.Frame {
		return []datasource.Frame{linesFrame(false, atSeconds(seconds...)...)}
	}
	count := func(frames []datasource.Frame) int {
		n := 0
		for _, f := range frames {
			n += f.Rows()
		}
		return n
	}

	capped := newSeenRows(time.Hour, 2)
	capped.fresh(rows(1, 2, 3))
	if got := count(capped.fresh(rows(3, 2))); got != 0 {
		t.Fatalf("%d rows came back as fresh among the two newest", got)
	}
	if got := count(capped.fresh(rows(1))); got != 1 {
		t.Fatalf("the oldest key should have been dropped by the cap: fresh rows = %d", got)
	}

	timed := newSeenRows(10*time.Second, 100)
	timed.fresh(rows(1))
	timed.fresh(rows(30))
	if got := count(timed.fresh(rows(1))); got != 1 {
		t.Fatalf("a key older than the window should be forgotten: fresh rows = %d", got)
	}
	if got := count(timed.fresh(rows(30))); got != 0 {
		t.Fatalf("a key inside the window should be remembered: fresh rows = %d", got)
	}
}

func partialLines(offsets ...int64) response {
	f := linesFrame(false, offsets...)
	f.Meta = &datasource.FrameMeta{Partial: true, Warning: "a shard failed"}
	return response{frames: []datasource.Frame{f}}
}

func TestTailFlagsAPartialPollAndFetchesTheMissingLinesOnTheNext(t *testing.T) {
	f := newFixture(t, partialLines(1, 2), lines(1, 2, 3), lines(4))
	s := f.open("u1", "query=%7Bapp%3D%22shop%22%7D&from="+fromParam)
	decode[readyData](t, s.next(t), "ready")

	firstEvent := s.next(t)
	first := decode[linesData](t, firstEvent, "lines")
	if !first.Partial || first.Warning != "a shard failed" || len(bodiesOf(t, firstEvent)) != 2 {
		t.Fatalf("first lines = %+v, want two lines flagged partial with the warning", first)
	}
	secondEvent := s.next(t)
	if second := decode[linesData](t, secondEvent, "lines"); second.Partial {
		t.Fatalf("second lines = %+v, want complete", second)
	}
	if got := bodiesOf(t, secondEvent); !slices.Equal(got, []string{"line 3"}) {
		t.Fatalf("second poll sent %v, want only the line the partial poll missed", got)
	}
	decode[linesData](t, s.next(t), "lines")
	seen := waitForRequests(t, f.logs, 3)
	if got := seen[1].Range.From; !got.Equal(fromTime) {
		t.Fatalf("poll after a partial one from = %v, want the cursor held at %v", got, fromTime)
	}
	if got := seen[2].Range.From; !got.Equal(fromTime) {
		t.Fatalf("poll after a complete one from = %v, want newest plus one nanosecond minus the lag, held at the start %v", got, fromTime)
	}
}

func TestTailAdvancesAfterThreeHeldPartialPolls(t *testing.T) {
	f := newFixture(t, partialLines(1), partialLines(1), partialLines(1), partialLines(1))
	s := f.open("u1", "query=%7Bapp%3D%22shop%22%7D&from="+fromParam)
	decode[readyData](t, s.next(t), "ready")
	for range 4 {
		decode[linesData](t, s.next(t), "lines")
	}
	seen := waitForRequests(t, f.logs, 4)
	for i := 1; i <= 3; i++ {
		if got := seen[i].Range.From; !got.Equal(fromTime) {
			t.Fatalf("poll %d from = %v, want the held cursor %v", i, got, fromTime)
		}
	}
}

func TestTailCapsTheCursorAtTheTimeOfThePollWhenALineIsStampedInTheFuture(t *testing.T) {
	future := time.Now().Add(time.Hour).UnixNano() - baseNs
	f := newFixture(t, lines(future), lines(1))
	s := f.open("u1", "query=%7Bapp%3D%22shop%22%7D&from="+fromParam)
	decode[readyData](t, s.next(t), "ready")
	s.next(t)
	seen := waitForRequests(t, f.logs, 2)
	if got := seen[1].Range.From; got.After(time.Now()) {
		t.Fatalf("second poll from = %v, want a cursor no later than now", got)
	}
}

func TestTailOnADatasourceWithLagKeepsTwoStreamsThatShareATimeAndALine(t *testing.T) {
	stream := func(name string) datasource.Frame {
		f := linesFrame(false, atSeconds(10)...)
		f.Labels = map[string]string{"pod": name}
		return f
	}
	f := newFixture(t, response{frames: []datasource.Frame{stream("a"), stream("b")}}, response{frames: []datasource.Frame{stream("a"), stream("b")}})
	s := f.open("u1", "query=%7Bapp%3D%22shop%22%7D&from="+fromParam)
	decode[readyData](t, s.next(t), "ready")
	if got := bodiesOf(t, s.next(t)); len(got) != 2 {
		t.Fatalf("lines = %v, want one from each stream", got)
	}
}

func TestTailOnADatasourceWithLagKeepsARepeatedLineWhenTheFrameHasNoNanosecondField(t *testing.T) {
	repeated := func(offsets ...int64) datasource.Frame {
		f := linesFrame(false, offsets...)
		f.Fields = f.Fields[:2]
		for i := range f.Fields[1].Values {
			f.Fields[1].Values[i] = "connection refused"
		}
		return f
	}
	f := newFixture(t,
		response{frames: []datasource.Frame{repeated(atSeconds(10)...)}},
		response{frames: []datasource.Frame{repeated(atSeconds(10, 12)...)}},
	)
	s := f.open("u1", "query=%7Bapp%3D%22shop%22%7D&from="+fromParam)
	decode[readyData](t, s.next(t), "ready")
	if got := bodiesOf(t, s.next(t)); len(got) != 1 {
		t.Fatalf("first poll lines = %v, want one", got)
	}
	if got := bodiesOf(t, s.next(t)); len(got) != 1 {
		t.Fatalf("second poll lines = %v, want only the repeat at the later time", got)
	}
}
