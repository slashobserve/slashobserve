package db_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

const lockWait = 150 * time.Millisecond

func advisoryLocks(t *testing.T, database *dbtest.Database) int {
	t.Helper()
	var n int
	if err := database.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM pg_locks WHERE locktype = 'advisory' AND pid <> pg_backend_pid()`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func holdLock(t *testing.T, d *db.DB, name string) (release func()) {
	t.Helper()
	held := make(chan struct{})
	free := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- d.WithSessionLock(t.Context(), name, func(context.Context) error {
			close(held)
			<-free
			return nil
		})
	}()
	select {
	case <-held:
	case <-time.After(10 * time.Second):
		t.Fatal("the first holder never got the lock")
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			close(free)
			if err := <-done; err != nil {
				t.Errorf("first holder: %v", err)
			}
		})
	}
}

func TestSessionLockMakesASecondHolderOfTheSameNameWait(t *testing.T) {
	database := dbtest.New(t)
	release := holdLock(t, database.DB, "provision")
	defer release()

	entered := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- database.WithSessionLock(t.Context(), "provision", func(context.Context) error {
			close(entered)
			return nil
		})
	}()
	select {
	case <-entered:
		t.Fatal("the second holder ran while the first held the lock")
	case <-time.After(lockWait):
	}
	release()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("the second holder never got the lock after the first returned")
	}
	if err := <-done; err != nil {
		t.Errorf("second holder: %v", err)
	}
}

func TestSessionLockDoesNotMakeOtherNamesWait(t *testing.T) {
	database := dbtest.New(t)
	release := holdLock(t, database.DB, "provision")
	defer release()

	ran := false
	if err := database.WithSessionLock(t.Context(), "service_accounts", func(context.Context) error {
		ran = true
		return nil
	}); err != nil || !ran {
		t.Fatalf("ran %v, err %v", ran, err)
	}
}

func TestSessionLockWaitEndsWithTheContext(t *testing.T) {
	database := dbtest.New(t)
	release := holdLock(t, database.DB, "provision")
	defer release()

	ctx, cancel := context.WithTimeout(t.Context(), lockWait)
	defer cancel()
	err := database.WithSessionLock(ctx, "provision", func(context.Context) error {
		t.Error("fn ran without the lock")
		return nil
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
}

func TestSessionLockIsReleasedWhenFnReturns(t *testing.T) {
	database := dbtest.New(t)
	boom := errors.New("boom")
	if err := database.WithSessionLock(t.Context(), "provision", func(context.Context) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want fn's error", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for advisoryLocks(t, database) != 0 {
		if time.Now().After(deadline) {
			t.Fatal("an advisory lock is still held after fn returned")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestSessionLockLeavesAFloorSizedPoolUsable(t *testing.T) {
	database := dbtest.New(t)
	d, err := openWith(t, db.Config{URL: database.URL() + "&pool_max_conns=2"})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	err = d.WithSessionLock(t.Context(), "provision", func(ctx context.Context) error {
		var wg sync.WaitGroup
		errs := make([]error, 4)
		for i := range errs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, errs[i] = d.Queries().GetInstance(ctx)
			}()
		}
		wg.Wait()
		return errors.Join(errs...)
	})
	if err != nil {
		t.Fatalf("pool queries inside the lock: %v", err)
	}
}
