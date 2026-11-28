package threads

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func (r *rig) commentRevision(t *testing.T, dash pgtype.UUID, panelID *string, body string) (thread string, run pgtype.UUID) {
	t.Helper()
	opened, _ := r.open(t, testUser, OpenInput{Target: target(KindDashboard, dash), Version: new(1), PanelID: panelID, Body: body})
	params := revisions.EnqueueParams{DashboardID: dash, CommentID: mustParse(t, opened.ID), Prompt: body, BaseVersion: 1, By: testUser}
	if panelID != nil {
		params.PanelID = *panelID
	}
	row, err := r.revisions.Submit(reading(t.Context()), params)
	if err != nil {
		t.Fatal(err)
	}
	return opened.ID, row.ID
}

func (r *rig) threadRow(t *testing.T, id string) dbsqlc.Thread {
	t.Helper()
	row, err := r.db.Queries().GetThread(reading(t.Context()), mustParse(t, id))
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func TestARunWithoutAThreadMessageLeavesItsCommentThreadAlone(t *testing.T) {
	r := newAgentRig(t, true)
	dash := r.createDashboard(t)
	r.addVersion(t, dash, 1)
	comment, queued := r.commentRevision(t, dash, new("rps"), "fix it")
	run, err := r.db.Queries().GetAgentRequestByID(reading(t.Context()), queued)
	if err != nil {
		t.Fatal(err)
	}
	before := r.threadRow(t, comment)

	if err := r.runEnded(t, succeededWithVersion(run, 2)); err != nil {
		t.Fatal(err)
	}
	after := r.threadRow(t, comment)
	if after.Status != StatusOpen || after.ResolvedByVersion.Valid || !after.UpdatedAt.Time.Equal(before.UpdatedAt.Time) {
		t.Fatalf("thread = %+v, was %+v", after, before)
	}
	if replies := r.runReplies(t, comment); len(replies) != 0 {
		t.Fatalf("replies = %+v, want none", replies)
	}
}
