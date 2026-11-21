package db_test

import (
	"context"
	"testing"
)

const runReadRestrictedVersion = 87

func TestRunReadRestrictedMigrationMarksRunsThatQueriedADatasourceOnlyAdminsQuery(t *testing.T) {
	ctx := context.Background()
	d := newMigratedTo(t, runReadRestrictedVersion-1)
	sqlDB := d.SQLDB(t)
	ada := convUser(t, sqlDB, "Ada")
	run := func(action, datasource, outcome string) string {
		id := foScalar(t, sqlDB, `INSERT INTO agent_requests (prompt, requested_by_kind, requested_by_id, requested_by_name, intent)
VALUES ('p', 'user', $1::uuid, 'Ada', 'ask') RETURNING id::text`, ada)
		if action != "" {
			d.Exec(t, `INSERT INTO audit_events (id, occurred_at, category, action, outcome, actor_kind, actor_id, initiator, run_id, origin, datasource_uid, server_id, server_version)
VALUES (gen_random_uuid(), now(), 'query', $1, $2, 'user', $3::uuid, 'agent', $4::uuid, 'agent', $5, 's', 'v')`, action, outcome, ada, id, datasource)
		}
		return id
	}
	for uid, audience := range map[string]string{"secrets": "admins", "prom": "everyone"} {
		d.Exec(t, `INSERT INTO datasources (uid, type, name, config, audience) VALUES ($1, 'prometheus', $1, '{}', $2)`, uid, audience)
	}
	rows := []struct {
		name, id string
		want     string
	}{
		{"a run that queried the audit trail", run("query.run", "slashobserve-audit", "success"), "true"},
		{"a run refused the audit trail", run("query.run", "slashobserve-audit", "denied"), "false"},
		{"a run whose audit trail query failed", run("query.run", "slashobserve-audit", "failed"), "true"},
		{"a run that queried an app datasource only Admins query", run("query.run", "secrets", "success"), "true"},
		{"a run refused an app datasource only Admins query", run("query.run", "secrets", "denied"), "false"},
		{"a run that queried another datasource", run("query.run", "prom", "success"), "false"},
		{"a run whose tool call named the audit datasource", run("agent.tool_call", "slashobserve-audit", "success"), "false"},
		{"a run with no queries", run("", "", ""), "false"},
	}

	if err := d.MigrateUpTo(ctx, runReadRestrictedVersion); err != nil {
		t.Fatalf("up: %v", err)
	}
	for _, r := range rows {
		if got := foScalar(t, sqlDB, `SELECT read_restricted::text FROM agent_requests WHERE id = $1::uuid`, r.id); got != r.want {
			t.Errorf("%s: read_restricted = %s, want %s", r.name, got, r.want)
		}
	}

	if err := d.MigrateDownTo(ctx, runReadRestrictedVersion-1); err != nil {
		t.Fatalf("down: %v", err)
	}
	if got := foScalar(t, sqlDB, `SELECT count(*)::text FROM pg_proc WHERE proname LIKE '%restricted%' OR proname LIKE '%run_in_flight%'`); got != "0" {
		t.Errorf("functions left after down = %s, want none", got)
	}
}
