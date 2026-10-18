package dbtest

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/tctest"
)

const (
	image        = "postgres:18.6-alpine3.23@sha256:885cf05d376c7cf27afef02073e6bdac3841252537f16e244fd1c1e6a7c99fb1"
	adminDB      = "postgres"
	templateDB   = "slashobserve_template"
	setupTimeout = 2 * time.Minute
)

var (
	baseURL string
	skipped bool
	counter atomic.Int64
)

// Main starts one Postgres container for the test binary and migrates a template database that every
// New call copies; use it as `os.Exit(dbtest.Main(m))` in TestMain. Under -short nothing starts and New skips.
func Main(m *testing.M) int {
	flag.Parse()
	if testing.Short() {
		skipped = true
		return m.Run()
	}
	ctx, cancel := context.WithTimeout(context.Background(), setupTimeout)
	container, err := tctest.Retry(ctx, func() (*tcpostgres.PostgresContainer, error) {
		return tcpostgres.Run(ctx, image,
			tcpostgres.WithDatabase(adminDB),
			tcpostgres.WithUsername("slashobserve"),
			tcpostgres.WithPassword("slashobserve"),
			tcpostgres.BasicWaitStrategies(),
		)
	})
	if err != nil {
		cancel()
		_ = testcontainers.TerminateContainer(container)
		fmt.Fprintln(os.Stderr, "start postgres:", err)
		return 1
	}
	defer func() { _ = testcontainers.TerminateContainer(container) }()
	baseURL, err = container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		cancel()
		fmt.Fprintln(os.Stderr, "postgres connection string:", err)
		return 1
	}
	err = migrateTemplate(ctx)
	cancel()
	if err != nil {
		fmt.Fprintln(os.Stderr, "migrate the template database:", err)
		return 1
	}
	return m.Run()
}

func migrateTemplate(ctx context.Context) error {
	if err := adminExec(ctx, "CREATE DATABASE "+templateDB); err != nil {
		return err
	}
	d, err := db.Open(ctx, urlFor(templateDB), quietLogger())
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Migrate(ctx)
}

type Database struct {
	*db.DB
	name string
}

func New(t testing.TB) *Database {
	t.Helper()
	return NewWithLogger(t, quietLogger())
}

func NewWithLogger(t testing.TB, logger *slog.Logger) *Database {
	t.Helper()
	return create(t, logger, " TEMPLATE "+templateDB)
}

func NewUnmigrated(t testing.TB) *Database {
	t.Helper()
	return create(t, quietLogger(), "")
}

func create(t testing.TB, logger *slog.Logger, options string) *Database {
	t.Helper()
	if skipped {
		t.Skip("Postgres tests are skipped with -short")
	}
	ctx := context.Background()
	name := fmt.Sprintf("test_%d", counter.Add(1))
	if err := adminExec(ctx, "CREATE DATABASE "+name+options); err != nil {
		t.Fatalf("create database: %v", err)
	}
	d, err := db.Open(ctx, urlFor(name), logger)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() {
		d.Close()
		if err := adminExec(context.Background(), "DROP DATABASE "+name+" WITH (FORCE)"); err != nil {
			t.Errorf("drop database: %v", err)
		}
	})
	return &Database{DB: d, name: name}
}

func (d *Database) Exec(t testing.TB, sql string, args ...any) {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), urlFor(d.name))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	if _, err := conn.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

func (d *Database) SQLDB(t testing.TB) *sql.DB {
	t.Helper()
	cfg, err := pgx.ParseConfig(urlFor(d.name))
	if err != nil {
		t.Fatalf("parse database url: %v", err)
	}
	sqlDB := stdlib.OpenDB(*cfg)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return sqlDB
}

func adminExec(ctx context.Context, sql string) error {
	conn, err := pgx.Connect(ctx, urlFor(adminDB))
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()
	_, err = conn.Exec(ctx, sql)
	return err
}

func urlFor(database string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		panic(err)
	}
	u.Path = "/" + database
	return u.String()
}

func quietLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}
