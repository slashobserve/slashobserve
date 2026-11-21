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
	"sync"
	"testing"
	"time"

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

func (r *databaseRig) runUntilIdle(t *testing.T, endpoint string) {
	t.Helper()
	u, err := url.Parse(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	x := New(trail.NewReader(r.db.DB, slog.New(slog.DiscardHandler)), NewCursor(r.db.Queries()), NewSender(u, nil).Send, &lag{}, slog.New(slog.DiscardHandler))
	x.Wait = func(context.Context, time.Duration) error {
		cancel()
		return context.Canceled
	}
	if err := x.Run(ctx); err != nil {
		t.Fatal(err)
	}
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
	if at.Xid != 0 || at.Seq != 0 {
		t.Fatalf("cursor moved to %+v after a failed post", at)
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
	earlier := last
	earlier.Seq--
	if err := c.Advance(t.Context(), earlier); err != nil {
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
