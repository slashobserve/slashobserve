package db_test

import (
	"database/sql"
	"testing"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

const findingsDashboard = "00000000-0000-4000-8000-000000000001"

type requestRow struct {
	source, status, dashboardID, panelID, title string
	intent                                      string
	baseVersion                                 int
	windowed                                    bool
}

const insertRequestSQL = `INSERT INTO agent_requests (prompt, requested_by_kind, requested_by_id, requested_by_name,
  source, status, dashboard_id, base_version, panel_id, title, context_from, context_to)
VALUES ('p', 'user', gen_random_uuid(), 'seed', $1, $2, nullif($3, '')::uuid, nullif($4, 0), nullif($5, ''), nullif($6, ''),
  CASE WHEN $7 THEN now() - interval '1 hour' END, CASE WHEN $7 THEN now() END)`

const insertRequestWithStateSQL = `INSERT INTO agent_requests (prompt, requested_by_kind, requested_by_id, requested_by_name,
  intent, kind, status, state, dashboard_id, base_version, panel_id, title, context_from, context_to)
VALUES ('p', 'user', gen_random_uuid(), 'seed', $1, CASE WHEN $1 = 'ask' AND $3 <> '' THEN 'investigate' ELSE 'build' END, $2, $2, nullif($3, '')::uuid, nullif($4, 0), nullif($5, ''), nullif($6, ''),
  CASE WHEN $7 THEN now() - interval '1 hour' END, CASE WHEN $7 THEN now() END)`

func insertRequest(sqlDB *sql.DB, r requestRow) error {
	return execRequest(sqlDB, insertRequestSQL, r)
}

func insertRequestWithState(sqlDB *sql.DB, r requestRow) error {
	r.source = r.intent
	return execRequest(sqlDB, insertRequestWithStateSQL, r)
}

func execRequest(sqlDB *sql.DB, query string, r requestRow) error {
	if r.status == "" {
		r.status = "queued"
	}
	_, err := sqlDB.Exec(query, r.source, r.status, r.dashboardID, r.baseVersion, r.panelID, r.title, r.windowed)
	return err
}

func insertFinding(sqlDB *sql.DB, evidence string) error {
	_, err := sqlDB.Exec(`INSERT INTO findings (title, markdown, evidence, time_from, time_to, created_by_kind, created_by_id, created_by_name)
VALUES ('t', 'm', $1::jsonb, now() - interval '1 hour', now(), 'user', gen_random_uuid(), 'seed')`, evidence)
	return err
}

func insertObservedFinding(sqlDB *sql.DB, evidence string) error {
	_, err := sqlDB.Exec(`WITH o AS (INSERT INTO observations (folder_id, title, created_by_kind, created_by_id, created_by_name)
  VALUES (general_folder_id(), 't', 'user', gen_random_uuid(), 'seed') RETURNING id)
INSERT INTO findings (observation_id, title, markdown, evidence, time_from, time_to, created_by_kind, created_by_id, created_by_name)
SELECT o.id, 't', 'm', $1::jsonb, now() - interval '1 hour', now(), 'user', gen_random_uuid(), 'seed' FROM o`, evidence)
	return err
}

func TestInvestigateRowsFollowTheirShape(t *testing.T) {
	d := dbtest.New(t)
	seedDashboard(t, d, findingsDashboard, 1)
	sqlDB := d.SQLDB(t)
	cases := []struct {
		name string
		row  requestRow
		want string
	}{
		{"an ask without a dashboard", requestRow{intent: "ask", windowed: true}, ""},
		{"an ask about a panel without a dashboard", requestRow{intent: "ask", panelID: "p1"}, "agent_requests_ask_shape_check"},
		{"an apply with a window", requestRow{intent: "apply", dashboardID: findingsDashboard, baseVersion: 1, windowed: true}, "agent_requests_intent_context_check"},
		{"a succeeded ask without a version", requestRow{intent: "ask", status: "succeeded"}, ""},
		{"an answered apply on a dashboard without a version", requestRow{intent: "apply", status: "succeeded", dashboardID: findingsDashboard, baseVersion: 1}, ""},
		{"an apply without a dashboard", requestRow{intent: "apply", status: "succeeded"}, "agent_requests_ask_target_check"},
		{"a succeeded apply with a dashboard but no version", requestRow{intent: "apply", status: "succeeded", dashboardID: findingsDashboard}, ""},
		{"a first running ask on a dashboard", requestRow{intent: "ask", status: "running", dashboardID: findingsDashboard}, ""},
		{"a second running ask on a dashboard", requestRow{intent: "ask", status: "running", dashboardID: findingsDashboard}, ""},
		{"a first running apply on a dashboard", requestRow{intent: "apply", status: "running", dashboardID: findingsDashboard, baseVersion: 1}, ""},
		{"a second running apply on a dashboard", requestRow{intent: "apply", status: "running", dashboardID: findingsDashboard, baseVersion: 1}, "agent_requests_running_dashboard_idx"},
	}
	for _, c := range cases {
		err := insertRequestWithState(sqlDB, c.row)
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
		err := insertObservedFinding(sqlDB, c.evidence)
		if got := constraintOf(err); got != c.want || (c.want == "" && err != nil) {
			t.Errorf("%s: error = %v, want violation %q", c.name, err, c.want)
		}
	}
}
