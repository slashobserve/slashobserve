package db_test

import (
	"database/sql"
	"testing"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

const findingsDashboard = "00000000-0000-4000-8000-000000000001"

type requestRow struct {
	source, status, dashboardID, panelID, title string
	baseVersion                                 int
	windowed                                    bool
}

const insertRequestSQL = `INSERT INTO agent_requests (prompt, requested_by_kind, requested_by_id, requested_by_name,
  source, status, dashboard_id, base_version, panel_id, title, context_from, context_to)
VALUES ('p', 'user', gen_random_uuid(), 'seed', $1, $2, nullif($3, '')::uuid, nullif($4, 0), nullif($5, ''), nullif($6, ''),
  CASE WHEN $7 THEN now() - interval '1 hour' END, CASE WHEN $7 THEN now() END)`

const insertRequestWithStateSQL = `INSERT INTO agent_requests (prompt, requested_by_kind, requested_by_id, requested_by_name,
  source, status, state, dashboard_id, base_version, panel_id, title, context_from, context_to)
VALUES ('p', 'user', gen_random_uuid(), 'seed', $1, $2, $2, nullif($3, '')::uuid, nullif($4, 0), nullif($5, ''), nullif($6, ''),
  CASE WHEN $7 THEN now() - interval '1 hour' END, CASE WHEN $7 THEN now() END)`

func insertRequest(sqlDB *sql.DB, r requestRow) error {
	return execRequest(sqlDB, insertRequestSQL, r)
}

func insertRequestWithState(sqlDB *sql.DB, r requestRow) error {
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
		{"an investigation without a dashboard", requestRow{source: "investigate", windowed: true}, ""},
		{"an investigation of a panel without a dashboard", requestRow{source: "investigate", panelID: "p1"}, "agent_requests_investigate_shape_check"},
		{"an observe request with a window", requestRow{source: "observe", windowed: true}, "agent_requests_context_check"},
		{"a succeeded investigation without a version", requestRow{source: "investigate", status: "succeeded"}, ""},
		{"an answered prompt on a dashboard without a version", requestRow{source: "prompt", status: "succeeded", dashboardID: findingsDashboard, baseVersion: 1}, ""},
		{"an answered comment on a dashboard without a version", requestRow{source: "comment", status: "succeeded", dashboardID: findingsDashboard, baseVersion: 1}, ""},
		{"a succeeded prompt without a dashboard", requestRow{source: "prompt", status: "succeeded"}, "agent_requests_succeeded_check"},
		{"an answered observe request without a dashboard or version", requestRow{source: "observe", status: "succeeded"}, ""},
		{"a succeeded observe request with a dashboard but no version", requestRow{source: "observe", status: "succeeded", dashboardID: findingsDashboard}, "agent_requests_succeeded_check"},
		{"a first running investigation on a dashboard", requestRow{source: "investigate", status: "running", dashboardID: findingsDashboard}, ""},
		{"a second running investigation on a dashboard", requestRow{source: "investigate", status: "running", dashboardID: findingsDashboard}, ""},
		{"a first running prompt on a dashboard", requestRow{source: "prompt", status: "running", dashboardID: findingsDashboard, baseVersion: 1}, ""},
		{"a second running prompt on a dashboard", requestRow{source: "prompt", status: "running", dashboardID: findingsDashboard, baseVersion: 1}, "agent_requests_running_dashboard_idx"},
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
