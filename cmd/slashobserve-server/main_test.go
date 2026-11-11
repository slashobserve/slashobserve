package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pressly/goose/v3"

	"github.com/slashobserve/slashobserve/internal/config"
	"github.com/slashobserve/slashobserve/internal/datasource/provision"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/logtest"
)

func logExit(t *testing.T, err error) (map[string]any, *logtest.Capture) {
	t.Helper()
	logger, logs := logtest.New()
	logger.Error("server exiting", exitAttrs(err)...)
	return logs.Find(t, "server exiting"), logs
}

func TestUnparsableDatabaseURLIsReportedWithoutTheURL(t *testing.T) {
	url := fmt.Sprintf("postgres://%s:%s@db.internal:5432/app%%zz", "bob", "hunter2")
	_, err := db.Open(context.Background(), db.Config{URL: url}, slog.New(slog.DiscardHandler))
	if err == nil {
		t.Fatal("db.Open accepted an invalid URL")
	}
	record, logs := logExit(t, databaseExit(config.Config{DatabaseURL: url}, err))
	if record["error_code"] != "invalid_database_url" {
		t.Errorf("record = %v", record)
	}
	logs.AssertAbsent(t, "bob", "hunter2", "db.internal", "app%zz")
}

func TestUnreachableDatabaseIsReportedByHostAndErrno(t *testing.T) {
	url := fmt.Sprintf("postgres://%s:%s@127.0.0.1:1/appdb", "bob", "hunter2")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := db.Open(ctx, db.Config{URL: url}, slog.New(slog.DiscardHandler))
	if err == nil {
		t.Fatal("db.Open connected to a closed port")
	}
	record, logs := logExit(t, databaseExit(config.Config{DatabaseURL: url}, err))
	logs.AssertSafeFailure(t, record, "database_unavailable", "bob", "hunter2", "appdb")
	if record["db_host"] != "127.0.0.1" || record["db_port"] != float64(1) {
		t.Errorf("db_host/db_port = %v/%v, want 127.0.0.1/1", record["db_host"], record["db_port"])
	}
	if record["errno"] != syscall.ECONNREFUSED.Error() {
		t.Errorf("errno = %v, want %q", record["errno"], syscall.ECONNREFUSED.Error())
	}
}

func TestMigrationFailureNamesTheVersionAndSQLState(t *testing.T) {
	failed := fmt.Errorf("apply migrations: %w", &goose.PartialError{
		Failed: &goose.MigrationResult{Source: &goose.Source{Version: 17}},
		Err:    &pgconn.PgError{Code: "42P07", Message: `relation "secret_table" already exists`},
	})
	record, logs := logExit(t, migrationExit(failed))
	logs.AssertSafeFailure(t, record, "migration_failed", "secret_table")
	if record["migration_version"] != float64(17) || record["sqlstate"] != "42P07" {
		t.Errorf("record = %v, want migration_version 17 and sqlstate 42P07", record)
	}
}

func TestSpecUpgradeMigrationFailureNamesTheDashboardAndVersionWithoutSpecContent(t *testing.T) {
	failed := fmt.Errorf("apply migrations: %w", &goose.PartialError{
		Failed: &goose.MigrationResult{Source: &goose.Source{Version: 10}},
		Err: &db.SpecUpgradeError{
			DashboardID: "00000000-0000-4000-8000-00000000000a",
			Version:     3,
			Err:         errors.New("the upgraded spec is not a valid Spec: SECRET"),
		},
	})
	record, logs := logExit(t, migrationExit(failed))
	logs.AssertSafeFailure(t, record, "migration_failed", "SECRET")
	if record["migration_version"] != float64(10) || record["dashboard_id"] != "00000000-0000-4000-8000-00000000000a" || record["dashboard_version"] != float64(3) {
		t.Errorf("record = %v, want migration_version 10, the dashboard id and version 3", record)
	}
}

func TestDatasourceFailureNamesTheUIDAndOnlyServerWrittenText(t *testing.T) {
	sources := []config.Datasource{
		{UID: "prom_bad", Type: "prometheus", Prometheus: &config.PrometheusDatasource{URL: fmt.Sprintf("ftp://%s:%s@metrics.internal", "user", "pw")}},
	}
	_, err := provision.Build(slog.New(slog.DiscardHandler), sources)
	if err == nil {
		t.Fatal("Build accepted an invalid datasource")
	}
	record, logs := logExit(t, datasourcesExit(fmt.Errorf("wrapped: %w", err)))
	if record["datasource_uid"] != "prom_bad" || record["detail"] != "prometheus: url scheme must be http or https" {
		t.Errorf("record = %v", record)
	}
	logs.AssertAbsent(t, "user:pw", "metrics.internal")

	_, err = provision.Build(slog.New(slog.DiscardHandler), []config.Datasource{
		{UID: "ch_bad", Type: "clickhouse", ClickHouse: &config.ClickHouseDatasource{Username: "u"}},
	})
	record, _ = logExit(t, datasourcesExit(err))
	if record["datasource_uid"] != "ch_bad" || record["detail"] != "clickhouse address is required" {
		t.Errorf("record = %v, want the ClickHouse constructor message", record)
	}

	opaque := datasourcesExit(&provision.BuildError{UID: "ch", Err: errors.New("dial db.internal: refused")})
	record, logs = logExit(t, opaque)
	if _, ok := record["detail"]; ok || record["datasource_uid"] != "ch" || record["error_type"] == nil {
		t.Errorf("record = %v, want the uid and type without detail", record)
	}
	logs.AssertAbsent(t, "db.internal")
}

func TestConfigErrorKeepsOnlyTheServerWrittenMessage(t *testing.T) {
	_, err := config.Load(func(key string) (string, bool) {
		if key == "SLASHOBSERVE_PUBLIC_URL" {
			return "http://admin:pa55w0rd@host.example/?token=abc", true
		}
		return "", false
	})
	if err == nil {
		t.Fatal("config.Load accepted an invalid configuration")
	}
	record, logs := logExit(t, described("invalid_config", err))
	detail, _ := record["detail"].(string)
	if !strings.Contains(detail, "SLASHOBSERVE_PUBLIC_URL: must not have a query, fragment or user info") {
		t.Errorf("detail = %q", detail)
	}
	logs.AssertAbsent(t, "admin", "pa55w0rd", "host.example", "token=abc")
}

func TestUnknownExitErrorIsReportedByTypeOnly(t *testing.T) {
	record, logs := logExit(t, errors.New("dial tcp 10.1.2.3:5432: secret"))
	logs.AssertSafeFailure(t, record, "internal", "10.1.2.3", "secret")
}

func TestReportedEffortIsOnlyAnAnthropicSetting(t *testing.T) {
	tests := []struct {
		provider string
		effort   string
		want     string
	}{
		{config.LLMProviderAnthropic, "high", "high"},
		{config.LLMProviderAnthropic, "", "medium"},
		{config.LLMProviderOpenAI, "medium", ""},
		{config.LLMProviderReplay, "medium", ""},
		{"", "medium", ""},
	}
	for _, tt := range tests {
		t.Run(tt.provider+"/"+tt.effort, func(t *testing.T) {
			if got := reportedEffort(config.LLM{Provider: tt.provider, Effort: tt.effort}); got != tt.want {
				t.Fatalf("reportedEffort = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVersionLineHasTheVersionAndTheCommit(t *testing.T) {
	defer func(v, c string) { version, commit = v, c }(version, commit)
	version, commit = "0.1.0", "abc1234"
	if got := versionLine(); got != "0.1.0 (abc1234)" {
		t.Fatalf("versionLine() = %q", got)
	}
}

func TestPoolFloorGrowsWithTheWorkers(t *testing.T) {
	tests := []struct{ workers, want int }{{1, 10}, {2, 12}, {8, 24}, {64, 136}}
	for _, tt := range tests {
		if got := poolFloor(tt.workers); got != tt.want {
			t.Errorf("poolFloor(%d) = %d, want %d", tt.workers, got, tt.want)
		}
	}
}

func TestDirectURLToAnotherDatabaseIsReportedWithoutEitherURL(t *testing.T) {
	cfg := config.Config{
		DatabaseURL:       fmt.Sprintf("postgres://%s:%s@pool.internal:6432/appdb", "bob", "hunter2"),
		DatabaseDirectURL: fmt.Sprintf("postgres://%s:%s@pg.internal:5432/otherdb", "carol", "swordfish"),
	}
	record, logs := logExit(t, databaseExit(cfg, fmt.Errorf("open: %w", db.ErrDirectURLMismatch)))
	if record["error_code"] != "database_direct_url_mismatch" {
		t.Errorf("record = %v", record)
	}
	logs.AssertAbsent(t, "bob", "hunter2", "pool.internal", "appdb", "carol", "swordfish", "pg.internal", "otherdb")
}

func TestUnparsableDirectURLIsReportedWithoutTheURL(t *testing.T) {
	url := fmt.Sprintf("postgres://%s:%s@pg.internal:5432/app%%zz", "carol", "swordfish")
	_, parseErr := pgconn.ParseConfig(url)
	if parseErr == nil {
		t.Fatal("pgconn.ParseConfig accepted an invalid URL")
	}
	cfg := config.Config{DatabaseURL: fmt.Sprintf("postgres://%s:%s@pool.internal:6432/appdb", "bob", "hunter2"), DatabaseDirectURL: url}
	record, logs := logExit(t, databaseExit(cfg, fmt.Errorf("%w: %w", db.ErrDirectURL, parseErr)))
	if record["error_code"] != "invalid_database_direct_url" {
		t.Errorf("record = %v", record)
	}
	logs.AssertAbsent(t, "bob", "hunter2", "pool.internal", "carol", "swordfish", "pg.internal", "app%zz")
}

func TestUnreachableDirectURLIsReportedByTheDirectHost(t *testing.T) {
	url := fmt.Sprintf("postgres://%s:%s@127.0.0.1:1/otherdb", "carol", "swordfish")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, connErr := pgconn.Connect(ctx, url)
	if connErr == nil {
		t.Fatal("pgconn.Connect reached a closed port")
	}
	cfg := config.Config{DatabaseURL: fmt.Sprintf("postgres://%s:%s@pool.internal:6432/appdb", "bob", "hunter2"), DatabaseDirectURL: url}
	record, logs := logExit(t, databaseExit(cfg, fmt.Errorf("%w: %w", db.ErrDirectURL, connErr)))
	logs.AssertSafeFailure(t, record, "database_unavailable", "bob", "hunter2", "pool.internal", "appdb", "carol", "swordfish", "otherdb")
	if record["db_host"] != "127.0.0.1" || record["db_port"] != float64(1) || record["connection"] != "direct" {
		t.Errorf("record = %v, want the direct host and port", record)
	}
}
