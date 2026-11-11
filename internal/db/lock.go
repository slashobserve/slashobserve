package db

import (
	"context"
	"time"
)

const sessionLockCloseTimeout = 5 * time.Second

// Closing the connection releases the lock even when an unlock would fail, and the hijacked connection no longer counts against the pool, so fn can use the pool at its floor.
func (d *DB) WithSessionLock(ctx context.Context, name string, fn func(context.Context) error) error {
	pooled, err := d.direct.Acquire(ctx)
	if err != nil {
		return failOrCancel(ctx, "acquire session lock connection", err)
	}
	conn := pooled.Hijack()
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sessionLockCloseTimeout)
		defer cancel()
		_ = conn.Close(closeCtx)
	}()
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock(hashtextextended('session:' || $1, 0))", name); err != nil {
		return failOrCancel(ctx, "take session lock "+name, err)
	}
	return fn(ctx)
}
