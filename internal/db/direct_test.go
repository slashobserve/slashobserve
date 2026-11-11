package db_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

const directApplication = "slashobserve_direct"

func openWith(t *testing.T, cfg db.Config) (*db.DB, error) {
	t.Helper()
	d, err := db.Open(t.Context(), cfg, slog.New(slog.DiscardHandler))
	if err == nil {
		t.Cleanup(d.Close)
	}
	return d, err
}

func TestListenRunsOnTheDirectConnectionAndHearsPooledNotifications(t *testing.T) {
	database := dbtest.New(t)
	d, err := openWith(t, db.Config{URL: database.URL(), DirectURL: database.URL() + "&application_name=" + directApplication})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	listening := make(chan struct{})
	payloads := make(chan string, 1)
	done := make(chan error, 1)
	go func() {
		done <- d.Listen(ctx, "direct_test", func() { close(listening) }, func(payload string) { payloads <- payload })
	}()
	select {
	case <-listening:
	case <-time.After(10 * time.Second):
		t.Fatal("Listen did not start")
	}
	if err := d.Queries().NotifyEvent(ctx, dbsqlc.NotifyEventParams{Channel: "direct_test", Payload: "hello"}); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-payloads:
		if got != "hello" {
			t.Fatalf("payload = %q, want hello", got)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the notification sent through the pool never arrived")
	}
	var listeners int
	row := database.SQLDB(t).QueryRow(`SELECT count(*) FROM pg_stat_activity WHERE application_name = $1 AND query LIKE 'LISTEN%'`, directApplication)
	if err := row.Scan(&listeners); err != nil {
		t.Fatal(err)
	}
	if listeners != 1 {
		t.Fatalf("LISTEN backends on the direct connection = %d, want 1", listeners)
	}
	cancel()
	<-done
}

func TestMigrateRunsOverTheDirectConnection(t *testing.T) {
	database := dbtest.NewUnmigrated(t)
	d, err := openWith(t, db.Config{URL: database.URL(), DirectURL: database.URL()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := d.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if _, err := d.Queries().GetInstance(t.Context()); err != nil {
		t.Fatalf("the migrated schema is missing: %v", err)
	}
}

func TestDirectURLToAnotherDatabaseIsRefusedBeforeMigrating(t *testing.T) {
	main := dbtest.New(t)
	other := dbtest.NewUnmigrated(t)
	_, err := openWith(t, db.Config{URL: main.URL(), DirectURL: other.URL()})
	if !errors.Is(err, db.ErrDirectURLMismatch) {
		t.Fatalf("Open = %v, want ErrDirectURLMismatch", err)
	}
	var migrated bool
	if err := other.SQLDB(t).QueryRow(`SELECT to_regclass('goose_db_version') IS NOT NULL`).Scan(&migrated); err != nil {
		t.Fatal(err)
	}
	if migrated {
		t.Fatal("a migration ran on the database the direct URL reaches")
	}
}

func TestDirectURLThatDoesNotParseIsReported(t *testing.T) {
	database := dbtest.New(t)
	_, err := openWith(t, db.Config{URL: database.URL(), DirectURL: "postgres://bob@db.internal:5432/app%zz"})
	var parseErr *pgconn.ParseConfigError
	if !errors.Is(err, db.ErrDirectURL) || !errors.As(err, &parseErr) {
		t.Fatalf("Open = %v, want ErrDirectURL wrapping a parse error", err)
	}
}
