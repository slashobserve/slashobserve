package revisions_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

type claimedWait struct {
	source string
	waited time.Duration
}

type endedRun struct {
	kind    string
	outcome string
}

type recordingMetrics struct {
	mu    sync.Mutex
	waits []claimedWait
	runs  []endedRun
}

func (r *recordingMetrics) RequestClaimed(source string, waited time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.waits = append(r.waits, claimedWait{source: source, waited: waited})
}

func (r *recordingMetrics) AgentRun(kind, outcome string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs = append(r.runs, endedRun{kind: kind, outcome: outcome})
}

func (r *recordingMetrics) recordedWaits() []claimedWait {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]claimedWait(nil), r.waits...)
}

func (r *recordingMetrics) recordedRuns() []endedRun {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]endedRun(nil), r.runs...)
}

func meteredService(t *testing.T, reviser revisions.Reviser, metrics *recordingMetrics) (*revisions.Service, *dbtest.Database) {
	t.Helper()
	d := dbtest.New(t)
	return newTestService(d.DB, reviser, quietLogger(), revisions.WithMetrics(metrics)), d
}

func TestTheServiceReportsTheQueueWaitOfAFirstAttempt(t *testing.T) {
	metrics := &recordingMetrics{}
	svc, d := meteredService(t, succeedWith(2), metrics)
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, "add a panel")

	startWorker(t, svc, d, revisions.WorkerOptions{})
	waitForStatus(t, svc, dash, rev.ID, revisions.StatusSucceeded)

	calls := metrics.recordedWaits()
	if len(calls) != 1 || calls[0].source != string(revisions.SourcePrompt) || calls[0].waited < 0 {
		t.Fatalf("queue wait observations = %+v, want one for the prompt with a non-negative wait", calls)
	}
}

func TestTheServiceDoesNotReportAReclaimAfterALostLease(t *testing.T) {
	metrics := &recordingMetrics{}
	svc, d := meteredService(t, succeedWith(2), metrics)
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, "add a panel")
	claimed, found := claimOne(t, svc)
	if !found {
		t.Fatal("the request was not claimed")
	}
	d.Exec(t, "UPDATE agent_requests SET lease_until = clock_timestamp() - interval '1 second' WHERE id = $1", claimed.ID)

	startWorker(t, svc, d, revisions.WorkerOptions{})
	done := waitForStatus(t, svc, dash, rev.ID, revisions.StatusSucceeded)

	if done.Attempts != 2 {
		t.Fatalf("attempts = %d, want the worker to have taken the second attempt", done.Attempts)
	}
	if calls := metrics.recordedWaits(); len(calls) != 0 {
		t.Fatalf("queue wait observations = %+v, want none for a re-claim", calls)
	}
}

func TestTheServiceDoesNotReportAReclaimAfterAShutdownRelease(t *testing.T) {
	started := make(chan struct{})
	metrics := &recordingMetrics{}
	svc, d := meteredService(t, reviserFunc(func(ctx context.Context, _ revisions.Request) (revisions.Result, error) {
		close(started)
		<-ctx.Done()
		return revisions.Result{}, ctx.Err()
	}), metrics)
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, "add a panel")
	stop := startWorker(t, svc, d, revisions.WorkerOptions{})
	<-started
	stop()

	next := newTestService(d.DB, succeedWith(2), quietLogger(), revisions.WithMetrics(metrics))
	startWorker(t, next, d, revisions.WorkerOptions{})
	waitForStatus(t, next, dash, rev.ID, revisions.StatusSucceeded)

	if calls := metrics.recordedWaits(); len(calls) != 1 {
		t.Fatalf("queue wait observations = %+v, want one for the request's first claim only", calls)
	}
}

func TestEveryEndedRunIsCountedByItsKindAndOutcome(t *testing.T) {
	cases := []struct {
		name    string
		reviser revisions.Reviser
		status  revisions.Status
		want    endedRun
	}{
		{"a saved version", succeedWith(2), revisions.StatusSucceeded, endedRun{kind: "revise", outcome: "succeeded"}},
		{"a failure with its code", reviserFunc(func(context.Context, revisions.Request) (revisions.Result, error) {
			return revisions.Result{FailureCode: revisions.FailureRateLimited}, errors.New("failed")
		}), revisions.StatusFailed, endedRun{kind: "revise", outcome: "rate_limited"}},
		{"a failure without a code", reviserFunc(func(context.Context, revisions.Request) (revisions.Result, error) {
			return revisions.Result{}, errors.New("boom")
		}), revisions.StatusFailed, endedRun{kind: "revise", outcome: "internal"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			metrics := &recordingMetrics{}
			svc, d := meteredService(t, tc.reviser, metrics)
			dash := seedDashboard(t, d, 1)
			rev := submit(t, svc, dash, "add a panel")

			startWorker(t, svc, d, revisions.WorkerOptions{})
			waitForStatus(t, svc, dash, rev.ID, tc.status)

			if got := metrics.recordedRuns(); len(got) != 1 || got[0] != tc.want {
				t.Fatalf("ended runs = %+v, want %+v once", got, tc.want)
			}
		})
	}
}
