package runsession

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/url"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/claims"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	busyRunCalls   = 40
	busyPoolConns  = 4
	otherCallLimit = 5 * time.Second
)

func smallPool(t *testing.T, d *dbtest.Database, conns int) *db.DB {
	t.Helper()
	u, err := url.Parse(d.URL())
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("pool_max_conns", strconv.Itoa(conns))
	u.RawQuery = q.Encode()
	pool, err := db.Open(context.Background(), db.Config{URL: u.String()}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestABusyRunDoesNotStarveOtherRunsOrItsOwnLeaseReport(t *testing.T) {
	d := dbtest.New(t)
	r := newRigOn(t, d, smallPool(t, d, busyPoolConns))
	busy := r.revise()
	other := r.suggest()
	r.read(busy, 0)
	r.read(other, 0)

	rowLock, err := d.SQLDB(t).Begin()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rowLock.Rollback() })
	if _, err := rowLock.Exec("SELECT 1 FROM run_sessions WHERE agent_request_id = $1 FOR UPDATE", mustUUID(t, busy.ID)); err != nil {
		t.Fatal(err)
	}

	var calls sync.WaitGroup
	failures := make(chan error, busyRunCalls)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	for n := range busyRunCalls {
		calls.Go(func() {
			input := json.RawMessage(`{"q":"` + strconv.Itoa(n) + `"}`)
			if _, err := r.svc.Call(ctx, busy, Call{Name: "list_metrics", Input: input}, readOK); err != nil {
				failures <- err
			}
		})
	}
	time.Sleep(300 * time.Millisecond)

	otherCtx, otherCancel := context.WithTimeout(context.Background(), otherCallLimit)
	defer otherCancel()
	if _, err := r.svc.Call(otherCtx, other, Call{Name: "list_metrics", Input: json.RawMessage(`{"q":"x"}`)}, readOK); err != nil {
		t.Fatalf("another run's call waited behind the busy run: %v", err)
	}

	leaseCtx, leaseCancel := context.WithTimeout(context.Background(), otherCallLimit)
	defer leaseCancel()
	holder := claims.Claimer{Kind: revisions.AssignGrant, ID: grantID, Person: userID}
	answer, err := r.claims.Report(leaseCtx, holder, claims.ReportInput{RunID: busy.ID, Attempt: busy.Attempt})
	if err != nil || answer.Stop {
		t.Fatalf("the busy run's lease report = %+v, %v", answer, err)
	}

	if err := rowLock.Rollback(); err != nil {
		t.Fatal(err)
	}
	calls.Wait()
	close(failures)
	for err := range failures {
		t.Errorf("a queued call of the busy run failed: %v", err)
	}
}
