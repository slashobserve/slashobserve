package revisions_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	"github.com/slashobserve/slashobserve/internal/logtest"
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
	return slog.New(slog.DiscardHandler)
}

func newService(t *testing.T, reviser revisions.Reviser) (*revisions.Service, *dbtest.Database) {
	t.Helper()
	d := dbtest.New(t)
	return revisions.NewService(d.DB, reviser, quietLogger()), d
}

func seedDashboard(t *testing.T, d *dbtest.Database, versions int) pgtype.UUID {
	t.Helper()
	ctx := context.Background()
	author, err := db.ParseUUID(userID)
	if err != nil {
		t.Fatal(err)
	}
	row, err := d.Queries().CreateDashboard(ctx, dbsqlc.CreateDashboardParams{
		Title: "t", Tags: []string{}, CreatedByKind: "user", CreatedByID: author, CreatedByName: "Ada", CreatedVia: "direct",
	})
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
	row, err := svc.Submit(context.Background(), revisions.EnqueueParams{
		DashboardID: dashboard, Source: revisions.SourcePrompt, Prompt: prompt, BaseVersion: 1, By: user,
	})
	if err != nil {
		t.Fatalf("submit %q: %v", prompt, err)
	}
	return get(t, svc, dashboard, row.ID.String())
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
	return startWorkerLogged(t, svc, d, opts, quietLogger())
}

func startWorkerLogged(t *testing.T, svc *revisions.Service, d *dbtest.Database, opts revisions.WorkerOptions, logger *slog.Logger) (stop func()) {
	t.Helper()
	if opts.Poll == 0 {
		opts.Poll = 20 * time.Millisecond
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		revisions.NewWorker(svc, d.Listen, logger, opts).Run(ctx)
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

func logRecords(t *testing.T, logs *logtest.Capture, msg string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, record := range logs.Records(t) {
		if record["msg"] == msg {
			out = append(out, record)
		}
	}
	return out
}

func waitForLog(t *testing.T, logs *logtest.Capture, msg string) []map[string]any {
	t.Helper()
	var records []map[string]any
	waitFor(t, fmt.Sprintf("a %q log record", msg), func() bool {
		records = logRecords(t, logs, msg)
		return len(records) > 0
	})
	return records
}

func findEventually(t *testing.T, logs *logtest.Capture, msg string) map[string]any {
	t.Helper()
	waitForLog(t, logs, msg)
	return logs.Find(t, msg)
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
	var svc *revisions.Service
	var d *dbtest.Database
	var dash pgtype.UUID
	observeRunning := make(chan struct{})
	releaseObserve := make(chan struct{})
	svc, d = newService(t, reviserFunc(func(ctx context.Context, req revisions.Request) (revisions.Result, error) {
		if req.Source == revisions.SourceObserve {
			close(observeRunning)
			select {
			case <-releaseObserve:
			case <-ctx.Done():
				return revisions.Result{}, ctx.Err()
			}
			saveVersionFor(t, d, dash, req.ID)
		}
		return revisions.Result{Version: 2}, nil
	}))
	dash = seedDashboard(t, d, 2)

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
	eventsOf := func(id string) []events.Event {
		mu.Lock()
		defer mu.Unlock()
		var out []events.Event
		for _, ev := range payloads {
			if ev.RevisionID == id {
				out = append(out, ev)
			}
		}
		return out
	}

	observe, err := svc.SubmitObserve(context.Background(), user, "build a dashboard")
	if err != nil {
		t.Fatal(err)
	}
	rev := submit(t, svc, dash, "go")
	startWorker(t, svc, d, revisions.WorkerOptions{})
	<-observeRunning
	waitForStatus(t, svc, dash, rev.ID, revisions.StatusSucceeded)
	waitFor(t, "three revision.updated events", func() bool { return len(eventsOf(rev.ID)) == 3 })

	if got := eventsOf(observe.ID.String()); len(got) != 0 {
		t.Fatalf("a queued or running observe request sent %+v", got)
	}

	close(releaseObserve)
	waitFor(t, "the observe request to succeed", func() bool {
		row, err := svc.GetByID(context.Background(), observe.ID)
		return err == nil && row.Status == string(revisions.StatusSucceeded)
	})
	waitFor(t, "the observe request's revision.updated event", func() bool { return len(eventsOf(observe.ID.String())) == 1 })

	got := eventsOf(rev.ID)
	for i, want := range []string{"queued", "running", "succeeded"} {
		ev := got[i]
		if ev.Status != want || ev.RevisionID != rev.ID || ev.DashboardID != dash.String() {
			t.Errorf("event %d = %+v, want status %s", i, ev, want)
		}
	}
	if got[2].Version != 2 {
		t.Errorf("the succeeded event carries version %d, want 2", got[2].Version)
	}
	observed := eventsOf(observe.ID.String())
	if len(observed) != 1 || observed[0].Status != "succeeded" || observed[0].DashboardID != dash.String() || observed[0].Version != 2 {
		t.Errorf("observe request events = %+v, want one succeeded event for the dashboard it created", observed)
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

func TestWorkerLogsCarryTheRequestIdAsAgentRequestIdAndNoRunText(t *testing.T) {
	logger, logs := logtest.New()
	d := dbtest.New(t)
	svc := revisions.NewService(d.DB, reviserFunc(func(_ context.Context, req revisions.Request) (revisions.Result, error) {
		if req.Prompt == "boom" {
			panic("secret prompt text")
		}
		return revisions.Result{Version: 2}, nil
	}), logger)
	dash := seedDashboard(t, d, 1)
	boom := submit(t, svc, dash, "boom")
	startWorkerLogged(t, svc, d, revisions.WorkerOptions{}, logger)
	waitForStatus(t, svc, dash, boom.ID, revisions.StatusFailed)
	fine := submit(t, svc, dash, "fine")
	waitForStatus(t, svc, dash, fine.ID, revisions.StatusSucceeded)

	if got := findEventually(t, logs, "agent run panicked"); got["agent_request_id"] != boom.ID || got["panic_type"] != "string" {
		t.Errorf("panic record = %v", got)
	}
	failed := findEventually(t, logs, "agent request failed")
	logs.AssertSafeFailure(t, failed, "agent_run_panicked", "secret prompt text")
	if failed["agent_request_id"] != boom.ID {
		t.Errorf("failure record = %v", failed)
	}
	if got := findEventually(t, logs, "agent request succeeded"); got["agent_request_id"] != fine.ID {
		t.Errorf("success record = %v", got)
	}
	logs.AssertAbsent(t, "revision_id")
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

func TestShutdownReleasesARunningRequestWithoutSpendingAnAttempt(t *testing.T) {
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
	if left.Status != revisions.StatusQueued || left.Attempts != 0 || left.FinishedAt != nil {
		t.Fatalf("after shutdown = %+v", left)
	}

	next := revisions.NewService(d.DB, succeedWith(2), quietLogger())
	startWorker(t, next, d, revisions.WorkerOptions{})
	done := waitForStatus(t, next, dash, rev.ID, revisions.StatusSucceeded)
	if done.Attempts != 1 {
		t.Fatalf("the next process should run attempt 1, got %+v", done)
	}
}

func saveVersionFor(t *testing.T, d *dbtest.Database, dash pgtype.UUID, requestID string) {
	d.Exec(t, "UPDATE dashboard_versions SET agent_request_id = $2 WHERE dashboard_id = $1 AND version = 2", dash, mustUUID(t, requestID))
}

func TestShutdownAfterTheVersionWasSavedRecordsSuccess(t *testing.T) {
	started := make(chan struct{})
	var svc *revisions.Service
	var d *dbtest.Database
	var dash pgtype.UUID
	svc, d = newService(t, reviserFunc(func(ctx context.Context, req revisions.Request) (revisions.Result, error) {
		saveVersionFor(t, d, dash, req.ID)
		close(started)
		<-ctx.Done()
		return revisions.Result{}, ctx.Err()
	}))
	dash = seedDashboard(t, d, 2)
	rev := submit(t, svc, dash, "go")
	stop := startWorker(t, svc, d, revisions.WorkerOptions{})
	<-started
	stop()

	done := get(t, svc, dash, rev.ID)
	if done.Status != revisions.StatusSucceeded || done.ResultVersion == nil || *done.ResultVersion != 2 || done.Attempts != 1 {
		t.Fatalf("after shutdown = %+v", done)
	}
}

func TestFailedRunWithASavedVersionIsRecordedAsSuccess(t *testing.T) {
	cases := map[string]struct {
		opts   revisions.WorkerOptions
		revise func(ctx context.Context) error
	}{
		"error": {revise: func(context.Context) error { return errors.New("the loop gave up after saving") }},
		"timeout": {
			opts:   revisions.WorkerOptions{RunTimeout: 100 * time.Millisecond},
			revise: func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() },
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var d *dbtest.Database
			var dash pgtype.UUID
			svc, d := newService(t, reviserFunc(func(ctx context.Context, req revisions.Request) (revisions.Result, error) {
				saveVersionFor(t, d, dash, req.ID)
				return revisions.Result{}, tc.revise(ctx)
			}))
			dash = seedDashboard(t, d, 2)
			rev := submit(t, svc, dash, "go")
			startWorker(t, svc, d, tc.opts)
			done := waitForStatus(t, svc, dash, rev.ID, revisions.StatusSucceeded)
			if done.ResultVersion == nil || *done.ResultVersion != 2 || done.Error != "" || done.Attempts != 1 {
				t.Fatalf("revision = %+v", done)
			}
		})
	}
}

func TestFailedCheckAfterAFailedRunRecordsTheFailureWithoutAFreeRerun(t *testing.T) {
	var calls atomic.Int32
	var d *dbtest.Database
	svc, d := newService(t, reviserFunc(func(context.Context, revisions.Request) (revisions.Result, error) {
		calls.Add(1)
		d.Exec(t, "ALTER TABLE dashboard_versions RENAME COLUMN agent_request_id TO agent_request_id_broken")
		return revisions.Result{}, errors.New("boom")
	}))
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, "go")
	startWorker(t, svc, d, revisions.WorkerOptions{})

	failed := waitForStatus(t, svc, dash, rev.ID, revisions.StatusFailed)
	if failed.Attempts != 1 || failed.Error != "boom" || failed.FinishedAt == nil || calls.Load() != 1 {
		t.Fatalf("revision = %+v after %d calls", failed, calls.Load())
	}
}

func breakVersionsAfterClaim(t *testing.T, d *dbtest.Database) {
	t.Helper()
	d.Exec(t, `CREATE FUNCTION break_versions() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'dashboard_versions' AND column_name = 'agent_request_id') THEN
    ALTER TABLE dashboard_versions RENAME COLUMN agent_request_id TO agent_request_id_broken;
  END IF;
  RETURN NEW;
END $$`)
	d.Exec(t, "CREATE TRIGGER break_after_claim AFTER UPDATE ON agent_requests FOR EACH ROW WHEN (NEW.status = 'running' AND OLD.status <> 'running') EXECUTE FUNCTION break_versions()")
}

func TestPersistentFailureToCheckTheSavedVersionReleasesThenFailsTheRequest(t *testing.T) {
	var calls atomic.Int32
	svc, d := newService(t, reviserFunc(func(context.Context, revisions.Request) (revisions.Result, error) {
		calls.Add(1)
		return revisions.Result{Version: 2}, nil
	}))
	breakVersionsAfterClaim(t, d)
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, "go")
	id := mustUUID(t, rev.ID)
	startWorker(t, svc, d, revisions.WorkerOptions{Workers: 1})

	for released := int32(1); released < 3; released++ {
		var row dbsqlc.AgentRequest
		waitFor(t, fmt.Sprintf("release number %d", released), func() bool {
			var err error
			row, err = d.Queries().GetAgentRequest(context.Background(), dbsqlc.GetAgentRequestParams{ID: id, DashboardID: dash})
			return err == nil && row.Status == string(revisions.StatusQueued) && row.CheckFailures == released
		})
		if row.Attempts != 0 || row.FinishedAt.Valid {
			t.Fatalf("release %d left %+v", released, row)
		}
		d.Exec(t, "ALTER TABLE dashboard_versions RENAME COLUMN agent_request_id_broken TO agent_request_id")
	}

	failed := waitForStatus(t, svc, dash, rev.ID, revisions.StatusFailed)
	if failed.Error != "could not check the request's saved version" || failed.Attempts != 1 || calls.Load() != 0 {
		t.Fatalf("revision = %+v after %d calls", failed, calls.Load())
	}
}

func TestCheckFailedReleaseKeepsTheRecordedSteps(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	breakVersionsAfterClaim(t, d)
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, "go")
	id := mustUUID(t, rev.ID)
	d.Exec(t, `UPDATE agent_requests SET steps = '[{"at":"2026-01-01T00:00:00Z","attempt":1,"phase":"query","tool":"query_range","status":"ok","message":"Queried","duration_ms":1}]'::jsonb WHERE id = $1`, id)
	startWorker(t, svc, d, revisions.WorkerOptions{Workers: 1})

	var row dbsqlc.AgentRequest
	waitFor(t, "the check-failed release", func() bool {
		var err error
		row, err = d.Queries().GetAgentRequest(context.Background(), dbsqlc.GetAgentRequestParams{ID: id, DashboardID: dash})
		return err == nil && row.Status == string(revisions.StatusQueued) && row.CheckFailures == 1
	})
	var stored []revisions.Step
	if err := json.Unmarshal(row.Steps, &stored); err != nil {
		t.Fatal(err)
	}
	if row.Attempts != 0 || len(stored) != 1 || stored[0].Message != "Queried" {
		t.Fatalf("release left attempts %d and steps %s, want attempts 0 and the recorded step", row.Attempts, row.Steps)
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

func TestExhaustedRequestWithASavedVersionSucceeds(t *testing.T) {
	var calls atomic.Int32
	svc, d := newService(t, reviserFunc(func(context.Context, revisions.Request) (revisions.Result, error) {
		calls.Add(1)
		return revisions.Result{Version: 9}, nil
	}))
	dash := seedDashboard(t, d, 2)
	rev := submit(t, svc, dash, "saved by attempt 2, then the process died")
	saveVersionFor(t, d, dash, rev.ID)
	d.Exec(t, "UPDATE agent_requests SET status = 'running', attempts = 2, started_at = clock_timestamp(), lease_until = clock_timestamp() - interval '1 second' WHERE id = $1", mustUUID(t, rev.ID))

	startWorker(t, svc, d, revisions.WorkerOptions{})
	done := waitForStatus(t, svc, dash, rev.ID, revisions.StatusSucceeded)
	if done.ResultVersion == nil || *done.ResultVersion != 2 || done.Error != "" || done.Attempts != 2 || done.FinishedAt == nil || calls.Load() != 0 {
		t.Fatalf("revision = %+v after %d calls", done, calls.Load())
	}
}

func TestExhaustedThreadRequestWithASavedVersionResolvesTheThread(t *testing.T) {
	svc, d := newService(t, succeedWith(9))
	ctx := context.Background()
	dash := seedDashboard(t, d, 2)
	root, err := d.Queries().CreateComment(ctx, dbsqlc.CreateCommentParams{
		DashboardID: dash, Version: 1, Body: "fix", AuthorKind: "user", AuthorID: mustUUID(t, userID), AuthorName: "Ada",
	})
	if err != nil {
		t.Fatal(err)
	}
	row, err := svc.SubmitForThread(ctx, dash, root.ID, nil, "go", user)
	if err != nil {
		t.Fatal(err)
	}
	saveVersionFor(t, d, dash, row.ID.String())
	d.Exec(t, "UPDATE agent_requests SET status = 'running', attempts = 2, started_at = clock_timestamp(), lease_until = clock_timestamp() - interval '1 second' WHERE id = $1", row.ID)

	startWorker(t, svc, d, revisions.WorkerOptions{})
	waitForStatus(t, svc, dash, row.ID.String(), revisions.StatusSucceeded)
	got, err := d.Queries().GetComment(ctx, dbsqlc.GetCommentParams{ID: root.ID, DashboardID: dash})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "resolved" || got.ResolvedByVersion.Int32 != 2 {
		t.Fatalf("thread = %+v", got)
	}
}

func TestExpiredRunningRequestIsRetriedBeforeANewerOneOfItsDashboard(t *testing.T) {
	var mu sync.Mutex
	var order []string
	svc, d := newService(t, reviserFunc(func(_ context.Context, req revisions.Request) (revisions.Result, error) {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, req.Prompt)
		return revisions.Result{Version: 2}, nil
	}))
	dash := seedDashboard(t, d, 1)
	older := submit(t, svc, dash, "older, still queued")
	crashed := submit(t, svc, dash, "newer, crashed once")
	d.Exec(t, "UPDATE agent_requests SET status = 'running', attempts = 1, started_at = clock_timestamp(), lease_until = clock_timestamp() - interval '1 second' WHERE id = $1", mustUUID(t, crashed.ID))

	startWorker(t, svc, d, revisions.WorkerOptions{})
	waitForStatus(t, svc, dash, crashed.ID, revisions.StatusSucceeded)
	waitForStatus(t, svc, dash, older.ID, revisions.StatusSucceeded)
	mu.Lock()
	defer mu.Unlock()
	if len(order) != 2 || order[0] != "newer, crashed once" {
		t.Fatalf("run order = %v", order)
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

	d.Exec(t, "UPDATE agent_requests SET status = 'succeeded', result_version = 1 WHERE id = $1", mustUUID(t, first.ID))
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

func saveNextVersion(ctx context.Context, t *testing.T, d *dbtest.Database, req revisions.Request) (int, error) {
	dash, err := db.ParseUUID(req.DashboardID)
	if err != nil {
		return 0, err
	}
	latest, err := d.Queries().GetDashboardLatestVersion(ctx, dash)
	if err != nil {
		return 0, err
	}
	requestID, err := db.ParseUUID(req.ID)
	if err != nil {
		return 0, err
	}
	author, err := db.ParseUUID(userID)
	if err != nil {
		return 0, err
	}
	next := latest + 1
	if _, err := d.Queries().InsertDashboardVersion(ctx, dbsqlc.InsertDashboardVersionParams{
		DashboardID: dash, Version: next, ParentVersion: pgtype.Int4{Int32: latest, Valid: true}, Spec: []byte(`{}`), Summary: "s",
		AuthorKind: "user", AuthorID: author, AuthorName: "Ada", AgentRequestID: requestID,
	}); err != nil {
		return 0, err
	}
	d.Exec(t, "UPDATE dashboards SET latest_version = $2 WHERE id = $1", dash, next)
	return int(next), nil
}

func TestRequestSentFromACommentThreadResolvesTheThreadWithTheSavedVersion(t *testing.T) {
	var d *dbtest.Database
	var svc *revisions.Service
	svc, d = newService(t, reviserFunc(func(ctx context.Context, req revisions.Request) (revisions.Result, error) {
		if req.Prompt == "fail" {
			return revisions.Result{}, errors.New("model call failed")
		}
		version, err := saveNextVersion(ctx, t, d, req)
		return revisions.Result{Version: version}, err
	}))
	ctx := context.Background()
	dash := seedDashboard(t, d, 3)
	author := mustUUID(t, userID)
	newComment := func(parent pgtype.UUID) dbsqlc.DashboardComment {
		t.Helper()
		params := dbsqlc.CreateCommentParams{
			DashboardID: dash, Version: 1, Body: "split by route", AuthorKind: "user", AuthorID: author, AuthorName: "Ada", ParentID: parent,
		}
		if !parent.Valid {
			params.PanelID = pgtype.Text{String: "p1", Valid: true}
		}
		row, err := d.Queries().CreateComment(ctx, params)
		if err != nil {
			t.Fatal(err)
		}
		return row
	}
	thread := func(id pgtype.UUID) dbsqlc.DashboardComment {
		t.Helper()
		row, err := d.Queries().GetComment(ctx, dbsqlc.GetCommentParams{ID: id, DashboardID: dash})
		if err != nil {
			t.Fatal(err)
		}
		return row
	}
	firstVersion := 1
	baseVersion := &firstVersion
	sendFromThread := func(comment pgtype.UUID, prompt string) revisions.Revision {
		t.Helper()
		row, err := svc.SubmitForThread(ctx, dash, comment, baseVersion, prompt, user)
		if err != nil {
			t.Fatal(err)
		}
		return get(t, svc, dash, row.ID.String())
	}

	root := newComment(pgtype.UUID{})
	reply := newComment(root.ID)
	rev := sendFromThread(reply.ID, "go")
	if rev.Source != revisions.SourceComment || rev.CommentID != root.ID.String() || rev.PanelID != "p1" || rev.BaseVersion == nil || *rev.BaseVersion != 1 {
		t.Fatalf("enqueued revision = %+v", rev)
	}
	if _, err := svc.SubmitForThread(ctx, dash, mustUUID(t, "00000000-0000-0000-0000-000000000000"), nil, "go", user); !errors.Is(err, revisions.ErrCommentNotFound) {
		t.Fatalf("unknown comment: %v", err)
	}

	startWorker(t, svc, d, revisions.WorkerOptions{})
	waitForStatus(t, svc, dash, rev.ID, revisions.StatusSucceeded)
	resolved := thread(root.ID)
	if resolved.Status != "resolved" || !resolved.ResolvedByVersion.Valid || resolved.ResolvedByVersion.Int32 != 4 || resolved.ResolvedByName.String != "Ada" || resolved.ResolvedByKind.String != "user" || !resolved.ResolvedAt.Valid {
		t.Fatalf("thread after the request succeeded = %+v", resolved)
	}
	if got := thread(reply.ID); got.ResolvedByVersion.Valid {
		t.Fatalf("a reply carries resolved_by_version: %+v", got)
	}

	other := newComment(pgtype.UUID{})
	failedOther := sendFromThread(other.ID, "fail")
	waitForStatus(t, svc, dash, failedOther.ID, revisions.StatusFailed)
	if row := thread(other.ID); row.Status != "open" || row.ResolvedByVersion.Valid {
		t.Fatalf("a failed request touched the thread: %+v", row)
	}

	earlier := newComment(pgtype.UUID{})
	d.Exec(t, "UPDATE dashboard_comments SET status = 'resolved', resolved_at = now(), resolved_by_kind = 'user', resolved_by_id = $2, resolved_by_name = 'Grace', resolved_by_version = 1 WHERE id = $1", earlier.ID, author)
	before := thread(earlier.ID)
	again := sendFromThread(earlier.ID, "go")
	waitForStatus(t, svc, dash, again.ID, revisions.StatusSucceeded)
	after := thread(earlier.ID)
	if after.ResolvedByName.String != "Grace" || after.ResolvedByVersion.Int32 != 1 || !after.UpdatedAt.Time.Equal(before.UpdatedAt.Time) {
		t.Fatalf("an already resolved thread changed: before %+v, after %+v", before, after)
	}
}
