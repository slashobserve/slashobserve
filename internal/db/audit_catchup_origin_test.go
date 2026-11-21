package db_test

import (
	"context"
	"testing"
)

const catchUpOriginVersion = 80

func TestTheCatchUpRecordsAnActivityRowOfARunWithNoOriginAsTheAgent(t *testing.T) {
	ctx := context.Background()
	d := newMigratedTo(t, catchUpOriginVersion-1)
	d.Exec(t, `INSERT INTO activity (type, target_kind, target_id, actor_kind, actor_id, actor_name, run_id, data)
VALUES ('dashboard.deleted', 'dashboard', '77777777-7777-4777-8777-777777777777', 'user', gen_random_uuid(), 'Ada', gen_random_uuid(), '{}')`)
	d.Exec(t, `INSERT INTO activity (type, target_kind, target_id, actor_kind, actor_id, actor_name, data)
VALUES ('dashboard.deleted', 'dashboard', '66666666-6666-4666-8666-666666666666', 'user', gen_random_uuid(), 'Ada', '{}')`)
	if err := d.MigrateUpTo(ctx, catchUpOriginVersion); err != nil {
		t.Fatalf("up: %v", err)
	}
	d.Exec(t, `SELECT audit_events_catch_up('-infinity', 'catch-up', 'catch-up', false)`)

	origins := map[string]string{}
	rows, err := d.SQLDB(t).QueryContext(ctx, `SELECT target_id, origin FROM audit_events WHERE server_id = 'catch-up'`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var target, origin string
		if err := rows.Scan(&target, &origin); err != nil {
			t.Fatal(err)
		}
		origins[target] = origin
	}
	if got := origins["77777777-7777-4777-8777-777777777777"]; got != "agent" {
		t.Errorf("an activity row of a run with no origin was caught up as %q, want agent", got)
	}
	if got := origins["66666666-6666-4666-8666-666666666666"]; got != "unknown" {
		t.Errorf("an activity row of a person with no origin was caught up as %q, want unknown", got)
	}
}
