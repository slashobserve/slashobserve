package db_test

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

type commentRow struct {
	dashboard         string
	version           int
	panel             any
	parent            any
	status            string
	resolved          bool
	resolvedByKind    any
	resolvedByVersion any
}

func insertComment(sqlDB *sql.DB, c commentRow) (string, error) {
	var id string
	err := sqlDB.QueryRow(`INSERT INTO dashboard_comments (dashboard_id, version, panel_id, parent_id, body, status, resolved_at, resolved_by_kind, resolved_by_version, author_kind, author_id, author_name)
VALUES ($1::uuid, $2, $3, $4::uuid, 'b', $5, CASE WHEN $6::boolean THEN now() END, $7, $8::integer, 'user', gen_random_uuid(), 'n')
RETURNING id::text`, c.dashboard, c.version, c.panel, c.parent, c.status, c.resolved, c.resolvedByKind, c.resolvedByVersion).Scan(&id)
	return id, err
}

func constraintOf(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.ConstraintName
	}
	return ""
}

func seedCommentFixtures(t *testing.T, d *dbtest.Database) {
	t.Helper()
	seedDashboard(t, d, dashboardA, 3)
	seedDashboard(t, d, dashboardB, 1)
	for v := 1; v <= 3; v++ {
		seedVersion(t, d, dashboardA, v, "{}")
	}
	seedVersion(t, d, dashboardB, 1, "{}")
}

func TestCommentThreadConstraints(t *testing.T) {
	d := dbtest.New(t)
	sqlDB := d.SQLDB(t)
	seedCommentFixtures(t, d)
	rootA, err := insertComment(sqlDB, commentRow{dashboard: dashboardA, version: 2, panel: "p", status: "open"})
	if err != nil {
		t.Fatal(err)
	}
	rootB, err := insertComment(sqlDB, commentRow{dashboard: dashboardB, version: 1, panel: "p", status: "open"})
	if err != nil {
		t.Fatal(err)
	}
	const (
		replyCheck     = "dashboard_comments_reply_check"
		versionCheck   = "dashboard_comments_resolved_by_version_check"
		versionFkey    = "dashboard_comments_resolved_by_version_fkey"
		parentFkey     = "dashboard_comments_parent_id_fkey"
		parentDashFkey = "dashboard_comments_parent_dashboard_fkey"
		missingParent  = "00000000-0000-4000-8000-0000000000ff"
	)
	tests := []struct {
		name       string
		row        commentRow
		constraint string
	}{
		{"whole-dashboard root", commentRow{dashboard: dashboardA, version: 1, status: "open"}, ""},
		{"open reply", commentRow{dashboard: dashboardA, version: 2, panel: "p", parent: rootA, status: "open"}, ""},
		{"whole-dashboard reply", commentRow{dashboard: dashboardA, version: 2, parent: rootA, status: "open"}, ""},
		{"resolved reply", commentRow{dashboard: dashboardA, version: 2, panel: "p", parent: rootA, status: "resolved", resolved: true}, replyCheck},
		{"open reply with a resolver", commentRow{dashboard: dashboardA, version: 2, panel: "p", parent: rootA, status: "open", resolvedByKind: "user"}, replyCheck},
		{"resolved root with a version above its own", commentRow{dashboard: dashboardA, version: 2, panel: "p", status: "resolved", resolved: true, resolvedByVersion: 3}, ""},
		{"resolved root with its own version", commentRow{dashboard: dashboardA, version: 2, panel: "p", status: "resolved", resolved: true, resolvedByVersion: 2}, ""},
		{"resolved root without a version", commentRow{dashboard: dashboardA, version: 2, panel: "p", status: "resolved", resolved: true}, ""},
		{"resolved root with a version below its own", commentRow{dashboard: dashboardA, version: 2, panel: "p", status: "resolved", resolved: true, resolvedByVersion: 1}, versionCheck},
		{"open root with a resolved-by version", commentRow{dashboard: dashboardA, version: 1, panel: "p", status: "open", resolvedByVersion: 2}, versionCheck},
		{"resolved by a version the dashboard lacks", commentRow{dashboard: dashboardB, version: 1, panel: "p", status: "resolved", resolved: true, resolvedByVersion: 2}, versionFkey},
		{"reply to an unknown comment", commentRow{dashboard: dashboardA, version: 2, panel: "p", parent: missingParent, status: "open"}, parentFkey},
		{"reply to a comment of another dashboard", commentRow{dashboard: dashboardA, version: 1, panel: "p", parent: rootB, status: "open"}, parentDashFkey},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := insertComment(sqlDB, tt.row)
			if got := constraintOf(err); got != tt.constraint || (tt.constraint == "" && err != nil) {
				t.Fatalf("insert = %v (constraint %q), want constraint %q", err, got, tt.constraint)
			}
		})
	}
}

func TestDeletingTheResolvingVersionKeepsTheResolvedComment(t *testing.T) {
	d := dbtest.New(t)
	sqlDB := d.SQLDB(t)
	seedCommentFixtures(t, d)
	id, err := insertComment(sqlDB, commentRow{dashboard: dashboardA, version: 1, panel: "p", status: "resolved", resolved: true, resolvedByVersion: 3})
	if err != nil {
		t.Fatal(err)
	}
	d.Exec(t, `DELETE FROM dashboard_versions WHERE dashboard_id = $1::uuid AND version = 3`, dashboardA)
	var status string
	var by sql.NullInt32
	if err := sqlDB.QueryRow(`SELECT status, resolved_by_version FROM dashboard_comments WHERE id = $1::uuid`, id).Scan(&status, &by); err != nil {
		t.Fatal(err)
	}
	if status != "resolved" || by.Valid {
		t.Fatalf("status %q, resolved_by_version %v, want resolved and NULL", status, by)
	}
}

func TestDeletingAThreadRootDeletesItsReplies(t *testing.T) {
	d := dbtest.New(t)
	sqlDB := d.SQLDB(t)
	seedCommentFixtures(t, d)
	root, err := insertComment(sqlDB, commentRow{dashboard: dashboardA, version: 1, panel: "p", status: "open"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := insertComment(sqlDB, commentRow{dashboard: dashboardA, version: 1, panel: "p", parent: root, status: "open"}); err != nil {
		t.Fatal(err)
	}
	d.Exec(t, `DELETE FROM dashboard_comments WHERE id = $1::uuid`, root)
	var n int
	if err := sqlDB.QueryRow(`SELECT count(*) FROM dashboard_comments WHERE dashboard_id = $1::uuid`, dashboardA).Scan(&n); err != nil || n != 0 {
		t.Fatalf("%d comments left, err %v", n, err)
	}
}
