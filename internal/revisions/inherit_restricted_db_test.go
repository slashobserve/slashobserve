package revisions_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

type answeredRun struct {
	run, observation pgtype.UUID
}

func answeredObservation(t *testing.T, d *dbtest.Database, run pgtype.UUID) answeredRun {
	t.Helper()
	var observation pgtype.UUID
	if err := d.SQLDB(t).QueryRowContext(t.Context(), `SELECT observation_id FROM findings WHERE agent_request_id = $1`, run).Scan(&observation); err != nil {
		t.Fatal(err)
	}
	return answeredRun{run: run, observation: observation}
}

func (a answeredRun) threadAsk(t *testing.T, d *dbtest.Database, next pgtype.UUID) {
	t.Helper()
	var thread, question pgtype.UUID
	if err := d.SQLDB(t).QueryRowContext(t.Context(), `INSERT INTO threads (observation_id) VALUES ($1) RETURNING id`, a.observation).Scan(&thread); err != nil {
		t.Fatal(err)
	}
	d.Exec(t, `INSERT INTO thread_messages (thread_id, body, author_kind, author_id, author_name, run_id, run_assigned_kind) VALUES ($1, 'Root changed it', 'user', $2::uuid, 'Ada', $3, 'builtin')`, thread, userID, a.run)
	if err := d.SQLDB(t).QueryRowContext(t.Context(), `INSERT INTO thread_messages (thread_id, body, author_kind, author_id, author_name) VALUES ($1, 'and then?', 'user', $2::uuid, 'Ada') RETURNING id`, thread, userID).Scan(&question); err != nil {
		t.Fatal(err)
	}
	d.Exec(t, `UPDATE agent_requests SET thread_message_id = $2 WHERE id = $1`, next, question)
}

func (a answeredRun) apply(t *testing.T, d *dbtest.Database, next pgtype.UUID) {
	t.Helper()
	dash := seedDashboard(t, d, 1)
	d.Exec(t, `UPDATE agent_requests SET intent = 'apply', dashboard_id = $2, base_version = 1, applies_finding_id = (SELECT id FROM findings WHERE agent_request_id = $3) WHERE id = $1`, next, dash, a.run)
}

func (a answeredRun) continuation(t *testing.T, d *dbtest.Database, next pgtype.UUID) {
	t.Helper()
	d.Exec(t, `UPDATE agent_requests SET continues_id = $2 WHERE id = $1`, next, a.run)
}

func (a answeredRun) askAbout(t *testing.T, d *dbtest.Database, next pgtype.UUID) {
	t.Helper()
	d.Exec(t, `UPDATE agent_requests SET kind = 'investigate', subject_observation_id = $2 WHERE id = $1`, next, a.observation)
}

func TestARunThatStartsFromAnAnswerThatReadAdminOnlyDataIsMarkedLikeIt(t *testing.T) {
	cases := map[string]func(answeredRun, *testing.T, *dbtest.Database, pgtype.UUID){
		"a thread ask in a thread that holds the answer": answeredRun.threadAsk,
		"an apply of the answer":                         answeredRun.apply,
		"a continuation of the run":                      answeredRun.continuation,
		"an ask about the observation that holds it":     answeredRun.askAbout,
	}
	for name, startFrom := range cases {
		for _, restricted := range []bool{false, true} {
			t.Run(name, func(t *testing.T) {
				svc, d := newService(t, succeedWith(2))
				ctx := reading(context.Background())
				first, err := svc.SubmitObserve(ctx, user, "who changed the checkout dashboard")
				if err != nil {
					t.Fatal(err)
				}
				answerWith(t, d, first.ID, "Root changed it at 09:40")
				d.Exec(t, `UPDATE agent_requests SET read_restricted = $2 WHERE id = $1`, first.ID, restricted)
				next, err := svc.SubmitObserve(ctx, user, "what else did they change")
				if err != nil {
					t.Fatal(err)
				}
				startFrom(answeredObservation(t, d, first.ID), t, d, next.ID)

				if claimed, found := claimOne(t, svc); !found || claimed.ID != next.ID || claimed.ReadRestricted != restricted {
					t.Fatalf("answer restricted %v: claimed %v (found %v) marked %v", restricted, claimed.ID, found, claimed.ReadRestricted)
				}
				stored, err := svc.GetByID(ctx, next.ID)
				if err != nil {
					t.Fatal(err)
				}
				if stored.ReadRestricted != restricted {
					t.Fatalf("answer restricted %v: the run is stored marked %v", restricted, stored.ReadRestricted)
				}
			})
		}
	}
}
