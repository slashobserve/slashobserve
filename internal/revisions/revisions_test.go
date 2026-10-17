package revisions_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/events"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const userID = "11111111-1111-4111-8111-111111111111"

var user = auth.Principal{Kind: auth.KindUser, ID: userID, Name: "Ada", Email: "ada@example.com"}

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

type reviserFunc func(ctx context.Context, req revisions.Request) (revisions.Result, error)

func (f reviserFunc) Revise(ctx context.Context, req revisions.Request) (revisions.Result, error) {
	return f(ctx, req)
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newService(t *testing.T, reviser revisions.Reviser) (*revisions.Service, *dbtest.Database) {
	t.Helper()
	d := dbtest.New(t)
	return revisions.NewService(d.DB, reviser, quietLogger()), d
}

func seedDashboard(t *testing.T, d *dbtest.Database, versions int) pgtype.UUID {
	t.Helper()
	ctx := context.Background()
	row, err := d.Queries().CreateDashboard(ctx, dbsqlc.CreateDashboardParams{Title: "t", Tags: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	author, err := db.ParseUUID(userID)
	if err != nil {
		t.Fatal(err)
	}
	for v := 1; v <= versions; v++ {
		params := dbsqlc.InsertDashboardVersionParams{
			DashboardID: row.ID, Version: int32(v), Spec: []byte(`{}`), Summary: "s",
			AuthorKind: "user", AuthorID: author, AuthorName: "Ada",
		}
		if v > 1 {
			params.ParentVersion = pgtype.Int4{Int32: int32(v - 1), Valid: true}
		}
		if _, err := d.Queries().InsertDashboardVersion(ctx, params); err != nil {
			t.Fatal(err)
		}
	}
	if versions > 1 {
		d.Exec(t, "UPDATE dashboards SET latest_version = $2 WHERE id = $1", row.ID, versions)
	}
	return row.ID
}

func submit(t *testing.T, svc *revisions.Service, dashboard pgtype.UUID, prompt string) revisions.Revision {
	t.Helper()
	rev, err := svc.Submit(context.Background(), revisions.EnqueueParams{
		DashboardID: dashboard, Source: revisions.SourcePrompt, Prompt: prompt, BaseVersion: 1, By: user,
	})
	if err != nil {
		t.Fatalf("submit %q: %v", prompt, err)
	}
	return rev
}

func mustUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	id, err := db.ParseUUID(s)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func get(t *testing.T, svc *revisions.Service, dashboard pgtype.UUID, id string) revisions.Revision {
	t.Helper()
	rev, err := svc.Get(context.Background(), dashboard, mustUUID(t, id))
	if err != nil {
		t.Fatal(err)
	}
	return rev
}

func startWorker(t *testing.T, svc *revisions.Service, d *dbtest.Database, opts revisions.WorkerOptions) (stop func()) {
	t.Helper()
	if opts.Poll == 0 {
		opts.Poll = 20 * time.Millisecond
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		revisions.NewWorker(svc, d.Listen, quietLogger(), opts).Run(ctx)
	}()
	var once sync.Once
	stop = func() {
		once.Do(func() {
			cancel()
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				t.Error("worker did not stop")
			}
		})
	}
	t.Cleanup(stop)
	return stop
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitForStatus(t *testing.T, svc *revisions.Service, dashboard pgtype.UUID, id string, want revisions.Status) revisions.Revision {
	t.Helper()
	var rev revisions.Revision
	waitFor(t, fmt.Sprintf("revision %s to be %s", id, want), func() bool {
		rev = get(t, svc, dashboard, id)
		return rev.Status == want
	})
	return rev
}

func succeedWith(version int) reviserFunc {
	return func(context.Context, revisions.Request) (revisions.Result, error) {
		return revisions.Result{Version: version}, nil
	}
}

func TestRequestRunsAndSucceeds(t *testing.T) {
	var got revisions.Request
	svc, d := newService(t, reviserFunc(func(_ context.Context, req revisions.Request) (revisions.Result, error) {
		got = req
		return revisions.Result{Version: 2}, nil
	}))
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, "add a p99 panel")
	if rev.Status != revisions.StatusQueued || rev.Attempts != 0 || rev.Source != revisions.SourcePrompt {
		t.Fatalf("enqueued revision = %+v", rev)
	}

	startWorker(t, svc, d, revisions.WorkerOptions{})
	done := waitForStatus(t, svc, dash, rev.ID, revisions.StatusSucceeded)

	if done.ResultVersion == nil || *done.ResultVersion != 2 || done.Attempts != 1 || done.Error != "" || done.StartedAt == nil || done.FinishedAt == nil {
		t.Fatalf("finished revision = %+v", done)
	}
	if got.ID != rev.ID || got.DashboardID != dash.String() || got.Prompt != "add a p99 panel" || got.BaseVersion != 1 || got.RequestedBy != user {
		t.Fatalf("the reviser received %+v", got)
	}
}

func TestEventsAnnounceEveryStatusChange(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)

	var mu sync.Mutex
	var payloads []events.Event
	listening := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = d.Listen(ctx, events.Channel, func() { close(listening) }, func(payload string) {
			var ev events.Event
			if err := json.Unmarshal([]byte(payload), &ev); err == nil && ev.Type == events.TypeRevisionUpdated {
				mu.Lock()
				payloads = append(payloads, ev)
				mu.Unlock()
			}
		})
	}()
	<-listening

	rev := submit(t, svc, dash, "go")
	startWorker(t, svc, d, revisions.WorkerOptions{})
	waitForStatus(t, svc, dash, rev.ID, revisions.StatusSucceeded)
	waitFor(t, "three revision.updated events", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(payloads) == 3
	})

	mu.Lock()
	defer mu.Unlock()
	for i, want := range []string{"queued", "running", "succeeded"} {
		ev := payloads[i]
		if ev.Status != want || ev.RevisionID != rev.ID || ev.DashboardID != dash.String() {
			t.Errorf("event %d = %+v, want status %s", i, ev, want)
		}
	}
	if payloads[2].Version != 2 {
		t.Errorf("the succeeded event carries version %d, want 2", payloads[2].Version)
	}
}

func TestIdleWorkerWakesOnNotifyBeforeThePollInterval(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	startWorker(t, svc, d, revisions.WorkerOptions{Poll: time.Hour})
	time.Sleep(300 * time.Millisecond)
	rev := submit(t, svc, dash, "go")
	waitForStatus(t, svc, dash, rev.ID, revisions.StatusSucceeded)
}

func TestFailedRunStoresATruncatedError(t *testing.T) {
	svc, d := newService(t, reviserFunc(func(context.Context, revisions.Request) (revisions.Result, error) {
		return revisions.Result{}, errors.New(strings.Repeat("é", 900) + "\x00")
	}))
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, "go")
	startWorker(t, svc, d, revisions.WorkerOptions{})
	failed := waitForStatus(t, svc, dash, rev.ID, revisions.StatusFailed)
	if failed.Error != strings.Repeat("é", revisions.MaxErrorRunes) || failed.ResultVersion != nil || failed.Attempts != 1 {
		t.Fatalf("failed revision = %+v", failed)
	}
}

func TestPanicInTheReviserFailsOnlyThatRequest(t *testing.T) {
	var calls atomic.Int32
	svc, d := newService(t, reviserFunc(func(_ context.Context, req revisions.Request) (revisions.Result, error) {
		if req.Prompt == "boom" {
			panic("secret prompt text")
		}
		calls.Add(1)
		return revisions.Result{Version: 2}, nil
	}))
	dash := seedDashboard(t, d, 1)
	boom := submit(t, svc, dash, "boom")
	fine := submit(t, svc, dash, "fine")
	startWorker(t, svc, d, revisions.WorkerOptions{})
	failed := waitForStatus(t, svc, dash, boom.ID, revisions.StatusFailed)
	if failed.Error != "the agent run crashed" {
		t.Fatalf("error = %q", failed.Error)
	}
	waitForStatus(t, svc, dash, fine.ID, revisions.StatusSucceeded)
}

func TestRunTimeoutFailsTheRequest(t *testing.T) {
	svc, d := newService(t, reviserFunc(func(ctx context.Context, _ revisions.Request) (revisions.Result, error) {
		<-ctx.Done()
		return revisions.Result{}, ctx.Err()
	}))
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, "slow")
	startWorker(t, svc, d, revisions.WorkerOptions{RunTimeout: 100 * time.Millisecond})
	failed := waitForStatus(t, svc, dash, rev.ID, revisions.StatusFailed)
	if failed.Error != "the agent run timed out" {
		t.Fatalf("error = %q", failed.Error)
	}
}

func TestShutdownLeavesARunningRequestForTheNextProcess(t *testing.T) {
	started := make(chan struct{})
	svc, d := newService(t, reviserFunc(func(ctx context.Context, _ revisions.Request) (revisions.Result, error) {
		close(started)
		<-ctx.Done()
		return revisions.Result{}, ctx.Err()
	}))
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, "go")
	stop := startWorker(t, svc, d, revisions.WorkerOptions{})
	<-started
	stop()

	left := get(t, svc, dash, rev.ID)
	if left.Status != revisions.StatusRunning || left.Attempts != 1 || left.FinishedAt != nil {
		t.Fatalf("after shutdown = %+v", left)
	}

	d.Exec(t, "UPDATE agent_requests SET lease_until = clock_timestamp() - interval '1 second' WHERE id = $1", mustUUID(t, rev.ID))
	next := revisions.NewService(d.DB, succeedWith(2), quietLogger())
	startWorker(t, next, d, revisions.WorkerOptions{})
	done := waitForStatus(t, next, dash, rev.ID, revisions.StatusSucceeded)
	if done.Attempts != 2 {
		t.Fatalf("the retry should be attempt 2, got %+v", done)
	}
}

func TestExpiredLeaseIsRetriedOnce(t *testing.T) {
	var calls atomic.Int32
	svc, d := newService(t, reviserFunc(func(context.Context, revisions.Request) (revisions.Result, error) {
		calls.Add(1)
		return revisions.Result{Version: 2}, nil
	}))
	dash := seedDashboard(t, d, 1)

	crashed := submit(t, svc, dash, "crashed once")
	d.Exec(t, "UPDATE agent_requests SET status = 'running', attempts = 1, started_at = clock_timestamp(), lease_until = clock_timestamp() + interval '1 hour' WHERE id = $1", mustUUID(t, crashed.ID))
	startWorker(t, svc, d, revisions.WorkerOptions{})
	time.Sleep(300 * time.Millisecond)
	if got := get(t, svc, dash, crashed.ID); got.Status != revisions.StatusRunning || calls.Load() != 0 {
		t.Fatalf("a request with a live lease must be left alone: %+v, %d calls", got, calls.Load())
	}

	d.Exec(t, "UPDATE agent_requests SET lease_until = clock_timestamp() - interval '1 second' WHERE id = $1", mustUUID(t, crashed.ID))
	done := waitForStatus(t, svc, dash, crashed.ID, revisions.StatusSucceeded)
	if done.Attempts != 2 || calls.Load() != 1 {
		t.Fatalf("retried revision = %+v after %d calls", done, calls.Load())
	}
}

func TestSecondExpiredLeaseFailsWithoutAnotherRun(t *testing.T) {
	var calls atomic.Int32
	svc, d := newService(t, reviserFunc(func(context.Context, revisions.Request) (revisions.Result, error) {
		calls.Add(1)
		return revisions.Result{Version: 2}, nil
	}))
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, "crashed twice")
	d.Exec(t, "UPDATE agent_requests SET status = 'running', attempts = 2, started_at = clock_timestamp(), lease_until = clock_timestamp() - interval '1 second' WHERE id = $1", mustUUID(t, rev.ID))

	startWorker(t, svc, d, revisions.WorkerOptions{})
	failed := waitForStatus(t, svc, dash, rev.ID, revisions.StatusFailed)
	if failed.Error != revisions.ExhaustedMessage || failed.Attempts != 2 || calls.Load() != 0 {
		t.Fatalf("exhausted revision = %+v after %d calls", failed, calls.Load())
	}
}

func TestExhaustedRequestDoesNotBlockItsDashboard(t *testing.T) {
	svc, d := newService(t, succeedWith(3))
	dash := seedDashboard(t, d, 2)
	stuck := submit(t, svc, dash, "stuck")
	next := submit(t, svc, dash, "next")
	d.Exec(t, "UPDATE agent_requests SET status = 'running', attempts = 2, lease_until = clock_timestamp() - interval '1 second' WHERE id = $1", mustUUID(t, stuck.ID))
	startWorker(t, svc, d, revisions.WorkerOptions{})
	waitForStatus(t, svc, dash, stuck.ID, revisions.StatusFailed)
	waitForStatus(t, svc, dash, next.ID, revisions.StatusSucceeded)
}

func TestLateResultFromAnExpiredAttemptIsDropped(t *testing.T) {
	release := make(chan struct{})
	firstStarted := make(chan struct{})
	first, d := newService(t, reviserFunc(func(ctx context.Context, _ revisions.Request) (revisions.Result, error) {
		close(firstStarted)
		select {
		case <-release:
		case <-ctx.Done():
		}
		return revisions.Result{Version: 3}, nil
	}))
	dash := seedDashboard(t, d, 1)
	rev := submit(t, first, dash, "go")
	startWorker(t, first, d, revisions.WorkerOptions{Workers: 1})
	<-firstStarted

	d.Exec(t, "UPDATE agent_requests SET lease_until = clock_timestamp() - interval '1 second' WHERE id = $1", mustUUID(t, rev.ID))
	second := revisions.NewService(d.DB, succeedWith(7), quietLogger())
	startWorker(t, second, d, revisions.WorkerOptions{Workers: 1})
	done := waitForStatus(t, second, dash, rev.ID, revisions.StatusSucceeded)
	if done.Attempts != 2 || done.ResultVersion == nil || *done.ResultVersion != 7 {
		t.Fatalf("second attempt = %+v", done)
	}

	close(release)
	time.Sleep(300 * time.Millisecond)
	final := get(t, first, dash, rev.ID)
	if final.ResultVersion == nil || *final.ResultVersion != 7 || final.Status != revisions.StatusSucceeded || final.Error != "" {
		t.Fatalf("the stale attempt overwrote the result: %+v", final)
	}
}

func TestRequestWithASavedVersionFinishesWithoutRunningAgain(t *testing.T) {
	var calls atomic.Int32
	svc, d := newService(t, reviserFunc(func(context.Context, revisions.Request) (revisions.Result, error) {
		calls.Add(1)
		return revisions.Result{Version: 9}, nil
	}))
	dash := seedDashboard(t, d, 2)
	rev := submit(t, svc, dash, "saved before the crash")
	d.Exec(t, "UPDATE dashboard_versions SET agent_request_id = $2 WHERE dashboard_id = $1 AND version = 2", dash, mustUUID(t, rev.ID))
	startWorker(t, svc, d, revisions.WorkerOptions{})
	done := waitForStatus(t, svc, dash, rev.ID, revisions.StatusSucceeded)
	if done.ResultVersion == nil || *done.ResultVersion != 2 || calls.Load() != 0 {
		t.Fatalf("revision = %+v after %d agent calls", done, calls.Load())
	}
}

func TestDashboardsRunInParallelButEachRunsOneRequestAtATime(t *testing.T) {
	type tracker struct {
		mu     sync.Mutex
		active map[string]int
		order  map[string][]string
		peak   int
		busy   int
		wrong  []string
	}
	tr := &tracker{active: map[string]int{}, order: map[string][]string{}}
	bothRunning := make(chan struct{})
	var once sync.Once
	svc, d := newService(t, reviserFunc(func(_ context.Context, req revisions.Request) (revisions.Result, error) {
		tr.mu.Lock()
		tr.active[req.DashboardID]++
		tr.busy++
		tr.peak = max(tr.peak, tr.busy)
		if tr.active[req.DashboardID] > 1 {
			tr.wrong = append(tr.wrong, req.DashboardID)
		}
		tr.order[req.DashboardID] = append(tr.order[req.DashboardID], req.Prompt)
		if tr.busy == 2 {
			once.Do(func() { close(bothRunning) })
		}
		tr.mu.Unlock()

		select {
		case <-bothRunning:
		case <-time.After(5 * time.Second):
		}
		time.Sleep(20 * time.Millisecond)

		tr.mu.Lock()
		tr.active[req.DashboardID]--
		tr.busy--
		tr.mu.Unlock()
		return revisions.Result{Version: 2}, nil
	}))
	a, b := seedDashboard(t, d, 1), seedDashboard(t, d, 1)
	var all []revisions.Revision
	for i := range 4 {
		all = append(all, submit(t, svc, a, fmt.Sprintf("a%d", i)), submit(t, svc, b, fmt.Sprintf("b%d", i)))
	}
	startWorker(t, svc, d, revisions.WorkerOptions{Workers: 4})
	for _, rev := range all {
		dashboard := mustUUID(t, rev.DashboardID)
		waitForStatus(t, svc, dashboard, rev.ID, revisions.StatusSucceeded)
	}

	tr.mu.Lock()
	defer tr.mu.Unlock()
	if len(tr.wrong) > 0 {
		t.Errorf("a dashboard ran two requests at once: %v", tr.wrong)
	}
	if tr.peak != 2 {
		t.Errorf("peak concurrency = %d, want 2 (one per dashboard)", tr.peak)
	}
	for id, prompts := range tr.order {
		for i, p := range prompts {
			if want := fmt.Sprintf("%c%d", p[0], i); p != want {
				t.Errorf("dashboard %s ran %v, want creation order", id, prompts)
				break
			}
		}
	}
}

func TestCompetingWorkersRunEveryRequestExactlyOnce(t *testing.T) {
	var mu sync.Mutex
	runs := map[string]int{}
	active := map[string]int{}
	var violations []string
	reviser := reviserFunc(func(_ context.Context, req revisions.Request) (revisions.Result, error) {
		mu.Lock()
		runs[req.ID]++
		active[req.DashboardID]++
		if active[req.DashboardID] > 1 {
			violations = append(violations, req.DashboardID)
		}
		mu.Unlock()
		time.Sleep(5 * time.Millisecond)
		mu.Lock()
		active[req.DashboardID]--
		mu.Unlock()
		return revisions.Result{Version: 2}, nil
	})
	svc, d := newService(t, reviser)
	var all []revisions.Revision
	for range 8 {
		dash := seedDashboard(t, d, 1)
		for i := range revisions.MaxActive {
			all = append(all, submit(t, svc, dash, fmt.Sprintf("p%d", i)))
		}
	}
	for range 3 {
		startWorker(t, revisions.NewService(d.DB, reviser, quietLogger()), d, revisions.WorkerOptions{Workers: 3})
	}
	for _, rev := range all {
		waitForStatus(t, svc, mustUUID(t, rev.DashboardID), rev.ID, revisions.StatusSucceeded)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(violations) > 0 {
		t.Errorf("a dashboard ran two requests at once: %v", violations)
	}
	for _, rev := range all {
		if runs[rev.ID] != 1 {
			t.Errorf("request %s ran %d times", rev.ID, runs[rev.ID])
		}
	}
}

func TestActiveRequestsPerDashboardAreCapped(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash, other := seedDashboard(t, d, 1), seedDashboard(t, d, 1)
	var first revisions.Revision
	for i := range revisions.MaxActive {
		rev := submit(t, svc, dash, fmt.Sprintf("p%d", i))
		if i == 0 {
			first = rev
		}
	}
	_, err := svc.Submit(context.Background(), revisions.EnqueueParams{DashboardID: dash, Source: revisions.SourcePrompt, Prompt: "one too many", BaseVersion: 1, By: user})
	if !errors.Is(err, revisions.ErrTooManyRevisions) {
		t.Fatalf("sixth submit: %v", err)
	}
	submit(t, svc, other, "another dashboard is unaffected")

	d.Exec(t, "UPDATE agent_requests SET status = 'succeeded' WHERE id = $1", mustUUID(t, first.ID))
	submit(t, svc, dash, "room again")
}

func TestConcurrentSubmitsNeverExceedTheCap(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	var accepted, rejected atomic.Int32
	var wg sync.WaitGroup
	for i := range 24 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.Submit(context.Background(), revisions.EnqueueParams{
				DashboardID: dash, Source: revisions.SourcePrompt, Prompt: fmt.Sprintf("p%d", i), BaseVersion: 1, By: user,
			})
			switch {
			case err == nil:
				accepted.Add(1)
			case errors.Is(err, revisions.ErrTooManyRevisions):
				rejected.Add(1)
			default:
				t.Errorf("submit: %v", err)
			}
		}()
	}
	wg.Wait()
	if accepted.Load() != revisions.MaxActive || rejected.Load() != 24-revisions.MaxActive {
		t.Fatalf("accepted %d and rejected %d", accepted.Load(), rejected.Load())
	}
	items, _, err := svc.List(context.Background(), dash, 100, "")
	if err != nil || len(items) != revisions.MaxActive {
		t.Fatalf("stored %d requests, err %v", len(items), err)
	}
}

func TestSubmitWithoutAnAgentIsRefused(t *testing.T) {
	svc, d := newService(t, nil)
	dash := seedDashboard(t, d, 1)
	if svc.Available() {
		t.Fatal("a service without a reviser reports itself available")
	}
	_, err := svc.Submit(context.Background(), revisions.EnqueueParams{DashboardID: dash, Source: revisions.SourcePrompt, Prompt: "x", BaseVersion: 1, By: user})
	if !errors.Is(err, revisions.ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	items, _, err := svc.List(context.Background(), dash, 10, "")
	if err != nil || len(items) != 0 {
		t.Fatalf("nothing may be queued without an agent: %v, %v", items, err)
	}
}

func TestListPagesNewestFirst(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	var ids []string
	for i := range 4 {
		ids = append([]string{submit(t, svc, dash, fmt.Sprintf("p%d", i)).ID}, ids...)
	}
	first, next, err := svc.List(context.Background(), dash, 3, "")
	if err != nil || next == "" || len(first) != 3 {
		t.Fatalf("first page: %v, next %q, %v", len(first), next, err)
	}
	second, end, err := svc.List(context.Background(), dash, 3, next)
	if err != nil || end != "" || len(second) != 1 {
		t.Fatalf("second page: %v, next %q, %v", len(second), end, err)
	}
	var got []string
	for _, r := range append(first, second...) {
		got = append(got, r.ID)
	}
	if strings.Join(got, ",") != strings.Join(ids, ",") {
		t.Fatalf("order = %v, want %v", got, ids)
	}
	if _, _, err := svc.List(context.Background(), dash, 3, "!!!"); !errors.Is(err, revisions.ErrInvalidCursor) {
		t.Fatalf("bad cursor: %v", err)
	}
}
