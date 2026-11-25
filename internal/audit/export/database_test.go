package export

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/slashobserve/slashobserve/internal/audit/trail"
	"github.com/slashobserve/slashobserve/internal/audit/trail/trailtest"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

type otlpReceiver struct {
	mu      sync.Mutex
	actions []string
	status  int
	server  *httptest.Server
}

func newOTLPReceiver(t *testing.T) *otlpReceiver {
	t.Helper()
	r := &otlpReceiver{status: http.StatusOK}
	r.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.status/100 == 2 {
			var parsed struct {
				ResourceLogs []struct {
					ScopeLogs []struct {
						LogRecords []struct {
							EventName string `json:"eventName"`
						} `json:"logRecords"`
					} `json:"scopeLogs"`
				} `json:"resourceLogs"`
			}
			if err := json.Unmarshal(body, &parsed); err != nil {
				t.Errorf("body is not OTLP JSON: %v", err)
			}
			for _, rl := range parsed.ResourceLogs {
				for _, sl := range rl.ScopeLogs {
					for _, rec := range sl.LogRecords {
						r.actions = append(r.actions, rec.EventName)
					}
				}
			}
		}
		w.WriteHeader(r.status)
	}))
	t.Cleanup(r.server.Close)
	return r
}

func (r *otlpReceiver) received() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.actions)
}

func (r *otlpReceiver) answer(status int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status = status
}

type databaseRig struct {
	db   *dbtest.Database
	base time.Time
	n    int
}

func newDatabaseRig(t *testing.T) *databaseRig {
	t.Helper()
	return &databaseRig{db: dbtest.New(t), base: time.Now().UTC().Truncate(time.Hour).Add(2 * time.Hour)}
}

func (r *databaseRig) insert(t *testing.T, actions ...string) {
	t.Helper()
	for _, a := range actions {
		r.n++
		trailtest.Insert(t, r.db, trailtest.Event{At: r.base.Add(time.Duration(r.n) * time.Minute), Action: a})
	}
}

func (r *databaseRig) runUntilIdle(t *testing.T, endpoint string) *counters {
	t.Helper()
	u, err := url.Parse(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	counted := &counters{}
	x := New(trail.NewReader(r.db.DB, slog.New(slog.DiscardHandler)), NewCursor(r.db.Queries()), NewSender(u, nil).Send, counted, slog.New(slog.DiscardHandler))
	x.Wait = func(context.Context, time.Duration) error {
		cancel()
		return context.Canceled
	}
	if err := x.Run(ctx); err != nil {
		t.Fatal(err)
	}
	return counted
}

func TestEventsAreExportedInOrderOnce(t *testing.T) {
	r := newDatabaseRig(t)
	rx := newOTLPReceiver(t)
	r.insert(t, "a.one", "b.two", "c.three")
	r.runUntilIdle(t, rx.server.URL)
	r.runUntilIdle(t, rx.server.URL)
	want := []string{"a.one", "b.two", "c.three"}
	if got := rx.received(); !slices.Equal(got, want) {
		t.Fatalf("received %v, want %v", got, want)
	}
}

func TestAFailedPostLeavesTheCursorAndTheNextRunSendsTheSameEvents(t *testing.T) {
	r := newDatabaseRig(t)
	rx := newOTLPReceiver(t)
	r.insert(t, "a.one", "b.two")
	rx.answer(http.StatusServiceUnavailable)
	r.runUntilIdle(t, rx.server.URL)
	if got := rx.received(); len(got) != 0 {
		t.Fatalf("received %v from a failing receiver", got)
	}
	at, err := NewCursor(r.db.Queries()).Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if at.Exists {
		t.Fatalf("cursor moved to %+v after a failed post", at.At)
	}
	rx.answer(http.StatusOK)
	r.runUntilIdle(t, rx.server.URL)
	want := []string{"a.one", "b.two"}
	if got := rx.received(); !slices.Equal(got, want) {
		t.Fatalf("received %v, want %v", got, want)
	}
}

func TestARestartResumesAfterTheLastExportedEvent(t *testing.T) {
	r := newDatabaseRig(t)
	rx := newOTLPReceiver(t)
	r.insert(t, "a.one", "b.two")
	r.runUntilIdle(t, rx.server.URL)
	r.insert(t, "c.three", "d.four")
	r.runUntilIdle(t, rx.server.URL)
	want := []string{"a.one", "b.two", "c.three", "d.four"}
	if got := rx.received(); !slices.Equal(got, want) {
		t.Fatalf("received %v, want %v", got, want)
	}
}

func TestTheCursorNeverMovesBackward(t *testing.T) {
	r := newDatabaseRig(t)
	rx := newOTLPReceiver(t)
	r.insert(t, "a.one", "b.two")
	r.runUntilIdle(t, rx.server.URL)
	c := NewCursor(r.db.Queries())
	last, err := c.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	earlier := last.At
	earlier.Seq--
	if err := c.Advance(t.Context(), earlier, r.base); err != nil {
		t.Fatal(err)
	}
	after, err := c.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if after != last {
		t.Fatalf("cursor went from %+v to %+v", last, after)
	}
}

func TestLagIsTheGapBetweenTheNewestEventAndTheLastExportedOne(t *testing.T) {
	r := newDatabaseRig(t)
	rx := newOTLPReceiver(t)
	c := NewCursor(r.db.Queries())
	if lag, err := c.Lag(t.Context()); err != nil || lag != 0 {
		t.Fatalf("lag with no events = %v, %v", lag, err)
	}
	r.insert(t, "a.one", "b.two", "c.three")
	if lag, err := c.Lag(t.Context()); err != nil || lag != 2*time.Minute {
		t.Fatalf("lag before the first export = %v, %v, want the span of the stored events", lag, err)
	}
	rx.answer(http.StatusServiceUnavailable)
	r.runUntilIdle(t, rx.server.URL)
	if lag, _ := c.Lag(t.Context()); lag != 2*time.Minute {
		t.Fatalf("lag after a failed export = %v", lag)
	}
	rx.answer(http.StatusOK)
	r.runUntilIdle(t, rx.server.URL)
	if lag, err := c.Lag(t.Context()); err != nil || lag != 0 {
		t.Fatalf("lag when caught up = %v, %v", lag, err)
	}
	r.insert(t, "d.four", "e.five")
	if lag, _ := c.Lag(t.Context()); lag != 2*time.Minute {
		t.Fatalf("lag with two events waiting = %v, want 2m0s", lag)
	}
}

func TestLagShowsEventsAnOpenTransactionHidesFromTheExport(t *testing.T) {
	r := newDatabaseRig(t)
	rx := newOTLPReceiver(t)
	r.insert(t, "a.one")
	r.runUntilIdle(t, rx.server.URL)
	conn, err := pgx.Connect(t.Context(), r.db.URL())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(context.WithoutCancel(t.Context())) }()
	tx, err := conn.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(t.Context())) }()
	if _, err := tx.Exec(t.Context(), "SELECT pg_current_xact_id()"); err != nil {
		t.Fatal(err)
	}
	r.insert(t, "b.two", "c.three")
	r.runUntilIdle(t, rx.server.URL)
	if got := rx.received(); !slices.Equal(got, []string{"a.one"}) {
		t.Fatalf("received %v: the open transaction should hide the later events from the export", got)
	}
	lag, err := NewCursor(r.db.Queries()).Lag(t.Context())
	if err != nil || lag != 2*time.Minute {
		t.Fatalf("lag = %v, %v, want 2m0s", lag, err)
	}
}

func (r *databaseRig) deleteAsRetentionWould(t *testing.T, actions ...string) {
	t.Helper()
	for _, a := range actions {
		r.db.Exec(t, `DO $$ BEGIN SET LOCAL session_replication_role = replica; DELETE FROM audit_events WHERE action = '`+a+`'; END $$`)
	}
}

func (r *databaseRig) setPosition(t *testing.T, action string, xid int) {
	t.Helper()
	r.db.Exec(t, `DO $$ BEGIN SET LOCAL session_replication_role = replica; UPDATE audit_events SET xid = '`+strconv.Itoa(xid)+`'::text::xid8 WHERE action = '`+action+`'; END $$`)
}

func TestEventsAreExportedInNumericTransactionOrderWhenTheDigitsGrow(t *testing.T) {
	r := newDatabaseRig(t)
	rx := newOTLPReceiver(t)
	r.insert(t, "a.one", "b.two")
	r.setPosition(t, "a.one", 90)
	r.setPosition(t, "b.two", 100)
	r.runUntilIdle(t, rx.server.URL)
	if got := rx.received(); !slices.Equal(got, []string{"a.one", "b.two"}) {
		t.Fatalf("received %v", got)
	}
}

func TestEventsDeletedBeforeTheyWereExportedAreReportedAsAGap(t *testing.T) {
	r := newDatabaseRig(t)
	rx := newOTLPReceiver(t)
	r.insert(t, "a.one", "b.two")
	r.runUntilIdle(t, rx.server.URL)
	r.insert(t, "c.three", "d.four")
	r.deleteAsRetentionWould(t, "a.one", "b.two", "c.three")
	counted := r.runUntilIdle(t, rx.server.URL)
	if counted.dropped[DroppedRetention] != 1 {
		t.Fatalf("dropped = %v, want one gap", counted.dropped)
	}
	if got := rx.received(); !slices.Equal(got, []string{"a.one", "b.two", "d.four"}) {
		t.Fatalf("received %v", got)
	}
	if counted := r.runUntilIdle(t, rx.server.URL); len(counted.dropped) != 0 {
		t.Fatalf("a caught up export reported %v", counted.dropped)
	}
}

func TestNoGapIsReportedWhileEveryEventAfterThePositionIsRetained(t *testing.T) {
	r := newDatabaseRig(t)
	rx := newOTLPReceiver(t)
	r.insert(t, "a.one", "b.two")
	r.runUntilIdle(t, rx.server.URL)
	r.insert(t, "c.three")
	if counted := r.runUntilIdle(t, rx.server.URL); len(counted.dropped) != 0 {
		t.Fatalf("dropped = %v", counted.dropped)
	}
}

func TestLagIsZeroOnceEveryEventIsExportedEvenWhenTheLastOneHappenedBeforeEarlierOnes(t *testing.T) {
	r := newDatabaseRig(t)
	rx := newOTLPReceiver(t)
	trailtest.Insert(t, r.db, trailtest.Event{At: r.base.Add(10 * time.Minute), Action: "a.one"})
	trailtest.Insert(t, r.db, trailtest.Event{At: r.base.Add(time.Minute), Action: "b.two"})
	r.runUntilIdle(t, rx.server.URL)
	if got := rx.received(); !slices.Equal(got, []string{"a.one", "b.two"}) {
		t.Fatalf("received %v", got)
	}
	lag, err := NewCursor(r.db.Queries()).Lag(t.Context())
	if err != nil || lag != 0 {
		t.Fatalf("lag = %v, %v, want 0: everything was exported", lag, err)
	}
}

func TestNoGapIsReportedWhenTheExportedEventWasDeletedAndTheNextOneFollowsIt(t *testing.T) {
	r := newDatabaseRig(t)
	rx := newOTLPReceiver(t)
	r.insert(t, "a.one", "b.two")
	r.runUntilIdle(t, rx.server.URL)
	r.deleteAsRetentionWould(t, "a.one", "b.two")
	r.insert(t, "c.three")
	counted := r.runUntilIdle(t, rx.server.URL)
	if len(counted.dropped) != 0 {
		t.Fatalf("dropped = %v: nothing after the exported event was lost", counted.dropped)
	}
	if got := rx.received(); !slices.Equal(got, []string{"a.one", "b.two", "c.three"}) {
		t.Fatalf("received %v", got)
	}
}
