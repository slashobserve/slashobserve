package findings_test

import (
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func (r rig) chatsOn(t *testing.T, by auth.Principal, dashboard pgtype.UUID, everyone bool) []revisions.Chat {
	t.Helper()
	service := revisions.NewService(r.db.DB, slog.New(slog.DiscardHandler))
	chats, _, err := service.Chats(reading(t.Context()), by, revisions.ChatFilter{DashboardID: dashboard, Everyone: everyone}, 20, "")
	if err != nil {
		t.Fatal(err)
	}
	return chats
}

func (r rig) makeDashboardFrom(t *testing.T, by auth.Principal, run pgtype.UUID) pgtype.UUID {
	t.Helper()
	answer := r.save(t, by, findings.SaveInput{AgentRequestID: run})
	made, _, err := r.service.SaveAsDashboard(reading(t.Context()), by, mustUUID(t, answer.ID))
	if err != nil {
		t.Fatal(err)
	}
	return mustUUID(t, made.ID)
}

func TestADashboardMadeFromAnAnswerTakesItsChatForTheOwnerOnly(t *testing.T) {
	r := newRig(t)
	run := r.chat(t, editor, pgtype.UUID{})
	made := r.makeDashboardFrom(t, editor, run)

	mine := r.chatsOn(t, editor, made, false)
	if len(mine) != 1 || mine[0].ID != run || mine[0].ReadOnly {
		t.Fatalf("the owner finds %d chats on the new dashboard, want their own chat writable: %+v", len(mine), mine)
	}
	if others := r.chatsOn(t, otherEditor, made, false); len(others) != 0 {
		t.Errorf("a second editor finds %d chats on the dashboard made from someone else's answer", len(others))
	}
	if others := r.chatsOn(t, viewer, made, false); len(others) != 0 {
		t.Errorf("a viewer finds %d chats on the dashboard made from someone else's answer", len(others))
	}
	seen := r.chatsOn(t, admin, made, true)
	if len(seen) != 1 || !seen[0].ReadOnly {
		t.Errorf("an Admin reading everyone's chats finds %+v, want the one chat read-only", seen)
	}
}

func TestAnotherPersonsDashboardFromAnAnswerLeavesTheOwnersChatAlone(t *testing.T) {
	r := newRig(t)
	run := r.chat(t, editor, pgtype.UUID{})
	answer := r.save(t, editor, findings.SaveInput{AgentRequestID: run})
	if _, _, err := r.service.SaveAsObservation(reading(t.Context()), editor, run); err != nil {
		t.Fatal(err)
	}
	made, _, err := r.service.SaveAsDashboard(reading(t.Context()), otherEditor, mustUUID(t, answer.ID))
	if err != nil {
		t.Fatal(err)
	}
	if chats := r.chatsOn(t, editor, mustUUID(t, made.ID), false); len(chats) != 0 {
		t.Errorf("the owner finds %d chats on a dashboard someone else made from their saved answer", len(chats))
	}
}

func TestAChatAlreadyAboutADashboardStaysThereWhenAnAnswerBecomesAnother(t *testing.T) {
	r := newRig(t)
	run := r.chat(t, editor, pgtype.UUID{})
	r.db.Exec(t, `UPDATE chats SET dashboard_id = $1 WHERE id = $2`, r.open, run)
	made := r.makeDashboardFrom(t, editor, run)

	if chats := r.chatsOn(t, editor, made, false); len(chats) != 0 {
		t.Errorf("the new dashboard took %d chats that were already about another dashboard", len(chats))
	}
	if chats := r.chatsOn(t, editor, r.open, false); len(chats) != 1 {
		t.Errorf("the chat left the dashboard it was about: %d chats there", len(chats))
	}
}
