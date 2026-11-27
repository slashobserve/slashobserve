package stream

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/events"
	"github.com/slashobserve/slashobserve/internal/folders"
)

func finalEvent(t *testing.T, lines <-chan string, done <-chan struct{}) (name, data string) {
	t.Helper()
	note := func(line string) {
		if after, ok := strings.CutPrefix(line, "event: "); ok {
			name, data = after, ""
		}
		if after, ok := strings.CutPrefix(line, "data: "); ok {
			data = after
		}
	}
	timeout := time.After(waitTimeout)
	for {
		select {
		case line := <-lines:
			note(line)
		case <-done:
			for {
				select {
				case line := <-lines:
					note(line)
				default:
					return name, data
				}
			}
		case <-timeout:
			t.Fatalf("the stream stayed open for %s", waitTimeout)
		}
	}
}

func expectClosedBecause(t *testing.T, lines <-chan string, done <-chan struct{}, reason string) {
	t.Helper()
	name, data := finalEvent(t, lines, done)
	if name != "closed" || data != `{"reason":"`+reason+`"}` {
		t.Fatalf("the stream ended after event %q with data %s, want closed with reason %s", name, data, reason)
	}
}

func (s *openStream) expectClosedBecause(t *testing.T, reason string) {
	t.Helper()
	expectClosedBecause(t, s.lines, s.done, reason)
}

func (s *activityStream) expectClosedBecause(t *testing.T, reason string) {
	t.Helper()
	expectClosedBecause(t, s.lines, s.done, reason)
}

func refusal(t *testing.T, srv *httptest.Server, path string) (int, map[string]string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/api/v1"+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, body
}

func TestACheckEndsAStreamOnlyForLostAccessOrRepeatedFailure(t *testing.T) {
	cases := []struct {
		name          string
		valid, failed bool
		failuresLeft  int
		want          closeReason
	}{
		{"the check passed", true, false, 2, stillOpen},
		{"access is gone", false, false, 2, closedAccessLost},
		{"one failure with another allowed", true, true, 1, stillOpen},
		{"the last allowed failure", true, true, 0, closedCheckFailed},
		{"the check failed because the client left", false, true, 1, unannounced},
	}
	for _, tc := range cases {
		if got := endAfterCheck(tc.valid, tc.failed, tc.failuresLeft); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestClosedIsSentOnlyToAClientThatIsStillThere(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gone, leave := context.WithCancel(t.Context())
	leave()
	cases := []struct {
		name   string
		ctx    context.Context
		reason closeReason
		want   string
	}{
		{"a reason for a client that is there", t.Context(), closedLifetime, "event: closed\ndata: {\"reason\":\"lifetime\"}\n\n"},
		{"an end the stream already explained", t.Context(), unannounced, ""},
		{"a client that left", gone, closedLifetime, ""},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequestWithContext(tc.ctx, http.MethodGet, "/", nil)
		sayClosed(c, tc.reason)
		if got := rec.Body.String(); got != tc.want {
			t.Errorf("%s: wrote %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestStreamSaysWhyBeforeTheServerEndsIt(t *testing.T) {
	cases := []struct {
		name   string
		end    func(*fakeSessions, *fakeAccess, *events.Broker)
		reason string
	}{
		{"credential no longer valid", func(s *fakeSessions, _ *fakeAccess, _ *events.Broker) { s.valid.Store(false) }, "access_lost"},
		{"view removed", func(_ *fakeSessions, a *fakeAccess, _ *events.Broker) { a.view.Store(false) }, "access_lost"},
		{"dashboard deleted", func(_ *fakeSessions, a *fakeAccess, _ *events.Broker) { a.err.Store(&folders.ErrNotFound) }, "access_lost"},
		{"credential checks keep failing", func(s *fakeSessions, _ *fakeAccess, _ *events.Broker) { s.failNext.Store(1000) }, "check_failed"},
		{"access checks keep failing", func(_ *fakeSessions, a *fakeAccess, _ *events.Broker) { a.failNext.Store(1000) }, "check_failed"},
		{"server shutting down", func(_ *fakeSessions, _ *fakeAccess, b *events.Broker) { b.Close() }, "stopping"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sessions := &fakeSessions{}
			sessions.valid.Store(true)
			access := newFakeAccess()
			broker := events.NewBroker(nil, slog.New(slog.DiscardHandler))
			t.Cleanup(broker.Close)
			s := followEvents(t, serveEvents(t, broker, testPing, sessions, access, eventViewer, nil), "")
			s.waitFor(t, "event: ready")
			s.waitFor(t, ": ping")

			tc.end(sessions, access, broker)
			s.expectClosedBecause(t, tc.reason)
		})
	}
}

func TestStreamSaysWhyAfterAMoveTheCallerCanNoLongerSee(t *testing.T) {
	cases := []struct {
		name   string
		revoke func(*fakeAccess)
		reason string
	}{
		{"view removed", func(a *fakeAccess) { a.view.Store(false) }, "access_lost"},
		{"access lookup failed", func(a *fakeAccess) { a.failNext.Store(1) }, "check_failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			access := newFakeAccess()
			s, notices := openFedStream(t, access)
			tc.revoke(access)
			notices <- movedNotice
			s.expectClosedBecause(t, tc.reason)
		})
	}
}

func TestStreamSaysWhenItsLifetimeIsReached(t *testing.T) {
	sessions := &fakeSessions{}
	sessions.valid.Store(true)
	broker := events.NewBroker(nil, slog.New(slog.DiscardHandler))
	t.Cleanup(broker.Close)
	h := NewHandler(broker, fakeDashboards{}, sessions, newFakeAccess(), allowAll, slog.New(slog.DiscardHandler))
	h.lifetime = 20 * time.Millisecond

	followEvents(t, serveRoutes(t, eventViewer, h.Register), "").expectClosedBecause(t, "lifetime")
}

type heldAccess struct {
	entered chan struct{}
	release chan struct{}
}

func (h heldAccess) Dashboard(context.Context, auth.Principal, pgtype.UUID) (folders.DashboardAccess, error) {
	select {
	case h.entered <- struct{}{}:
	default:
	}
	<-h.release
	return folders.DashboardAccess{Abilities: folders.Abilities{View: true}}, nil
}

func TestStreamSaysWhenTheBrokerDroppedItForBeingSlow(t *testing.T) {
	const moreThanTheBrokerBuffers = 64
	access := heldAccess{entered: make(chan struct{}, 1), release: make(chan struct{})}
	s, notices := openFedStream(t, access)
	notices <- movedNotice
	select {
	case <-access.entered:
	case <-time.After(waitTimeout):
		t.Fatal("the stream did not check access for the move")
	}
	for range moreThanTheBrokerBuffers {
		notices <- movedNotice
	}
	close(access.release)

	s.expectClosedBecause(t, "slow_subscriber")
}

func TestTooManyStreamsSaysWhoseLimitWasReached(t *testing.T) {
	sessions := &fakeSessions{}
	sessions.valid.Store(true)
	path := "/dashboards/" + dashboardID + "/events"
	cases := []struct {
		name         string
		slots        *slots
		open, status int
		reason       string
	}{
		{"the caller's limit", newSlots(2, 10), 2, http.StatusTooManyRequests, "per_caller"},
		{"the server's limit", newSlots(2, 1), 1, http.StatusServiceUnavailable, "server_capacity"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			broker := events.NewBroker(nil, slog.New(slog.DiscardHandler))
			t.Cleanup(broker.Close)
			h := NewHandler(broker, fakeDashboards{}, sessions, newFakeAccess(), allowAll, slog.New(slog.DiscardHandler))
			h.slots = tc.slots
			srv := serveRoutes(t, eventViewer, h.Register)
			for range tc.open {
				followEvents(t, srv, "").waitFor(t, "event: ready")
			}

			status, body := refusal(t, srv, path)

			if status != tc.status || body["code"] != "too_many_streams" || body["reason"] != tc.reason || body["error"] == "" {
				t.Fatalf("one stream too many = %d %v, want %d too_many_streams with reason %s", status, body, tc.status, tc.reason)
			}
		})
	}
}

type emptyLog struct {
	fail atomic.Pointer[error]
}

func (l *emptyLog) Audience(context.Context, auth.Principal) (folders.Audience, error) {
	return folders.Audience{}, nil
}

func (l *emptyLog) Start(context.Context, string) (activity.Position, bool, error) {
	return activity.Position{}, false, nil
}

func (l *emptyLog) Next(_ context.Context, _ folders.Audience, _ activity.Filter, from activity.Position, _ int) ([]activity.Event, activity.Position, bool, error) {
	if err := l.fail.Load(); err != nil {
		return nil, from, false, *err
	}
	return nil, from, false, nil
}

type emptyActivity struct {
	log        *emptyLog
	principals *fakePrincipals
	broker     *events.Broker
	handler    *ActivityHandler
	server     *httptest.Server
}

func newEmptyActivity(t *testing.T) *emptyActivity {
	t.Helper()
	a := &emptyActivity{log: &emptyLog{}, principals: &fakePrincipals{}, broker: events.NewBroker(nil, slog.New(slog.DiscardHandler))}
	t.Cleanup(a.broker.Close)
	a.principals.becomes(viewer)
	a.handler = NewActivityHandler(a.broker, a.log, a.principals, slog.New(slog.DiscardHandler))
	a.handler.pingInterval = testPing
	a.server = serveRoutes(t, viewer, a.handler.Register)
	return a
}

func (a *emptyActivity) open(t *testing.T) *openStream {
	t.Helper()
	s := followPath(t, a.server, "/activity/stream", "")
	s.waitFor(t, "event: ready")
	return s
}

func TestActivityStreamSaysWhyBeforeTheServerEndsIt(t *testing.T) {
	unavailable := errors.New("database unavailable")
	cases := []struct {
		name   string
		end    func(*emptyActivity)
		reason string
	}{
		{"credential revoked", func(a *emptyActivity) { a.principals.current.Store(nil) }, "access_lost"},
		{"credential checks keep failing", func(a *emptyActivity) { a.principals.err.Store(&unavailable) }, "check_failed"},
		{"the log cannot be read", func(a *emptyActivity) { a.log.fail.Store(&unavailable) }, "read_failed"},
		{"server shutting down", func(a *emptyActivity) { a.broker.Close() }, "stopping"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := newEmptyActivity(t)
			s := a.open(t)
			s.waitFor(t, ": ping")

			tc.end(a)

			s.expectClosedBecause(t, tc.reason)
		})
	}
}

func TestActivityStreamSaysWhenItsLifetimeIsReached(t *testing.T) {
	a := newEmptyActivity(t)
	a.handler.pingInterval = time.Hour
	a.handler.lifetime = 20 * time.Millisecond

	followPath(t, a.server, "/activity/stream", "").expectClosedBecause(t, "lifetime")
}

func TestActivityTooManyStreamsSaysWhoseLimitWasReached(t *testing.T) {
	a := newEmptyActivity(t)
	for range maxActivityStreamsPerUser {
		a.open(t)
	}

	status, body := refusal(t, a.server, "/activity/stream")

	if status != http.StatusTooManyRequests || body["code"] != "too_many_streams" || body["reason"] != "per_caller" {
		t.Fatalf("one stream too many = %d %v, want 429 too_many_streams with reason per_caller", status, body)
	}
}
