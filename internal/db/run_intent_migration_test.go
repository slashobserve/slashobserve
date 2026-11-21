package db_test

import (
	"context"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/audit"
)

const (
	runIntentVersion       = 84
	runIntentMigrationFile = "00084_run_intent.sql"
	intentDashboard        = "00000000-0000-4000-8000-0000000084d1"
)

func TestRunIntentMigrationBackfillsIntentDropsSourceAndRollsBack(t *testing.T) {
	ctx := context.Background()
	d := newMigratedTo(t, runIntentVersion-1)
	sqlDB := d.SQLDB(t)
	ada := convUser(t, sqlDB, "Ada")
	seedDashboard(t, d, intentDashboard, 1)
	seedVersion(t, d, intentDashboard, 1, "{}")
	insert := func(source, extra, values string) string {
		args := []any{ada, source}
		if strings.Contains(values, "$3") {
			args = append(args, intentDashboard)
		}
		return foScalar(t, sqlDB, `INSERT INTO agent_requests (prompt, requested_by_kind, requested_by_id, requested_by_name, source`+extra+`)
VALUES ('p', 'user', $1::uuid, 'Ada', $2`+values+`) RETURNING id::text`, args...)
	}
	rows := []struct {
		name, id, intent, kind, sourceAfterDown string
	}{
		{"a new-dashboard ask", insert("observe", "", ""), "ask", "build", "observe"},
		{"a build that saved a dashboard", insert("observe", ", dashboard_id, result_version, status, state", ", $3::uuid, 1, 'succeeded', 'succeeded'"), "apply", "build", "observe"},
		{"a build that names a dashboard and saved no version", insert("observe", ", dashboard_id, status, state", ", $3::uuid, 'failed', 'failed'"), "ask", "investigate", "investigate"},
		{"a dashboard chat revision", insert("prompt", ", dashboard_id, base_version", ", $3::uuid, 1"), "apply", "build", "prompt"},
		{"a suggestion", insert("suggest", ", dashboard_id, base_version, panel_id", ", $3::uuid, 1, 'a'"), "suggest", "build", "suggest"},
		{"an investigation", insert("investigate", ", context_from, context_to", ", now() - interval '1 hour', now()"), "ask", "investigate", "investigate"},
		{"a Home investigation", insert("investigate", "", ""), "ask", "investigate", "investigate"},
	}

	if err := d.MigrateUpTo(ctx, runIntentVersion); err != nil {
		t.Fatalf("up: %v", err)
	}
	intentOf := func(id string) string {
		return foScalar(t, sqlDB, `SELECT intent FROM agent_requests WHERE id = $1::uuid`, id)
	}
	for _, r := range rows {
		if got := intentOf(r.id); got != r.intent {
			t.Errorf("%s: intent = %q, want %q", r.name, got, r.intent)
		}
		if got := foScalar(t, sqlDB, `SELECT kind FROM agent_requests WHERE id = $1::uuid`, r.id); got != r.kind {
			t.Errorf("%s: kind = %q, want %q", r.name, got, r.kind)
		}
	}
	if got := foScalar(t, sqlDB, `SELECT count(*)::text FROM information_schema.columns WHERE table_name = 'agent_requests' AND column_name = 'source'`); got != "0" {
		t.Errorf("source columns after up = %s, want none", got)
	}
	if got := foScalar(t, sqlDB, `SELECT count(*)::text FROM pg_trigger WHERE tgname IN ('agent_requests_intent_of_source', 'agent_requests_old_server_chat')`); got != "0" {
		t.Errorf("old-server triggers after up = %s, want none", got)
	}
	if got := foScalar(t, sqlDB, `SELECT string_agg(conname || ':' || convalidated::text, ',' ORDER BY conname) FROM pg_constraint WHERE conrelid = 'agent_requests'::regclass AND NOT convalidated`); got != "" {
		t.Errorf("constraints left unvalidated: %s", got)
	}
	if _, err := sqlDB.Exec(`INSERT INTO agent_requests (prompt, requested_by_kind, requested_by_id, requested_by_name, intent, base_version) VALUES ('p', 'user', $1::uuid, 'Ada', 'ask', 1)`, ada); constraintOf(err) != "agent_requests_ask_shape_check" {
		t.Errorf("an ask with a base version: error = %v, want agent_requests_ask_shape_check", err)
	}
	if _, err := sqlDB.Exec(`INSERT INTO agent_requests (prompt, requested_by_kind, requested_by_id, requested_by_name, intent) VALUES ('p', 'user', $1::uuid, 'Ada', 'apply')`, ada); constraintOf(err) != "agent_requests_ask_target_check" {
		t.Errorf("an apply with no dashboard: error = %v, want agent_requests_ask_target_check", err)
	}
	if _, err := sqlDB.Exec(`INSERT INTO agent_requests (prompt, requested_by_kind, requested_by_id, requested_by_name, intent, kind, dashboard_id) VALUES ('p', 'user', $1::uuid, 'Ada', 'ask', 'build', $2::uuid)`, ada, intentDashboard); constraintOf(err) != "agent_requests_build_ask_check" {
		t.Errorf("a build ask about a dashboard: error = %v, want agent_requests_build_ask_check", err)
	}
	if _, err := sqlDB.Exec(`INSERT INTO agent_requests (prompt, requested_by_kind, requested_by_id, requested_by_name, intent, kind, dashboard_id, base_version) VALUES ('p', 'user', $1::uuid, 'Ada', 'apply', 'investigate', $2::uuid, 1)`, ada, intentDashboard); constraintOf(err) != "agent_requests_kind_check" {
		t.Errorf("an investigate apply: error = %v, want agent_requests_kind_check", err)
	}

	runMigrationHalf(t, sqlDB, runIntentMigrationFile, "Up")
	for _, r := range rows {
		if got := intentOf(r.id); got != r.intent {
			t.Errorf("after a second up, %s: intent = %q, want %q", r.name, got, r.intent)
		}
	}

	if err := d.MigrateDownTo(ctx, runIntentVersion-1); err != nil {
		t.Fatalf("down: %v", err)
	}
	for _, r := range rows {
		if got := foScalar(t, sqlDB, `SELECT source FROM agent_requests WHERE id = $1::uuid`, r.id); got != r.sourceAfterDown {
			t.Errorf("after down, %s: source = %q, want %q", r.name, got, r.sourceAfterDown)
		}
	}
	if got := foScalar(t, sqlDB, `SELECT count(*)::text FROM information_schema.columns WHERE table_name = 'agent_requests' AND column_name IN ('intent', 'applies_finding_id')`); got != "0" {
		t.Errorf("intent columns after down = %s, want none", got)
	}
	if got := foScalar(t, sqlDB, `SELECT count(*)::text FROM pg_trigger WHERE tgname = 'agent_requests_old_server_chat'`); got != "1" {
		t.Errorf("old-server chat trigger after down = %s, want it back", got)
	}
	if err := d.MigrateUpTo(ctx, runIntentVersion); err != nil {
		t.Fatalf("up again: %v", err)
	}
	if got := intentOf(rows[1].id); got != "apply" {
		t.Errorf("after up again, a saved build: intent = %q, want apply", got)
	}
}

func TestAnOldServersInsertDuringTheIntentBackfillGetsAnIntent(t *testing.T) {
	d := newMigratedTo(t, runIntentVersion-1)
	sqlDB := d.SQLDB(t)
	ada := convUser(t, sqlDB, "Ada")
	seedDashboard(t, d, intentDashboard, 1)
	body := readMigrationUp(t, runIntentMigrationFile)
	head, _, ok := strings.Cut(body, "ALTER TABLE agent_requests\n  DROP CONSTRAINT IF EXISTS agent_requests_intent_check")
	if !ok {
		t.Fatal("the migration no longer adds agent_requests_intent_check after the backfill")
	}
	for _, statement := range migrationStatements(head) {
		if _, err := sqlDB.Exec(statement); err != nil {
			t.Fatalf("%v\n%s", err, statement)
		}
	}
	cases := []struct{ source, extra, values, want, kind string }{
		{"observe", "", "", "ask", "build"},
		{"prompt", ", dashboard_id, base_version", ", $3::uuid, 1", "apply", "build"},
		{"suggest", ", dashboard_id, base_version, panel_id", ", $3::uuid, 1, 'a'", "suggest", "build"},
		{"investigate", "", "", "ask", "investigate"},
		{"investigate", ", dashboard_id", ", $3::uuid", "ask", "investigate"},
	}
	for _, c := range cases {
		args := []any{ada, c.source}
		if strings.Contains(c.values, "$3") {
			args = append(args, intentDashboard)
		}
		id := foScalar(t, sqlDB, `INSERT INTO agent_requests (prompt, requested_by_kind, requested_by_id, requested_by_name, source`+c.extra+`)
VALUES ('p', 'user', $1::uuid, 'Ada', $2`+c.values+`) RETURNING id::text`, args...)
		if got := foScalar(t, sqlDB, `SELECT intent FROM agent_requests WHERE id = $1::uuid`, id); got != c.want {
			t.Errorf("an old server's %s run: intent = %q, want %q", c.source, got, c.want)
		}
		if got := foScalar(t, sqlDB, `SELECT kind FROM agent_requests WHERE id = $1::uuid`, id); got != c.kind {
			t.Errorf("an old server's %s run: kind = %q, want %q", c.source, got, c.kind)
		}
		if c.source == "observe" {
			d.Exec(t, `UPDATE agent_requests SET dashboard_id = $2::uuid, result_version = 1, status = 'succeeded', state = 'succeeded' WHERE id = $1::uuid`, id, intentDashboard)
			if got := foScalar(t, sqlDB, `SELECT intent FROM agent_requests WHERE id = $1::uuid`, id); got != "apply" {
				t.Errorf("an old server's build that saved: intent = %q, want apply", got)
			}
		}
	}
}

func readMigrationUp(t *testing.T, file string) string {
	t.Helper()
	raw, err := fs.ReadFile(os.DirFS("migrations"), file)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	_, up, ok := strings.Cut(body, "-- +goose Up\n")
	if !ok {
		t.Fatalf("%s has no Up section", file)
	}
	up, _, _ = strings.Cut(up, "-- +goose Down\n")
	return up
}

func TestRunEventsKeepTheirIntentInTheAuditCatchUp(t *testing.T) {
	ctx := context.Background()
	d := newMigratedTo(t, runIntentVersion-1)
	sqlDB := d.SQLDB(t)
	ada := convUser(t, sqlDB, "Ada")
	seedDashboard(t, d, intentDashboard, 1)
	seedVersion(t, d, intentDashboard, 1, "{}")
	saved := foScalar(t, sqlDB, `INSERT INTO agent_requests (prompt, requested_by_kind, requested_by_id, requested_by_name, source, dashboard_id, result_version, status, state)
VALUES ('p', 'user', $1::uuid, 'Ada', 'observe', $2::uuid, 1, 'succeeded', 'succeeded') RETURNING id::text`, ada, intentDashboard)
	event := func(target, data string) string {
		return foScalar(t, sqlDB, `INSERT INTO activity (type, target_kind, target_id, actor_kind, actor_id, actor_name, origin, data)
VALUES ('run.queued', 'run', $1::uuid, 'user', $2::uuid, 'Ada', 'web', $3::jsonb) RETURNING seq::text`, target, ada, data)
	}
	cases := []struct{ name, seq, intent, sourceAfterDown string }{
		{"a build that later saved", event(saved, `{"source": "observe", "attempt": 1}`), "apply", "observe"},
		{"a deleted revision", event("00000000-0000-4000-8000-0000000084e1", `{"source": "comment", "attempt": 1}`), "apply", "prompt"},
		{"a deleted investigation", event("00000000-0000-4000-8000-0000000084e2", `{"source": "investigate", "attempt": 1}`), "ask", "observe"},
		{"a deleted suggestion", event("00000000-0000-4000-8000-0000000084e3", `{"source": "suggest", "attempt": 1}`), "suggest", "suggest"},
	}
	if err := d.MigrateUpTo(ctx, runIntentVersion); err != nil {
		t.Fatalf("up: %v", err)
	}
	if got := foScalar(t, sqlDB, `SELECT array_to_string(keys, ',') FROM audit_activity_actions WHERE type = 'run.queued'`); !strings.HasPrefix(got, "intent,") {
		t.Errorf("run.queued backfills keys %s, want intent in place of source", got)
	}
	job := audit.New(audit.Config{DB: d.DB}).PartitionJob()
	for range 2 {
		if err := job(ctx); err != nil {
			t.Fatal(err)
		}
	}
	want := func(intent string) string { return `{"intent": "` + intent + `", "attempt": 1}` }
	for _, c := range cases {
		if got := foScalar(t, sqlDB, `SELECT data::text FROM activity WHERE seq = $1::bigint`, c.seq); got != want(c.intent) {
			t.Errorf("%s: activity data = %s, want %s", c.name, got, want(c.intent))
		}
		if got := foScalar(t, sqlDB, `SELECT data::text FROM audit_events WHERE activity_seq = $1::bigint`, c.seq); got != want(c.intent) {
			t.Errorf("%s: audit data = %s, want %s", c.name, got, want(c.intent))
		}
	}

	if err := d.MigrateDownTo(ctx, runIntentVersion-1); err != nil {
		t.Fatalf("down: %v", err)
	}
	for _, c := range cases {
		if got := foScalar(t, sqlDB, `SELECT data->>'source' FROM activity WHERE seq = $1::bigint`, c.seq); got != c.sourceAfterDown {
			t.Errorf("after down, %s: source = %q, want %q", c.name, got, c.sourceAfterDown)
		}
	}
}
