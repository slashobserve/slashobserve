package db_test

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

func pooledStatementTimeout(t *testing.T, d *db.DB) (setting string, backend int) {
	t.Helper()
	err := d.Pool().QueryRow(t.Context(), "SELECT current_setting('statement_timeout'), pg_backend_pid()").Scan(&setting, &backend)
	if err != nil {
		t.Fatalf("read statement_timeout: %v", err)
	}
	return setting, backend
}

func readWithinSetting(t *testing.T, d *db.DB, timeout time.Duration) string {
	t.Helper()
	var setting string
	err := d.ReadWithin(t.Context(), timeout, func(tx db.Rows) error {
		rows, err := tx.Query(t.Context(), "SELECT current_setting('statement_timeout')")
		if err != nil {
			return err
		}
		setting, err = pgx.CollectExactlyOneRow(rows, pgx.RowTo[string])
		return err
	})
	if err != nil {
		t.Fatalf("ReadWithin: %v", err)
	}
	return setting
}

func TestReadWithinLeavesThePooledConnectionsStatementTimeoutAlone(t *testing.T) {
	database := dbtest.New(t)
	d, err := openWith(t, db.Config{URL: database.URL() + "&pool_max_conns=1"})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	before, backend := pooledStatementTimeout(t, d)

	if inside := readWithinSetting(t, d, 1234*time.Millisecond); inside != "1234ms" {
		t.Fatalf("statement_timeout inside ReadWithin = %q", inside)
	}
	slow := d.ReadWithin(t.Context(), 10*time.Millisecond, func(tx db.Rows) error {
		rows, err := tx.Query(t.Context(), "SELECT pg_sleep(1)")
		if err != nil {
			return err
		}
		rows.Close()
		return rows.Err()
	})
	var pgErr *pgconn.PgError
	if !errors.As(slow, &pgErr) || pgErr.Code != "57014" {
		t.Fatalf("a slow read under a 10ms timeout returned %v", slow)
	}

	after, again := pooledStatementTimeout(t, d)
	if again != backend {
		t.Fatalf("the pool of one used backend %d, then %d", backend, again)
	}
	if after != before {
		t.Errorf("statement_timeout on the reused connection is %q, was %q", after, before)
	}
}
