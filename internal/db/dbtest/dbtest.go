package dbtest

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"sync"
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

	onDemand     bool
	onDemandOnce sync.Once
	stopOnDemand func()
	onDemandErr  error
)

// Main starts one Postgres container for the test binary and migrates a template database that every
// New call copies; use it as `os.Exit(dbtest.Main(m))` in TestMain. Under -short nothing starts and New skips.
func Main(m *testing.M) int {
	flag.Parse()
	if testing.Short() {
		skipped = true
		return m.Run()
	}
	stop, err := startContainer()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer stop()
	return m.Run()
}

// MainOnDemand is Main for a package whose tests mostly need no database: the container starts when the first
// test asks for a database and is removed when the binary ends, so the other tests run without Docker.
func MainOnDemand(m *testing.M) int {
	flag.Parse()
	if testing.Short() {
		skipped = true
		return m.Run()
	}
	onDemand = true
	defer func() {
		if stopOnDemand != nil {
			stopOnDemand()
		}
	}()
	return m.Run()
}

func ensureStarted(t testing.TB) {
	t.Helper()
	onDemandOnce.Do(func() { stopOnDemand, onDemandErr = startContainer() })
	if onDemandErr != nil {
		t.Fatal(onDemandErr)
	}
}

func startContainer() (func(), error) {
	ctx, cancel := context.WithTimeout(context.Background(), setupTimeout)
	defer cancel()
	container, err := tctest.Retry(ctx, func() (*tcpostgres.PostgresContainer, error) {
		return tcpostgres.Run(ctx, image,
			tcpostgres.WithDatabase(adminDB),
			tcpostgres.WithUsername("slashobserve"),
			tcpostgres.WithPassword("slashobserve"),
			tcpostgres.BasicWaitStrategies(),
		)
	})
	stop := func() { _ = testcontainers.TerminateContainer(container) }
	if err != nil {
		stop()
		return nil, fmt.Errorf("start postgres: %w", err)
	}
	if baseURL, err = container.ConnectionString(ctx, "sslmode=disable"); err != nil {
		stop()
		return nil, fmt.Errorf("postgres connection string: %w", err)
	}
	if err = migrateTemplate(ctx); err != nil {
		stop()
		return nil, fmt.Errorf("migrate the template database: %w", err)
	}
	return stop, nil
}

func migrateTemplate(ctx context.Context) error {
	if err := adminExec(ctx, "CREATE DATABASE "+templateDB); err != nil {
		return err
	}
	d, err := db.Open(ctx, db.Config{URL: urlFor(templateDB)}, quietLogger())
	if err != nil {
		return err
	}
	defer d.Close()
	_, err = d.Migrate(ctx)
	return err
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

type Template struct {
	prepare func(ctx context.Context, d *db.DB) error
	once    sync.Once
	name    string
	err     error
}

func NewTemplate(prepare func(ctx context.Context, d *db.DB) error) *Template {
	return &Template{prepare: prepare}
}

func (tpl *Template) New(t testing.TB) *Database {
	t.Helper()
	if skipped {
		t.Skip("Postgres tests are skipped with -short")
	}
	if onDemand {
		ensureStarted(t)
	}
	tpl.once.Do(func() { tpl.name, tpl.err = buildTemplate(tpl.prepare) })
	if tpl.err != nil {
		t.Fatalf("prepare the template database: %v", tpl.err)
	}
	return create(t, quietLogger(), " TEMPLATE "+tpl.name)
}

func buildTemplate(prepare func(ctx context.Context, d *db.DB) error) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), setupTimeout)
	defer cancel()
	name := fmt.Sprintf("template_%d", counter.Add(1))
	if err := adminExec(ctx, "CREATE DATABASE "+name); err != nil {
		return "", err
	}
	d, err := db.Open(ctx, db.Config{URL: urlFor(name)}, quietLogger())
	if err != nil {
		return "", err
	}
	defer d.Close()
	return name, prepare(ctx, d)
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
	if onDemand {
		ensureStarted(t)
	}
	ctx := context.Background()
	name := fmt.Sprintf("test_%d", counter.Add(1))
	if err := adminExec(ctx, "CREATE DATABASE "+name+options); err != nil {
		t.Fatalf("create database: %v", err)
	}
	d, err := db.Open(ctx, db.Config{URL: urlFor(name)}, logger)
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

func (d *Database) URL() string {
	return urlFor(d.name)
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

type AuditEvent struct {
	Action     string
	Outcome    string
	Reason     string
	ActorKind  string
	ActorID    string
	Initiator  string
	Origin     string
	TargetKind string
	TargetID   string
	Data       map[string]any
}

func (d *Database) AuditEvents(t testing.TB, action string) []AuditEvent {
	t.Helper()
	rows, err := d.SQLDB(t).QueryContext(context.Background(), `
		SELECT action, outcome, coalesce(reason, ''), actor_kind, coalesce(actor_id::text, ''), initiator, origin,
		       coalesce(target_kind, ''), coalesce(target_id, ''),
		       data::text
		FROM audit_events WHERE action = $1 ORDER BY occurred_at, seq`, action)
	if err != nil {
		t.Fatalf("read audit events: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var out []AuditEvent
	for rows.Next() {
		var e AuditEvent
		var data string
		if err := rows.Scan(&e.Action, &e.Outcome, &e.Reason, &e.ActorKind, &e.ActorID, &e.Initiator, &e.Origin, &e.TargetKind, &e.TargetID, &data); err != nil {
			t.Fatalf("scan audit event: %v", err)
		}
		if err := json.Unmarshal([]byte(data), &e.Data); err != nil {
			t.Fatalf("decode audit event data: %v", err)
		}
		if len(e.Data) == 0 {
			e.Data = nil
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read audit events: %v", err)
	}
	return out
}
