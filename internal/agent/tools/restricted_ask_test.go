package tools

import (
	"errors"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/folders"
)

func (r *accessRig) runner(t *testing.T) string {
	t.Helper()
	var account string
	if err := r.db.SQLDB(t).QueryRowContext(reading(t.Context()), `INSERT INTO service_accounts (name, takes_runs) VALUES ('runner', true) RETURNING id::text`).Scan(&account); err != nil {
		t.Fatal(err)
	}
	return account
}

func (r *accessRig) runWroteInto(t *testing.T, observation string, restricted bool) {
	t.Helper()
	var run string
	if err := r.db.SQLDB(t).QueryRowContext(reading(t.Context()), `INSERT INTO agent_requests (intent, prompt, requested_by_kind, requested_by_id, requested_by_name, status, state, failure_code, read_restricted)
		VALUES ('ask', 'who changed it', 'user', $1::uuid, 'Root', 'failed', 'failed', 'internal', $2) RETURNING id::text`, adminCaller.ID, restricted).Scan(&run); err != nil {
		t.Fatal(err)
	}
	r.db.Exec(t, `INSERT INTO observation_blocks (observation_id, kind, position, body, created_by_kind, created_by_id, created_by_name, run_id) VALUES ($1::uuid, 'text', 99, '{"markdown":"Root changed it"}', 'user', $2::uuid, 'Root', $3::uuid)`, observation, adminCaller.ID, run)
}

func TestAnMCPAskInAnObservationThreadThatHoldsARestrictedAnswerIsMarkedAndItsAnswerStaysWithAdmins(t *testing.T) {
	for _, restricted := range []bool{false, true} {
		r := newAccessRig(t)
		account := r.runner(t)
		observation := r.privateFinding(t, adminCaller).ObservationID
		r.runWroteInto(t, observation, restricted)

		created := r.call(t, adminCaller, nameCreateComment, `{"target":"observation:`+observation+`","body":"@[runner](service_account:`+account+`) what else changed?","to":"agent"}`)
		out, ok := created.Content.(commentResultOut)
		if !ok || out.Run == nil {
			t.Fatalf("create = %s", textOf(t, created))
		}
		run := mustUUID(t, out.Run.ID)
		queued, err := r.db.Queries().GetAgentRequestByID(reading(t.Context()), run)
		if err != nil || queued.ReadRestricted != restricted || !queued.SubjectObservationID.Valid {
			t.Fatalf("the observation holds a restricted answer %v: the ask about %v is marked %v, %v", restricted, queued.SubjectObservationID, queued.ReadRestricted, err)
		}

		r.db.Exec(t, `UPDATE agent_requests SET state = 'succeeded', status = 'succeeded', result_kind = 'observation', finished_at = now() WHERE id = $1`, run)
		if _, _, err := r.findings.Save(reading(t.Context()), adminCaller, findings.SaveInput{AgentRequestID: run, Title: "Root changed more", Markdown: "and the alerts", From: time.Now().Add(-time.Hour), To: time.Now()}); err != nil {
			t.Fatal(err)
		}
		saved, _, err := r.findings.SaveAsObservation(reading(t.Context()), adminCaller, run)
		after, readErr := r.db.Queries().GetAgentRequestByID(reading(t.Context()), run)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if restricted {
			if !errors.Is(err, folders.ErrRestrictedAnswer) || after.ObservationID.Valid || after.SubjectObservationID != mustUUID(t, observation) || !after.ReadRestricted {
				t.Fatalf("saving a restricted answer about the observation: err %v, run %+v", err, after)
			}
			continue
		}
		if err != nil || saved.Restricted || !after.ObservationID.Valid || after.SubjectObservationID != mustUUID(t, observation) {
			t.Fatalf("saving an unrestricted answer: %+v, %v, run %+v", saved, err, after)
		}
	}
}
