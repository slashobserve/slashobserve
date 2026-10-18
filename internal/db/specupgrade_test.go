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

func TestUpgradeStoredSpecsIsIdempotent(t *testing.T) {
	d := dbtest.New(t)
	seedDashboard(t, d, dashboardA, len(legacyNames))
	for i, name := range legacyNames {
		seedVersion(t, d, dashboardA, i+1, readUpgradeFile(t, "legacy/"+name))
	}
	sqlDB := d.SQLDB(t)

	if err := runUpgrade(t, sqlDB); err != nil {
		t.Fatal(err)
	}
	first := make([]string, len(legacyNames))
	for i := range legacyNames {
		first[i] = storedSpec(t, sqlDB, dashboardA, i+1)
	}
	if err := runUpgrade(t, sqlDB); err != nil {
		t.Fatalf("second run: %v", err)
	}
	for i := range legacyNames {
		if got := storedSpec(t, sqlDB, dashboardA, i+1); !bytes.Equal([]byte(got), []byte(first[i])) {
			t.Errorf("version %d changed on the second run", i+1)
		}
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
	seedDashboard(t, d, dashboardA, 1)
	const broken = `{"timeRange": {}, "time": {}}`
	seedVersion(t, d, dashboardA, 1, broken)
	sqlDB := d.SQLDB(t)

	err := runUpgrade(t, sqlDB)
	if err == nil {
		t.Fatal("UpgradeStoredSpecs succeeded on an ambiguous spec")
	}
	if !strings.Contains(err.Error(), dashboardA) || !strings.Contains(err.Error(), " version 1") {
		t.Errorf("error %q does not name the dashboard and version", err)
	}
	if got := decodeJSON(t, storedSpec(t, sqlDB, dashboardA, 1)); !reflect.DeepEqual(got, decodeJSON(t, broken)) {
		t.Error("the row changed although the migration failed")
	}
}

func TestEvolveMigrationIsRegistered(t *testing.T) {
	d := dbtest.New(t)
	var latest int64
	if err := d.SQLDB(t).QueryRow(`SELECT max(version_id) FROM goose_db_version`).Scan(&latest); err != nil {
		t.Fatal(err)
	}
	if latest != 10 {
		t.Fatalf("latest applied migration = %d, want 10", latest)
	}
}
