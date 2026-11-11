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
	requestOnReply = "00000000-0000-4000-8000-0000000000e1"
	requestOnRoot  = "00000000-0000-4000-8000-0000000000e2"
	findingThread  = "00000000-0000-4000-8000-0000000000f1"
	findingID      = "00000000-0000-4000-8000-0000000000f2"
	requestOnFound = "00000000-0000-4000-8000-0000000000e4"
	runFKName      = "agent_requests_comment_id_fkey"
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
	seedRequest(t, d, requestOnReply, replyID)
	seedRequest(t, d, requestOnRoot, rootOpenID)
}

func seedRequest(t *testing.T, d *dbtest.Database, id, commentID string) {
	t.Helper()
	d.Exec(t, `INSERT INTO agent_requests (id, dashboard_id, source, comment_id, prompt, base_version, requested_by_kind, requested_by_id, requested_by_name)
VALUES ($1::uuid, $2::uuid, 'comment', $3::uuid, 'p', 1, 'user', gen_random_uuid(), 'Ada')`, id, dashboardA, commentID)
}

func commentFKTarget(t *testing.T, d *dbtest.Database) string {
	t.Helper()
	var target string
	err := d.SQLDB(t).QueryRow(`SELECT confrelid::regclass::text FROM pg_constraint WHERE conname = $1 AND conrelid = 'agent_requests'::regclass`, runFKName).Scan(&target)
	if err != nil {
		t.Fatalf("constraint %s: %v", runFKName, err)
	}
	return target
}

func TestThreadsUpCopiesTheCommentsAndKeepsTheirIDs(t *testing.T) {
	ctx := context.Background()
	d := dbtest.NewUnmigrated(t)
	if err := d.MigrateUpTo(ctx, threadsVersion-1); err != nil {
		t.Fatal(err)
	}
	if target := commentFKTarget(t, d); target != "dashboard_comments" {
		t.Fatalf("%s points at %s before the migration, want dashboard_comments", runFKName, target)
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

	if target := commentFKTarget(t, d); target != "threads" {
		t.Errorf("%s points at %s, want threads", runFKName, target)
	}
	for id, want := range map[string]string{requestOnReply: rootOpenID, requestOnRoot: rootOpenID} {
		var got string
		if err := sqlDB.QueryRow(`SELECT comment_id::text FROM agent_requests WHERE id = $1::uuid`, id).Scan(&got); err != nil || got != want {
			t.Errorf("request %s comment_id = %q, %v, want the root %s", id, got, err, want)
		}
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

func TestThreadsDownRestoresTheDashboardCommentsAndDropsTheRest(t *testing.T) {
	ctx := context.Background()
	d := dbtest.NewUnmigrated(t)
	if err := d.MigrateUpTo(ctx, threadsVersion); err != nil {
		t.Fatal(err)
	}
	seedDashboard(t, d, dashboardA, 2)
	seedVersion(t, d, dashboardA, 1, "{}")
	seedVersion(t, d, dashboardA, 2, "{}")
	d.Exec(t, `INSERT INTO threads (id, dashboard_id, dashboard_version, panel_id, created_at, updated_at)
VALUES ($1::uuid, $2::uuid, 1, 'rps', '2026-01-01T10:00:00Z', '2026-01-01T10:00:00Z')`, rootOpenID, dashboardA)
	d.Exec(t, `INSERT INTO thread_messages (id, thread_id, body, author_kind, author_id, author_name, created_at, updated_at, edited_at)
VALUES ($1::uuid, $1::uuid, 'root body', 'user', gen_random_uuid(), 'Ada', '2026-01-01T10:00:00Z', '2026-01-01T10:30:00Z', '2026-01-01T10:30:00Z')`, rootOpenID)
	d.Exec(t, `INSERT INTO thread_messages (id, thread_id, body, author_kind, author_id, author_name, author_email, created_at, updated_at)
VALUES ($1::uuid, $2::uuid, 'reply body', 'service_account', gen_random_uuid(), 'bot', 'bot@example.com', '2026-01-01T11:00:00Z', '2026-01-01T11:00:00Z')`, replyID, rootOpenID)
	d.Exec(t, `INSERT INTO threads (id, dashboard_id, dashboard_version, status, resolved_at, resolved_by_kind, resolved_by_id, resolved_by_name, resolved_by_version, created_at, updated_at)
VALUES ($1::uuid, $2::uuid, 1, 'resolved', '2026-01-02T10:00:00Z', 'user', $3::uuid, 'Bo', 2, '2026-01-01T12:00:00Z', '2026-01-02T10:00:00Z')`, resolvedID, dashboardA, resolverID)
	d.Exec(t, `INSERT INTO thread_messages (id, thread_id, body, author_kind, author_id, author_name, created_at, updated_at)
VALUES ($1::uuid, $1::uuid, 'resolved body', 'user', gen_random_uuid(), 'Cy', '2026-01-01T12:00:00Z', '2026-01-01T12:00:00Z')`, resolvedID)
	d.Exec(t, `INSERT INTO findings (id, dashboard_id, title, markdown, time_from, time_to, created_by_kind, created_by_id, created_by_name)
VALUES ($1::uuid, $2::uuid, 't', 'm', now() - interval '1 hour', now(), 'user', gen_random_uuid(), 'Ada')`, findingID, dashboardA)
	d.Exec(t, `INSERT INTO threads (id, finding_id) VALUES ($1::uuid, $2::uuid)`, findingThread, findingID)
	d.Exec(t, `INSERT INTO thread_messages (id, thread_id, body, author_kind, author_id, author_name)
VALUES ($1::uuid, $1::uuid, 'on a finding', 'user', gen_random_uuid(), 'Ada')`, findingThread)
	seedRequest(t, d, requestOnRoot, rootOpenID)
	seedRequest(t, d, requestOnFound, findingThread)
	d.Exec(t, `INSERT INTO activity (type, target_kind, target_id, actor_kind, actor_id, actor_name, data)
VALUES ('thread.opened', 'thread', $1::uuid, 'user', gen_random_uuid(), 'Ada', '{}')`, rootOpenID)

	if err := d.MigrateDownTo(ctx, threadsVersion-1); err != nil {
		t.Fatalf("down: %v", err)
	}
	sqlDB := d.SQLDB(t)

	var tables, threadEvents int
	if err := sqlDB.QueryRow(`SELECT (SELECT count(*) FROM pg_tables WHERE tablename IN ('threads', 'thread_messages', 'thread_mentions')), (SELECT count(*) FROM activity WHERE target_kind = 'thread')`).Scan(&tables, &threadEvents); err != nil {
		t.Fatal(err)
	}
	if tables != 0 || threadEvents != 0 {
		t.Errorf("%d thread tables and %d thread events survived the down migration", tables, threadEvents)
	}

	type commentRow struct {
		parent, panel, status, resolver, body, email *string
		version, resolvedByVersion                   *int
		resolvedAt, updatedAt                        string
	}
	read := func(id string) commentRow {
		var r commentRow
		err := sqlDB.QueryRow(`SELECT parent_id::text, panel_id, status, resolved_by_name, body, author_email, version, resolved_by_version, coalesce(resolved_at::text, ''), updated_at::text FROM dashboard_comments WHERE id = $1::uuid`, id).
			Scan(&r.parent, &r.panel, &r.status, &r.resolver, &r.body, &r.email, &r.version, &r.resolvedByVersion, &r.resolvedAt, &r.updatedAt)
		if err != nil {
			t.Fatalf("comment %s: %v", id, err)
		}
		return r
	}
	root := read(rootOpenID)
	if root.parent != nil || *root.panel != "rps" || *root.status != "open" || *root.version != 1 || *root.body != "root body" {
		t.Errorf("root = %+v", root)
	}
	reply := read(replyID)
	if *reply.parent != rootOpenID || *reply.panel != "rps" || *reply.status != "open" || *reply.version != 1 || *reply.body != "reply body" ||
		*reply.email != "bot@example.com" || reply.resolver != nil || reply.resolvedAt != "" {
		t.Errorf("reply = %+v", reply)
	}
	resolved := read(resolvedID)
	if resolved.parent != nil || *resolved.status != "resolved" || *resolved.resolver != "Bo" || *resolved.resolvedByVersion != 2 || resolved.resolvedAt == "" {
		t.Errorf("resolved = %+v", resolved)
	}

	var comments int
	if err := sqlDB.QueryRow(`SELECT count(*) FROM dashboard_comments`).Scan(&comments); err != nil {
		t.Fatal(err)
	}
	if comments != 3 {
		t.Errorf("%d comments restored, want 3 and none for the finding thread", comments)
	}
	if target := commentFKTarget(t, d); target != "dashboard_comments" {
		t.Errorf("%s points at %s, want dashboard_comments", runFKName, target)
	}
	for id, want := range map[string]*string{requestOnRoot: new(rootOpenID), requestOnFound: nil} {
		var got *string
		if err := sqlDB.QueryRow(`SELECT comment_id::text FROM agent_requests WHERE id = $1::uuid`, id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if (got == nil) != (want == nil) || got != nil && *got != *want {
			t.Errorf("request %s comment_id = %v, want %v", id, got, want)
		}
	}

	if err := d.MigrateUpTo(ctx, threadsVersion); err != nil {
		t.Fatalf("up again: %v", err)
	}
	var threads int
	if err := sqlDB.QueryRow(`SELECT count(*) FROM threads`).Scan(&threads); err != nil || threads != 2 {
		t.Errorf("%d threads after migrating up again, err %v, want the 2 dashboard threads", threads, err)
	}
}
