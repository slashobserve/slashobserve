package db_test

import (
	"testing"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

const (
	v010Version     = 24
	postV010Version = 25
)

func TestUpgradeFromV010BackfillsItsData(t *testing.T) {
	ctx := t.Context()
	d := dbtest.NewUnmigrated(t)
	if err := d.MigrateUpTo(ctx, v010Version); err != nil {
		t.Fatal(err)
	}
	const (
		named     = "00000000-0000-4000-8000-0000000000a1"
		unnamed   = "00000000-0000-4000-8000-0000000000a2"
		dashboard = "00000000-0000-4000-8000-000000000001"
	)
	d.Exec(t, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, name) VALUES
  ($1::uuid, 'https://idp', 'a', 'ada@example.com', 'Ada'),
  ($2::uuid, 'https://idp', 'b', 'bo@example.com', '')`, named, unnamed)
	d.Exec(t, `INSERT INTO folders (title) VALUES ('My Dashboards')`)
	seedImport(t, d, dashboard, "shop", "2026-01-01T00:00:00Z")
	d.Exec(t, `INSERT INTO agent_requests (dashboard_id, source, prompt, base_version, requested_by_kind, requested_by_id, requested_by_name)
VALUES ($1::uuid, 'prompt', 'p', 1, 'user', $2::uuid, 'Ada')`, dashboard, named)

	if err := d.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	sqlDB := d.SQLDB(t)
	rows, err := sqlDB.Query(`SELECT owner_id::text, title, created_by_name FROM folders WHERE personal ORDER BY owner_id`)
	if err != nil {
		t.Fatal(err)
	}
	type folder struct{ owner, title, createdBy string }
	var personal []folder
	for rows.Next() {
		var f folder
		if err := rows.Scan(&f.owner, &f.title, &f.createdBy); err != nil {
			t.Fatal(err)
		}
		personal = append(personal, f)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	want := []folder{{named, "My Dashboards", "Ada"}, {unnamed, "My Dashboards", "bo@example.com"}}
	if len(personal) != len(want) || personal[0] != want[0] || personal[1] != want[1] {
		t.Errorf("personal folders = %v, want %v", personal, want)
	}

	var sharedFolders int
	var grafanaUID string
	var requestTitle *string
	if err := sqlDB.QueryRow(`SELECT
  (SELECT count(*) FROM folders WHERE title = 'My Dashboards' AND NOT personal),
  (SELECT grafana_uid FROM imports WHERE dashboard_id = $1::uuid),
  (SELECT title FROM agent_requests WHERE dashboard_id = $1::uuid)`, dashboard).Scan(&sharedFolders, &grafanaUID, &requestTitle); err != nil {
		t.Fatal(err)
	}
	if sharedFolders != 1 || grafanaUID != "shop" || requestTitle != nil {
		t.Errorf("after the upgrade: shared My Dashboards folders %d, grafana_uid %q, request title %v; want 1, shop, none",
			sharedFolders, grafanaUID, requestTitle)
	}
}

func TestPostV010MigrationRollsBack(t *testing.T) {
	ctx := t.Context()
	d := dbtest.NewUnmigrated(t)
	if err := d.MigrateUpTo(ctx, postV010Version); err != nil {
		t.Fatal(err)
	}
	seedImport(t, d, "00000000-0000-4000-8000-000000000001", "shop", "2026-01-01T00:00:00Z")
	d.Exec(t, `INSERT INTO grafana_connections (name, url, token_ciphertext) VALUES ('g', 'https://grafana.example.com', '\x01')`)
	sqlDB := d.SQLDB(t)
	if err := insertRequest(sqlDB, requestRow{source: "investigate", title: "Why"}); err != nil {
		t.Fatal(err)
	}
	if err := insertFinding(sqlDB, `[]`); err != nil {
		t.Fatal(err)
	}
	if err := d.MigrateDownTo(ctx, v010Version); err != nil {
		t.Fatalf("down to %d: %v", v010Version, err)
	}
	var tables, columns, investigations int
	if err := sqlDB.QueryRow(`SELECT
  (SELECT count(*) FROM information_schema.tables WHERE table_name IN ('findings', 'grafana_connections')),
  (SELECT count(*) FROM information_schema.columns WHERE (table_name = 'imports' AND column_name = 'grafana_uid')
    OR (table_name = 'agent_requests' AND column_name IN ('context_from', 'context_to', 'title', 'pinned_at', 'archived_at'))),
  (SELECT count(*) FROM agent_requests WHERE source = 'investigate')`).Scan(&tables, &columns, &investigations); err != nil {
		t.Fatal(err)
	}
	if tables != 0 || columns != 0 || investigations != 0 {
		t.Errorf("after rolling back: findings and grafana_connections tables %d, added columns %d, investigate rows %d", tables, columns, investigations)
	}
}
