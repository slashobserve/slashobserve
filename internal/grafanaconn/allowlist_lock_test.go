package grafanaconn_test

import (
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/grafanaconn"
)

func holdingTheAllowlistLock(t *testing.T, r *rig) (release func()) {
	t.Helper()
	held := make(chan struct{})
	free := make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		finished <- r.db.InTx(reading(t.Context()), func(q *dbsqlc.Queries) error {
			if err := q.LockOutboundDomains(reading(t.Context())); err != nil {
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

func expectBlockedUntil(t *testing.T, name string, done <-chan error, release func(), whileBlocked func()) {
	t.Helper()
	select {
	case err := <-done:
		t.Fatalf("%s finished (%v) while the outbound allowlist was locked", name, err)
	case <-time.After(300 * time.Millisecond):
	}
	if whileBlocked != nil {
		whileBlocked()
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

func addForConnection(t *testing.T, r *rig, host string) {
	t.Helper()
	if _, err := r.domains.SaveOnAllowedHost(reading(t.Context()), r.admin, host, func(*dbsqlc.Queries) error { return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestCreatingAConnectionWaitsForTheOutboundAllowlistLockEvenWhenTheHostIsAllowed(t *testing.T) {
	r := newRig(t)
	addForConnection(t, r, grafanaHost)
	release := holdingTheAllowlistLock(t, r)
	done := make(chan error, 1)
	go func() {
		_, err := r.svc.Create(reading(t.Context()), r.admin, grafanaconn.Input{Name: "Production", URL: grafanaURL, Token: grafanaToken})
		done <- err
	}()
	expectBlockedUntil(t, "create", done, release, nil)
}

func TestMovingAConnectionToAnAllowedHostWaitsForTheOutboundAllowlistLock(t *testing.T) {
	r := newRig(t)
	conn := r.connect(t)
	addForConnection(t, r, "grafana2.example.com")
	release := holdingTheAllowlistLock(t, r)
	done := make(chan error, 1)
	newURL, replacement := "https://grafana2.example.com", "glsa_new"
	go func() {
		_, err := r.svc.Update(reading(t.Context()), r.admin, uuidOf(t, conn.ID), grafanaconn.Patch{URL: &newURL, Token: &replacement})
		done <- err
	}()
	expectBlockedUntil(t, "update", done, release, func() {
		row, err := r.db.Queries().GetGrafanaConnection(reading(t.Context()), uuidOf(t, conn.ID))
		if err != nil {
			t.Fatal(err)
		}
		if row.Url != grafanaURL {
			t.Fatalf("the connection already moved to %s while the allowlist was locked", row.Url)
		}
	})
}
