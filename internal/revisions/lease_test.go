package revisions_test

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/logtest"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func leaseUntil(t *testing.T, d *dbtest.Database, id string) time.Time {
	t.Helper()
	row, err := d.Queries().GetAgentRequestByID(context.Background(), mustUUID(t, id))
	if err != nil {
		t.Fatal(err)
	}
	return row.LeaseUntil.Time
}

func claimOne(t *testing.T, svc *revisions.Service) (dbsqlc.AgentRequest, bool) {
	t.Helper()
	row, found, err := svc.Claim(context.Background())
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	return row, found
}

func TestClaimSetsTheLeaseToItsTTL(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, "add a panel")

	before := time.Now()
	if _, found := claimOne(t, svc); !found {
		t.Fatal("nothing claimed")
	}
	got := leaseUntil(t, d, rev.ID).Sub(before)
	if got < revisions.LeaseTTL-5*time.Second || got > revisions.LeaseTTL+5*time.Second {
		t.Fatalf("lease runs %s from the claim, want about %s", got, revisions.LeaseTTL)
	}
}

func TestRenewingExtendsTheLease(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, "add a panel")
	claimed, _ := claimOne(t, svc)
	d.Exec(t, "UPDATE agent_requests SET lease_until = clock_timestamp() + interval '2 seconds' WHERE id = $1", claimed.ID)

	before := time.Now()
	if err := svc.RenewLease(context.Background(), claimed); err != nil {
		t.Fatalf("renew: %v", err)
	}
	if got := leaseUntil(t, d, rev.ID).Sub(before); got < revisions.LeaseTTL-5*time.Second {
		t.Fatalf("renewed lease runs %s from now, want about %s", got, revisions.LeaseTTL)
	}
}

func TestRenewingAfterTheAttemptChangedLosesTheLease(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, "add a panel")
	first, _ := claimOne(t, svc)
	d.Exec(t, "UPDATE agent_requests SET lease_until = clock_timestamp() - interval '1 second' WHERE id = $1", first.ID)
	second, found := claimOne(t, svc)
	if !found || second.Attempts != 2 {
		t.Fatalf("reclaim = %+v, found %v", second, found)
	}

	if err := svc.RenewLease(context.Background(), first); !errors.Is(err, revisions.ErrLeaseLost) {
		t.Fatalf("renew by the first attempt = %v, want ErrLeaseLost", err)
	}
	if err := svc.RenewLease(context.Background(), second); err != nil {
		t.Fatalf("renew by the second attempt: %v", err)
	}
	if got := leaseUntil(t, d, rev.ID); !got.After(time.Now()) {
		t.Fatalf("lease %s was not extended", got)
	}
}

func TestRenewingAFinishedRequestLosesTheLease(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	submit(t, svc, dash, "add a panel")
	claimed, _ := claimOne(t, svc)
	d.Exec(t, "UPDATE agent_requests SET state = 'failed', status = 'failed' WHERE id = $1", claimed.ID)

	if err := svc.RenewLease(context.Background(), claimed); !errors.Is(err, revisions.ErrLeaseLost) {
		t.Fatalf("renew = %v, want ErrLeaseLost", err)
	}
}

func TestRunningRequestIsReclaimedOnlyAfterItsLeaseExpires(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	submit(t, svc, dash, "add a panel")
	claimed, _ := claimOne(t, svc)

	if _, found := claimOne(t, svc); found {
		t.Fatal("a request with a live lease was claimed again")
	}
	if err := svc.RenewLease(context.Background(), claimed); err != nil {
		t.Fatal(err)
	}
	if _, found := claimOne(t, svc); found {
		t.Fatal("a renewed request was claimed again")
	}

	d.Exec(t, "UPDATE agent_requests SET lease_until = clock_timestamp() - interval '1 second' WHERE id = $1", claimed.ID)
	again, found := claimOne(t, svc)
	if !found || again.ID != claimed.ID || again.Attempts != 2 {
		t.Fatalf("reclaim = %+v, found %v", again, found)
	}
}

func startHeartbeatWorker(t *testing.T, svc *revisions.Service, d *dbtest.Database, logger *slog.Logger, heartbeat time.Duration) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	worker := revisions.NewWorker(svc, d.Listen, logger, revisions.WorkerOptions{Workers: 1, Poll: 20 * time.Millisecond})
	worker.SetHeartbeat(heartbeat)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		worker.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		wg.Wait()
	})
}

func TestLongRunKeepsItsLeaseAndIsNotReclaimed(t *testing.T) {
	var calls atomic.Int32
	svc, d := newService(t, reviserFunc(func(ctx context.Context, _ revisions.Request) (revisions.Result, error) {
		calls.Add(1)
		select {
		case <-time.After(1500 * time.Millisecond):
			return revisions.Result{Version: 2}, nil
		case <-ctx.Done():
			return revisions.Result{}, ctx.Err()
		}
	}))
	svc.SetLeaseTTL(300 * time.Millisecond)
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, "add a panel")

	startHeartbeatWorker(t, svc, d, quietLogger(), 50*time.Millisecond)
	startHeartbeatWorker(t, svc, d, quietLogger(), 50*time.Millisecond)
	done := waitForStatus(t, svc, dash, rev.ID, revisions.StatusSucceeded)

	if got := calls.Load(); got != 1 || done.Attempts != 1 {
		t.Fatalf("the run was started %d times with %d attempts, want one of each", got, done.Attempts)
	}
}

func TestLostLeaseCancelsTheRunAndDropsItsWork(t *testing.T) {
	logger, logs := logtest.New()
	started := make(chan struct{})
	var canceled atomic.Bool
	svc, d := newService(t, reviserFunc(func(ctx context.Context, _ revisions.Request) (revisions.Result, error) {
		close(started)
		<-ctx.Done()
		canceled.Store(true)
		return revisions.Result{Version: 2}, nil
	}))
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, "add a panel")
	startHeartbeatWorker(t, svc, d, logger, 20*time.Millisecond)
	<-started

	d.Exec(t, "UPDATE agent_requests SET attempts = 2 WHERE id = $1", mustUUID(t, rev.ID))

	waitFor(t, "the run to be canceled", canceled.Load)
	record := findEventually(t, logs, "agent request lease lost; dropping this attempt's work")
	logs.AssertSafeFailure(t, record, "agent_request_lease_lost", "add a panel")
	time.Sleep(100 * time.Millisecond)
	got := get(t, svc, dash, rev.ID)
	if got.Status != revisions.StatusRunning || got.Attempts != 2 || got.ResultVersion != nil {
		t.Fatalf("the dropped attempt wrote to the request: %+v", got)
	}
	assertNoFailureCode(t, svc, rev.ID)
}

func TestAReviserThatLostItsLeaseDropsItsWork(t *testing.T) {
	logger, logs := logtest.New()
	var calls atomic.Int32
	svc, d := newService(t, reviserFunc(func(context.Context, revisions.Request) (revisions.Result, error) {
		calls.Add(1)
		return revisions.Result{FailureCode: revisions.FailureLeaseExpired}, revisions.ErrLeaseLost
	}))
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, "add a panel")
	startHeartbeatWorker(t, svc, d, logger, 20*time.Millisecond)

	record := findEventually(t, logs, "agent request lease lost; dropping this attempt's work")
	logs.AssertSafeFailure(t, record, "agent_request_lease_lost", "add a panel")
	time.Sleep(100 * time.Millisecond)
	got := get(t, svc, dash, rev.ID)
	if got.Status != revisions.StatusRunning || got.ResultVersion != nil || calls.Load() != 1 {
		t.Fatalf("the dropped attempt wrote to the request: %+v after %d runs", got, calls.Load())
	}
	assertNoFailureCode(t, svc, rev.ID)
}
