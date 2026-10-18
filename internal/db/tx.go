package db

import (
	"context"
	"fmt"
	"time"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const rollbackTimeout = 5 * time.Second

func (d *DB) InTx(ctx context.Context, fn func(*dbsqlc.Queries) error) error {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	rollback := func() {
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rollbackTimeout)
		defer cancel()
		if rerr := tx.Rollback(rctx); rerr != nil {
			d.logger.WarnContext(ctx, "roll back transaction", logattrs.Failure("rollback_failed", rerr)...)
		}
	}
	defer func() {
		if p := recover(); p != nil {
			rollback()
			panic(p)
		}
	}()
	if err := fn(dbsqlc.New(tx)); err != nil {
		rollback()
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}
