package revisions_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/revisions"
)

type claimedWait struct {
	source string
	waited time.Duration
}

type recordingWorkerMetrics struct {
	mu    sync.Mutex
	calls []claimedWait
}

func (r *recordingWorkerMetrics) RequestClaimed(source string, waited time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, claimedWait{source: source, waited: waited})
}

func (r *recordingWorkerMetrics) recorded() []claimedWait {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]claimedWait(nil), r.calls...)
}

func TestWorkerReportsTheQueueWaitOfAFirstAttempt(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, "add a panel")
	metrics := &recordingWorkerMetrics{}

	startWorker(t, svc, d, revisions.WorkerOptions{Metrics: metrics})
	waitForStatus(t, svc, dash, rev.ID, revisions.StatusSucceeded)

	calls := metrics.recorded()
	if len(calls) != 1 || calls[0].source != string(revisions.SourcePrompt) || calls[0].waited < 0 {
		t.Fatalf("queue wait observations = %+v, want one for the prompt with a non-negative wait", calls)
	}
}

func TestWorkerDoesNotReportAReclaimAfterALostLease(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, "add a panel")
	claimed, found := claimOne(t, svc)
	if !found {
		t.Fatal("the request was not claimed")
	}
	d.Exec(t, "UPDATE agent_requests SET lease_until = clock_timestamp() - interval '1 second' WHERE id = $1", claimed.ID)
	metrics := &recordingWorkerMetrics{}

	startWorker(t, svc, d, revisions.WorkerOptions{Metrics: metrics})
	done := waitForStatus(t, svc, dash, rev.ID, revisions.StatusSucceeded)

	if done.Attempts != 2 {
		t.Fatalf("attempts = %d, want the worker to have taken the second attempt", done.Attempts)
	}
	if calls := metrics.recorded(); len(calls) != 0 {
		t.Fatalf("queue wait observations = %+v, want none for a re-claim", calls)
	}
}

func TestWorkerDoesNotReportAReclaimAfterAShutdownRelease(t *testing.T) {
	started := make(chan struct{})
	svc, d := newService(t, reviserFunc(func(ctx context.Context, _ revisions.Request) (revisions.Result, error) {
		close(started)
		<-ctx.Done()
		return revisions.Result{}, ctx.Err()
	}))
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, "add a panel")
	metrics := &recordingWorkerMetrics{}
	stop := startWorker(t, svc, d, revisions.WorkerOptions{Metrics: metrics})
	<-started
	stop()

	next := newTestService(d.DB, succeedWith(2), quietLogger())
	startWorker(t, next, d, revisions.WorkerOptions{Metrics: metrics})
	waitForStatus(t, next, dash, rev.ID, revisions.StatusSucceeded)

	if calls := metrics.recorded(); len(calls) != 1 {
		t.Fatalf("queue wait observations = %+v, want one for the request's first claim only", calls)
	}
}
