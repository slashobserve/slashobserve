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
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/events"
	"github.com/slashobserve/slashobserve/internal/folders"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

const (
	quietPeriod = 150 * time.Millisecond
	viewerID    = "33333333-3333-4333-8333-333333333333"
	adminID     = "55555555-5555-4555-8555-555555555555"
)

var (
	viewer = auth.Principal{Kind: auth.KindUser, ID: viewerID, Name: "Vic", Role: auth.RoleViewer}
	admin  = auth.Principal{Kind: auth.KindUser, ID: adminID, Name: "Root", Role: auth.RoleAdmin}
	author = actor.Actor{Kind: auth.KindUser, ID: "11111111-1111-4111-8111-111111111111", Name: "Eve"}
)

type notifier struct {
	handle atomic.Pointer[func(string)]
	ready  chan struct{}
}

func (n *notifier) listen(ctx context.Context, _ string, onListening func(), handle func(string)) error {
	n.handle.Store(&handle)
	onListening()
	close(n.ready)
	<-ctx.Done()
	return ctx.Err()
}

func (n *notifier) wake(seq int64) {
	(*n.handle.Load())(fmt.Sprintf(`{"seq":%d}`, seq))
}

type fakePrincipals struct {
	current atomic.Pointer[auth.Principal]
	err     atomic.Pointer[error]
	calls   atomic.Int32
}

func (f *fakePrincipals) RequestPrincipal(*http.Request) (auth.Principal, bool, error) {
	f.calls.Add(1)
	if err := f.err.Load(); err != nil {
		return auth.Principal{}, false, *err
	}
	p := f.current.Load()
	if p == nil {
		return auth.Principal{}, false, nil
	}
	return *p, true, nil
}

func (f *fakePrincipals) becomes(p auth.Principal) { f.current.Store(&p) }

func (f *fakePrincipals) waitForChecks(t *testing.T, n int32) {
	t.Helper()
	target := f.calls.Load() + n
	deadline := time.Now().Add(waitTimeout)
	for f.calls.Load() < target {
		if time.Now().After(deadline) {
			t.Fatalf("the stream did not re-read its principal %d times", n)
		}
		time.Sleep(time.Millisecond)
	}
}

type activityRig struct {
	db         *dbtest.Database
	broker     *events.Broker
	notifier   *notifier
	principals *fakePrincipals
	handler    *ActivityHandler
	server     *httptest.Server
	caller     atomic.Pointer[auth.Principal]
}

func newActivityRig(t *testing.T, caller auth.Principal) *activityRig {
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
	select {
	case <-n.ready:
	case <-time.After(waitTimeout):
		t.Fatal("the broker did not start listening")
	}
	r := &activityRig{db: d, broker: broker, notifier: n, principals: &fakePrincipals{}}
	r.caller.Store(&caller)
	r.principals.becomes(caller)
	r.handler = NewActivityHandler(broker, activity.NewStore(d.DB, folders.NewAccess(d.DB)), r.principals, slog.New(slog.DiscardHandler))
	r.handler.pingInterval = time.Hour
	r.handler.retryInterval = 20 * time.Millisecond
	engine := gin.New()
	api := engine.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), *r.caller.Load()))
	})
	r.handler.Register(auth.NewRoutes(api))
	r.server = httptest.NewServer(engine)
	t.Cleanup(r.server.Close)
	t.Cleanup(r.server.CloseClientConnections)
	return r
}

var targets atomic.Int64

func nextTarget() pgtype.UUID {
	id, err := db.ParseUUID(fmt.Sprintf("00000000-0000-4000-8000-%012d", targets.Add(1)))
	if err != nil {
		panic(err)
	}
	return id
}

func visibleToEveryone() activity.Entry {
	return activity.Entry{Type: activity.TypeAnnotationCreated, Owner: activity.General()}
}

func visibleToActorAndAdmins() activity.Entry {
	return activity.Entry{Type: activity.TypeFindingCreated}
}

func appendIn(ctx context.Context, q *dbsqlc.Queries, e activity.Entry) (int64, error) {
	e.TargetID = nextTarget()
	e.Actor = author
	return activity.Append(ctx, q, e, nil)
}

func (r *activityRig) append(t *testing.T, e activity.Entry) int64 {
	t.Helper()
	var seq int64
	err := r.db.InTx(t.Context(), func(q *dbsqlc.Queries) error {
		var err error
		seq, err = appendIn(t.Context(), q, e)
		return err
	})
	if err != nil {
		t.Fatalf("append %s: %v", e.Type, err)
	}
	return seq
}

type heldAppend struct {
	seq     int64
	release chan struct{}
	done    chan error
}

func (r *activityRig) appendHeld(t *testing.T, e activity.Entry) *heldAppend {
	t.Helper()
	h := &heldAppend{release: make(chan struct{}), done: make(chan error, 1)}
	appended := make(chan struct{})
	go func() {
		h.done <- r.db.InTx(context.Background(), func(q *dbsqlc.Queries) error {
			seq, err := appendIn(context.Background(), q, e)
			if err != nil {
				return err
			}
			h.seq = seq
			close(appended)
			<-h.release
			return nil
		})
	}()
	select {
	case <-appended:
	case err := <-h.done:
		t.Fatalf("held append failed: %v", err)
	case <-time.After(waitTimeout):
		t.Fatal("held append did not run")
	}
	return h
}

func (h *heldAppend) commit(t *testing.T) {
	t.Helper()
	close(h.release)
	if err := <-h.done; err != nil {
		t.Fatalf("held append: %v", err)
	}
}

type frame struct {
	id, event, data string
}

type activityStream struct {
	lines chan string
	done  chan struct{}
}

func (r *activityRig) request(t *testing.T, query, lastEventID string) *http.Request {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.server.URL+"/api/v1/activity/stream?"+query, nil)
	if err != nil {
		t.Fatal(err)
	}
	if lastEventID != "" {
		req.Header.Set("Last-Event-ID", lastEventID)
	}
	return req
}

func (r *activityRig) refused(t *testing.T, query, lastEventID string) (status int, code string) {
	t.Helper()
	resp, err := http.DefaultClient.Do(r.request(t, query, lastEventID))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var body struct{ Code string }
	_ = json.NewDecoder(resp.Body).Decode(&body)
	return resp.StatusCode, body.Code
}

func (r *activityRig) open(t *testing.T, query, lastEventID string) *activityStream {
	t.Helper()
	resp, err := http.DefaultClient.Do(r.request(t, query, lastEventID))
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
	return s
}

func (s *activityStream) nextFrame(t *testing.T) frame {
	t.Helper()
	var f frame
	timeout := time.After(waitTimeout)
	for {
		select {
		case line := <-s.lines:
			switch {
			case line == "":
				if f.event != "" {
					return f
				}
				f = frame{}
			case strings.HasPrefix(line, "id: "):
				f.id = strings.TrimPrefix(line, "id: ")
			case strings.HasPrefix(line, "event: "):
				f.event = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				f.data = strings.TrimPrefix(line, "data: ")
			}
		case <-s.done:
			t.Fatal("stream ended before the next frame")
		case <-timeout:
			t.Fatalf("no frame within %s", waitTimeout)
		}
	}
}

func (s *activityStream) expectActivity(t *testing.T, seq int64) activity.EventJSON {
	t.Helper()
	f := s.nextFrame(t)
	if f.event != "activity" || f.id != strconv.FormatInt(seq, 10) {
		t.Fatalf("frame %+v, want activity %d", f, seq)
	}
	var ev activity.EventJSON
	if err := json.Unmarshal([]byte(f.data), &ev); err != nil {
		t.Fatalf("data %q: %v", f.data, err)
	}
	if ev.Seq != seq {
		t.Fatalf("data seq %d, want %d", ev.Seq, seq)
	}
	return ev
}

func (s *activityStream) expectEvent(t *testing.T, name string) {
	t.Helper()
	if f := s.nextFrame(t); f.event != name || f.data != "{}" {
		t.Fatalf("frame %+v, want %s with {}", f, name)
	}
}

func (s *activityStream) expectNoActivity(t *testing.T) {
	t.Helper()
	timer := time.After(quietPeriod)
	for {
		select {
		case line := <-s.lines:
			if strings.HasPrefix(line, "event: activity") {
				t.Fatalf("unexpected frame %q", line)
			}
		case <-s.done:
			t.Fatal("stream ended while it should be quiet")
		case <-timer:
			return
		}
	}
}

func (s *activityStream) closedWithin(d time.Duration) bool {
	select {
	case <-s.done:
		return true
	case <-time.After(d):
		return false
	}
}

func TestActivityStreamSendsAnEventAfterAnAppendAndAWake(t *testing.T) {
	r := newActivityRig(t, viewer)
	r.append(t, visibleToEveryone())
	s := r.open(t, "", "")
	s.expectEvent(t, "ready")

	seq := r.append(t, visibleToEveryone())
	r.notifier.wake(seq)

	ev := s.expectActivity(t, seq)
	if ev.Type != activity.TypeAnnotationCreated || ev.Target.Kind != "annotation" || ev.Actor.Label != "Eve" {
		t.Fatalf("event = %+v", ev)
	}
}

func TestActivityStreamResumesAfterLastEventID(t *testing.T) {
	r := newActivityRig(t, viewer)
	first := r.append(t, visibleToEveryone())
	second := r.append(t, visibleToEveryone())
	third := r.append(t, visibleToEveryone())

	s := r.open(t, "", strconv.FormatInt(first, 10))

	s.expectEvent(t, "ready")
	s.expectActivity(t, second)
	s.expectActivity(t, third)
	s.expectNoActivity(t)
}

func TestActivityStreamStartsAtTheBeginningForALastEventIDOfZero(t *testing.T) {
	r := newActivityRig(t, viewer)
	first := r.append(t, visibleToEveryone())
	second := r.append(t, visibleToEveryone())

	s := r.open(t, "", "0")

	s.expectEvent(t, "ready")
	s.expectActivity(t, first)
	s.expectActivity(t, second)
	s.expectNoActivity(t)
}

func TestActivityStreamSendsResetForAnUnknownLastEventID(t *testing.T) {
	r := newActivityRig(t, viewer)
	r.append(t, visibleToEveryone())
	s := r.open(t, "", "999999")

	s.expectEvent(t, "ready")
	s.expectEvent(t, "reset")
	s.expectNoActivity(t)
	seq := r.append(t, visibleToEveryone())
	r.notifier.wake(seq)
	s.expectActivity(t, seq)
}

func TestActivityStreamRefusesBadRequests(t *testing.T) {
	tests := []struct {
		name        string
		query       string
		lastEventID string
	}{
		{"a Last-Event-ID that is not a number", "", "abc"},
		{"a negative Last-Event-ID", "", "-1"},
		{"an unknown type", "type=dashboard.renamed", ""},
		{"a dashboard that is not a UUID", "dashboard_id=checkout", ""},
	}
	r := newActivityRig(t, viewer)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, code := r.refused(t, tt.query, tt.lastEventID)

			if status != http.StatusBadRequest || code != "invalid_request" {
				t.Fatalf("status %d code %q, want 400 invalid_request", status, code)
			}
		})
	}
}

func TestActivityStreamFiltersByType(t *testing.T) {
	r := newActivityRig(t, admin)
	s := r.open(t, url.Values{"type": {activity.TypeFindingCreated}}.Encode(), "")
	s.expectEvent(t, "ready")

	r.append(t, visibleToEveryone())
	wanted := r.append(t, visibleToActorAndAdmins())
	r.notifier.wake(wanted)

	s.expectActivity(t, wanted)
	s.expectNoActivity(t)
}

func TestActivityStreamDeliversAHeldRowAfterTheRetry(t *testing.T) {
	r := newActivityRig(t, viewer)
	s := r.open(t, "", "")
	s.expectEvent(t, "ready")

	held := r.appendHeld(t, visibleToEveryone())
	later := r.append(t, visibleToEveryone())
	r.notifier.wake(later)
	s.expectNoActivity(t)

	held.commit(t)

	s.expectActivity(t, held.seq)
	s.expectActivity(t, later)
}

func TestActivityStreamHidesNoFolderRowsAfterAnAdminBecomesAViewer(t *testing.T) {
	r := newActivityRig(t, admin)
	r.handler.pingInterval = 10 * time.Millisecond
	s := r.open(t, "", "")
	s.expectEvent(t, "ready")
	seen := r.append(t, visibleToActorAndAdmins())
	r.notifier.wake(seen)
	s.expectActivity(t, seen)

	r.principals.becomes(auth.Principal{Kind: auth.KindUser, ID: adminID, Name: "Root", Role: auth.RoleViewer})
	r.principals.waitForChecks(t, 2)
	hidden := r.append(t, visibleToActorAndAdmins())
	r.notifier.wake(hidden)
	s.expectNoActivity(t)

	shown := r.append(t, visibleToEveryone())
	r.notifier.wake(shown)
	s.expectActivity(t, shown)
}

func TestActivityStreamEndsAtThePingWhenTheCallerIsGone(t *testing.T) {
	forbidden := auth.Principal{Kind: auth.KindUser, ID: "not-a-uuid", Role: auth.RoleViewer}
	tests := []struct {
		name   string
		revoke func(*fakePrincipals)
	}{
		{"credential revoked", func(f *fakePrincipals) { f.current.Store(nil) }},
		{"principal without a usable id", func(f *fakePrincipals) { f.becomes(forbidden) }},
		{"credentials cannot be checked repeatedly", func(f *fakePrincipals) {
			err := errors.New("database unavailable")
			f.err.Store(&err)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newActivityRig(t, viewer)
			r.handler.pingInterval = 10 * time.Millisecond
			s := r.open(t, "", "")
			s.expectEvent(t, "ready")

			tt.revoke(r.principals)

			if !s.closedWithin(waitTimeout) {
				t.Fatal("the stream outlived the caller")
			}
		})
	}
}

func TestActivityStreamSurvivesASingleCheckFailure(t *testing.T) {
	r := newActivityRig(t, viewer)
	r.handler.pingInterval = 10 * time.Millisecond
	s := r.open(t, "", "")
	s.expectEvent(t, "ready")

	err := errors.New("database unavailable")
	r.principals.err.Store(&err)
	r.principals.waitForChecks(t, 1)
	r.principals.err.Store(nil)

	if s.closedWithin(10 * quietPeriod / 3) {
		t.Fatal("one failed check must not close the stream")
	}
}

func TestActivityStreamAnswers429BeyondTheLimitPerPrincipal(t *testing.T) {
	r := newActivityRig(t, viewer)
	for range maxActivityStreamsPerUser {
		r.open(t, "", "").expectEvent(t, "ready")
	}

	status, code := r.refused(t, "", "")

	if status != http.StatusTooManyRequests || code != "too_many_streams" {
		t.Fatalf("status %d code %q, want 429 too_many_streams", status, code)
	}
}

func TestActivityStreamAnswers503WhenTheServerIsShuttingDown(t *testing.T) {
	r := newActivityRig(t, viewer)
	r.broker.Close()

	status, code := r.refused(t, "", "")

	if status != http.StatusServiceUnavailable || code != "shutting_down" {
		t.Fatalf("status %d code %q, want 503 shutting_down", status, code)
	}
}

func TestSlotsRefuseBeyondTheLimitsAndFreeOnRelease(t *testing.T) {
	s := newSlots(2, 3)

	for _, key := range []string{"a", "a", "b"} {
		if status, _ := s.acquire(key); status != 0 {
			t.Fatalf("acquire %s refused with %d", key, status)
		}
	}
	if status, code := s.acquire("a"); status != http.StatusTooManyRequests || code != "too_many_streams" {
		t.Errorf("third slot of one principal: %d %q, want 429", status, code)
	}
	if status, code := s.acquire("c"); status != http.StatusServiceUnavailable || code != "too_many_streams" {
		t.Errorf("fourth slot of the process: %d %q, want 503", status, code)
	}
	s.release("a")
	if status, _ := s.acquire("c"); status != 0 {
		t.Errorf("acquire after release refused with %d", status)
	}
}
