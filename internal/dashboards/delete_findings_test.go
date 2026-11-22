package dashboards

import (
	"database/sql"
	"testing"
)

type answerAppliedByLaterRun struct {
	answerRun string
	answer    string
	applyRun  string
}

func (r *rig) seedAnswerRunScannedBeforeTheRunThatAppliesIt(t *testing.T, dashboard string, chat *string) answerAppliedByLaterRun {
	t.Helper()
	sqlDB := r.db.SQLDB(t)
	var seeded answerAppliedByLaterRun
	if err := sqlDB.QueryRowContext(t.Context(), `INSERT INTO agent_requests (intent, kind, prompt, requested_by_kind, requested_by_id, requested_by_name, dashboard_id, chat_id)
		VALUES ('ask', 'investigate', 'which panel is busiest', 'user', $1, 'Ada', $2, $3) RETURNING id`, testUser.ID, dashboard, chat).Scan(&seeded.answerRun); err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.QueryRowContext(t.Context(), `INSERT INTO findings (agent_request_id, dashboard_id, title, markdown, time_from, time_to, created_by_kind, created_by_id, created_by_name)
		VALUES ($1, $2, 'Answer', 'It is the first one', now() - interval '1 hour', now(), 'user', $3, 'Ada') RETURNING id`, seeded.answerRun, dashboard, testUser.ID).Scan(&seeded.answer); err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.QueryRowContext(t.Context(), `INSERT INTO agent_requests (intent, kind, prompt, requested_by_kind, requested_by_id, requested_by_name, dashboard_id, base_version, applies_finding_id, chat_id, created_at)
		VALUES ('apply', 'build', 'revise the dashboard', 'user', $1, 'Ada', $2, 1, $3, $4, now() - interval '1 hour') RETURNING id`, testUser.ID, dashboard, seeded.answer, chat).Scan(&seeded.applyRun); err != nil {
		t.Fatal(err)
	}
	return seeded
}

func (r *rig) rowCount(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestDeletingADashboardDeletesAnAnswerThatALaterRunRevisedFrom(t *testing.T) {
	tests := []struct {
		name     string
		withChat bool
	}{
		{"runs outside a chat", false},
		{"runs in a chat the delete empties", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRig(t, true)
			id := r.createDashboard(t)
			var chat *string
			if tt.withChat {
				var chatID string
				if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `INSERT INTO chats (owner_kind, owner_id, dashboard_id) VALUES ('user', $1, $2) RETURNING id`, testUser.ID, id.String()).Scan(&chatID); err != nil {
					t.Fatal(err)
				}
				chat = &chatID
			}
			seeded := r.seedAnswerRunScannedBeforeTheRunThatAppliesIt(t, id.String(), chat)
			if err := r.service.Delete(reading(t.Context()), testUser, id); err != nil {
				t.Fatalf("delete: %v", err)
			}
			if n := r.rowCount(t, `SELECT count(*) FROM dashboards WHERE id = $1`, id.String()); n != 0 {
				t.Fatalf("%d dashboard rows left", n)
			}
			if n := r.rowCount(t, `SELECT count(*) FROM agent_requests WHERE id IN ($1, $2)`, seeded.answerRun, seeded.applyRun); n != 0 {
				t.Fatalf("%d runs left for a deleted dashboard", n)
			}
			if n := r.rowCount(t, `SELECT count(*) FROM findings WHERE id = $1`, seeded.answer); n != 0 {
				t.Fatalf("%d answers left for a deleted dashboard", n)
			}
		})
	}
}

func TestDeletingADashboardLeavesAnotherDashboardsRunThatRevisedFromItsAnswer(t *testing.T) {
	r := newRig(t, true)
	doomed := r.createDashboard(t)
	kept := r.createDashboard(t, "p99")
	seeded := r.seedAnswerRunScannedBeforeTheRunThatAppliesIt(t, doomed.String(), nil)
	if _, err := r.db.SQLDB(t).ExecContext(t.Context(), `UPDATE agent_requests SET dashboard_id = $1 WHERE id = $2`, kept.String(), seeded.applyRun); err != nil {
		t.Fatal(err)
	}
	if err := r.service.Delete(reading(t.Context()), testUser, doomed); err != nil {
		t.Fatalf("delete: %v", err)
	}
	var applies sql.NullString
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT applies_finding_id::text FROM agent_requests WHERE id = $1`, seeded.applyRun).Scan(&applies); err != nil {
		t.Fatalf("the other dashboard's run was deleted: %v", err)
	}
	if applies.Valid {
		t.Fatalf("applies_finding_id = %s, want it cleared with the answer", applies.String)
	}
}

func TestDeletingAUserDeletesTheAnswerOfARunOnTheirPersonalDashboard(t *testing.T) {
	r := newRig(t, true)
	sqlDB := r.db.SQLDB(t)
	var user, personal string
	if err := sqlDB.QueryRowContext(t.Context(), `INSERT INTO users (oidc_issuer, oidc_subject, email, name) VALUES ('https://idp', 'grace', 'grace@example.com', 'Grace') RETURNING id::text`).Scan(&user); err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.QueryRowContext(t.Context(), `SELECT id::text FROM folders WHERE owner_id = $1`, user).Scan(&personal); err != nil {
		t.Fatal(err)
	}
	id := r.createDashboard(t)
	if _, err := sqlDB.ExecContext(t.Context(), `UPDATE dashboards SET folder_id = $1 WHERE id = $2`, personal, id.String()); err != nil {
		t.Fatal(err)
	}
	seeded := r.seedAnswerRunScannedBeforeTheRunThatAppliesIt(t, id.String(), nil)
	if _, err := sqlDB.ExecContext(t.Context(), `DELETE FROM users WHERE id = $1`, user); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	if n := r.rowCount(t, `SELECT count(*) FROM agent_requests WHERE id IN ($1, $2)`, seeded.answerRun, seeded.applyRun); n != 0 {
		t.Fatalf("%d runs left for a deleted personal dashboard", n)
	}
}

func TestDeletingADashboardKeepsAnAnswerSavedAsAnObservation(t *testing.T) {
	r := newRig(t, true)
	id := r.createDashboard(t)
	sqlDB := r.db.SQLDB(t)
	var run, observation, finding string
	if err := sqlDB.QueryRowContext(t.Context(), `INSERT INTO agent_requests (intent, kind, prompt, requested_by_kind, requested_by_id, requested_by_name, dashboard_id)
		VALUES ('ask', 'investigate', 'which panel is busiest', 'user', $1, 'Ada', $2) RETURNING id`, testUser.ID, id.String()).Scan(&run); err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.QueryRowContext(t.Context(), `INSERT INTO observations (folder_id, title, created_by_kind, created_by_id, created_by_name, run_id)
		VALUES (general_folder_id(), 'Busiest panel', 'user', $1, 'Ada', $2) RETURNING id`, testUser.ID, run).Scan(&observation); err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.QueryRowContext(t.Context(), `INSERT INTO findings (agent_request_id, observation_id, title, markdown, time_from, time_to, created_by_kind, created_by_id, created_by_name)
		VALUES ($1, $2, 'Answer', 'It is the first one', now() - interval '1 hour', now(), 'user', $3, 'Ada') RETURNING id`, run, observation, testUser.ID).Scan(&finding); err != nil {
		t.Fatal(err)
	}
	if err := r.service.Delete(reading(t.Context()), testUser, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	var runID sql.NullString
	if err := sqlDB.QueryRowContext(t.Context(), `SELECT agent_request_id::text FROM findings WHERE id = $1 AND observation_id = $2`, finding, observation).Scan(&runID); err != nil {
		t.Fatalf("the saved answer did not survive: %v", err)
	}
	if runID.Valid {
		t.Fatalf("agent_request_id = %s, want it cleared with the deleted run", runID.String)
	}
	if n := r.rowCount(t, `SELECT count(*) FROM observations WHERE id = $1`, observation); n != 1 {
		t.Fatalf("%d observations left, want the one the answer was saved as", n)
	}
}

func TestDeletingADashboardStillFlagsTheObservationItsRestrictedAnswerWasSavedAs(t *testing.T) {
	tests := []struct {
		name       string
		withBlock  bool
		wantCopied bool
	}{
		{"the answer's block carries the run", true, true},
		{"only the finding names the run", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRig(t, true)
			id := r.createDashboard(t)
			sqlDB := r.db.SQLDB(t)
			var run, observation string
			if err := sqlDB.QueryRowContext(t.Context(), `INSERT INTO agent_requests (intent, kind, prompt, requested_by_kind, requested_by_id, requested_by_name, dashboard_id, read_restricted)
				VALUES ('ask', 'investigate', 'who changed it', 'user', $1, 'Ada', $2, true) RETURNING id`, testUser.ID, id.String()).Scan(&run); err != nil {
				t.Fatal(err)
			}
			if err := sqlDB.QueryRowContext(t.Context(), `INSERT INTO observations (folder_id, title, created_by_kind, created_by_id, created_by_name, run_id)
				VALUES (general_folder_id(), 'Who changed it', 'user', $1, 'Ada', $2) RETURNING id`, testUser.ID, run).Scan(&observation); err != nil {
				t.Fatal(err)
			}
			if _, err := sqlDB.ExecContext(t.Context(), `INSERT INTO findings (agent_request_id, dashboard_id, observation_id, title, markdown, time_from, time_to, created_by_kind, created_by_id, created_by_name)
				VALUES ($1, $2, $3, 'Answer', 'Root changed it', now() - interval '1 hour', now(), 'user', $4, 'Ada')`, run, id.String(), observation, testUser.ID); err != nil {
				t.Fatal(err)
			}
			if tt.withBlock {
				if _, err := sqlDB.ExecContext(t.Context(), `INSERT INTO observation_blocks (observation_id, kind, position, body, created_by_kind, created_by_id, created_by_name, run_id)
					VALUES ($1, 'text', 0, '{"markdown":"Root changed it"}', 'user', $2, 'Ada', $3)`, observation, testUser.ID, run); err != nil {
					t.Fatal(err)
				}
			}
			if err := r.service.Delete(reading(t.Context()), testUser, id); err != nil {
				t.Fatalf("delete: %v", err)
			}
			var copied bool
			if err := sqlDB.QueryRowContext(t.Context(), `SELECT copied_restricted_answer FROM observations WHERE id = $1`, observation).Scan(&copied); err != nil {
				t.Fatal(err)
			}
			if copied != tt.wantCopied {
				t.Fatalf("copied_restricted_answer = %t, want %t", copied, tt.wantCopied)
			}
		})
	}
}
