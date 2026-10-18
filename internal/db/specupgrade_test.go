package db_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

const (
	dashboardA = "00000000-0000-4000-8000-00000000000a"
	dashboardB = "00000000-0000-4000-8000-00000000000b"
)

var legacyNames = []string{"minimal.json", "shop-overview.json", "checkout-service-health.json"}

func readUpgradeFile(t *testing.T, rel string) string {
	t.Helper()
	root, err := os.OpenRoot("../../pkg/spec/testdata/upgrade")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	data, err := root.ReadFile(rel)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func decodeJSON(t *testing.T, text string) any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("decode %s: %v", text, err)
	}
	return v
}

func seedVersion(t *testing.T, d *dbtest.Database, dashboardID string, version int, specText string) {
	t.Helper()
	d.Exec(t, `INSERT INTO dashboard_versions (dashboard_id, version, parent_version, spec, summary, author_kind, author_id, author_name)
VALUES ($1::uuid, $2, CASE WHEN $2 = 1 THEN NULL ELSE $2 - 1 END, $3::jsonb, 'seed', 'user', gen_random_uuid(), 'seed')`,
		dashboardID, version, specText)
}

func seedDashboard(t *testing.T, d *dbtest.Database, dashboardID string, latestVersion int) {
	t.Helper()
	d.Exec(t, `INSERT INTO dashboards (id, title, latest_version) VALUES ($1::uuid, 'T', $2)`, dashboardID, latestVersion)
}

func runUpgrade(t *testing.T, sqlDB *sql.DB) error {
	t.Helper()
	tx, err := sqlDB.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.UpgradeStoredSpecs(context.Background(), tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func storedSpec(t *testing.T, sqlDB *sql.DB, dashboardID string, version int) string {
	t.Helper()
	var text string
	err := sqlDB.QueryRow(`SELECT spec::text FROM dashboard_versions WHERE dashboard_id = $1::uuid AND version = $2`,
		dashboardID, version).Scan(&text)
	if err != nil {
		t.Fatal(err)
	}
	return text
}

func TestUpgradeStoredSpecs(t *testing.T) {
	d := dbtest.New(t)
	seedDashboard(t, d, dashboardA, 3)
	seedDashboard(t, d, dashboardB, 1)
	for i, name := range legacyNames {
		seedVersion(t, d, dashboardA, i+1, readUpgradeFile(t, "legacy/"+name))
	}
	checkout := readUpgradeFile(t, "evolved/checkout-service-health.json")
	seedVersion(t, d, dashboardB, 1, checkout)
	sqlDB := d.SQLDB(t)

	if err := runUpgrade(t, sqlDB); err != nil {
		t.Fatal(err)
	}

	for i, name := range legacyNames {
		want := decodeJSON(t, readUpgradeFile(t, "evolved/"+name))
		if got := decodeJSON(t, storedSpec(t, sqlDB, dashboardA, i+1)); !reflect.DeepEqual(got, want) {
			t.Errorf("dashboard A version %d differs from the %s golden", i+1, name)
		}
	}
	if got := decodeJSON(t, storedSpec(t, sqlDB, dashboardB, 1)); !reflect.DeepEqual(got, decodeJSON(t, checkout)) {
		t.Error("the already evolved dashboard B version 1 changed")
	}
}

func rowVersions(t *testing.T, sqlDB *sql.DB) map[string]string {
	t.Helper()
	rows, err := sqlDB.Query(`SELECT dashboard_id::text || '/' || version, xmin::text FROM dashboard_versions`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	versions := map[string]string{}
	for rows.Next() {
		var key, xmin string
		if err := rows.Scan(&key, &xmin); err != nil {
			t.Fatal(err)
		}
		versions[key] = xmin
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return versions
}

func TestUpgradeStoredSpecsIsIdempotent(t *testing.T) {
	d := dbtest.New(t)
	seedDashboard(t, d, dashboardA, len(legacyNames))
	seedDashboard(t, d, dashboardB, 1)
	for i, name := range legacyNames {
		seedVersion(t, d, dashboardA, i+1, readUpgradeFile(t, "legacy/"+name))
	}
	seedVersion(t, d, dashboardB, 1, readUpgradeFile(t, "evolved/checkout-service-health.json"))
	sqlDB := d.SQLDB(t)

	if err := runUpgrade(t, sqlDB); err != nil {
		t.Fatal(err)
	}
	first := make([]string, len(legacyNames))
	for i := range legacyNames {
		first[i] = storedSpec(t, sqlDB, dashboardA, i+1)
	}
	written := rowVersions(t, sqlDB)
	if err := runUpgrade(t, sqlDB); err != nil {
		t.Fatalf("second run: %v", err)
	}
	for i := range legacyNames {
		if got := storedSpec(t, sqlDB, dashboardA, i+1); !bytes.Equal([]byte(got), []byte(first[i])) {
			t.Errorf("version %d changed on the second run", i+1)
		}
	}
	if got := rowVersions(t, sqlDB); !reflect.DeepEqual(got, written) {
		t.Errorf("the second run rewrote rows: xmin was %v, now %v", written, got)
	}
}

func TestUpgradeStoredSpecsPages(t *testing.T) {
	restore := db.SetSpecUpgradeBatch(2)
	defer restore()
	d := dbtest.New(t)
	seedDashboard(t, d, dashboardA, 3)
	seedDashboard(t, d, dashboardB, 2)
	legacy := readUpgradeFile(t, "legacy/minimal.json")
	want := decodeJSON(t, readUpgradeFile(t, "evolved/minimal.json"))
	for version := 1; version <= 3; version++ {
		seedVersion(t, d, dashboardA, version, legacy)
	}
	for version := 1; version <= 2; version++ {
		seedVersion(t, d, dashboardB, version, legacy)
	}
	sqlDB := d.SQLDB(t)

	if err := runUpgrade(t, sqlDB); err != nil {
		t.Fatal(err)
	}

	for _, id := range []string{dashboardA, dashboardB} {
		for version := 1; version <= 3; version++ {
			if id == dashboardB && version == 3 {
				continue
			}
			if got := decodeJSON(t, storedSpec(t, sqlDB, id, version)); !reflect.DeepEqual(got, want) {
				t.Errorf("dashboard %s version %d was not upgraded", id, version)
			}
		}
	}
}

func TestUpgradeStoredSpecsFailsLoudly(t *testing.T) {
	d := dbtest.New(t)
	seedDashboard(t, d, dashboardA, 2)
	legacy := readUpgradeFile(t, "legacy/minimal.json")
	const broken = `{"timeRange": {}, "time": {}}`
	seedVersion(t, d, dashboardA, 1, legacy)
	seedVersion(t, d, dashboardA, 2, broken)
	sqlDB := d.SQLDB(t)

	err := runUpgrade(t, sqlDB)
	if err == nil {
		t.Fatal("UpgradeStoredSpecs succeeded on an ambiguous spec")
	}
	if !strings.Contains(err.Error(), dashboardA) || !strings.Contains(err.Error(), " version 2") {
		t.Errorf("error %q does not name the dashboard and version", err)
	}
	if got := decodeJSON(t, storedSpec(t, sqlDB, dashboardA, 2)); !reflect.DeepEqual(got, decodeJSON(t, broken)) {
		t.Error("the broken row changed although the migration failed")
	}
	if got := decodeJSON(t, storedSpec(t, sqlDB, dashboardA, 1)); !reflect.DeepEqual(got, decodeJSON(t, legacy)) {
		t.Error("the earlier good row was upgraded although the migration failed")
	}
}

func TestUpgradeStoredSpecsReportsNoSpecValuesWhenTheUpgradedSpecIsInvalid(t *testing.T) {
	d := dbtest.New(t)
	seedDashboard(t, d, dashboardA, 2)
	legacy := readUpgradeFile(t, "legacy/minimal.json")
	invalid := strings.Replace(legacy, `"now-15m"`, `"SECRET-not-a-time"`, 1)
	if invalid == legacy {
		t.Fatal("the legacy fixture no longer carries the time range the test rewrites")
	}
	seedVersion(t, d, dashboardA, 1, invalid)
	seedVersion(t, d, dashboardA, 2, legacy)
	sqlDB := d.SQLDB(t)

	err := runUpgrade(t, sqlDB)
	if err == nil {
		t.Fatal("UpgradeStoredSpecs succeeded although the upgraded spec is invalid")
	}
	if !strings.Contains(err.Error(), dashboardA) || !strings.Contains(err.Error(), " version 1") {
		t.Errorf("error %q does not name the dashboard and version", err)
	}
	for _, want := range []string{"is not a valid Spec", "code "} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q, so the Parse branch was not reached", err, want)
		}
	}
	if strings.Contains(err.Error(), "SECRET") {
		t.Errorf("error %q leaks spec content", err)
	}
	if got := decodeJSON(t, storedSpec(t, sqlDB, dashboardA, 1)); !reflect.DeepEqual(got, decodeJSON(t, invalid)) {
		t.Error("the invalid row changed although the migration failed")
	}
	if got := decodeJSON(t, storedSpec(t, sqlDB, dashboardA, 2)); !reflect.DeepEqual(got, decodeJSON(t, legacy)) {
		t.Error("the legacy row changed although the migration failed")
	}
}

func TestUpgradeStoredSpecsBoundsPagesByBytes(t *testing.T) {
	defer db.SetSpecUpgradeBatchBudget(1)()
	d := dbtest.New(t)
	seedDashboard(t, d, dashboardA, 3)
	legacy := readUpgradeFile(t, "legacy/minimal.json")
	want := decodeJSON(t, readUpgradeFile(t, "evolved/minimal.json"))
	for version := 1; version <= 3; version++ {
		seedVersion(t, d, dashboardA, version, legacy)
	}
	sqlDB := d.SQLDB(t)

	tx, err := sqlDB.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	size, err := db.FirstSpecPageLen(context.Background(), tx)
	_ = tx.Rollback()
	if err != nil {
		t.Fatal(err)
	}
	if size != 1 {
		t.Errorf("first page holds %d rows, want 1 because one spec exceeds the byte budget", size)
	}

	if err := runUpgrade(t, sqlDB); err != nil {
		t.Fatal(err)
	}
	for version := 1; version <= 3; version++ {
		if got := decodeJSON(t, storedSpec(t, sqlDB, dashboardA, version)); !reflect.DeepEqual(got, want) {
			t.Errorf("version %d was not upgraded", version)
		}
	}
}

func appliedEvolveMigrations(t *testing.T, sqlDB *sql.DB) int {
	t.Helper()
	var n int
	if err := sqlDB.QueryRow(`SELECT count(*) FROM goose_db_version WHERE version_id = 10 AND is_applied`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestEvolveMigrationIsRegistered(t *testing.T) {
	d := dbtest.New(t)
	if got := appliedEvolveMigrations(t, d.SQLDB(t)); got != 1 {
		t.Fatalf("migration 10 is applied %d times, want 1", got)
	}
}

func TestEvolveMigrationThroughTheProvider(t *testing.T) {
	ctx := context.Background()
	d := dbtest.NewUnmigrated(t)
	if err := d.MigrateUpTo(ctx, 9); err != nil {
		t.Fatal(err)
	}
	seedDashboard(t, d, dashboardA, len(legacyNames))
	for i, name := range legacyNames {
		seedVersion(t, d, dashboardA, i+1, readUpgradeFile(t, "legacy/"+name))
	}
	sqlDB := d.SQLDB(t)
	if got := appliedEvolveMigrations(t, sqlDB); got != 0 {
		t.Fatalf("migration 10 is applied %d times before the upgrade, want 0", got)
	}

	if err := d.MigrateUpTo(ctx, 10); err != nil {
		t.Fatal(err)
	}

	if got := appliedEvolveMigrations(t, sqlDB); got != 1 {
		t.Errorf("migration 10 is applied %d times after Up, want 1", got)
	}
	for i, name := range legacyNames {
		want := decodeJSON(t, readUpgradeFile(t, "evolved/"+name))
		if got := decodeJSON(t, storedSpec(t, sqlDB, dashboardA, i+1)); !reflect.DeepEqual(got, want) {
			t.Errorf("version %d differs from the %s golden after Up", i+1, name)
		}
	}

	err := d.MigrateDownTo(ctx, 9)
	if err == nil || !strings.Contains(err.Error(), "cannot be reverted") {
		t.Fatalf("DownTo(9) error = %v, want the refusal to revert migration 10", err)
	}
	if got := appliedEvolveMigrations(t, sqlDB); got != 1 {
		t.Errorf("migration 10 is applied %d times after the refused down, want 1", got)
	}
}
