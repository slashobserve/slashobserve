package db_test

import (
	"database/sql"
	"testing"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

const (
	findingsVersion           = 36
	agentRequestTitlesVersion = 37
	findingsDashboard         = "00000000-0000-4000-8000-000000000001"
)

func insertRequest(sqlDB *sql.DB, columns, values string, args ...any) error {
	_, err := sqlDB.Exec(`INSERT INTO agent_requests (prompt, requested_by_kind, requested_by_id, requested_by_name, `+columns+`)
VALUES ('p', 'user', gen_random_uuid(), 'seed', `+values+`)`, args...)
	return err
}

func insertFinding(sqlDB *sql.DB, evidence string) error {
	_, err := sqlDB.Exec(`INSERT INTO findings (title, markdown, evidence, time_from, time_to, created_by_kind, created_by_id, created_by_name)
VALUES ('t', 'm', $1::jsonb, now() - interval '1 hour', now(), 'user', gen_random_uuid(), 'seed')`, evidence)
	return err
}

func TestInvestigateRowsFollowTheirShape(t *testing.T) {
	ctx := t.Context()
	d := dbtest.NewUnmigrated(t)
	if err := d.MigrateUpTo(ctx, findingsVersion); err != nil {
		t.Fatal(err)
	}
	seedDashboard(t, d, findingsDashboard, 1)
	sqlDB := d.SQLDB(t)
	cases := []struct {
		name, columns, values string
		args                  []any
		want                  string
	}{
		{"an investigation without a dashboard", "source, context_from, context_to",
			"'investigate', now() - interval '1 hour', now()", nil, ""},
		{"an investigation of a panel without a dashboard", "source, panel_id",
			"'investigate', 'p1'", nil, "agent_requests_investigate_shape_check"},
		{"an observe request with a window", "source, context_from, context_to",
			"'observe', now() - interval '1 hour', now()", nil, "agent_requests_context_check"},
		{"a succeeded investigation without a version", "source, status",
			"'investigate', 'succeeded'", nil, ""},
		{"a succeeded prompt without a version", "source, status, dashboard_id, base_version",
			"'prompt', 'succeeded', $1::uuid, 1", []any{findingsDashboard}, "agent_requests_succeeded_check"},
		{"a first running investigation on a dashboard", "source, status, dashboard_id",
			"'investigate', 'running', $1::uuid", []any{findingsDashboard}, ""},
		{"a second running investigation on a dashboard", "source, status, dashboard_id",
			"'investigate', 'running', $1::uuid", []any{findingsDashboard}, ""},
		{"a first running prompt on a dashboard", "source, status, dashboard_id, base_version",
			"'prompt', 'running', $1::uuid, 1", []any{findingsDashboard}, ""},
		{"a second running prompt on a dashboard", "source, status, dashboard_id, base_version",
			"'prompt', 'running', $1::uuid, 1", []any{findingsDashboard}, "agent_requests_running_dashboard_idx"},
	}
	for _, c := range cases {
		err := insertRequest(sqlDB, c.columns, c.values, c.args...)
		if got := constraintOf(err); got != c.want || (c.want == "" && err != nil) {
			t.Errorf("%s: error = %v, want violation %q", c.name, err, c.want)
		}
	}
}

func TestFindingsRejectMoreThanFourEvidencePanels(t *testing.T) {
	d := dbtest.New(t)
	sqlDB := d.SQLDB(t)
	cases := []struct {
		name, evidence, want string
	}{
		{"four panels", `[{}, {}, {}, {}]`, ""},
		{"five panels", `[{}, {}, {}, {}, {}]`, "findings_evidence_check"},
		{"an object", `{}`, "findings_evidence_check"},
	}
	for _, c := range cases {
		err := insertFinding(sqlDB, c.evidence)
		if got := constraintOf(err); got != c.want || (c.want == "" && err != nil) {
			t.Errorf("%s: error = %v, want violation %q", c.name, err, c.want)
		}
	}
}

func TestFindingsMigrationsRollBack(t *testing.T) {
	ctx := t.Context()
	d := dbtest.NewUnmigrated(t)
	if err := d.MigrateUpTo(ctx, agentRequestTitlesVersion); err != nil {
		t.Fatal(err)
	}
	sqlDB := d.SQLDB(t)
	if err := insertRequest(sqlDB, "source, title", "'investigate', 'Why'"); err != nil {
		t.Fatal(err)
	}
	if err := insertFinding(sqlDB, `[]`); err != nil {
		t.Fatal(err)
	}
	for _, version := range []int64{findingsVersion, findingsVersion - 1} {
		if err := d.MigrateDownTo(ctx, version); err != nil {
			t.Fatalf("down to %d: %v", version, err)
		}
	}
	var table, columns, investigations int
	if err := sqlDB.QueryRow(`SELECT
  (SELECT count(*) FROM information_schema.tables WHERE table_name = 'findings'),
  (SELECT count(*) FROM information_schema.columns WHERE table_name = 'agent_requests' AND column_name IN ('context_from', 'context_to', 'title', 'pinned_at', 'archived_at')),
  (SELECT count(*) FROM agent_requests WHERE source = 'investigate')`).Scan(&table, &columns, &investigations); err != nil {
		t.Fatal(err)
	}
	if table != 0 || columns != 0 || investigations != 0 {
		t.Errorf("after rolling back: findings tables %d, new agent_requests columns %d, investigate rows %d", table, columns, investigations)
	}
}
