package stream

import (
	"bufio"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/events"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/logtest"
	"github.com/slashobserve/slashobserve/internal/opaquetoken"
	"github.com/slashobserve/slashobserve/internal/users"
)

const (
	testPing    = 10 * time.Millisecond
	waitTimeout = 5 * time.Second
	dashboardID = "44444444-4444-4444-8444-444444444444"
)

type fakeDashboards struct{}

func (fakeDashboards) LatestVersion(context.Context, pgtype.UUID) (int, error) { return 1, nil }

type fakeSessions struct {
	valid    atomic.Bool
	failNext atomic.Int32
	calls    atomic.Int32
	role     atomic.Value
}

func (f *fakeSessions) RequestPrincipal(*http.Request) (auth.Principal, bool, error) {
	f.calls.Add(1)
	if f.failNext.Load() > 0 {
		f.failNext.Add(-1)
		return auth.Principal{}, false, errors.New("database unavailable")
	}
	role, ok := f.role.Load().(auth.Role)
	if !ok {
		role = auth.RoleViewer
	}
	return auth.Principal{Kind: auth.KindUser, ID: "u1", Role: role}, f.valid.Load(), nil
}

type fakeAccess struct {
	lastRole atomic.Value
	view     atomic.Bool
	failNext atomic.Int32
	err      atomic.Pointer[error]
}

func (f *fakeAccess) Dashboard(_ context.Context, p auth.Principal, _ pgtype.UUID) (folders.DashboardAccess, error) {
	f.lastRole.Store(p.Role)
	if f.failNext.Load() > 0 {
		f.failNext.Add(-1)
		return folders.DashboardAccess{}, errors.New("database unavailable")
	}
	if err := f.err.Load(); err != nil {
		return folders.DashboardAccess{}, *err
	}
	return folders.DashboardAccess{Abilities: folders.Abilities{View: f.view.Load()}}, nil
}

func allowAll(*gin.Context) {}

func newFakeAccess() *fakeAccess {
	f := &fakeAccess{}
	f.view.Store(true)
	return f
}

type openStream struct {
	lines chan string
	done  chan struct{}
}

var eventViewer = auth.Principal{Kind: auth.KindUser, ID: "u1", Role: auth.RoleViewer}

func newEventServer(t *testing.T, sessions Principals, access Access, caller auth.Principal, extra func(auth.Routes)) *httptest.Server {
	t.Helper()
	broker := events.NewBroker(nil, slog.New(slog.DiscardHandler))
	t.Cleanup(broker.Close)
	return serveEvents(t, broker, testPing, sessions, access, caller, extra)
}

func serveEvents(t *testing.T, broker *events.Broker, ping time.Duration, sessions Principals, access Access, caller auth.Principal, extra func(auth.Routes)) *httptest.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := NewHandler(broker, fakeDashboards{}, sessions, access, allowAll, slog.New(slog.DiscardHandler))
	h.pingInterval = ping
	h.roleInterval = testPing
	return serveRoutes(t, caller, func(routes auth.Routes) {
		h.Register(routes)
		if extra != nil {
			extra(routes)
		}
	})
}

func serveRoutes(t *testing.T, caller auth.Principal, register func(auth.Routes)) *httptest.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), caller))
	})
	register(auth.NewRoutes(api))
	srv := httptest.NewServer(engine)
	t.Cleanup(srv.Close)
	t.Cleanup(srv.CloseClientConnections)
	return srv
}

func followEvents(t *testing.T, srv *httptest.Server, cookie string) *openStream {
	t.Helper()
	return followPath(t, srv, "/dashboards/"+dashboardID+"/events", cookie)
}

func followPath(t *testing.T, srv *httptest.Server, path, cookie string) *openStream {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/api/v1"+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	s := &openStream{lines: make(chan string, 1024), done: make(chan struct{})}
	go func() {
		defer close(s.done)
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			select {
			case s.lines <- scanner.Text():
			default:
			}
		}
	}()
	return s
}

func openEventStream(t *testing.T, sessions Principals, access Access) *openStream {
	t.Helper()
	return followEvents(t, newEventServer(t, sessions, access, eventViewer, nil), "")
}

func (s *openStream) waitFor(t *testing.T, prefix string) {
	t.Helper()
	timeout := time.After(waitTimeout)
	for {
		select {
		case line := <-s.lines:
			if strings.HasPrefix(line, prefix) {
				return
			}
		case <-s.done:
			t.Fatalf("stream ended before %q", prefix)
		case <-timeout:
			t.Fatalf("no %q within %s", prefix, waitTimeout)
		}
	}
}

func (s *openStream) closedWithin(d time.Duration) bool {
	select {
	case <-s.done:
		return true
	case <-time.After(d):
		return false
	}
}

func TestStreamEndsWhenTheCredentialStopsBeingValid(t *testing.T) {
	sessions := &fakeSessions{}
	sessions.valid.Store(true)
	s := openEventStream(t, sessions, newFakeAccess())
	s.waitFor(t, "event: ready")
	s.waitFor(t, ": ping")
	s.waitFor(t, ": ping")

	sessions.valid.Store(false)
	if !s.closedWithin(waitTimeout) {
		t.Fatal("the stream outlived its session")
	}
}

func TestStreamEndsWhenItsPersonIsSignedOutEverywhere(t *testing.T) {
	database := dbtest.New(t)
	logger := slog.New(slog.DiscardHandler)
	authService, err := auth.New(auth.Config{PublicURL: &url.URL{Scheme: "http", Host: "localhost:8080"}, IssuerURL: "http://issuer.example/dex"}, database.Queries(), logger)
	if err != nil {
		t.Fatal(err)
	}
	person, err := database.Queries().UpsertUser(t.Context(), dbsqlc.UpsertUserParams{OidcIssuer: "i", OidcSubject: "s", Email: "ada@example.com", Name: "Ada"})
	if err != nil {
		t.Fatal(err)
	}
	token, hash, err := opaquetoken.New("sos_")
	if err != nil {
		t.Fatal(err)
	}
	err = database.Queries().CreateSession(t.Context(), dbsqlc.CreateSessionParams{TokenHash: hash, UserID: person.ID, ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	caller := auth.Principal{Kind: auth.KindUser, ID: person.ID.String(), Role: auth.RoleAdmin}
	srv := newEventServer(t, authService, newFakeAccess(), caller, func(routes auth.Routes) {
		users.NewHandler(database.DB, authService, folders.NewAccess(database.DB), logger).Register(routes)
	})
	s := followEvents(t, srv, "so_session="+token)
	s.waitFor(t, "event: ready")
	s.waitFor(t, ": ping")

	req, err := http.NewRequestWithContext(t.Context(), http.MethodDelete, srv.URL+"/api/v1/users/"+person.ID.String()+"/sessions", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("sign out everywhere = %d, want 204", resp.StatusCode)
	}
	if !s.closedWithin(waitTimeout) {
		t.Fatal("the stream outlived the sign out everywhere of its person")
	}
}

func TestStreamSurvivesASingleCheckFailure(t *testing.T) {
	sessions := &fakeSessions{}
	sessions.valid.Store(true)
	s := openEventStream(t, sessions, newFakeAccess())
	s.waitFor(t, "event: ready")

	sessions.failNext.Store(1)
	for sessions.failNext.Load() > 0 {
		time.Sleep(testPing)
	}
	s.waitFor(t, ": ping")
	s.waitFor(t, ": ping")
	if s.closedWithin(5 * testPing) {
		t.Fatal("one failed lookup must not close the stream")
	}
}

func TestStreamEndsAfterConsecutiveCheckFailures(t *testing.T) {
	sessions := &fakeSessions{}
	sessions.valid.Store(true)
	s := openEventStream(t, sessions, newFakeAccess())
	s.waitFor(t, "event: ready")

	sessions.failNext.Store(1000)
	if !s.closedWithin(waitTimeout) {
		t.Fatal("the stream stayed open while its credentials could not be checked")
	}
}

func TestStreamEndsAtThePingAfterTheCallerLosesView(t *testing.T) {
	cases := []struct {
		name   string
		revoke func(*fakeAccess)
	}{
		{"view removed", func(a *fakeAccess) { a.view.Store(false) }},
		{"folder access removed", func(a *fakeAccess) { a.err.Store(&folders.ErrNoAccess) }},
		{"dashboard deleted", func(a *fakeAccess) { a.err.Store(&folders.ErrNotFound) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sessions := &fakeSessions{}
			sessions.valid.Store(true)
			access := newFakeAccess()
			s := openEventStream(t, sessions, access)
			s.waitFor(t, "event: ready")
			s.waitFor(t, ": ping")

			tc.revoke(access)
			if !s.closedWithin(waitTimeout) {
				t.Fatal("the stream outlived the caller's access")
			}
		})
	}
}

func TestStreamTreatsAnAccessLookupFailureLikeACredentialCheckFailure(t *testing.T) {
	sessions := &fakeSessions{}
	sessions.valid.Store(true)
	access := newFakeAccess()
	s := openEventStream(t, sessions, access)
	s.waitFor(t, "event: ready")

	access.failNext.Store(1)
	for access.failNext.Load() > 0 {
		time.Sleep(testPing)
	}
	s.waitFor(t, ": ping")
	s.waitFor(t, ": ping")
	if s.closedWithin(5 * testPing) {
		t.Fatal("one failed access lookup must not close the stream")
	}

	access.failNext.Store(1000)
	if !s.closedWithin(waitTimeout) {
		t.Fatal("the stream stayed open while access could not be checked")
	}
}

type failingDashboards struct{ err error }

func (f failingDashboards) LatestVersion(context.Context, pgtype.UUID) (int, error) { return 0, f.err }

func TestClientGoingAwayWhileReadingTheVersionIsNotAServerError(t *testing.T) {
	cases := []struct {
		name          string
		err           error
		cancelRequest bool
		wantStatus    int
		wantError     bool
	}{
		{"request canceled", errors.Join(errors.New("latest version"), context.Canceled), true, clientgone.Status, false},
		{"server side cancellation", errors.Join(errors.New("latest version"), context.Canceled), false, http.StatusInternalServerError, true},
		{"server deadline", errors.Join(errors.New("latest version"), context.DeadlineExceeded), false, http.StatusInternalServerError, true},
		{"database failure", errors.New("database exploded"), false, http.StatusInternalServerError, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			logger, logs := logtest.New()
			broker := events.NewBroker(nil, slog.New(slog.DiscardHandler))
			t.Cleanup(broker.Close)
			h := NewHandler(broker, failingDashboards{tc.err}, &fakeSessions{}, newFakeAccess(), allowAll, logger)
			engine := gin.New()
			h.Register(auth.NewRoutes(engine.Group("/api/v1", func(c *gin.Context) {
				c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), auth.Principal{Kind: auth.KindUser, ID: "u1", Role: auth.RoleViewer}))
			})))
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if tc.cancelRequest {
				cancel()
			}
			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/dashboards/"+dashboardID+"/events", nil))
			if rec.Code != tc.wantStatus {
				t.Errorf("status %d, want %d", rec.Code, tc.wantStatus)
			}
			if got := strings.Contains(logs.String(), `"level":"ERROR"`); got != tc.wantError {
				t.Errorf("ERROR logged = %v, want %v: %s", got, tc.wantError, logs.String())
			}
			if !tc.wantError && rec.Body.Len() != 0 {
				t.Errorf("body %q, want none", rec.Body.String())
			}
		})
	}
}

func TestStreamChecksAccessAsTheRoleTheCallerHasNow(t *testing.T) {
	sessions := &fakeSessions{}
	sessions.valid.Store(true)
	sessions.role.Store(auth.RoleEditor)
	access := newFakeAccess()
	s := openEventStream(t, sessions, access)
	s.waitFor(t, "event: ready")
	s.waitFor(t, ": ping")
	if got := access.lastRole.Load(); got != auth.RoleEditor {
		t.Fatalf("access was checked as %v before the demotion", got)
	}

	sessions.role.Store(auth.RoleViewer)
	deadline := time.After(waitTimeout)
	for access.lastRole.Load() != auth.RoleViewer {
		select {
		case <-deadline:
			t.Fatal("the stream kept checking access as the role it had when it connected")
		case <-time.After(testPing):
		}
	}
}

func TestStreamTellsTheClientWhenItsRoleChanges(t *testing.T) {
	sessions := &fakeSessions{}
	sessions.valid.Store(true)
	sessions.role.Store(auth.RoleViewer)
	s := openEventStream(t, sessions, newFakeAccess())
	s.waitFor(t, "event: ready")

	sessions.role.Store(auth.RoleEditor)
	s.waitFor(t, "event: role.changed")
	s.waitFor(t, `data: {"dashboard_id":"`+dashboardID+`","role":"editor"}`)
}

type feed chan string

func (f feed) listen(ctx context.Context, _ string, _ func(), handle func(string)) error {
	for {
		select {
		case payload := <-f:
			handle(payload)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func openFedStream(t *testing.T, access Access) (*openStream, feed) {
	t.Helper()
	sessions := &fakeSessions{}
	sessions.valid.Store(true)
	notices := make(feed, 1)
	broker := events.NewBroker(notices.listen, slog.New(slog.DiscardHandler))
	go broker.Run(t.Context())
	t.Cleanup(broker.Close)
	s := followEvents(t, serveEvents(t, broker, time.Hour, sessions, access, eventViewer, nil), "")
	s.waitFor(t, "event: ready")
	return s, notices
}

const movedNotice = `{"type":"dashboard.moved","dashboard_id":"` + dashboardID + `","author":{"kind":"user","id":"u2","name":"Grace"}}`

func TestStreamDeliversAMoveToACallerWhoStillSeesTheDashboard(t *testing.T) {
	s, notices := openFedStream(t, newFakeAccess())
	notices <- movedNotice
	s.waitFor(t, "event: dashboard.moved")
}

func TestStreamEndsInsteadOfDeliveringAMoveTheCallerCanNoLongerSee(t *testing.T) {
	cases := []struct {
		name   string
		revoke func(*fakeAccess)
	}{
		{"view removed", func(a *fakeAccess) { a.view.Store(false) }},
		{"folder access removed", func(a *fakeAccess) { a.err.Store(&folders.ErrNoAccess) }},
		{"access lookup failed", func(a *fakeAccess) { a.failNext.Store(1) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			access := newFakeAccess()
			s, notices := openFedStream(t, access)
			tc.revoke(access)
			notices <- movedNotice
			timeout := time.After(waitTimeout)
			for {
				select {
				case line := <-s.lines:
					if strings.Contains(line, "dashboard.moved") || strings.Contains(line, "Grace") {
						t.Fatalf("the stream sent %q to a caller who lost the dashboard", line)
					}
				case <-s.done:
					return
				case <-timeout:
					t.Fatal("the stream stayed open after a move the caller can no longer see")
				}
			}
		})
	}
}
