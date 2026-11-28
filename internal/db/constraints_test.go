package db_test

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

func constraintOf(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.ConstraintName
	}
	return ""
}

func TestAnnotationKindNeedsACreator(t *testing.T) {
	d := dbtest.New(t)
	_, err := d.SQLDB(t).Exec(`INSERT INTO annotations (panel_id, time_from, text, created_by, created_by_kind) VALUES ('p', now(), 'x', NULL, 'user')`)
	if got := constraintOf(err); got != "annotations_created_by_kind_id_check" {
		t.Fatalf("error = %v, want an annotations_created_by_kind_id_check violation", err)
	}
}

const dashboardA = "00000000-0000-4000-8000-00000000000a"

func seedDashboard(t *testing.T, d *dbtest.Database, dashboardID string, latestVersion int) {
	t.Helper()
	d.Exec(t, `INSERT INTO dashboards (id, title, latest_version, created_by_kind, created_by_id, created_by_name, created_via)
VALUES ($1::uuid, 'T', $2, 'user', gen_random_uuid(), 'seed', 'direct')`, dashboardID, latestVersion)
}
