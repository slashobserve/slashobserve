package db_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/audit"
)

const auditEventsVersion = 74

func TestTheAuditBackfillMapsEveryActivityType(t *testing.T) {
	ctx := context.Background()
	d := newMigratedTo(t, auditEventsVersion-1)
	for _, typ := range activity.Types() {
		kind, _, _ := strings.Cut(typ, ".")
		d.Exec(t, `INSERT INTO activity (type, target_kind, target_id, actor_kind, actor_id, actor_name, origin, data)
VALUES ($1, $2, gen_random_uuid(), 'user', gen_random_uuid(), 'Ada', 'web', '{"title": "Checkout"}')`, typ, kind)
	}
	d.Exec(t, `INSERT INTO activity (type, target_kind, target_id, actor_kind, actor_id, actor_name, data)
VALUES ('dashboard.deleted', 'dashboard', '66666666-6666-4666-8666-666666666666', 'user', gen_random_uuid(), 'Ada', '{}')`)
	if err := d.MigrateUpTo(ctx, auditEventsVersion); err != nil {
		t.Fatalf("up: %v", err)
	}

	rows, err := d.SQLDB(t).QueryContext(ctx, `SELECT a.type, e.action, e.category, e.outcome, e.initiator, e.origin, coalesce(e.data->>'result', ''), coalesce(e.data->>'title', ''), e.target_id = a.target_id::text, e.xid = a.xid, e.occurred_at = a.created_at
FROM activity a JOIN audit_events e ON e.xid = a.xid AND e.target_id = a.target_id::text AND e.server_id = 'backfill'
WHERE a.origin = 'web'`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	seen := map[string]bool{}
	for rows.Next() {
		var typ, action, category, outcome, initiator, origin, result, title string
		var sameTarget, samePosition, sameTime bool
		if err := rows.Scan(&typ, &action, &category, &outcome, &initiator, &origin, &result, &title, &sameTarget, &samePosition, &sameTime); err != nil {
			t.Fatal(err)
		}
		seen[typ] = true
		want, _ := activity.AuditAction(typ)
		spec, _ := audit.Lookup(want)
		if audit.Action(action) != want || audit.Category(category) != spec.Category {
			t.Errorf("%s backfilled as %s (%s), want %s (%s)", typ, action, category, want, spec.Category)
		}
		wantTitle := ""
		if slices.Contains(spec.Data, "title") {
			wantTitle = "Checkout"
		}
		if outcome != "success" || initiator != "person" || origin != "web" || title != wantTitle || !sameTarget || !samePosition || !sameTime {
			t.Errorf("%s backfilled as %s by a %s from %s titled %q (target %v, position %v, time %v)", typ, outcome, initiator, origin, title, sameTarget, samePosition, sameTime)
		}
		if wantResult, isRunEnd := strings.CutPrefix(typ, "run."); isRunEnd && action == "run.finish" && result != wantResult {
			t.Errorf("%s backfilled with result %q, want %q", typ, result, wantResult)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for _, typ := range activity.Types() {
		if !seen[typ] {
			t.Errorf("activity type %s has no backfilled audit event", typ)
		}
	}

	var origin string
	if err := d.SQLDB(t).QueryRowContext(ctx, `SELECT origin FROM audit_events WHERE target_id = '66666666-6666-4666-8666-666666666666'`).Scan(&origin); err != nil {
		t.Fatal(err)
	}
	if origin != "unknown" {
		t.Errorf("an activity row with no origin backfilled from %q, want unknown", origin)
	}
}

func TestTheAuditEventsMigrationCreatesTodaysPartitionAndRollsBack(t *testing.T) {
	ctx := context.Background()
	d := newMigratedTo(t, auditEventsVersion)
	var ahead int
	err := d.SQLDB(t).QueryRowContext(ctx, `SELECT count(*) FROM pg_catalog.pg_inherits i JOIN pg_catalog.pg_class c ON c.oid = i.inhrelid
WHERE i.inhparent = 'audit_events'::regclass AND c.relname >= 'audit_events_' || to_char(now() AT TIME ZONE 'UTC', 'YYYYMMDD')`).Scan(&ahead)
	if err != nil {
		t.Fatal(err)
	}
	if ahead != audit.DaysAhead+1 {
		t.Fatalf("partitions from today = %d, want %d", ahead, audit.DaysAhead+1)
	}
	var defaults int
	if err := d.SQLDB(t).QueryRowContext(ctx, `SELECT count(*) FROM pg_catalog.pg_partitioned_table WHERE partrelid = 'audit_events'::regclass AND partdefid <> 0`).Scan(&defaults); err != nil {
		t.Fatal(err)
	}
	if defaults != 0 {
		t.Fatal("audit_events has a default partition")
	}
	if err := d.MigrateDownTo(ctx, auditEventsVersion-1); err != nil {
		t.Fatalf("down: %v", err)
	}
	var exists bool
	if err := d.SQLDB(t).QueryRowContext(ctx, `SELECT to_regclass('audit_events') IS NOT NULL OR to_regclass('audit_events_seq') IS NOT NULL`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("the audit table or its sequence is left after the down migration")
	}
	if err := d.MigrateUpTo(ctx, auditEventsVersion); err != nil {
		t.Fatalf("up again: %v", err)
	}
}
