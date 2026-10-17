package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

const listenCloseTimeout = 5 * time.Second

func (d *DB) Listen(ctx context.Context, channel string, onListening func(), handle func(payload string)) error {
	pooled, err := d.pool.Acquire(ctx)
	if err != nil {
		return failOrCancel(ctx, "acquire listen connection", err)
	}
	conn := pooled.Hijack()
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), listenCloseTimeout)
		defer cancel()
		_ = conn.Close(closeCtx)
	}()
	if _, err := conn.Exec(ctx, "LISTEN "+pgx.Identifier{channel}.Sanitize()); err != nil {
		return failOrCancel(ctx, "listen on "+channel, err)
	}
	onListening()
	for {
		notification, err := conn.WaitForNotification(ctx)
		if err != nil {
			return failOrCancel(ctx, "wait for notification on "+channel, err)
		}
		handle(notification.Payload)
	}
}

func failOrCancel(ctx context.Context, action string, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	return fmt.Errorf("%s: %w", action, err)
}
