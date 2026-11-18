package revisions_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func answerInFolder(t *testing.T, d *dbtest.Database, id pgtype.UUID, markdown, folder string) {
	t.Helper()
	d.Exec(t, `UPDATE agent_requests SET state = 'succeeded', status = 'succeeded', result_kind = 'observation', finished_at = clock_timestamp() WHERE id = $1`, id)
	d.Exec(t, `WITH o AS (INSERT INTO observations (folder_id, title, created_by_kind, created_by_id, created_by_name)
  VALUES (`+folder+`, 'answer', 'user', $3::uuid, 'Ada') RETURNING id),
f AS (INSERT INTO findings (id, observation_id, agent_request_id, title, markdown, time_from, time_to, created_by_kind, created_by_id, created_by_name)
  SELECT o.id, o.id, $1, 'answer', $2, now() - interval '1 hour', now(), 'user', $3::uuid, 'Ada' FROM o RETURNING observation_id)
UPDATE agent_requests SET observation_id = (SELECT observation_id FROM f) WHERE id = $1`, id, markdown, userID)
}

func TestSearchFindsAChatByTheTextOfItsVisibleAnswer(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	ctx := context.Background()
	d.Exec(t, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, name) VALUES ($1::uuid, 'https://idp', 'Ada', 'Ada@example.com', 'Ada')`, userID)

	shared, err := svc.SubmitObserve(ctx, user, "why is checkout slow")
	if err != nil {
		t.Fatal(err)
	}
	answerInFolder(t, d, shared.ID, "PgBouncer saturation explains it", "general_folder_id()")
	hidden, err := svc.SubmitObserve(ctx, user, "anything odd today")
	if err != nil {
		t.Fatal(err)
	}
	answerInFolder(t, d, hidden.ID, "PgBouncer saturation again", "(SELECT id FROM folders WHERE owner_id = '"+userID+"'::uuid)")

	filter := revisions.ListFilter{Scope: revisions.ScopeMine, Query: "bouncer saturation"}
	rows, _, err := svc.ListRequests(ctx, user, filter, 10, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("the owner finds %d chats by answer text, want both", len(rows))
	}
	count, err := svc.CountRequests(ctx, user, filter)
	if err != nil || count.Total != 2 {
		t.Fatalf("count = %+v, %v, want 2", count, err)
	}

	filter.Scope = revisions.ScopeAll
	rows, _, err = svc.ListRequests(ctx, bea, filter, 10, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != shared.ID {
		t.Fatalf("an Admin finds %d chats, want only the one whose answer sits in a folder they can see", len(rows))
	}
	count, err = svc.CountRequests(ctx, bea, filter)
	if err != nil || count.Total != 1 {
		t.Fatalf("an Admin's count = %+v, %v, want 1: the total must not reveal a hidden answer", count, err)
	}
}
