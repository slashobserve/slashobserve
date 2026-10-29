package stream

import (
	"bufio"
	"context"
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

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/events"
	"github.com/slashobserve/slashobserve/internal/logtest"
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
}

func (f *fakeSessions) Valid(*http.Request) (bool, error) {
	f.calls.Add(1)
	if f.failNext.Load() > 0 {
		f.failNext.Add(-1)
		return false, errors.New("database unavailable")
	}
	return f.valid.Load(), nil
}

type openStream struct {
	lines chan string
	done  chan struct{}
}

func openEventStream(t *testing.T, sessions Sessions) *openStream {
	t.Helper()
	gin.SetMode(gin.TestMode)
	broker := events.NewBroker(nil, slog.New(slog.DiscardHandler))
	t.Cleanup(broker.Close)
	h := NewHandler(broker, fakeDashboards{}, sessions, slog.New(slog.DiscardHandler))
	h.pingInterval = testPing
	engine := gin.New()
	api := engine.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), auth.Principal{Kind: auth.KindUser, ID: "u1"}))
	})
	h.Register(api)
	srv := httptest.NewServer(engine)
	t.Cleanup(srv.Close)
	t.Cleanup(srv.CloseClientConnections)

	resp, err := http.Get(srv.URL + "/api/v1/dashboards/" + dashboardID + "/events")
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
	s := openEventStream(t, sessions)
	s.waitFor(t, "event: ready")
	s.waitFor(t, ": ping")
	s.waitFor(t, ": ping")

	sessions.valid.Store(false)
	if !s.closedWithin(waitTimeout) {
		t.Fatal("the stream outlived its session")
	}
}

func TestStreamSurvivesASingleCheckFailure(t *testing.T) {
	sessions := &fakeSessions{}
	sessions.valid.Store(true)
	s := openEventStream(t, sessions)
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
	s := openEventStream(t, sessions)
	s.waitFor(t, "event: ready")

	sessions.failNext.Store(1000)
	if !s.closedWithin(waitTimeout) {
		t.Fatal("the stream stayed open while its credentials could not be checked")
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
			h := NewHandler(broker, failingDashboards{tc.err}, &fakeSessions{}, logger)
			engine := gin.New()
			h.Register(engine.Group("/api/v1"))
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
