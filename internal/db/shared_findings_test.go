package db_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

const (
	sharedFindingsVersion = 32

	sfDashboard       = "00000000-0000-4000-8000-0000000005a1"
	sfUserFinding     = "00000000-0000-4000-8000-0000000005b1"
	sfDashboardFind   = "00000000-0000-4000-8000-0000000005b2"
	sfServiceFinding  = "00000000-0000-4000-8000-0000000005b3"
	sfGoneUserFinding = "00000000-0000-4000-8000-0000000005b4"
	sfUserThread      = "00000000-0000-4000-8000-0000000005c1"
	sfDashboardThread = "00000000-0000-4000-8000-0000000005c2"
	sfServiceAccount  = "00000000-0000-4000-8000-0000000005d1"
	sfGoneUser        = "00000000-0000-4000-8000-0000000005d2"
)

func seedSharedFindingsFixtures(t *testing.T, d *dbtest.Database) (user string) {
	t.Helper()
	if err := d.SQLDB(t).QueryRow(`INSERT INTO users (oidc_issuer, oidc_subject, email, name) VALUES ('https://idp', 'ada', 'ada@example.com', 'Ada') RETURNING id::text`).Scan(&user); err != nil {
		t.Fatal(err)
	}
	d.Exec(t, `INSERT INTO service_accounts (id, name) VALUES ($1::uuid, 'ci')`, sfServiceAccount)
	seedDashboard(t, d, sfDashboard, 1)
	seedVersion(t, d, sfDashboard, 1, "{}")
	finding := func(id, kind, creator, dashboard string) {
		d.Exec(t, `INSERT INTO findings (id, dashboard_id, title, markdown, time_from, time_to, created_by_kind, created_by_id, created_by_name)
VALUES ($1::uuid, nullif($2, '')::uuid, 't', 'm', now() - interval '1 hour', now(), $3, $4::uuid, 'seed')`, id, dashboard, kind, creator)
	}
	finding(sfUserFinding, "user", user, "")
	finding(sfDashboardFind, "user", user, sfDashboard)
	finding(sfServiceFinding, "service_account", sfServiceAccount, "")
	finding(sfGoneUserFinding, "user", sfGoneUser, "")
	d.Exec(t, `INSERT INTO threads (id, finding_id) VALUES ($1::uuid, $2::uuid), ($3::uuid, $4::uuid)`,
		sfUserThread, sfUserFinding, sfDashboardThread, sfDashboardFind)
	activity := func(typ, kind, target, dashboard string) {
		d.Exec(t, `INSERT INTO activity (type, target_kind, target_id, dashboard_id, folder_id, actor_kind, actor_id, actor_name)
VALUES ($1, $2, $3::uuid, nullif($4, '')::uuid, CASE WHEN $4 = '' THEN NULL ELSE general_folder_id() END, 'user', $5::uuid, 'Ada')`, typ, kind, target, dashboard, user)
	}
	activity("finding.created", "finding", sfUserFinding, "")
	activity("thread.created", "thread", sfUserThread, "")
	activity("finding.created", "finding", sfDashboardFind, sfDashboard)
	activity("thread.created", "thread", sfDashboardThread, sfDashboard)
	return user
}

func migrateToSharedFindings(t *testing.T) (*dbtest.Database, string) {
	t.Helper()
	ctx := context.Background()
	d := dbtest.NewUnmigrated(t)
	if err := d.MigrateUpTo(ctx, sharedFindingsVersion-1); err != nil {
		t.Fatal(err)
	}
	user := seedSharedFindingsFixtures(t, d)
	if err := d.MigrateUpTo(ctx, sharedFindingsVersion); err != nil {
		t.Fatalf("up: %v", err)
	}
	return d, user
}

func personalFolderOf(t *testing.T, sqlDB *sql.DB, user string) string {
	t.Helper()
	var id string
	if err := sqlDB.QueryRow(`SELECT id::text FROM folders WHERE personal AND owner_id = $1::uuid`, user).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestSharedFindingsBackfillPlacesFindings(t *testing.T) {
	d, user := migrateToSharedFindings(t)
	sqlDB := d.SQLDB(t)
	personal := personalFolderOf(t, sqlDB, user)

	want := map[string]string{
		sfUserFinding:     personal,
		sfDashboardFind:   "",
		sfServiceFinding:  "",
		sfGoneUserFinding: "",
	}
	for id, folder := range want {
		var got string
		if err := sqlDB.QueryRow(`SELECT COALESCE(folder_id::text, '') FROM findings WHERE id = $1::uuid`, id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != folder {
			t.Errorf("finding %s: folder_id = %q, want %q", id, got, folder)
		}
	}
}

func TestSharedFindingsBackfillFollowsActivityToTheFolder(t *testing.T) {
	d, user := migrateToSharedFindings(t)
	sqlDB := d.SQLDB(t)
	personal := personalFolderOf(t, sqlDB, user)

	type row struct{ finding, folder string }
	read := func(target string) row {
		var r row
		err := sqlDB.QueryRow(`SELECT COALESCE(finding_id::text, ''), COALESCE(folder_id::text, '') FROM activity WHERE target_id = $1::uuid`, target).Scan(&r.finding, &r.folder)
		if err != nil {
			t.Fatalf("activity of %s: %v", target, err)
		}
		return r
	}
	for _, target := range []string{sfUserFinding, sfUserThread} {
		if got := read(target); got != (row{sfUserFinding, personal}) {
			t.Errorf("activity of %s = %+v, want finding %s in folder %s", target, got, sfUserFinding, personal)
		}
	}
	var general string
	if err := sqlDB.QueryRow(`SELECT general_folder_id()::text`).Scan(&general); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{sfDashboardFind, sfDashboardThread} {
		if got := read(target); got != (row{"", general}) {
			t.Errorf("activity of dashboard finding target %s = %+v, want no finding and folder %s", target, got, general)
		}
	}
}

func TestSharedFindingsPlacementCheckRejectsADashboardAndAFolder(t *testing.T) {
	d, user := migrateToSharedFindings(t)
	personal := personalFolderOf(t, d.SQLDB(t), user)
	_, err := d.SQLDB(t).Exec(`UPDATE findings SET folder_id = $1::uuid WHERE id = $2::uuid`, personal, sfDashboardFind)
	if got := constraintOf(err); got != "findings_placement_check" {
		t.Fatalf("error = %v, want a findings_placement_check violation", err)
	}
}

func TestSharedFindingsUserDeletionRemovesTheirPersonalFolderContents(t *testing.T) {
	d, user := migrateToSharedFindings(t)
	sqlDB := d.SQLDB(t)
	personal := personalFolderOf(t, sqlDB, user)
	d.Exec(t, `INSERT INTO saved_searches (folder_id, kind, title, datasource_uid, query, time_from, time_to, created_by_kind, created_by_id, created_by_name)
VALUES ($1::uuid, 'logs', 't', 'loki', '{app="x"}', 'now-1h', 'now', 'user', $2::uuid, 'Ada')`, personal, user)
	d.Exec(t, `UPDATE dashboards SET folder_id = $1::uuid WHERE id = $2::uuid`, personal, sfDashboard)

	if _, err := sqlDB.Exec(`DELETE FROM users WHERE id = $1::uuid`, user); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	var searches, filed, dashboards, folders int
	err := sqlDB.QueryRow(`SELECT
  (SELECT count(*) FROM saved_searches),
  (SELECT count(*) FROM findings WHERE id = $1::uuid),
  (SELECT count(*) FROM dashboards WHERE id = $2::uuid),
  (SELECT count(*) FROM folders WHERE id = $3::uuid)`, sfUserFinding, sfDashboard, personal).Scan(&searches, &filed, &dashboards, &folders)
	if err != nil {
		t.Fatal(err)
	}
	if searches != 0 || filed != 0 || dashboards != 0 || folders != 0 {
		t.Fatalf("left %d saved searches, %d findings, %d dashboards and %d folders, want none", searches, filed, dashboards, folders)
	}
	var kept int
	if err := sqlDB.QueryRow(`SELECT count(*) FROM findings WHERE id IN ($1::uuid, $2::uuid)`, sfServiceFinding, sfGoneUserFinding).Scan(&kept); err != nil || kept != 2 {
		t.Fatalf("unfiled findings kept = %d, %v, want 2", kept, err)
	}
}

func TestSharedFindingsFolderHoldingContentsCannotBeDeleted(t *testing.T) {
	d, _ := migrateToSharedFindings(t)
	sqlDB := d.SQLDB(t)
	cases := map[string]string{
		"saved search": `INSERT INTO saved_searches (folder_id, kind, title, datasource_uid, query, time_from, time_to, created_by_kind, created_by_id, created_by_name)
VALUES ($1::uuid, 'logs', 't', 'loki', '{app="x"}', 'now-1h', 'now', 'user', gen_random_uuid(), 'Ada')`,
		"filed finding": `INSERT INTO findings (folder_id, title, markdown, time_from, time_to, created_by_kind, created_by_id, created_by_name)
VALUES ($1::uuid, 't', 'm', now() - interval '1 hour', now(), 'user', gen_random_uuid(), 'Ada')`,
	}
	for name, insert := range cases {
		t.Run(name, func(t *testing.T) {
			var folder string
			if err := sqlDB.QueryRow(`INSERT INTO folders (title) VALUES ($1) RETURNING id::text`, "Holds a "+name).Scan(&folder); err != nil {
				t.Fatal(err)
			}
			d.Exec(t, insert, folder)
			_, err := sqlDB.Exec(`DELETE FROM folders WHERE id = $1::uuid`, folder)
			if got := constraintOf(err); got == "" {
				t.Fatalf("delete folder error = %v, want a foreign key violation", err)
			}
		})
	}
}

func TestSharedFindingsDownRestoresTheOldTriggerAndMigratesUpAgain(t *testing.T) {
	ctx := context.Background()
	d, _ := migrateToSharedFindings(t)
	sqlDB := d.SQLDB(t)
	d.Exec(t, `INSERT INTO activity (type, target_kind, target_id, actor_kind, actor_id, actor_name, saved_search_id)
VALUES ('saved_search.created', 'saved_search', gen_random_uuid(), 'user', gen_random_uuid(), 'Ada', gen_random_uuid()),
  ('finding.moved', 'finding', $1::uuid, 'user', gen_random_uuid(), 'Ada', NULL)`, sfUserFinding)

	if err := d.MigrateDownTo(ctx, sharedFindingsVersion-1); err != nil {
		t.Fatalf("down: %v", err)
	}
	var trigger, replaced, searches, findingFolder, activityColumns, moved, savedSearchRows int
	err := sqlDB.QueryRow(`SELECT
  (SELECT count(*) FROM pg_trigger WHERE tgname = 'users_personal_dashboards'),
  (SELECT count(*) FROM pg_trigger WHERE tgname = 'users_personal_folder_contents'),
  (SELECT count(*) FROM pg_class WHERE relname = 'saved_searches'),
  (SELECT count(*) FROM information_schema.columns WHERE table_name = 'findings' AND column_name = 'folder_id'),
  (SELECT count(*) FROM information_schema.columns WHERE table_name = 'activity' AND column_name IN ('finding_id', 'saved_search_id')),
  (SELECT count(*) FROM activity WHERE type = 'finding.moved'),
  (SELECT count(*) FROM activity WHERE target_kind = 'saved_search')`).Scan(&trigger, &replaced, &searches, &findingFolder, &activityColumns, &moved, &savedSearchRows)
	if err != nil {
		t.Fatal(err)
	}
	if trigger != 1 || replaced != 0 || searches != 0 || findingFolder != 0 || activityColumns != 0 || moved != 0 || savedSearchRows != 0 {
		t.Fatalf("after down: old trigger %d, new trigger %d, saved_searches %d, findings.folder_id %d, activity columns %d, finding.moved rows %d, saved_search rows %d",
			trigger, replaced, searches, findingFolder, activityColumns, moved, savedSearchRows)
	}

	if err := d.MigrateUpTo(ctx, sharedFindingsVersion); err != nil {
		t.Fatalf("up again: %v", err)
	}
}
