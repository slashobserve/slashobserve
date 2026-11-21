package observations_test

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func (r rig) chatRun(t *testing.T) (run, chat string) {
	t.Helper()
	err := r.db.SQLDB(t).QueryRowContext(reading(t.Context()), `WITH chat AS (
  INSERT INTO chats (owner_kind, owner_id) VALUES ('user', $1::uuid) RETURNING id)
INSERT INTO agent_requests (intent, prompt, requested_by_kind, requested_by_id, requested_by_name, chat_id)
SELECT 'ask', 'why', 'user', $1::uuid, 'Eve', chat.id FROM chat RETURNING id::text, chat_id::text`, editor.ID).Scan(&run, &chat)
	if err != nil {
		t.Fatal(err)
	}
	return run, chat
}

func (r rig) chatDashboard(t *testing.T, chat string) string {
	t.Helper()
	var dashboard *string
	if err := r.db.SQLDB(t).QueryRowContext(reading(t.Context()), `SELECT dashboard_id::text FROM chats WHERE id = $1::uuid`, chat).Scan(&dashboard); err != nil {
		t.Fatal(err)
	}
	if dashboard == nil {
		return ""
	}
	return *dashboard
}

func TestMakingADashboardFromOneBlockAttachesTheChatOfTheRunThatWroteIt(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	creating, creatingChat := r.chatRun(t)
	writing, writingChat := r.chatRun(t)
	r.db.Exec(t, `UPDATE observations SET run_id = $1::uuid WHERE id = $2::uuid`, creating, o.ID)
	block := r.addPanel(t, editor, o)
	r.db.Exec(t, `UPDATE observation_blocks SET run_id = $1::uuid WHERE id = $2::uuid`, writing, block.ID)

	made, _, _, err := r.service.MakeDashboard(reading(t.Context()), editor, pgtype.UUID{}, mustUUID(t, block.ID), false)
	if err != nil {
		t.Fatal(err)
	}
	if got := r.chatDashboard(t, writingChat); got != made.ID {
		t.Errorf("the chat of the run that wrote the block is about %q, want the new dashboard %s", got, made.ID)
	}
	if got := r.chatDashboard(t, creatingChat); got != "" {
		t.Errorf("the chat of the run that created the observation was moved onto %s by a dashboard made from another run's block", got)
	}
}

func TestMakingADashboardFromABlockWithoutARunFallsBackToTheObservationsRun(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	creating, creatingChat := r.chatRun(t)
	r.db.Exec(t, `UPDATE observations SET run_id = $1::uuid WHERE id = $2::uuid`, creating, o.ID)
	block := r.addPanel(t, editor, o)
	r.db.Exec(t, `UPDATE observation_blocks SET run_id = NULL WHERE id = $1::uuid`, block.ID)

	made, _, _, err := r.service.MakeDashboard(reading(t.Context()), editor, pgtype.UUID{}, mustUUID(t, block.ID), false)
	if err != nil {
		t.Fatal(err)
	}
	if got := r.chatDashboard(t, creatingChat); got != made.ID {
		t.Errorf("the chat is about %q, want the new dashboard %s", got, made.ID)
	}
}

func TestAChatThatCannotBeAttachedLeavesTheDashboardMadeFromTheObservation(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	run, chat := r.chatRun(t)
	r.db.Exec(t, `UPDATE observations SET run_id = $1::uuid WHERE id = $2::uuid`, run, o.ID)
	r.addPanel(t, editor, o)
	r.db.Exec(t, `CREATE FUNCTION refuse_chat_update() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'chat updates are refused'; END $$;
CREATE TRIGGER refuse_chat_update BEFORE UPDATE ON chats FOR EACH ROW EXECUTE FUNCTION refuse_chat_update();`)

	made, _, created, err := r.service.MakeDashboard(reading(t.Context()), editor, mustUUID(t, o.ID), pgtype.UUID{}, false)
	if err != nil || !created {
		t.Fatalf("make with a chat that cannot be attached: created %v: %v", created, err)
	}
	if got := r.chatDashboard(t, chat); got != "" {
		t.Errorf("the chat is about %s through a refused update", got)
	}
	again, _, created, err := r.service.MakeDashboard(reading(t.Context()), editor, mustUUID(t, o.ID), pgtype.UUID{}, false)
	if err != nil || created || again.ID != made.ID {
		t.Errorf("making it again: %q created %v, want the first %q: %v", again.ID, created, made.ID, err)
	}
}
