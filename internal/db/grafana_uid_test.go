package db_test

import (
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

func seedImport(t *testing.T, d *dbtest.Database, dashboardID, uid, createdAt string) {
	t.Helper()
	seedDashboard(t, d, dashboardID, 1)
	seedVersion(t, d, dashboardID, 1, "{}")
	d.Exec(t, `INSERT INTO imports (dashboard_id, version, source, importer_version, original, mapping, summary, datasources, report,
  title, created_by_kind, created_by_id, created_by_name, created_at)
VALUES ($1::uuid, 1, 'grafana', 3, '\x7b7d', '{}', jsonb_build_object('uid', $2::text), '[]', '{}', 'T', 'user', gen_random_uuid(), 'seed', $3::timestamptz)`,
		dashboardID, uid, createdAt)
}

func TestGrafanaUIDBackfillGivesEachUIDToItsFirstImport(t *testing.T) {
	ctx := t.Context()
	d := dbtest.NewUnmigrated(t)
	if err := d.MigrateUpTo(ctx, v010Version); err != nil {
		t.Fatal(err)
	}
	seeds := []struct {
		dashboard, uid, createdAt, want string
	}{
		{"00000000-0000-4000-8000-000000000001", "shop", "2026-01-02T00:00:00Z", ""},
		{"00000000-0000-4000-8000-000000000002", "shop", "2026-01-01T00:00:00Z", "shop"},
		{"00000000-0000-4000-8000-000000000003", "node_exporter-full", "2026-01-01T00:00:00Z", "node_exporter-full"},
		{"00000000-0000-4000-8000-000000000004", "", "2026-01-01T00:00:00Z", ""},
		{"00000000-0000-4000-8000-000000000005", "has space", "2026-01-01T00:00:00Z", ""},
		{"00000000-0000-4000-8000-000000000006", strings.Repeat("a", 41), "2026-01-01T00:00:00Z", ""},
		{"00000000-0000-4000-8000-000000000007", strings.Repeat("b", 40), "2026-01-01T00:00:00Z", strings.Repeat("b", 40)},
	}
	for _, s := range seeds {
		seedImport(t, d, s.dashboard, s.uid, s.createdAt)
	}
	if err := d.MigrateUpTo(ctx, postV010Version); err != nil {
		t.Fatal(err)
	}
	sqlDB := d.SQLDB(t)
	for _, s := range seeds {
		var got string
		if err := sqlDB.QueryRow(`SELECT grafana_uid FROM imports WHERE dashboard_id = $1::uuid`, s.dashboard).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != s.want {
			t.Errorf("dashboard %s with summary uid %q: grafana_uid = %q, want %q", s.dashboard, s.uid, got, s.want)
		}
	}

	_, err := sqlDB.Exec(`UPDATE imports SET grafana_uid = 'shop' WHERE dashboard_id = $1::uuid`, seeds[0].dashboard)
	if constraintOf(err) != "imports_grafana_uid_key" {
		t.Errorf("a second holder of a uid: error = %v, want an imports_grafana_uid_key violation", err)
	}
}
