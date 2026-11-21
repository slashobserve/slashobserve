package export

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
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
	limits []int
}

func (s *fakeSource) Since(_ context.Context, after filter.Position, limit int) ([]trail.Positioned, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.limits = append(s.limits, limit)
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
	exportedAt []time.Time
}

func (c *fakeCursor) Load(context.Context) (filter.Position, error) { return c.at, nil }

func (c *fakeCursor) Advance(_ context.Context, at filter.Position, occurredAt time.Time) error {
	if c.advanceErr != nil {
		return c.advanceErr
	}
	c.at = at
	c.exportedAt = append(c.exportedAt, occurredAt)
	c.advances++
	return nil
}

type counters struct {
	rejected int
	dropped  map[string]int
}

func (l *counters) ExportRejected(n int) { l.rejected += n }

func (l *counters) ExportDropped(reason string, n int) {
	if l.dropped == nil {
		l.dropped = map[string]int{}
	}
	l.dropped[reason] += n
}

type rig struct {
	source *fakeSource
	cursor *fakeCursor
	count  *counters
	posts  []string
	waits  []time.Duration
	post   func(body []byte) error
	answer Receipt
	logs   *bytes.Buffer
	stopAt func(r *rig) bool
	cancel context.CancelFunc
	x      *Exporter
}

func newRig(t *testing.T) (*rig, context.Context) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	r := &rig{source: &fakeSource{}, cursor: &fakeCursor{}, count: &counters{}, logs: &bytes.Buffer{}, cancel: cancel}
	r.x = New(r.source, r.cursor, func(_ context.Context, body []byte) (Receipt, error) {
		if r.post != nil {
			if err := r.post(body); err != nil {
				return Receipt{}, err
			}
		}
		r.posts = append(r.posts, string(body))
		return r.answer, nil
	}, r.count, slog.New(slog.NewTextHandler(r.logs, nil)))
	r.x.Wait = func(ctx context.Context, d time.Duration) error {
		r.waits = append(r.waits, d)
		if r.stopAt != nil && r.stopAt(r) {
			cancel()
		}
		return ctx.Err()
	}

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

func TestRejectedRecordsAreCountedLoggedAndPassed(t *testing.T) {
	r, ctx := newRig(t)
	r.source.add("a.one", "b.two")
	r.answer = Receipt{Rejected: 2, Message: "records too old"}
	r.stopAt = func(*rig) bool { return true }
	if err := r.x.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if r.cursor.at != (filter.Position{Xid: 102, Seq: 2}) {
		t.Fatalf("cursor = %+v, want it past the rejected records", r.cursor.at)
	}
	if r.count.rejected != 2 {
		t.Fatalf("rejected counted = %d, want 2", r.count.rejected)
	}
	logged := r.logs.String()
	if !strings.Contains(logged, "rejected=2") || !strings.Contains(logged, `"records too old"`) {
		t.Fatalf("log = %s", logged)
	}
}

func TestNothingIsCountedWhenTheReceiverRejectsNothing(t *testing.T) {
	r, ctx := newRig(t)
	r.source.add("a.one")
	r.stopAt = func(*rig) bool { return true }
	if err := r.x.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if r.count.rejected != 0 || strings.Contains(r.logs.String(), "rejected") {
		t.Fatalf("rejected %d, log %s", r.count.rejected, r.logs.String())
	}
}

func TestRejectedRecordsOfABatchWhoseCursorWriteFailedAreCountedWhenItIsSentAgain(t *testing.T) {
	r, ctx := newRig(t)
	r.source.add("a.one")
	r.answer = Receipt{Rejected: 1}
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
	if r.count.rejected != 1 {
		t.Fatalf("rejected counted = %d, want 1: counted only once the position moved", r.count.rejected)
	}
}

func (r *rig) refuseWhen(status int, bad func(body string) bool) {
	r.post = func(body []byte) error {
		if bad(string(body)) {
			return &StatusError{Status: status}
		}
		return nil
	}
}

func (r *rig) delivered() string {
	var seen []string
	for _, p := range r.posts {
		seen = append(seen, order(p)...)
	}
	return strings.Join(seen, ",")
}

func TestABatchTheReceiverCallsTooLargeIsHalvedUntilItIsAccepted(t *testing.T) {
	r, ctx := newRig(t)
	r.x.BatchSize = 8
	r.source.add("a.one", "b.two", "c.three", "d.four", "e.five")
	r.refuseWhen(http.StatusRequestEntityTooLarge, func(body string) bool { return len(order(body)) > 2 })
	r.stopAt = func(*rig) bool { return true }
	if err := r.x.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if got := r.delivered(); got != "a.one,b.two,c.three,d.four,e.five" {
		t.Fatalf("delivered %s", got)
	}
	if want := []int{8, 2, 2, 2, 2}; !slices.Equal(r.source.limits[:5], want) {
		t.Fatalf("batch sizes read = %v", r.source.limits)
	}
	if len(r.count.dropped) != 0 || len(r.waits) != 1 || r.waits[0] != PollEvery {
		t.Fatalf("dropped %v waits %v: halving is no failure", r.count.dropped, r.waits)
	}
}

func TestAnEventTheReceiverRefusesAloneIsSkippedAndCounted(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusRequestEntityTooLarge, http.StatusUnprocessableEntity} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			r, ctx := newRig(t)
			r.x.BatchSize = 5
			r.source.add("a.one", "b.two", "c.three", "d.four", "e.five")
			r.refuseWhen(status, func(body string) bool { return strings.Contains(body, `"c.three"`) })
			r.stopAt = func(*rig) bool { return true }
			if err := r.x.Run(ctx); err != nil {
				t.Fatal(err)
			}
			if got := r.delivered(); got != "a.one,b.two,d.four,e.five" {
				t.Fatalf("delivered %s", got)
			}
			if r.cursor.at != (filter.Position{Xid: 105, Seq: 5}) {
				t.Fatalf("cursor = %+v", r.cursor.at)
			}
			if r.count.dropped[DroppedRefused] != 1 || len(r.count.dropped) != 1 {
				t.Fatalf("dropped = %v", r.count.dropped)
			}
			logged := r.logs.String()
			if !strings.Contains(logged, "seq=3") || !strings.Contains(logged, "status="+strconv.Itoa(status)) {
				t.Fatalf("log = %s", logged)
			}
			if len(r.waits) != 1 {
				t.Fatalf("waits = %v: a skip waits for nothing", r.waits)
			}
		})
	}
}

func TestTheBatchSizeComesBackAfterAnEventIsSkipped(t *testing.T) {
	r, ctx := newRig(t)
	r.x.BatchSize = 4
	r.source.add("a.one", "b.two", "c.three", "d.four", "e.five", "f.six")
	r.refuseWhen(http.StatusBadRequest, func(body string) bool { return strings.Contains(body, `"a.one"`) })
	r.stopAt = func(*rig) bool { return true }
	if err := r.x.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if want := []int{4, 2, 1, 4}; !slices.Equal(r.source.limits[:4], want) {
		t.Fatalf("batch sizes read = %v, want %v first", r.source.limits, want)
	}
}

func TestOtherStatusesAreRetriedWithTheSameBatchAndNeverSkipped(t *testing.T) {
	for _, status := range []int{401, 403, 404, 408, 429, 500, 502, 503, 504} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			r, ctx := newRig(t)
			r.x.BatchSize = 4
			r.source.add("a.one", "b.two", "c.three")
			r.refuseWhen(status, func(string) bool { return true })
			r.stopAt = func(r *rig) bool { return len(r.waits) == 3 }
			if err := r.x.Run(ctx); err != nil {
				t.Fatal(err)
			}
			if r.cursor.advances != 0 || len(r.count.dropped) != 0 {
				t.Fatalf("advances %d dropped %v", r.cursor.advances, r.count.dropped)
			}
			for _, l := range r.source.limits {
				if l != 4 {
					t.Fatalf("batch sizes read = %v", r.source.limits)
				}
			}
			want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}
			if !slices.Equal(r.waits, want) {
				t.Fatalf("waits = %v, want %v", r.waits, want)
			}
		})
	}
}

func TestTheCursorStoresWhenTheLastExportedEventHappened(t *testing.T) {
	r, ctx := newRig(t)
	r.source.add("a.one", "b.two")
	r.stopAt = func(*rig) bool { return true }
	if err := r.x.Run(ctx); err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 11, 21, 10, 30, 15, 123456789, time.UTC)
	if len(r.cursor.exportedAt) != 1 || !r.cursor.exportedAt[0].Equal(want) {
		t.Fatalf("stored times = %v, want %v", r.cursor.exportedAt, want)
	}
}
