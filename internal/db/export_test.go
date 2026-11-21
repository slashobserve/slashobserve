package db

import (
	"context"
	"database/sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

func SetSpecUpgradeBatch(n int) (restore func()) {
	previous := specUpgradeBatch
	specUpgradeBatch = n
	return func() { specUpgradeBatch = previous }
}

func SetSpecUpgradeBatchBudget(n int) (restore func()) {
	previous := specUpgradeBatchBudget
	specUpgradeBatchBudget = n
	return func() { specUpgradeBatchBudget = previous }
}

func FirstSpecPageLen(ctx context.Context, tx *sql.Tx) (int, error) {
	page, err := readSpecPage(ctx, tx, nil)
	return len(page), err
}

func (d *DB) MigrateUpTo(ctx context.Context, version int64) error {
	_, err := d.migrator.UpTo(ctx, version)
	return err
}

func (d *DB) MigrateDownTo(ctx context.Context, version int64) error {
	_, err := d.migrator.DownTo(ctx, version)
	return err
}

func (d *DB) Pool() *pgxpool.Pool {
	return d.pool
}
