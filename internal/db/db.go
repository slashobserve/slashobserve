package db

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"math"
	"sync/atomic"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

const (
	minPoolConns    = 16
	poolMaxConnsKey = "pool_max_conns"
)

var (
	ErrMigrationsPending = errors.New("database migrations are pending")
	ErrDirectURL         = errors.New("direct database connection failed")
	ErrDirectURLMismatch = errors.New("the direct database URL reaches another database")
)

type Config struct {
	URL           string
	DirectURL     string
	MaxConnsFloor int
}

type DB struct {
	pool     *pgxpool.Pool
	direct   *pgxpool.Pool
	sqlDB    *sql.DB
	migrator *goose.Provider
	queries  *dbsqlc.Queries
	logger   *slog.Logger

	migrationsApplied atomic.Bool
}

func Open(ctx context.Context, dbCfg Config, logger *slog.Logger) (*DB, error) {
	cfg, err := poolConfig(dbCfg.URL, dbCfg.MaxConnsFloor)
	if err != nil {
		return nil, fmt.Errorf("configure database pool: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("configure database pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	direct, err := openDirect(ctx, pool, dbCfg.DirectURL)
	if err != nil {
		pool.Close()
		return nil, err
	}
	sqlDB := stdlib.OpenDBFromPool(direct)
	migrator, err := newMigrator(sqlDB, logger)
	if err != nil {
		_ = sqlDB.Close()
		closePools(pool, direct)
		return nil, err
	}
	return &DB{pool: pool, direct: direct, sqlDB: sqlDB, migrator: migrator, queries: dbsqlc.New(pool), logger: logger}, nil
}

func poolConfig(databaseURL string, floor int) (*pgxpool.Config, error) {
	connCfg, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	// Workers waiting on the claim lock each hold a pooled connection, so the CPU-derived default leaves little
	// headroom for request handlers on small hosts. An explicit pool_max_conns still wins.
	if _, explicit := connCfg.RuntimeParams[poolMaxConnsKey]; !explicit {
		cfg.MaxConns = max(cfg.MaxConns, minPoolConns, int32(max(0, min(floor, math.MaxInt32))))
	}
	return cfg, nil
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
		goose.WithGoMigrations(evolveSpecsMigration()),
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

// FailedMigrationVersion returns the version of the migration that failed, when err says which one did.
func FailedMigrationVersion(err error) (int64, bool) {
	var partial *goose.PartialError
	if errors.As(err, &partial) && partial.Failed != nil {
		return partial.Failed.Source.Version, true
	}
	return 0, false
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

func (d *DB) Stat() *pgxpool.Stat {
	return d.pool.Stat()
}

func (d *DB) Close() {
	if err := d.sqlDB.Close(); err != nil {
		d.logger.Warn("close database handle", logattrs.Failure("close_failed", err)...)
	}
	closePools(d.pool, d.direct)
}
