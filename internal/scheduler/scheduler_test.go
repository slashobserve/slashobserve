package scheduler

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/logtest"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

type recordedMetrics struct {
	mu       sync.Mutex
	started  []string
	finished []string
}

func (m *recordedMetrics) JobStarted(job string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.started = append(m.started, job)
}

func (m *recordedMetrics) JobFinished(job, outcome string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.finished = append(m.finished, job+"="+outcome)
}

func (m *recordedMetrics) finishedOf(job string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for _, f := range m.finished {
		if strings.HasPrefix(f, job+"=") {
			out = append(out, f)
		}
	}
	return out
}

func newScheduler(t *testing.T, d *dbtest.Database, instanceID string) (*Scheduler, *recordedMetrics) {
	t.Helper()
	logger, _ := logtest.New()
	metrics := &recordedMetrics{}
	return New(logger, d.DB, metrics, instanceID), metrics
}

func row(t *testing.T, d *dbtest.Database, name string) dbsqlc.Job {
	t.Helper()
	j, err := d.Queries().GetJob(t.Context(), name)
	if err != nil {
		t.Fatalf("read job %s: %v", name, err)
	}
	return j
}

func runUntil(t *testing.T, s *Scheduler, done <-chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		s.Run(ctx)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Error("the scheduler never reached the probe job")
	}
	cancel()
	<-stopped
}

func signal() (func(context.Context) error, chan struct{}) {
	ch := make(chan struct{})
	var once sync.Once
	return func(context.Context) error {
		once.Do(func() { close(ch) })
		return nil
	}, ch
}

func TestTwoSchedulersRunADueJobOnce(t *testing.T) {
	d := dbtest.New(t)
	var runs atomic.Int32
	count := func(context.Context) error {
		runs.Add(1)
		return nil
	}
	a, _ := newScheduler(t, d, "server-a")
	b, _ := newScheduler(t, d, "server-b")
	a.Register("a-shared", time.Hour, count)
	b.Register("a-shared", time.Hour, count)
	probeA, doneA := signal()
	probeB, doneB := signal()
	a.Register("z-probe-a", time.Hour, probeA)
	b.Register("z-probe-b", time.Hour, probeB)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); runUntil(t, a, doneA) }()
	go func() { defer wg.Done(); runUntil(t, b, doneB) }()
	wg.Wait()

	if got := runs.Load(); got != 1 {
		t.Fatalf("the shared job ran %d times, want once", got)
	}
	j := row(t, d, "a-shared")
	if j.LastOutcome != OutcomeSucceeded || j.LeasedBy != "" || j.LeaseUntil.Valid || j.LastError != "" {
		t.Errorf("row after the run %+v", j)
	}
	if got := j.NextRunAt.Time.Sub(j.LastStartedAt.Time); got != time.Hour {
		t.Errorf("next run %v after the start, want 1h", got)
	}
}

func TestRegisterKeepsTheStoredNextRun(t *testing.T) {
	d := dbtest.New(t)
	d.Exec(t, `INSERT INTO jobs (name, every_seconds, next_run_at) VALUES ('a-kept', 60, now() + interval '1 hour')`)
	before := row(t, d, "a-kept")
	s, _ := newScheduler(t, d, "server-a")
	var keptRan atomic.Bool
	s.Register("a-kept", 2*time.Hour, func(context.Context) error {
		keptRan.Store(true)
		return nil
	})
	probe, done := signal()
	s.Register("b-probe", time.Hour, probe)
	runUntil(t, s, done)

	if keptRan.Load() {
		t.Error("a job whose stored next run is in the future ran")
	}
	after := row(t, d, "a-kept")
	if !after.NextRunAt.Time.Equal(before.NextRunAt.Time) || after.EverySeconds != 7200 {
		t.Errorf("after Register: next run %v (was %v), every %ds, want the stored next run and 7200s", after.NextRunAt.Time, before.NextRunAt.Time, after.EverySeconds)
	}
	if j := row(t, d, HeartbeatJob); j.EverySeconds != 300 {
		t.Errorf("heartbeat row %+v, want a row every 300s", j)
	}
}

func TestFailedJobIsRecordedAndDueAgainAfterItsInterval(t *testing.T) {
	cases := []struct {
		name      string
		fn        func(context.Context) error
		errorType string
	}{
		{"returns an error", func(context.Context) error { return errors.New("token sok_secret rejected") }, "error_type=*errors.errorString"},
		{"panics", func(context.Context) error { panic("token sok_secret rejected") }, "error_type=scheduler.panicError panic_type=string"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := dbtest.New(t)
			s, metrics := newScheduler(t, d, "server-a")
			s.Register("broken", time.Hour, tc.fn)
			if err := s.RunNow(t.Context(), "broken"); err != nil {
				t.Fatal(err)
			}
			j := row(t, d, "broken")
			if j.LastOutcome != OutcomeFailed || j.LastError != tc.errorType || strings.Contains(j.LastError, "sok_secret") {
				t.Errorf("outcome %q, error %q, want failed and %q", j.LastOutcome, j.LastError, tc.errorType)
			}
			if got := j.NextRunAt.Time.Sub(j.LastStartedAt.Time); got != time.Hour {
				t.Errorf("due again %v after the start, want 1h", got)
			}
			if j.LeasedBy != "" || j.LeaseUntil.Valid || !j.LastFinishedAt.Valid {
				t.Errorf("lease after the failure %+v", j)
			}
			if got := metrics.finishedOf("broken"); !slices.Equal(got, []string{"broken=failed"}) {
				t.Errorf("metrics %v", got)
			}
		})
	}
}

func TestSuccessAfterAFailureClearsTheError(t *testing.T) {
	d := dbtest.New(t)
	s, _ := newScheduler(t, d, "server-a")
	fail := true
	s.Register("flaky", time.Hour, func(context.Context) error {
		if fail {
			return errors.New("boom")
		}
		return nil
	})
	if err := s.RunNow(t.Context(), "flaky"); err != nil {
		t.Fatal(err)
	}
	fail = false
	if err := s.RunNow(t.Context(), "flaky"); err != nil {
		t.Fatal(err)
	}
	if j := row(t, d, "flaky"); j.LastOutcome != OutcomeSucceeded || j.LastError != "" {
		t.Errorf("row %+v, want succeeded with no error", j)
	}
}

func TestExpiredLeaseIsTakenOver(t *testing.T) {
	d := dbtest.New(t)
	d.Exec(t, `INSERT INTO jobs (name, every_seconds, next_run_at, leased_by, lease_until, last_started_at)
		VALUES ('a-stuck', 600, now() - interval '11 minutes', 'crashed-server', now() - interval '1 second', now() - interval '601 seconds')`)
	s, _ := newScheduler(t, d, "server-b")
	run, done := signal()
	s.Register("a-stuck", 10*time.Minute, run)
	runUntil(t, s, done)

	j := row(t, d, "a-stuck")
	if j.LastOutcome != OutcomeSucceeded || j.LeasedBy != "" || time.Since(j.LastStartedAt.Time) > time.Minute {
		t.Errorf("row after the takeover %+v", j)
	}
}

func TestLiveLeaseIsNotTakenOver(t *testing.T) {
	d := dbtest.New(t)
	d.Exec(t, `INSERT INTO jobs (name, every_seconds, next_run_at, leased_by, lease_until, last_started_at)
		VALUES ('busy', 600, now() - interval '1 minute', 'server-a', now() + interval '5 minutes', now() - interval '1 minute')`)
	s, _ := newScheduler(t, d, "server-b")
	s.Register("busy", 10*time.Minute, func(context.Context) error {
		t.Error("a job leased by another server ran")
		return nil
	})
	if err := s.RunNow(t.Context(), "busy"); !errors.Is(err, ErrJobLeased) {
		t.Errorf("RunNow on a leased job: %v, want ErrJobLeased", err)
	}
	if err := s.RunNow(t.Context(), "missing"); !errors.Is(err, ErrUnknownJob) {
		t.Errorf("RunNow on an unregistered job: %v, want ErrUnknownJob", err)
	}
}

func TestJobIsCancelledAtLeaseEnd(t *testing.T) {
	d := dbtest.New(t)
	s, _ := newScheduler(t, d, "server-a")
	var entered, deadline time.Time
	s.Register("slow", time.Second, func(ctx context.Context) error {
		entered = time.Now()
		deadline, _ = ctx.Deadline()
		<-ctx.Done()
		return ctx.Err()
	})
	if err := s.RunNow(t.Context(), "slow"); err != nil {
		t.Fatal(err)
	}
	j := row(t, d, "slow")
	if j.LastOutcome != OutcomeFailed || j.LastError != "error_type=context.deadlineExceededError" {
		t.Errorf("outcome %q, error %q, want a failed run cut at the deadline", j.LastOutcome, j.LastError)
	}
	if lease := deadline.Sub(entered); lease <= 0 || lease > time.Second {
		t.Errorf("the run's deadline is %v after it started, want at most the 1s lease", lease)
	}
}

func TestLeaseIsCappedAtTenMinutes(t *testing.T) {
	d := dbtest.New(t)
	s, _ := newScheduler(t, d, "server-a")
	var lease dbsqlc.Job
	s.Register("nightly", 24*time.Hour, func(context.Context) error {
		lease = row(t, d, "nightly")
		return nil
	})
	if err := s.RunNow(t.Context(), "nightly"); err != nil {
		t.Fatal(err)
	}
	if got := lease.LeaseUntil.Time.Sub(lease.LastStartedAt.Time); got != maxLease || lease.LeasedBy != "server-a" {
		t.Errorf("lease of %v by %q while running, want 10m by server-a", got, lease.LeasedBy)
	}
}

func TestShutdownReleasesTheLeaseAndKeepsTheJobDue(t *testing.T) {
	d := dbtest.New(t)
	s, metrics := newScheduler(t, d, "server-a")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	s.Register("interrupted", time.Hour, func(runCtx context.Context) error {
		cancel()
		<-runCtx.Done()
		return runCtx.Err()
	})
	if err := s.RunNow(ctx, "interrupted"); err != nil {
		t.Fatal(err)
	}
	j := row(t, d, "interrupted")
	if j.LeasedBy != "" || j.LeaseUntil.Valid || j.LastOutcome != "" || j.LastFinishedAt.Valid || j.NextRunAt.Time.After(j.LastStartedAt.Time) {
		t.Errorf("row after a shutdown %+v, want the lease released and the job still due", j)
	}
	if got := metrics.finishedOf("interrupted"); !slices.Equal(got, []string{"interrupted=interrupted"}) {
		t.Errorf("metrics %v", got)
	}
}

func TestRegisterRefusesBadJobs(t *testing.T) {
	cases := []struct {
		name  string
		job   string
		every time.Duration
	}{
		{"under a second", "fast", 500 * time.Millisecond},
		{"fractional seconds", "odd", 1500 * time.Millisecond},
		{"a name already taken", HeartbeatJob, time.Minute},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := New(nil, nil, NopMetrics{}, "server-a")
			defer func() {
				if recover() == nil {
					t.Error("Register accepted the job")
				}
			}()
			s.Register(tc.job, tc.every, func(context.Context) error { return nil })
		})
	}
}
