package stream

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/audit/trail"
	"github.com/slashobserve/slashobserve/internal/audit/trail/trailtest"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/events"
)

type auditRig struct {
	db         *dbtest.Database
	broker     *events.Broker
	principals *fakePrincipals
	handler    *AuditHandler
	server     *httptest.Server
	caller     atomic.Pointer[auth.Principal]
	stopAudit  func()
}

func newAuditRig(t *testing.T, caller auth.Principal) *auditRig {
	t.Helper()
	gin.SetMode(gin.TestMode)
	d := dbtest.New(t)
	n := &notifier{ready: make(chan struct{})}
	broker := events.NewBroker(n.listen, slog.New(slog.DiscardHandler))
	ctx, cancel := context.WithCancel(context.Background())
	go broker.Run(ctx)
	t.Cleanup(func() {
		cancel()
		broker.Close()
	})
	<-n.ready

	recorder := audit.New(audit.Config{DB: d.DB, Server: "test", Version: "test", Logger: slog.New(slog.DiscardHandler)})
	audit.Install(recorder)
	t.Cleanup(func() { audit.Install(nil) })
	auditCtx, stopRecorder := context.WithCancel(context.Background())
	recorded := make(chan struct{})
	go func() {
		defer close(recorded)
		recorder.Run(auditCtx)
	}()

	r := &auditRig{db: d, broker: broker, principals: &fakePrincipals{}}
	r.stopAudit = func() {
		stopRecorder()
		<-recorded
	}
	t.Cleanup(r.stopAudit)
	r.caller.Store(&caller)
	r.principals.becomes(caller)
	r.handler = NewAuditHandler(broker, trail.NewReader(d.DB, slog.New(slog.DiscardHandler)), r.principals, slog.New(slog.DiscardHandler))
	r.handler.pollInterval = 10 * time.Millisecond
	r.handler.pingInterval = time.Hour
	mw, err := recorder.Middleware()
	if err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	api := engine.Group("/api/v1", mw, func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), *r.caller.Load()))
	})
	r.handler.Register(auth.NewRoutes(api))
	r.server = httptest.NewServer(engine)
	t.Cleanup(r.server.Close)
	t.Cleanup(r.server.CloseClientConnections)
	return r
}

func (r *auditRig) request(t *testing.T, q, lastEventID string) *http.Request {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.server.URL+"/api/v1/audit/stream?"+url.Values{"q": {q}}.Encode(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if lastEventID != "" {
		req.Header.Set("Last-Event-ID", lastEventID)
	}
	return req
}

func (r *auditRig) refused(t *testing.T, q, lastEventID string) (int, string) {
	t.Helper()
	resp, err := http.DefaultClient.Do(r.request(t, q, lastEventID))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var body struct{ Code string }
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, body.Code
}

func (r *auditRig) open(t *testing.T, q string) *activityStream {
	t.Helper()
	s, _ := r.openFrom(t, q, "")
	return s
}

func (r *auditRig) openFrom(t *testing.T, q, lastEventID string) (*activityStream, string) {
	t.Helper()
	resp, err := http.DefaultClient.Do(r.request(t, q, lastEventID))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	s := &activityStream{lines: make(chan string, 1024), done: make(chan struct{})}
	go func() {
		defer close(s.done)
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			s.lines <- scanner.Text()
		}
	}()
	ready := s.nextFrame(t)
	if ready.event != "ready" || ready.data != "{}" {
		t.Fatalf("frame %+v, want ready with {}", ready)
	}
	return s, ready.id
}

func (r *auditRig) searches(t *testing.T) []string {
	t.Helper()
	r.stopAudit()
	rows, err := r.db.SQLDB(t).QueryContext(t.Context(), `SELECT outcome || ' ' || coalesce(data->>'view', '') || ' ' || coalesce(data->>'query', '') FROM audit_events WHERE action = 'audit.search' ORDER BY seq`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestAuditStreamSendsNewMatchingEventsAndRecordsOneSearch(t *testing.T) {
	r := newAuditRig(t, admin)
	s := r.open(t, "outcome:denied")
	ids := trailtest.Insert(t, r.db, trailtest.Event{At: time.Now().UTC(), Outcome: "success"}, trailtest.Event{At: time.Now().UTC(), Outcome: "denied", Status: 403})
	f := s.nextFrame(t)
	var got struct {
		ID      string `json:"id"`
		Outcome string `json:"outcome"`
	}
	if err := json.Unmarshal([]byte(f.data), &got); err != nil || f.event != "audit" || got.ID != ids[1] || got.Outcome != "denied" || !strings.Contains(f.id, "-") {
		t.Fatalf("frame %+v (%v)", f, err)
	}
	trailtest.Insert(t, r.db, trailtest.Event{At: time.Now().UTC(), Outcome: "denied", Status: 403})
	if next := s.nextFrame(t); next.event != "audit" || next.id == "" || next.id == f.id {
		t.Fatalf("second frame %+v", next)
	}
	if got := r.searches(t); len(got) != 1 || got[0] != "success stream outcome:denied" {
		t.Fatalf("recorded searches %q", got)
	}
}

func TestAuditStreamReadyCarriesTheIDItResumesFromWithoutAGap(t *testing.T) {
	r := newAuditRig(t, admin)
	_, start := r.openFrom(t, "outcome:denied", "")
	if strings.Count(start, "-") != 2 {
		t.Fatalf("ready id %q, want xid-seq-micros", start)
	}
	ids := trailtest.Insert(t, r.db, trailtest.Event{At: time.Now().UTC(), Outcome: "denied", Status: 403})

	resumed, again := r.openFrom(t, "outcome:denied", start)
	if again != start {
		t.Errorf("ready of a stream resumed from %q has id %q", start, again)
	}
	var got struct {
		ID string `json:"id"`
	}
	if f := resumed.nextFrame(t); f.event != "audit" || json.Unmarshal([]byte(f.data), &got) != nil || got.ID != ids[0] {
		t.Fatalf("resumed from the ready id and got %+v, want %s", f, ids[0])
	}
}

func (s *activityStream) framesUntilClosed(t *testing.T) []frame {
	t.Helper()
	if !s.closedWithin(waitTimeout) {
		t.Fatal("the stream did not end")
	}
	var frames []frame
	var f frame
	for len(s.lines) > 0 {
		line := <-s.lines
		switch {
		case line == "" && f.event != "":
			frames = append(frames, f)
			f = frame{}
		case strings.HasPrefix(line, "event: "):
			f.event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			f.data = strings.TrimPrefix(line, "data: ")
		}
	}
	return frames
}

func TestAuditStreamEndsWhenTheCallerIsNoLongerAnAdmin(t *testing.T) {
	r := newAuditRig(t, admin)
	r.handler.pingInterval = 10 * time.Millisecond
	s := r.open(t, "action:dashboard.delete")
	r.principals.becomes(auth.Principal{Kind: auth.KindUser, ID: adminID, Name: "Root", Role: auth.RoleEditor})
	frames := s.framesUntilClosed(t)
	if len(frames) != 1 {
		t.Fatalf("frames %+v, want one forbidden frame", frames)
	}
	var body struct {
		Code       string `json:"code"`
		NeededRole string `json:"needed_role"`
	}
	if err := json.Unmarshal([]byte(frames[0].data), &body); err != nil || frames[0].event != "forbidden" || body.Code != "forbidden_role" || body.NeededRole != "admin" {
		t.Fatalf("frame %+v (%v)", frames[0], err)
	}
}

func TestAuditStreamEndsAtShutdownAfterItsSearchIsRecorded(t *testing.T) {
	r := newAuditRig(t, admin)
	s := r.open(t, "origin:mcp")
	r.broker.Close()
	if !s.closedWithin(waitTimeout) {
		t.Fatal("the stream outlived the shutdown")
	}
	if got := r.searches(t); len(got) != 1 || got[0] != "success stream origin:mcp" {
		t.Fatalf("recorded searches %q", got)
	}
}

func TestAuditStreamSaysWhyBeforeTheServerEndsIt(t *testing.T) {
	unavailable := errors.New("database unavailable")
	cases := []struct {
		name   string
		tune   func(*AuditHandler)
		end    func(*auditRig)
		reason string
	}{
		{"lifetime reached", func(h *AuditHandler) { h.lifetime = 100 * time.Millisecond }, func(*auditRig) {}, "lifetime"},
		{"credential checks keep failing", func(h *AuditHandler) { h.pingInterval = 10 * time.Millisecond }, func(r *auditRig) { r.principals.err.Store(&unavailable) }, "check_failed"},
		{"server shutting down", func(*AuditHandler) {}, func(r *auditRig) { r.broker.Close() }, "stopping"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newAuditRig(t, admin)
			tc.tune(r.handler)
			s := r.open(t, "")

			tc.end(r)

			s.expectClosedBecause(t, tc.reason)
		})
	}
}

func TestAuditStreamRefusesNonAdminsAndBadRequests(t *testing.T) {
	for name, p := range map[string]auth.Principal{
		"viewer":     viewer,
		"commenter":  {Kind: auth.KindUser, ID: viewerID, Role: auth.RoleCommenter},
		"editor":     {Kind: auth.KindUser, ID: viewerID, Role: auth.RoleEditor},
		"editor key": {Kind: auth.KindServiceAccount, ID: viewerID, Role: auth.RoleEditor},
		"anonymous":  {},
	} {
		r := newAuditRig(t, p)
		if status, code := r.refused(t, "", ""); status != http.StatusForbidden || code != "forbidden_role" {
			t.Errorf("%s = %d %s", name, status, code)
		}
	}
	r := newAuditRig(t, admin)
	for _, tc := range []struct {
		q, lastEventID string
		status         int
		code           string
	}{
		{"status:x", "", http.StatusUnprocessableEntity, trail.CodeQueryInvalid},
		{"", "12", http.StatusBadRequest, trail.CodeInvalidRequest},
	} {
		if status, code := r.refused(t, tc.q, tc.lastEventID); status != tc.status || code != tc.code {
			t.Errorf("q %q Last-Event-ID %q = %d %s", tc.q, tc.lastEventID, status, code)
		}
	}
	if got := r.searches(t); len(got) != 2 || got[0] != "failed stream status:x" || got[1] != "failed stream " {
		t.Fatalf("recorded searches %q", got)
	}
}

func TestAuditStreamAnswers429BeyondFivePerCallerAndTwentyPerServer(t *testing.T) {
	r := newAuditRig(t, admin)
	r.handler.pollInterval = time.Hour
	for range maxAuditStreamsPerUser {
		r.open(t, "")
	}
	status, code := r.refused(t, "", "")
	if status != http.StatusTooManyRequests || code != "too_many_streams" {
		t.Fatalf("a sixth stream for one caller = %d %s", status, code)
	}
	for i := maxAuditStreamsPerUser; i < maxAuditStreamsPerProc; i++ {
		other := auth.Principal{Kind: auth.KindServiceAccount, ID: fmt.Sprintf("00000000-0000-4000-8000-%012d", i), Role: auth.RoleAdmin}
		r.caller.Store(&other)
		r.open(t, "")
	}
	last := auth.Principal{Kind: auth.KindUser, ID: viewerID, Role: auth.RoleAdmin}
	r.caller.Store(&last)
	resp, err := http.DefaultClient.Do(r.request(t, "", ""))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var body struct{ Error, Code, Reason string }
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil || resp.StatusCode != http.StatusTooManyRequests || body.Code != "too_many_streams" || body.Reason != "server_capacity" || !strings.Contains(body.Error, "20") {
		t.Fatalf("a stream past the server's limit = %d %+v (%v)", resp.StatusCode, body, err)
	}
}

func TestAuditStreamEndsWhenItsPollTimesOutTwiceInARow(t *testing.T) {
	r := newAuditRig(t, admin)
	s := r.open(t, "outcome:denied")
	lock, err := r.db.SQLDB(t).BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Rollback() }()
	if _, err := lock.ExecContext(t.Context(), "LOCK TABLE audit_events IN ACCESS EXCLUSIVE MODE"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-s.done:
	case <-time.After(maxAuditPollTimeouts*trail.FollowTimeout + 3*waitTimeout):
		t.Fatal("the stream outlived two poll timeouts")
	}
	_ = lock.Rollback()
	var f frame
	for len(s.lines) > 0 {
		line := <-s.lines
		switch {
		case strings.HasPrefix(line, "event: "):
			f.event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			f.data = strings.TrimPrefix(line, "data: ")
		}
	}
	var body struct{ Error, Code string }
	if err := json.Unmarshal([]byte(f.data), &body); err != nil || f.event != "error" || body.Code != codeTooExpensive || !strings.Contains(body.Error, "too expensive to follow") {
		t.Fatalf("last frame %+v (%v)", f, err)
	}
}
