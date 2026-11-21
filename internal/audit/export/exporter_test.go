package export

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/audit/filter"
	"github.com/slashobserve/slashobserve/internal/audit/trail"
)

type fakeSource struct {
	mu     sync.Mutex
	events []trail.Positioned
	err    error
}

func (s *fakeSource) Since(_ context.Context, after filter.Position, limit int) ([]trail.Positioned, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return nil, s.err
	}
	var out []trail.Positioned
	for _, e := range s.events {
		if (e.At.Xid > after.Xid || (e.At.Xid == after.Xid && e.At.Seq > after.Seq)) && len(out) < limit {
			out = append(out, e)
		}
	}
	return out, nil
}

func (s *fakeSource) add(actions ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range actions {
		n := int64(len(s.events) + 1)
		e := fullEvent()
		e.Action, e.Seq = a, n
		s.events = append(s.events, trail.Positioned{Event: e, At: filter.Position{Xid: uint64(100 + n), Seq: n}})
	}
}

type fakeCursor struct {
	at         filter.Position
	advanceErr error
	advances   int
}

func (c *fakeCursor) Load(context.Context) (filter.Position, error) { return c.at, nil }

func (c *fakeCursor) Advance(_ context.Context, at filter.Position) error {
	if c.advanceErr != nil {
		return c.advanceErr
	}
	c.at = at
	c.advances++
	return nil
}

type lag struct{ values []time.Duration }

func (l *lag) ExportLag(d time.Duration) { l.values = append(l.values, d) }

type rig struct {
	source *fakeSource
	cursor *fakeCursor
	lag    *lag
	posts  []string
	waits  []time.Duration
	post   func(body []byte) error
	stopAt func(r *rig) bool
	cancel context.CancelFunc
	x      *Exporter
}

func newRig(t *testing.T) (*rig, context.Context) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	r := &rig{source: &fakeSource{}, cursor: &fakeCursor{}, lag: &lag{}, cancel: cancel}
	r.x = New(r.source, r.cursor, func(_ context.Context, body []byte) error {
		if r.post != nil {
			if err := r.post(body); err != nil {
				return err
			}
		}
		r.posts = append(r.posts, string(body))
		return nil
	}, r.lag, slog.New(slog.DiscardHandler))
	r.x.Wait = func(ctx context.Context, d time.Duration) error {
		r.waits = append(r.waits, d)
		if r.stopAt != nil && r.stopAt(r) {
			cancel()
		}
		return ctx.Err()
	}
	r.x.Now = func() time.Time { return time.Date(2026, 11, 21, 10, 31, 15, 0, time.UTC) }
	return r, ctx
}

func order(body string) []string {
	var out []string
	for _, a := range []string{"a.one", "b.two", "c.three", "d.four", "e.five"} {
		if i := strings.Index(body, `"eventName":"`+a+`"`); i >= 0 {
			out = append(out, a)
		}
	}
	return out
}

func TestRunPostsEachEventOnceInOrderAndMovesTheCursorAfterEachBatch(t *testing.T) {
	r, ctx := newRig(t)
	r.x.BatchSize = 2
	r.source.add("a.one", "b.two", "c.three", "d.four", "e.five")
	r.stopAt = func(*rig) bool { return true }
	if err := r.x.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(r.posts) != 3 {
		t.Fatalf("%d posts, want 3", len(r.posts))
	}
	var seen []string
	for _, p := range r.posts {
		seen = append(seen, order(p)...)
	}
	if strings.Join(seen, ",") != "a.one,b.two,c.three,d.four,e.five" {
		t.Fatalf("exported %v", seen)
	}
	if r.cursor.at != (filter.Position{Xid: 105, Seq: 5}) || r.cursor.advances != 3 {
		t.Fatalf("cursor %+v after %d advances", r.cursor.at, r.cursor.advances)
	}
	if len(r.waits) != 1 || r.waits[0] != PollEvery {
		t.Fatalf("waits = %v, want one poll", r.waits)
	}
}

func TestRunResumesAfterTheStoredCursor(t *testing.T) {
	r, ctx := newRig(t)
	r.source.add("a.one", "b.two", "c.three")
	r.cursor.at = filter.Position{Xid: 102, Seq: 2}
	r.stopAt = func(*rig) bool { return true }
	if err := r.x.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if len(r.posts) != 1 || strings.Join(order(r.posts[0]), ",") != "c.three" {
		t.Fatalf("posts = %v", r.posts)
	}
}

func TestAFailedPostLeavesTheCursorAndIsRetriedWithGrowingBackoff(t *testing.T) {
	r, ctx := newRig(t)
	r.source.add("a.one", "b.two")
	failures := 0
	r.post = func([]byte) error {
		if failures < 3 {
			failures++
			if r.cursor.at != (filter.Position{}) {
				t.Errorf("the cursor moved to %+v after a failed post", r.cursor.at)
			}
			return &StatusError{Status: 503}
		}
		return nil
	}
	r.stopAt = func(r *rig) bool { return len(r.posts) == 1 && len(r.waits) >= 4 }
	if err := r.x.Run(ctx); err != nil {
		t.Fatal(err)
	}
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, PollEvery}
	if len(r.waits) < 4 {
		t.Fatalf("waits = %v", r.waits)
	}
	for i, w := range want {
		if r.waits[i] != w {
			t.Fatalf("waits = %v, want %v first", r.waits, want)
		}
	}
	if r.cursor.at != (filter.Position{Xid: 102, Seq: 2}) {
		t.Fatalf("cursor = %+v after the retry succeeded", r.cursor.at)
	}
	if len(r.posts) != 1 || strings.Join(order(r.posts[0]), ",") != "a.one,b.two" {
		t.Fatalf("posts = %v", r.posts)
	}
}

func TestBackoffIsCappedAndHonoursRetryAfter(t *testing.T) {
	r, ctx := newRig(t)
	r.x.MaxBackoff = 8 * time.Second
	r.source.add("a.one")
	calls := 0
	r.post = func([]byte) error {
		calls++
		if calls == 6 {
			return &StatusError{Status: 429, RetryAfter: time.Hour}
		}
		return errors.New("refused")
	}
	r.stopAt = func(r *rig) bool { return len(r.waits) == 6 }
	if err := r.x.Run(ctx); err != nil {
		t.Fatal(err)
	}
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 8 * time.Second, 8 * time.Second}
	for i, w := range want {
		if r.waits[i] != w {
			t.Fatalf("waits = %v, want %v", r.waits, want)
		}
	}
}

func TestAFailedReadLeavesTheCursorAndIsRetried(t *testing.T) {
	r, ctx := newRig(t)
	r.source.err = errors.New("database down")
	r.stopAt = func(r *rig) bool { return len(r.waits) == 2 }
	if err := r.x.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if len(r.posts) != 0 || r.cursor.advances != 0 || r.waits[0] != time.Second || r.waits[1] != 2*time.Second {
		t.Fatalf("posts %d advances %d waits %v", len(r.posts), r.cursor.advances, r.waits)
	}
}

func TestACursorWriteFailureSendsTheBatchAgain(t *testing.T) {
	r, ctx := newRig(t)
	r.source.add("a.one")
	r.cursor.advanceErr = errors.New("write failed")
	r.stopAt = func(r *rig) bool {
		if len(r.waits) == 1 {
			r.cursor.advanceErr = nil
		}
		return len(r.posts) == 2
	}
	if err := r.x.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if len(r.posts) < 2 || r.posts[0] != r.posts[1] {
		t.Fatalf("the batch was not delivered again: %d posts", len(r.posts))
	}
	if r.cursor.at != (filter.Position{Xid: 101, Seq: 1}) {
		t.Fatalf("cursor = %+v", r.cursor.at)
	}
}

func TestRunReportsTheLagOfTheOldestWaitingEventAndZeroWhenCaughtUp(t *testing.T) {
	r, ctx := newRig(t)
	r.source.add("a.one")
	r.stopAt = func(*rig) bool { return true }
	if err := r.x.Run(ctx); err != nil {
		t.Fatal(err)
	}
	want := []time.Duration{time.Minute - 123456789*time.Nanosecond, 0, 0}
	if len(r.lag.values) != len(want) {
		t.Fatalf("lag = %v", r.lag.values)
	}
	for i, w := range want {
		if r.lag.values[i] != w {
			t.Fatalf("lag = %v, want %v", r.lag.values, want)
		}
	}
}

func TestRunEndsWithNoErrorWhenItsContextEnds(t *testing.T) {
	r, ctx := newRig(t)
	r.cancel()
	if err := r.x.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestRunIgnoresAFailureCausedByItsOwnCancellation(t *testing.T) {
	r, ctx := newRig(t)
	r.source.add("a.one")
	r.post = func([]byte) error {
		r.cancel()
		return context.Canceled
	}
	if err := r.x.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if r.cursor.advances != 0 {
		t.Fatal("the cursor moved after an interrupted post")
	}
}
