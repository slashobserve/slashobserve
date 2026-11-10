package db_test

import (
	"context"
	"testing"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

const (
	threadsVersion = 28
	rootOpenID     = "00000000-0000-4000-8000-0000000000c1"
	replyID        = "00000000-0000-4000-8000-0000000000c2"
	resolvedID     = "00000000-0000-4000-8000-0000000000c3"
	wholeID        = "00000000-0000-4000-8000-0000000000c4"
	resolverID     = "00000000-0000-4000-8000-0000000000d1"
)

func seedLegacyComments(t *testing.T, d *dbtest.Database) {
	t.Helper()
	seedDashboard(t, d, dashboardA, 3)
	for v := 1; v <= 3; v++ {
		seedVersion(t, d, dashboardA, v, "{}")
	}
	d.Exec(t, `INSERT INTO dashboard_comments (id, dashboard_id, version, panel_id, body, author_kind, author_id, author_name, author_email, created_at, updated_at)
VALUES ($1::uuid, $2::uuid, 1, 'rps', 'root body', 'user', gen_random_uuid(), 'Ada', 'ada@example.com', '2026-01-01T10:00:00Z', '2026-01-01T10:00:00Z')`, rootOpenID, dashboardA)
	d.Exec(t, `INSERT INTO dashboard_comments (id, dashboard_id, version, panel_id, parent_id, body, author_kind, author_id, author_name, created_at, updated_at)
VALUES ($1::uuid, $2::uuid, 1, 'rps', $3::uuid, 'reply body', 'service_account', gen_random_uuid(), 'bot', '2026-01-01T11:00:00Z', '2026-01-01T11:00:00Z')`, replyID, dashboardA, rootOpenID)
	d.Exec(t, `INSERT INTO dashboard_comments (id, dashboard_id, version, panel_id, body, status, resolved_at, resolved_by_kind, resolved_by_id, resolved_by_name, resolved_by_version, author_kind, author_id, author_name, created_at, updated_at)
VALUES ($1::uuid, $2::uuid, 2, 'errors', 'resolved body', 'resolved', '2026-01-02T10:00:00Z', 'user', $3::uuid, 'Bo', 3, 'user', gen_random_uuid(), 'Cy', '2026-01-01T12:00:00Z', '2026-01-02T10:00:00Z')`, resolvedID, dashboardA, resolverID)
	d.Exec(t, `INSERT INTO dashboard_comments (id, dashboard_id, version, body, author_kind, author_id, author_name)
VALUES ($1::uuid, $2::uuid, 3, 'whole dashboard', 'user', gen_random_uuid(), 'Di')`, wholeID, dashboardA)
}

func TestThreadsUpCopiesTheCommentsAndKeepsTheirIDs(t *testing.T) {
	ctx := context.Background()
	d := dbtest.NewUnmigrated(t)
	if err := d.MigrateUpTo(ctx, threadsVersion-1); err != nil {
		t.Fatal(err)
	}
	seedLegacyComments(t, d)
	if err := d.MigrateUpTo(ctx, threadsVersion); err != nil {
		t.Fatalf("up: %v", err)
	}
	sqlDB := d.SQLDB(t)

	var threads, messages int
	if err := sqlDB.QueryRow(`SELECT (SELECT count(*) FROM threads), (SELECT count(*) FROM thread_messages)`).Scan(&threads, &messages); err != nil {
		t.Fatal(err)
	}
	if threads != 3 || messages != 4 {
		t.Fatalf("%d threads and %d messages, want 3 and 4", threads, messages)
	}

	type threadRow struct {
		version, resolvedByVersion *int
		panel, status, resolver    *string
		resolvedByID               *string
	}
	read := func(id string) threadRow {
		var r threadRow
		err := sqlDB.QueryRow(`SELECT dashboard_version, panel_id, status, resolved_by_version, resolved_by_name, resolved_by_id::text FROM threads WHERE id = $1::uuid`, id).
			Scan(&r.version, &r.panel, &r.status, &r.resolvedByVersion, &r.resolver, &r.resolvedByID)
		if err != nil {
			t.Fatalf("thread %s: %v", id, err)
		}
		return r
	}
	open := read(rootOpenID)
	if *open.version != 1 || *open.panel != "rps" || *open.status != "open" || open.resolver != nil || open.resolvedByVersion != nil {
		t.Errorf("open thread = %+v", open)
	}
	resolved := read(resolvedID)
	if *resolved.version != 2 || *resolved.panel != "errors" || *resolved.status != "resolved" || *resolved.resolver != "Bo" || *resolved.resolvedByID != resolverID || *resolved.resolvedByVersion != 3 {
		t.Errorf("resolved thread = %+v", resolved)
	}
	whole := read(wholeID)
	if *whole.version != 3 || whole.panel != nil {
		t.Errorf("whole-dashboard thread = %+v", whole)
	}

	rows, err := sqlDB.Query(`SELECT id::text, thread_id::text, body, author_kind, author_name, author_email, created_at = updated_at, edited_at IS NULL FROM thread_messages ORDER BY created_at, id`)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rows.Close() })
	type messageRow struct {
		id, thread, body, kind, name string
		email                        *string
		untouched, unedited          bool
	}
	var got []messageRow
	for rows.Next() {
		var m messageRow
		if err := rows.Scan(&m.id, &m.thread, &m.body, &m.kind, &m.name, &m.email, &m.untouched, &m.unedited); err != nil {
			t.Fatal(err)
		}
		got = append(got, m)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	wantThreads := map[string]string{rootOpenID: rootOpenID, replyID: rootOpenID, resolvedID: resolvedID, wholeID: wholeID}
	bodies := map[string]string{rootOpenID: "root body", replyID: "reply body", resolvedID: "resolved body", wholeID: "whole dashboard"}
	if len(got) != len(wantThreads) {
		t.Fatalf("messages = %+v", got)
	}
	for _, m := range got {
		if m.thread != wantThreads[m.id] || m.body != bodies[m.id] || !m.untouched || !m.unedited {
			t.Errorf("message = %+v, want thread %s and body %q", m, wantThreads[m.id], bodies[m.id])
		}
	}
	if got[0].id != rootOpenID || *got[0].email != "ada@example.com" || got[1].id != replyID || got[1].kind != "service_account" || got[1].email != nil {
		t.Errorf("authors = %+v", got[:2])
	}
}

func TestThreadsDownRemovesThreadTablesAndActivity(t *testing.T) {
	ctx := context.Background()
	d := dbtest.NewUnmigrated(t)
	if err := d.MigrateUpTo(ctx, threadsVersion); err != nil {
		t.Fatal(err)
	}
	seedDashboard(t, d, dashboardA, 1)
	seedVersion(t, d, dashboardA, 1, "{}")
	d.Exec(t, `INSERT INTO threads (id, dashboard_id, dashboard_version) VALUES ($1::uuid, $2::uuid, 1)`, rootOpenID, dashboardA)
	d.Exec(t, `INSERT INTO activity (type, target_kind, target_id, actor_kind, actor_id, actor_name, data)
VALUES ('thread.opened', 'thread', $1::uuid, 'user', gen_random_uuid(), 'Ada', '{}')`, rootOpenID)

	if err := d.MigrateDownTo(ctx, threadsVersion-1); err != nil {
		t.Fatalf("down: %v", err)
	}
	var tables int
	if err := d.SQLDB(t).QueryRow(`SELECT count(*) FROM pg_tables WHERE tablename IN ('threads', 'thread_messages', 'thread_mentions')`).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if tables != 0 {
		t.Errorf("%d thread tables survived the down migration", tables)
	}
	if err := d.MigrateUpTo(ctx, threadsVersion); err != nil {
		t.Fatalf("up again: %v", err)
	}
}
