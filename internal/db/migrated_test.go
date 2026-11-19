package db_test

import (
	"context"
	"sync"
	"testing"

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

var migratedTemplates sync.Map

func newMigratedTo(t testing.TB, version int64) *dbtest.Database {
	t.Helper()
	tpl, _ := migratedTemplates.LoadOrStore(version, dbtest.NewTemplate(func(ctx context.Context, d *db.DB) error {
		return d.MigrateUpTo(ctx, version)
	}))
	return tpl.(*dbtest.Template).New(t)
}
