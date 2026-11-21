package findings_test

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/observations"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func (r rig) chat(t *testing.T, by auth.Principal, follows pgtype.UUID) pgtype.UUID {
	t.Helper()
	var raw string
	if err := r.db.SQLDB(t).QueryRowContext(reading(t.Context()), `WITH chat AS (
  INSERT INTO chats (id, owner_kind, owner_id) VALUES (coalesce($4::uuid, gen_random_uuid()), $1, $2::uuid)
  ON CONFLICT (id) DO UPDATE SET last_turn_at = now() RETURNING id)
INSERT INTO agent_requests (id, intent, prompt, requested_by_kind, requested_by_id, requested_by_name, chat_id)
SELECT CASE WHEN $4::uuid IS NULL THEN chat.id ELSE gen_random_uuid() END, 'ask', 'slowest routes in the last hour', $1, $2::uuid, $3, chat.id FROM chat
RETURNING id::text`,
		string(by.Kind), by.ID, by.Name, follows).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	return mustUUID(t, raw)
}

func (r rig) savedAnswer(t *testing.T, by auth.Principal, request pgtype.UUID) pgtype.UUID {
	t.Helper()
	r.save(t, by, findings.SaveInput{AgentRequestID: request, Title: "Slowest routes in the last hour"})
	saved, _, err := r.service.SaveAsObservation(reading(t.Context()), by, request)
	if err != nil {
		t.Fatal(err)
	}
	return mustUUID(t, saved.ObservationID)
}

func (r rig) chatTitle(t *testing.T, chat pgtype.UUID) string {
	t.Helper()
	row, err := r.db.Queries().GetAgentRequestByID(reading(t.Context()), chat)
	if err != nil {
		t.Fatal(err)
	}
	return row.Title.String
}

func (r rig) observationTitle(t *testing.T, id pgtype.UUID) string {
	t.Helper()
	var title string
	if err := r.db.SQLDB(t).QueryRowContext(reading(t.Context()), `SELECT title FROM observations WHERE id = $1`, id).Scan(&title); err != nil {
		t.Fatal(err)
	}
	return title
}

func (r rig) renameObservation(t *testing.T, by auth.Principal, id pgtype.UUID, title string) revisions.SharedTitle {
	t.Helper()
	o, err := r.observations.Update(reading(t.Context()), by, id, observations.UpdateInput{Title: &title})
	if err != nil {
		t.Fatal(err)
	}
	return o.ChatRename
}

func TestRenamingAChatRenamesTheObservationSavedFromIt(t *testing.T) {
	r := newRig(t)
	chat := r.chat(t, editor, pgtype.UUID{})
	follow := r.chat(t, editor, chat)
	observation := r.savedAnswer(t, editor, follow)

	got, err := r.observations.RenameFromChat(reading(t.Context()), editor, chat, "Routes chat")
	if err != nil || got != revisions.TitleRenamed {
		t.Fatalf("rename from the chat: %q, %v", got, err)
	}
	if title := r.observationTitle(t, observation); title != "Routes chat" {
		t.Fatalf("observation title %q", title)
	}
	if n := r.count(t, `SELECT count(*) FROM activity WHERE type = 'observation.updated' AND data ->> 'from_title' = 'Slowest routes in the last hour'`); n != 1 {
		t.Fatalf("%d rename events", n)
	}
	if got, err := r.observations.RenameFromChat(reading(t.Context()), editor, follow, "A later turn"); err != nil || got != "" {
		t.Fatalf("rename from a later turn: %q, %v", got, err)
	}
}

func TestRenamingTheObservationRenamesTheChatOnlyForWhoStartedIt(t *testing.T) {
	r := newRig(t)
	chat := r.chat(t, editor, pgtype.UUID{})
	observation := r.savedAnswer(t, editor, chat)
	r.db.Exec(t, `UPDATE observations SET folder_id = $2 WHERE id = $1`, observation, r.folder(t, "Incidents"))

	if got := r.renameObservation(t, otherEditor, observation, "Renamed by Ed"); got != revisions.TitleKept {
		t.Fatalf("another editor's rename: %q, want kept", got)
	}
	if title := r.chatTitle(t, chat); title != "" {
		t.Fatalf("chat title %q after another editor's rename", title)
	}
	if got := r.renameObservation(t, editor, observation, "Renamed by Eve"); got != revisions.TitleRenamed {
		t.Fatalf("the requester's rename: %q, want renamed", got)
	}
	if title := r.chatTitle(t, chat); title != "Renamed by Eve" {
		t.Fatalf("chat title %q", title)
	}
	long := "A title longer than a chat takes " + strings.Repeat("x", 100)
	if got := r.renameObservation(t, editor, observation, long); got != revisions.TitleKept {
		t.Fatalf("a title too long for a chat: %q, want kept", got)
	}
}

func TestAChatSharesItsTitleOnlyWithItsOneObservationThatTheCallerMayEdit(t *testing.T) {
	r := newRig(t)
	chat := r.chat(t, viewer, pgtype.UUID{})
	observation := r.savedAnswer(t, viewer, chat)
	shared := r.folder(t, "Incidents")
	r.db.Exec(t, `UPDATE observations SET folder_id = $2 WHERE id = $1`, observation, shared)

	if got, err := r.observations.RenameFromChat(reading(t.Context()), viewer, chat, "View only"); err != nil || got != revisions.TitleKept {
		t.Fatalf("an observation the caller may only view: %q, %v", got, err)
	}
	r.db.Exec(t, `INSERT INTO folder_permissions (folder_id, group_name, level) VALUES ($1, 'oncall', 'view')`, shared)
	if got, err := r.observations.RenameFromChat(reading(t.Context()), viewer, chat, "Hidden"); err != nil || got != "" {
		t.Fatalf("an observation the caller cannot see: %q, %v", got, err)
	}
	if title := r.observationTitle(t, observation); title != "Slowest routes in the last hour" {
		t.Fatalf("observation title %q", title)
	}

	two := r.chat(t, editor, pgtype.UUID{})
	r.savedAnswer(t, editor, two)
	r.savedAnswer(t, editor, r.chat(t, editor, two))
	if got, err := r.observations.RenameFromChat(reading(t.Context()), editor, two, "Two"); err != nil || got != "" {
		t.Fatalf("a chat with two observations: %q, %v", got, err)
	}

	onDashboard := r.chat(t, editor, pgtype.UUID{})
	r.db.Exec(t, `UPDATE agent_requests SET kind = 'investigate', dashboard_id = $2 WHERE id = $1`, onDashboard, r.open)
	dashboardObservation := r.savedAnswer(t, editor, onDashboard)
	if got := r.renameObservation(t, editor, dashboardObservation, "On a dashboard"); got != "" {
		t.Fatalf("an observation from a dashboard chat: %q", got)
	}

	other, err := r.observations.Create(reading(t.Context()), editor, observations.CreateInput{Title: "Written by hand", FolderID: r.general(t)})
	if err != nil {
		t.Fatal(err)
	}
	if got := r.renameObservation(t, editor, mustUUID(t, other.ID), "Still by hand"); got != "" {
		t.Fatalf("an observation not made from a chat: %q", got)
	}
}
