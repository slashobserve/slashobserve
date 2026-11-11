package db

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	directCheckKeyTries = 3
	directUnlockTimeout = 5 * time.Second
)

func openDirect(ctx context.Context, pool *pgxpool.Pool, directURL string) (*pgxpool.Pool, error) {
	if directURL == "" {
		return pool, nil
	}
	cfg, err := pgxpool.ParseConfig(directURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDirectURL, err)
	}
	direct, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDirectURL, err)
	}
	if err := direct.Ping(ctx); err != nil {
		direct.Close()
		return nil, fmt.Errorf("%w: %w", ErrDirectURL, err)
	}
	if err := checkSameDatabase(ctx, pool, direct); err != nil {
		direct.Close()
		return nil, err
	}
	return direct, nil
}

func closePools(pool, direct *pgxpool.Pool) {
	if direct != pool {
		direct.Close()
	}
	pool.Close()
}

// A session lock held on the direct connection blocks the pool's lock attempt only when both reach the same database.
func checkSameDatabase(ctx context.Context, pool, direct *pgxpool.Pool) error {
	conn, err := direct.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrDirectURL, err)
	}
	defer conn.Release()
	key, err := lockRandomKey(ctx, conn)
	if err != nil {
		return err
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), directUnlockTimeout)
		defer cancel()
		if _, err := conn.Exec(unlockCtx, "SELECT pg_advisory_unlock($1)", key); err != nil {
			_ = conn.Conn().Close(unlockCtx)
		}
	}()
	var acquired bool
	if err := pool.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock($1)", key).Scan(&acquired); err != nil {
		return fmt.Errorf("check the direct database URL: %w", err)
	}
	if acquired {
		return ErrDirectURLMismatch
	}
	return nil
}

func lockRandomKey(ctx context.Context, conn *pgxpool.Conn) (int64, error) {
	for range directCheckKeyTries {
		key := rand.Int64() //nolint:gosec // the key only avoids colliding with another advisory lock, it protects nothing
		var locked bool
		if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&locked); err != nil {
			return 0, fmt.Errorf("%w: %w", ErrDirectURL, err)
		}
		if locked {
			return key, nil
		}
	}
	return 0, fmt.Errorf("%w: no free advisory lock key", ErrDirectURL)
}
