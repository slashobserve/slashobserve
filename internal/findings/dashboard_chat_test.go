package findings_test

import (
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/pkg/spec"
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

const textEvidence = `{"id":"evidence-1","type":"text","title":"Notes","gridPos":{"x":0,"y":0,"w":12,"h":8},"content":"checkout-0"}`

func (d *documented) answerWithEvidence(t *testing.T, by auth.Principal, run pgtype.UUID) findings.Finding {
	t.Helper()
	return d.save(t, by, findings.SaveInput{AgentRequestID: run, Evidence: []spec.Panel{panelOf(t, textEvidence)}})
}

func (d *documented) makeDashboardFrom(t *testing.T, by auth.Principal, answer findings.Finding, want int) pgtype.UUID {
	t.Helper()
	rec := d.call(t, by, http.MethodPost, "/api/v1/findings/"+answer.ID+"/dashboard", want)
	if want != http.StatusCreated {
		return pgtype.UUID{}
	}
	return mustUUID(t, strings.TrimPrefix(rec.Header().Get("Location"), "/api/v1/dashboards/"))
}

func TestADashboardMadeFromAnAnswerTakesItsChatForTheOwnerOnly(t *testing.T) {
	d := newDocumented(t)
	run := d.chat(t, editor, pgtype.UUID{})
	made := d.makeDashboardFrom(t, editor, d.answerWithEvidence(t, editor, run), http.StatusCreated)

	mine := d.chatsOn(t, editor, made, false)
	if len(mine) != 1 || mine[0].ID != run || mine[0].ReadOnly {
		t.Fatalf("the owner finds %d chats on the new dashboard, want their own chat writable: %+v", len(mine), mine)
	}
	if others := d.chatsOn(t, otherEditor, made, false); len(others) != 0 {
		t.Errorf("a second editor finds %d chats on the dashboard made from someone else's answer", len(others))
	}
	if others := d.chatsOn(t, viewer, made, false); len(others) != 0 {
		t.Errorf("a viewer finds %d chats on the dashboard made from someone else's answer", len(others))
	}
	seen := d.chatsOn(t, admin, made, true)
	if len(seen) != 1 || !seen[0].ReadOnly {
		t.Errorf("an Admin reading everyone's chats finds %+v, want the one chat read-only", seen)
	}
}

func TestAnotherPersonsDashboardFromAnAnswerLeavesTheOwnersChatAlone(t *testing.T) {
	d := newDocumented(t)
	run := d.chat(t, editor, pgtype.UUID{})
	answer := d.answerWithEvidence(t, editor, run)
	saved, _, err := d.service.SaveAsObservation(reading(t.Context()), editor, run)
	if err != nil {
		t.Fatal(err)
	}
	d.db.Exec(t, `UPDATE observations SET folder_id = $2 WHERE id = $1`, saved.ObservationID, d.general(t))
	made := d.makeDashboardFrom(t, otherEditor, answer, http.StatusCreated)
	if chats := d.chatsOn(t, editor, made, false); len(chats) != 0 {
		t.Errorf("the owner finds %d chats on a dashboard someone else made from their saved answer", len(chats))
	}
	if chats := d.chatsOn(t, otherEditor, made, false); len(chats) != 0 {
		t.Errorf("the person who made the dashboard finds %d chats on it that are not theirs", len(chats))
	}
}

func TestAChatAlreadyAboutADashboardStaysThereWhenAnAnswerBecomesAnother(t *testing.T) {
	d := newDocumented(t)
	run := d.chat(t, editor, pgtype.UUID{})
	d.db.Exec(t, `UPDATE chats SET dashboard_id = $1 WHERE id = $2`, d.open, run)
	made := d.makeDashboardFrom(t, editor, d.answerWithEvidence(t, editor, run), http.StatusCreated)

	if chats := d.chatsOn(t, editor, made, false); len(chats) != 0 {
		t.Errorf("the new dashboard took %d chats that were already about another dashboard", len(chats))
	}
	if chats := d.chatsOn(t, editor, d.open, false); len(chats) != 1 {
		t.Errorf("the chat left the dashboard it was about: %d chats there", len(chats))
	}
}
