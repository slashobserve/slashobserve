package db_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/logtest"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

func TestRollbackFailureLogsOnlyCodeAndType(t *testing.T) {
	logger, logs := logtest.New()
	database := dbtest.NewWithLogger(t, logger)
	cause := errors.New("handler failed")

	err := database.InTx(t.Context(), func(q *dbsqlc.Queries) error {
		database.Exec(t, `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = current_database() AND pid <> pg_backend_pid() AND state = 'idle in transaction'`)
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := q.GetInstance(context.Background()); err != nil {
				return cause
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("the transaction connection was not terminated")
		return nil
	})
	if !errors.Is(err, cause) {
		t.Fatalf("InTx = %v, want the handler error", err)
	}
	record := logs.Find(t, "roll back transaction")
	logs.AssertSafeFailure(t, record, "rollback_failed", "terminating connection", "administrator command", "conn closed")
}
