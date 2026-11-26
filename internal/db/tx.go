package db

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const (
	rollbackTimeout = 5 * time.Second
	commitTimeout   = 5 * time.Second
)

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
	// A caller that leaves during COMMIT must not leave its outcome unknown to whoever audits it.
	cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), commitTimeout)
	defer cancel()
	if err := tx.Commit(cctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

type Rows interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func (d *DB) ReadWithin(ctx context.Context, timeout time.Duration, fn func(Rows) error) error {
	tx, err := d.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return fmt.Errorf("begin read: %w", err)
	}
	defer func() {
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rollbackTimeout)
		defer cancel()
		_ = tx.Rollback(rctx)
	}()
	if _, err := tx.Exec(ctx, "SELECT set_config('statement_timeout', $1, true)", strconv.FormatInt(timeout.Milliseconds(), 10)); err != nil {
		return fmt.Errorf("set statement timeout: %w", err)
	}
	return fn(tx)
}
