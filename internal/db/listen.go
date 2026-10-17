package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	listenCloseTimeout   = 5 * time.Second
	listenLivenessPeriod = 30 * time.Second
	listenPingTimeout    = 10 * time.Second
)

type notificationConn interface {
	WaitForNotification(ctx context.Context) (*pgconn.Notification, error)
	Ping(ctx context.Context) error
}

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
	return waitForNotifications(ctx, conn, channel, listenLivenessPeriod, handle)
}

func waitForNotifications(ctx context.Context, conn notificationConn, channel string, livenessPeriod time.Duration, handle func(payload string)) error {
	for {
		waitCtx, cancel := context.WithTimeout(ctx, livenessPeriod)
		notification, err := conn.WaitForNotification(waitCtx)
		idle := waitCtx.Err() != nil
		cancel()
		if err == nil {
			handle(notification.Payload)
			continue
		}
		if ctx.Err() != nil || !idle {
			return failOrCancel(ctx, "wait for notification on "+channel, err)
		}
		pingCtx, cancelPing := context.WithTimeout(ctx, listenPingTimeout)
		pingErr := conn.Ping(pingCtx)
		cancelPing()
		if pingErr != nil {
			return failOrCancel(ctx, "ping listen connection on "+channel, pingErr)
		}
	}
}

func failOrCancel(ctx context.Context, action string, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	return fmt.Errorf("%s: %w", action, err)
}
