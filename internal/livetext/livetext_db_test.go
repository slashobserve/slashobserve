package livetext_test

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/livetext"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

func startStore(t *testing.T, d *dbtest.Database) *livetext.Store {
	t.Helper()
	store := livetext.New(d.DB, d.Listen, slog.New(slog.DiscardHandler))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		store.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return store
}

func waitForText(t *testing.T, store *livetext.Store, run string, attempt int, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if got, ok := store.Text(run, attempt); ok && got == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	got, _ := store.Text(run, attempt)
	t.Fatalf("text for %s = %q, want %q", run, got, want)
}

func TestTwoStoresOnOneDatabaseSeeEachOthersText(t *testing.T) {
	d := dbtest.New(t)
	first := startStore(t, d)
	second := startStore(t, d)
	ctx := context.Background()

	var firstSeesSecond bool
	for i := 0; i < 50 && !firstSeesSecond; i++ {
		if err := second.Append(ctx, "run-1", 1, "probe"); err != nil {
			t.Fatal(err)
		}
		time.Sleep(100 * time.Millisecond)
		_, firstSeesSecond = first.Text("run-1", 1)
	}
	if !firstSeesSecond {
		t.Fatal("the listeners never came up")
	}

	if err := first.Append(ctx, "run-2", 1, "from the first, "); err != nil {
		t.Fatal(err)
	}
	if err := second.Append(ctx, "run-2", 1, "and the second"); err != nil {
		t.Fatal(err)
	}
	waitForText(t, first, "run-2", 1, "from the first, and the second")
	waitForText(t, second, "run-2", 1, "from the first, and the second")
}
