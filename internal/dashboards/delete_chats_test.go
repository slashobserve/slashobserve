package dashboards

import (
	"testing"

	"github.com/slashobserve/slashobserve/internal/revisions"
)

func chatCount(t *testing.T, r *rig, chat string) int {
	t.Helper()
	var n int
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM chats WHERE id = $1`, chat).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestDeletingADashboardDeletesTheChatsItsRunsLeftEmpty(t *testing.T) {
	r := newRig(t, true)
	id := r.createDashboard(t)
	prompt := "revise"
	started, err := r.revisions.Start(reading(t.Context()), testUser, revisions.StartRequest{Prompt: &prompt, DashboardID: new(id.String())})
	if err != nil {
		t.Fatal(err)
	}
	chat := started.ChatID.String()
	if chatCount(t, r, chat) != 1 {
		t.Fatalf("chat %s was not created", chat)
	}
	if err := r.service.Delete(reading(t.Context()), testUser, id); err != nil {
		t.Fatal(err)
	}
	if n := chatCount(t, r, chat); n != 0 {
		t.Fatalf("%d chat rows left for a dashboard whose runs are gone", n)
	}
}

func TestDeletingADashboardKeepsAChatThatStillHasOtherRequests(t *testing.T) {
	r := newRig(t, true)
	id := r.createDashboard(t)
	prompt := "revise"
	started, err := r.revisions.Start(reading(t.Context()), testUser, revisions.StartRequest{Prompt: &prompt, DashboardID: new(id.String())})
	if err != nil {
		t.Fatal(err)
	}
	chat := started.ChatID.String()
	if _, err := r.db.SQLDB(t).ExecContext(t.Context(), `INSERT INTO agent_requests (intent, kind, prompt, requested_by_kind, requested_by_id, requested_by_name, chat_id)
		VALUES ('ask', 'build', 'elsewhere', $1, $2, 'x', $3)`, string(testUser.Kind), testUser.ID, chat); err != nil {
		t.Fatal(err)
	}
	if err := r.service.Delete(reading(t.Context()), testUser, id); err != nil {
		t.Fatal(err)
	}
	var dashboard *string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT dashboard_id::text FROM chats WHERE id = $1`, chat).Scan(&dashboard); err != nil {
		t.Fatalf("the chat with another request was deleted: %v", err)
	}
	if dashboard != nil {
		t.Fatalf("chat dashboard_id = %s, want it cleared", *dashboard)
	}
}
