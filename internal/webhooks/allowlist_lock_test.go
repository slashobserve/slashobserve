package webhooks

import (
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func holdingTheAllowlistLock(t *testing.T, r *rig) (release func()) {
	t.Helper()
	held := make(chan struct{})
	free := make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		finished <- r.db.DB.InTx(t.Context(), func(q *dbsqlc.Queries) error {
			if err := q.LockOutboundDomains(t.Context()); err != nil {
				return err
			}
			close(held)
			<-free
			return nil
		})
	}()
	<-held
	var once sync.Once
	release = func() {
		once.Do(func() {
			close(free)
			if err := <-finished; err != nil {
				t.Error(err)
			}
		})
	}
	t.Cleanup(release)
	return release
}

func expectBlockedUntil(t *testing.T, name string, done <-chan error, release func()) {
	t.Helper()
	select {
	case err := <-done:
		t.Fatalf("%s finished (%v) while the outbound allowlist was locked", name, err)
	case <-time.After(300 * time.Millisecond):
	}
	release()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("%s after the lock was released: %v", name, err)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("%s did not finish after the lock was released", name)
	}
}

func TestCreateWaitsForTheOutboundAllowlistLockSoAHostCannotBeReleasedUnderIt(t *testing.T) {
	r := newRig(t)
	release := holdingTheAllowlistLock(t, r)
	done := make(chan error, 1)
	go func() {
		_, err := r.svc.Create(t.Context(), adminOne, Input{Name: "Pipeline", URL: receiverURL, EventTypes: []string{"finding.created"}})
		done <- err
	}()
	expectBlockedUntil(t, "create", done, release)
}

func TestUpdatingTheAddressWaitsForTheOutboundAllowlistLock(t *testing.T) {
	r := newRig(t)
	created := r.create(t, "finding.created")
	release := holdingTheAllowlistLock(t, r)
	done := make(chan error, 1)
	address := receiverURL + "/other"
	go func() {
		_, err := r.svc.Update(t.Context(), adminOne, r.uuid(t, created.Webhook.ID), Patch{URL: &address})
		done <- err
	}()
	expectBlockedUntil(t, "update", done, release)
}
