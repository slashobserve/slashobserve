package db_test

import (
	"context"
	"testing"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

const actorColumnsVersion = 26

const (
	actorUser         = "00000000-0000-4000-8000-0000000000b1"
	actorServiceKey   = "00000000-0000-4000-8000-0000000000b2"
	actorGone         = "00000000-0000-4000-8000-0000000000b3"
	actorRun          = "00000000-0000-4000-8000-0000000000c1"
	actorRunDashboard = "00000000-0000-4000-8000-0000000000d1"
	actorPlainDash    = "00000000-0000-4000-8000-0000000000d2"
)

func seedActorFixtures(t *testing.T, d *dbtest.Database) {
	t.Helper()
	d.Exec(t, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, name) VALUES ($1::uuid, 'https://idp', 'a', 'ada@example.com', 'Ada')`, actorUser)
	d.Exec(t, `INSERT INTO service_accounts (id, name) VALUES ($1::uuid, 'ci-deploys')`, actorServiceKey)
	seedDashboard(t, d, actorRunDashboard, 1)
	seedDashboard(t, d, actorPlainDash, 1)
	d.Exec(t, `INSERT INTO agent_requests (id, dashboard_id, source, prompt, base_version, requested_by_kind, requested_by_id, requested_by_name)
VALUES ($1::uuid, $2::uuid, 'prompt', 'p', 1, 'user', $3::uuid, 'Ada')`, actorRun, actorRunDashboard, actorUser)
	d.Exec(t, `INSERT INTO dashboard_versions (dashboard_id, version, spec, summary, author_kind, author_id, author_name, agent_request_id)
VALUES ($1::uuid, 1, '{}'::jsonb, 's', 'user', $2::uuid, 'Ada', $3::uuid)`, actorRunDashboard, actorUser, actorRun)
	seedVersion(t, d, actorPlainDash, 1, "{}")
	for _, creator := range []string{actorUser, actorServiceKey, actorGone} {
		d.Exec(t, `INSERT INTO annotations (panel_id, time_from, text, created_by) VALUES ($1, now(), 'x', $2::uuid)`, creator, creator)
	}
}

func TestActorColumnsBackfill(t *testing.T) {
	ctx := context.Background()
	d := dbtest.NewUnmigrated(t)
	if err := d.MigrateUpTo(ctx, postV010Version); err != nil {
		t.Fatal(err)
	}
	seedActorFixtures(t, d)
	if err := d.MigrateUpTo(ctx, actorColumnsVersion); err != nil {
		t.Fatal(err)
	}
	sqlDB := d.SQLDB(t)

	runs := map[string]string{actorRunDashboard: actorRun, actorPlainDash: ""}
	for dashboard, want := range runs {
		var got string
		if err := sqlDB.QueryRow(`SELECT COALESCE(created_run_id::text, '') FROM dashboards WHERE id = $1::uuid`, dashboard).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("dashboard %s: created_run_id = %q, want %q", dashboard, got, want)
		}
	}

	kinds := map[string]string{actorUser: "user", actorServiceKey: "service_account", actorGone: ""}
	for creator, want := range kinds {
		var got string
		if err := sqlDB.QueryRow(`SELECT COALESCE(created_by_kind, '') FROM annotations WHERE created_by = $1::uuid`, creator).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("annotation by %s: created_by_kind = %q, want %q", creator, got, want)
		}
	}
}

func TestActorColumnsDown(t *testing.T) {
	ctx := context.Background()
	d := dbtest.NewUnmigrated(t)
	if err := d.MigrateUpTo(ctx, actorColumnsVersion); err != nil {
		t.Fatal(err)
	}
	if err := d.MigrateDownTo(ctx, postV010Version); err != nil {
		t.Fatalf("down: %v", err)
	}
	var n int
	err := d.SQLDB(t).QueryRow(`SELECT count(*) FROM information_schema.columns WHERE table_name = 'annotations' AND column_name = 'created_by_kind'`).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("annotations.created_by_kind still exists after the down migration")
	}
}

func TestAnnotationKindNeedsACreator(t *testing.T) {
	d := dbtest.New(t)
	_, err := d.SQLDB(t).Exec(`INSERT INTO annotations (panel_id, time_from, text, created_by, created_by_kind) VALUES ('p', now(), 'x', NULL, 'user')`)
	if got := constraintOf(err); got != "annotations_created_by_kind_id_check" {
		t.Fatalf("error = %v, want an annotations_created_by_kind_id_check violation", err)
	}
}
