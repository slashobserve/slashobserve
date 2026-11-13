package revisions_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
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
	row, found, err := svc.ClaimRow(context.Background())
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
	if _, err := svc.RenewLease(context.Background(), claimed); err != nil {
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

	if _, err := svc.RenewLease(context.Background(), first); !errors.Is(err, revisions.ErrLeaseLost) {
		t.Fatalf("renew by the first attempt = %v, want ErrLeaseLost", err)
	}
	if _, err := svc.RenewLease(context.Background(), second); err != nil {
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

	if _, err := svc.RenewLease(context.Background(), claimed); !errors.Is(err, revisions.ErrLeaseLost) {
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
	if _, err := svc.RenewLease(context.Background(), claimed); err != nil {
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
