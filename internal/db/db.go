package db

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"sync/atomic"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

var ErrMigrationsPending = errors.New("database migrations are pending")

type DB struct {
	pool     *pgxpool.Pool
	sqlDB    *sql.DB
	migrator *goose.Provider
	queries  *dbsqlc.Queries
	logger   *slog.Logger

	migrationsApplied atomic.Bool
}

func Open(ctx context.Context, databaseURL string, logger *slog.Logger) (*DB, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("configure database pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	sqlDB := stdlib.OpenDBFromPool(pool)
	migrator, err := newMigrator(sqlDB, logger)
	if err != nil {
		_ = sqlDB.Close()
		pool.Close()
		return nil, err
	}
	return &DB{pool: pool, sqlDB: sqlDB, migrator: migrator, queries: dbsqlc.New(pool), logger: logger}, nil
}

func newMigrator(sqlDB *sql.DB, logger *slog.Logger) (*goose.Provider, error) {
	migrations, err := fs.Sub(migrationFiles, "migrations")
	if err != nil {
		return nil, fmt.Errorf("load embedded migrations: %w", err)
	}
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return nil, fmt.Errorf("create migration lock: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations,
		goose.WithSessionLocker(locker),
		goose.WithDisableGlobalRegistry(true),
		goose.WithSlog(logger),
	)
	if err != nil {
		return nil, fmt.Errorf("create migration provider: %w", err)
	}
	return provider, nil
}

func (d *DB) Migrate(ctx context.Context) error {
	results, err := d.migrator.Up(ctx)
	if err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	d.migrationsApplied.Store(true)
	for _, result := range results {
		d.logger.InfoContext(ctx, "applied migration",
			slog.Int64("version", result.Source.Version),
			slog.Duration("duration", result.Duration))
	}
	return nil
}

func (d *DB) Ping(ctx context.Context) error {
	return d.pool.Ping(ctx)
}

func (d *DB) CheckMigrations(ctx context.Context) error {
	if d.migrationsApplied.Load() {
		return nil
	}
	pending, err := d.migrator.HasPending(ctx)
	if err != nil {
		return fmt.Errorf("check migrations: %w", err)
	}
	if pending {
		return ErrMigrationsPending
	}
	return nil
}

func (d *DB) Queries() *dbsqlc.Queries {
	return d.queries
}

func (d *DB) Close() {
	if err := d.sqlDB.Close(); err != nil {
		d.logger.Warn("close database handle", slog.Any("error", err))
	}
	d.pool.Close()
}
