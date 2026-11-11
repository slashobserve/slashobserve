package db_test

import (
	"context"
	"testing"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

const (
	proposalsVersion   = 39
	proposalsDashboard = "00000000-0000-4000-8000-0000000000b1"
	proposalsAuthor    = "00000000-0000-4000-8000-0000000000b2"
	proposalsOther     = "00000000-0000-4000-8000-0000000000b3"
)

const insertProposalSQL = `INSERT INTO proposals (dashboard_id, state, title, author_kind, author_id, author_name, closed_at, closed_by_kind, closed_by_id, closed_by_name)
VALUES ($1::uuid, $2, 'Add latency panel', 'user', $3::uuid, 'Ada', $4::timestamptz, $5, $6::uuid, $7)
RETURNING id::text`

const insertProposalActivitySQL = `INSERT INTO activity (type, target_kind, target_id, actor_kind, actor_id, actor_name)
VALUES ('proposal.opened', 'proposal', $1::uuid, 'user', gen_random_uuid(), 'Ada')`

const insertObserveRunSQL = `INSERT INTO agent_requests (prompt, requested_by_kind, requested_by_id, requested_by_name, source, status, state, dashboard_id, result_kind, finished_at)
VALUES ('p', 'user', gen_random_uuid(), 'seed', 'observe', 'succeeded', 'succeeded', $1::uuid, $2, now())`

func TestProposalsConstraints(t *testing.T) {
	ctx := context.Background()
	d := dbtest.NewUnmigrated(t)
	if err := d.MigrateUpTo(ctx, proposalsVersion); err != nil {
		t.Fatal(err)
	}
	seedDashboard(t, d, proposalsDashboard, 2)
	seedVersion(t, d, proposalsDashboard, 1, "{}")
	seedVersion(t, d, proposalsDashboard, 2, "{}")
	sqlDB := d.SQLDB(t)

	var open string
	if err := sqlDB.QueryRow(insertProposalSQL, proposalsDashboard, "open", proposalsAuthor, nil, nil, nil, nil).Scan(&open); err != nil {
		t.Fatalf("an open proposal was refused: %v", err)
	}
	if _, err := sqlDB.Exec(`INSERT INTO proposal_revisions (proposal_id, revision, base_version, spec, summary) VALUES ($1::uuid, 1, 1, '{}', 'first')`, open); err != nil {
		t.Fatalf("revision 1 was refused: %v", err)
	}
	if _, err := sqlDB.Exec(`INSERT INTO proposal_revisions (proposal_id, revision, base_version, spec, summary) VALUES ($1::uuid, 2, 1, '[]', 'array')`, open); constraintOf(err) != "proposal_revisions_spec_check" {
		t.Errorf("a revision whose spec is an array: error = %v, want a violation of proposal_revisions_spec_check", err)
	}

	err := sqlDB.QueryRow(insertProposalSQL, proposalsDashboard, "open", proposalsAuthor, nil, nil, nil, nil).Scan(new(string))
	if constraintOf(err) != "proposals_open_author_key" {
		t.Errorf("a second open proposal by the same author: error = %v, want a violation of proposals_open_author_key", err)
	}
	err = sqlDB.QueryRow(insertProposalSQL, proposalsDashboard, "merged", proposalsOther, nil, nil, nil, nil).Scan(new(string))
	if constraintOf(err) != "proposals_closed_check" {
		t.Errorf("a merged proposal with no closed_at: error = %v, want a violation of proposals_closed_check", err)
	}
	var merged string
	err = sqlDB.QueryRow(insertProposalSQL, proposalsDashboard, "merged", proposalsAuthor, "2026-01-01T10:00:00Z", "user", proposalsAuthor, "Ada").Scan(&merged)
	if err != nil {
		t.Fatalf("a merged proposal with its closing actor was refused: %v", err)
	}

	_, err = sqlDB.Exec(`INSERT INTO threads (dashboard_id, dashboard_version, proposal_id) VALUES ($1::uuid, 1, $2::uuid)`, proposalsDashboard, open)
	if constraintOf(err) != "threads_target_check" {
		t.Errorf("a thread on a dashboard and a proposal: error = %v, want a violation of threads_target_check", err)
	}
	if _, err := sqlDB.Exec(`INSERT INTO threads (proposal_id) VALUES ($1::uuid)`, open); err != nil {
		t.Errorf("a thread on a proposal was refused: %v", err)
	}
	if _, err := sqlDB.Exec(insertProposalActivitySQL, open); err != nil {
		t.Errorf("a proposal activity row was refused: %v", err)
	}

	if _, err := sqlDB.Exec(`UPDATE dashboard_versions SET merged_proposal_id = $2::uuid WHERE dashboard_id = $1::uuid AND version = 1`, proposalsDashboard, merged); err != nil {
		t.Fatalf("a version naming a merged proposal was refused: %v", err)
	}
	_, err = sqlDB.Exec(`UPDATE dashboard_versions SET merged_proposal_id = $2::uuid WHERE dashboard_id = $1::uuid AND version = 2`, proposalsDashboard, merged)
	if constraintOf(err) != "dashboard_versions_merged_proposal_key" {
		t.Errorf("a second version naming the same proposal: error = %v, want a violation of dashboard_versions_merged_proposal_key", err)
	}

	if _, err := sqlDB.Exec(insertObserveRunSQL, proposalsDashboard, "proposal"); err != nil {
		t.Errorf("an observe run that succeeded with a proposal was refused: %v", err)
	}
	if _, err := sqlDB.Exec(insertObserveRunSQL, proposalsDashboard, "none"); constraintOf(err) != "agent_requests_succeeded_check" {
		t.Errorf("an observe run on a dashboard that succeeded with nothing: error = %v, want a violation of agent_requests_succeeded_check", err)
	}
}

func TestProposalsDown(t *testing.T) {
	ctx := context.Background()
	d := dbtest.NewUnmigrated(t)
	if err := d.MigrateUpTo(ctx, proposalsVersion); err != nil {
		t.Fatal(err)
	}
	seedDashboard(t, d, proposalsDashboard, 1)
	seedVersion(t, d, proposalsDashboard, 1, "{}")
	sqlDB := d.SQLDB(t)
	var id string
	if err := sqlDB.QueryRow(insertProposalSQL, proposalsDashboard, "open", proposalsAuthor, nil, nil, nil, nil).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec(`INSERT INTO threads (proposal_id) VALUES ($1::uuid)`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec(insertProposalActivitySQL, id); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec(insertObserveRunSQL, proposalsDashboard, "proposal"); err != nil {
		t.Fatal(err)
	}

	if err := d.MigrateDownTo(ctx, proposalsVersion-1); err != nil {
		t.Fatalf("down: %v", err)
	}
	var proposals *string
	if err := sqlDB.QueryRow(`SELECT to_regclass('proposals')::text`).Scan(&proposals); err != nil {
		t.Fatal(err)
	}
	if proposals != nil {
		t.Fatalf("table %s survives the down migration", *proposals)
	}
	var columns int
	if err := sqlDB.QueryRow(`SELECT count(*) FROM information_schema.columns
WHERE (table_name = 'dashboard_versions' AND column_name = 'merged_proposal_id') OR (table_name = 'threads' AND column_name = 'proposal_id')`).Scan(&columns); err != nil {
		t.Fatal(err)
	}
	if columns != 0 {
		t.Fatalf("%d proposal columns survive the down migration", columns)
	}
	var state, code string
	if err := sqlDB.QueryRow(`SELECT state, failure_code FROM agent_requests WHERE dashboard_id = $1::uuid`, proposalsDashboard).Scan(&state, &code); err != nil {
		t.Fatal(err)
	}
	if state != "failed" || code != "internal" {
		t.Fatalf("a run that succeeded with a proposal reads back %s with failure %q, want failed with internal", state, code)
	}

	if err := d.MigrateUpTo(ctx, proposalsVersion); err != nil {
		t.Fatalf("up again: %v", err)
	}
}
