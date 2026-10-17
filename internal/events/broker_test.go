package events

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

const (
	waitTimeout = 5 * time.Second
	quietPeriod = 20 * time.Millisecond
)

type fakeCall struct {
	handle func(string)
	fail   chan error
}

type fakeListener struct {
	calls chan *fakeCall
}

func newFakeListener() *fakeListener {
	return &fakeListener{calls: make(chan *fakeCall, 16)}
}

func (l *fakeListener) listen(ctx context.Context, channel string, onListening func(), handle func(string)) error {
	call := &fakeCall{handle: handle, fail: make(chan error, 1)}
	onListening()
	l.calls <- call
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-call.fail:
		return err
	}
}

func (l *fakeListener) nextCall(t *testing.T) *fakeCall {
	t.Helper()
	select {
	case call := <-l.calls:
		return call
	case <-time.After(waitTimeout):
		t.Fatal("listener was not started")
		return nil
	}
}

type harness struct {
	broker *Broker
	l      *fakeListener
	done   chan struct{}
	cancel context.CancelFunc
	ctx    context.Context
	ran    bool
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	l := newFakeListener()
	b := NewBroker(l.listen, slog.New(slog.NewTextHandler(io.Discard, nil)))
	b.minBackoff = time.Millisecond
	b.maxBackoff = 2 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	h := &harness{broker: b, l: l, done: make(chan struct{}), cancel: cancel, ctx: ctx}
	t.Cleanup(func() {
		cancel()
		b.Close()
		if !h.ran {
			return
		}
		select {
		case <-h.done:
		case <-time.After(waitTimeout):
			t.Error("Run did not exit")
		}
	})
	return h
}

func (h *harness) run() {
	h.ran = true
	go func() {
		defer close(h.done)
		h.broker.Run(h.ctx)
	}()
}

func (h *harness) subscribe(t *testing.T, dashboardID string) *Subscription {
	t.Helper()
	s, err := h.broker.Subscribe(dashboardID)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func (h *harness) listening(t *testing.T, dashboardIDs ...string) (*fakeCall, []*Subscription) {
	t.Helper()
	subs := make([]*Subscription, len(dashboardIDs))
	for i, id := range dashboardIDs {
		subs[i] = h.subscribe(t, id)
	}
	h.run()
	call := h.l.nextCall(t)
	for _, s := range subs {
		expectResync(t, s)
	}
	return call, subs
}

func recv(t *testing.T, s *Subscription) Event {
	t.Helper()
	select {
	case ev := <-s.Events():
		return ev
	case <-time.After(waitTimeout):
		t.Fatal("no event received")
		return Event{}
	}
}

func expectResync(t *testing.T, s *Subscription) {
	t.Helper()
	if ev := recv(t, s); ev.Type != TypeResync || ev.DashboardID != s.dashboardID {
		t.Fatalf("expected a resync for %s, got %+v", s.dashboardID, ev)
	}
}

func expectNone(t *testing.T, s *Subscription) {
	t.Helper()
	select {
	case ev := <-s.Events():
		t.Fatalf("unexpected event %+v", ev)
	case <-time.After(quietPeriod):
	}
}

func expectDone(t *testing.T, s *Subscription) {
	t.Helper()
	select {
	case <-s.Done():
	case <-time.After(waitTimeout):
		t.Fatal("subscription was not ended")
	}
}

func expectNotDone(t *testing.T, s *Subscription) {
	t.Helper()
	select {
	case <-s.Done():
		t.Fatal("subscription ended unexpectedly")
	default:
	}
}

func versionCreated(dashboardID string, version int) string {
	return fmt.Sprintf(`{"type":"version.created","dashboard_id":%q,"version":%d}`, dashboardID, version)
}

func TestBrokerDeliversOnlyToSubscribersOfTheDashboard(t *testing.T) {
	h := newHarness(t)
	call, subs := h.listening(t, "d1", "d1", "d2")

	call.handle(versionCreated("d1", 2))

	for _, s := range subs[:2] {
		ev := recv(t, s)
		if ev.Type != TypeVersionCreated || ev.DashboardID != "d1" || ev.Version != 2 {
			t.Fatalf("unexpected event %+v", ev)
		}
	}
	expectNone(t, subs[2])
}

func TestBrokerDropsSlowSubscriberOnBufferOverflow(t *testing.T) {
	h := newHarness(t)
	call, subs := h.listening(t, "d1", "d1")
	slow, fast := subs[0], subs[1]

	for i := range subscriptionBuffer {
		call.handle(versionCreated("d1", i))
		recv(t, fast)
	}
	expectNotDone(t, slow)
	if got := len(slow.Events()); got != subscriptionBuffer {
		t.Fatalf("slow buffer holds %d events, want %d", got, subscriptionBuffer)
	}

	call.handle(versionCreated("d1", 99))

	expectDone(t, slow)
	expectNotDone(t, fast)
	if ev := recv(t, fast); ev.Version != 99 {
		t.Fatalf("fast subscriber got %+v", ev)
	}
	call.handle(versionCreated("d1", 100))
	if ev := recv(t, fast); ev.Version != 100 {
		t.Fatalf("fast subscriber got %+v", ev)
	}
	expectDone(t, slow)
}

func TestBrokerSkipsMalformedPayloads(t *testing.T) {
	h := newHarness(t)
	call, subs := h.listening(t, "d1")
	s := subs[0]

	for _, payload := range []string{"garbage", "[]", `{"type":"version.created"}`, `{"dashboard_id":"d1"}`, `{"type":"","dashboard_id":"d1"}`} {
		call.handle(payload)
	}
	expectNone(t, s)
	expectNotDone(t, s)

	call.handle(versionCreated("d1", 4))
	if ev := recv(t, s); ev.Version != 4 {
		t.Fatalf("got %+v", ev)
	}
}

func TestBrokerResyncsOnEverySuccessfulListen(t *testing.T) {
	h := newHarness(t)
	call, subs := h.listening(t, "d1", "d2")
	expectNone(t, subs[0])

	call.fail <- errors.New("connection lost")
	h.l.nextCall(t)
	for _, s := range subs {
		expectResync(t, s)
	}
	expectNone(t, subs[0])
}

func TestBrokerDoesNotResyncSubscribersThatJoinAfterListening(t *testing.T) {
	h := newHarness(t)
	h.listening(t)

	late := h.subscribe(t, "d1")

	expectNone(t, late)
}

func TestBrokerCloseEndsSubscriptionsAndRun(t *testing.T) {
	h := newHarness(t)
	_, subs := h.listening(t, "d1", "d2")

	h.broker.Close()
	h.broker.Close()

	for _, s := range subs {
		expectDone(t, s)
		s.Close()
		s.Close()
	}
	select {
	case <-h.done:
	case <-time.After(waitTimeout):
		t.Fatal("Run did not exit after Close")
	}
}

func TestBrokerSubscribeAfterCloseFails(t *testing.T) {
	h := newHarness(t)
	h.broker.Close()

	s, err := h.broker.Subscribe("d1")

	if !errors.Is(err, ErrClosed) || s != nil {
		t.Fatalf("got %v, %v", s, err)
	}
}

func TestBrokerConcurrentSubscribeAndClose(t *testing.T) {
	h := newHarness(t)
	h.listening(t)

	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := h.broker.Subscribe("d1")
			if err != nil {
				if !errors.Is(err, ErrClosed) {
					t.Errorf("unexpected error %v", err)
				}
				return
			}
			s.Close()
			s.Close()
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			h.broker.Close()
		}()
	}
	wg.Wait()

	if _, err := h.broker.Subscribe("d1"); !errors.Is(err, ErrClosed) {
		t.Fatalf("got %v", err)
	}
}
