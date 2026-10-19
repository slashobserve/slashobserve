package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const rollbackTimeout = 5 * time.Second

type Savepoint func(fn func(*dbsqlc.Queries) error) error

func (d *DB) InTx(ctx context.Context, fn func(*dbsqlc.Queries) error) error {
	return d.InTxWithSavepoints(ctx, func(q *dbsqlc.Queries, _ Savepoint) error { return fn(q) })
}

// A failed savepoint is rolled back on its own and leaves the transaction usable; its error is returned for the caller to handle.
func (d *DB) InTxWithSavepoints(ctx context.Context, fn func(*dbsqlc.Queries, Savepoint) error) error {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	rollbackOf := func(t pgx.Tx) {
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rollbackTimeout)
		defer cancel()
		if rerr := t.Rollback(rctx); rerr != nil {
			d.logger.WarnContext(ctx, "roll back transaction", logattrs.Failure("rollback_failed", rerr)...)
		}
	}
	rollback := func() { rollbackOf(tx) }
	savepoint := func(sfn func(*dbsqlc.Queries) error) error {
		sp, err := tx.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin savepoint: %w", err)
		}
		defer func() {
			if p := recover(); p != nil {
				rollbackOf(sp)
				panic(p)
			}
		}()
		if err := sfn(dbsqlc.New(sp)); err != nil {
			rollbackOf(sp)
			return err
		}
		if err := sp.Commit(ctx); err != nil {
			return fmt.Errorf("release savepoint: %w", err)
		}
		return nil
	}
	defer func() {
		if p := recover(); p != nil {
			rollback()
			panic(p)
		}
	}()
	if err := fn(dbsqlc.New(tx), savepoint); err != nil {
		rollback()
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}
