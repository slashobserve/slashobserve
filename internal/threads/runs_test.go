package threads

import (
	"errors"
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

func (r *rig) runFor(t *testing.T, dash, run pgtype.UUID, resultVersion int32) dbsqlc.AgentRequest {
	t.Helper()
	row, err := r.db.Queries().GetAgentRequest(reading(t.Context()), dbsqlc.GetAgentRequestParams{ID: run, DashboardID: dash})
	if err != nil {
		t.Fatal(err)
	}
	row.ResultVersion = pgtype.Int4{Int32: resultVersion, Valid: true}
	row.State = string(revisions.StateSucceeded)
	return row
}

func (r *rig) resolveForRun(t *testing.T, run dbsqlc.AgentRequest) error {
	t.Helper()
	return r.db.InTx(reading(t.Context()), func(q *dbsqlc.Queries) error {
		return NewRuns(r.db.DB).RunEnded(reading(t.Context()), q, run)
	})
}

func (r *rig) threadRow(t *testing.T, id string) dbsqlc.Thread {
	t.Helper()
	row, err := r.db.Queries().GetThread(reading(t.Context()), mustParse(t, id))
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func TestRunResolvesItsThreadAsTheRequester(t *testing.T) {
	r := newAgentRig(t, true)
	dash := r.createDashboard(t, "rps", "errors")
	r.addVersion(t, dash, 1)
	comment, queued := r.commentRevision(t, dash, new("errors"), "show this per route")
	run := r.runFor(t, dash, queued, 2)

	if err := r.resolveForRun(t, run); err != nil {
		t.Fatal(err)
	}

	row := r.threadRow(t, comment)
	if row.Status != StatusResolved || row.ResolvedByName.String != "Ada" || row.ResolvedRunID != run.ID || row.ResolvedByVersion.Int32 != 2 {
		t.Fatalf("thread = %+v", row)
	}
	page, err := r.service.List(reading(t.Context()), testUser, ListQuery{Target: target(KindDashboard, dash), Status: StatusResolved, Limit: 10})
	if err != nil || len(page.Threads) != 1 || len(page.Messages) != 2 {
		t.Fatalf("page = %+v, err %v", page, err)
	}
	if resolver := page.Threads[0].ResolvedBy; resolver == nil || resolver.Name != "Ada" || resolver.JSON().Label != "Observe Agent for Ada" || resolver.RunID != run.ID.String() {
		t.Fatalf("resolver = %+v", resolver)
	}
	if reply := page.Messages[1]; reply.ThreadID != comment || reply.Body != "Saved version 2 of this dashboard." || reply.Author.RunID != run.ID.String() {
		t.Fatalf("the run's answer in the thread = %+v", reply)
	}

	var runID, actorName string
	var data []byte
	err = r.db.SQLDB(t).QueryRowContext(reading(t.Context()),
		`SELECT coalesce(run_id::text, ''), actor_name, data::text FROM activity WHERE type = 'thread.resolved' AND target_id = $1`, comment,
	).Scan(&runID, &actorName, &data)
	if err != nil {
		t.Fatal(err)
	}
	if runID != run.ID.String() || actorName != "Ada" {
		t.Fatalf("activity run %q actor %q, want run %s actor Ada", runID, actorName, run.ID)
	}
	if got := string(data); got != `{"object": {"id": "`+dash.String()+`", "kind": "dashboard"}, "resolved_by_version": 2}` {
		t.Fatalf("data = %s", got)
	}
}

func TestRunLeavesAResolvedThreadAlone(t *testing.T) {
	r := newAgentRig(t, true)
	dash := r.createDashboard(t)
	r.addVersion(t, dash, 1)
	comment, queued := r.commentRevision(t, dash, new("rps"), "fix it")
	if _, _, err := r.service.SetStatus(reading(t.Context()), testUser, mustParse(t, comment), StatusResolved, new(1)); err != nil {
		t.Fatal(err)
	}
	before := r.threadRow(t, comment)

	if err := r.resolveForRun(t, r.runFor(t, dash, queued, 2)); err != nil {
		t.Fatal(err)
	}

	after := r.threadRow(t, comment)
	if after.ResolvedByVersion.Int32 != 1 || after.ResolvedRunID.Valid || !after.UpdatedAt.Time.Equal(before.UpdatedAt.Time) {
		t.Fatalf("thread = %+v, was %+v", after, before)
	}
	if got := activityTypes(r.activity(t)); len(got) != 3 || got[1] != "thread.resolved" || got[2] != "thread.replied" {
		t.Fatalf("activity = %v, want the opening, the user's resolve and the run's answer", got)
	}
}

func TestRunWithoutACommentOrResultResolvesNothing(t *testing.T) {
	r := newAgentRig(t, true)
	dash := r.createDashboard(t)
	r.addVersion(t, dash, 1)
	comment, queued := r.commentRevision(t, dash, new("rps"), "fix it")
	run := r.runFor(t, dash, queued, 2)

	noComment := run
	noComment.CommentID = pgtype.UUID{}
	noResult := run
	noResult.ResultVersion = pgtype.Int4{}
	for name, row := range map[string]dbsqlc.AgentRequest{"no comment": noComment, "no result version": noResult} {
		if err := r.resolveForRun(t, row); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if got := r.threadRow(t, comment).Status; got != StatusOpen {
		t.Fatalf("status = %s, want open", got)
	}
}

func TestRunSkipsAThreadOfADeletedDashboard(t *testing.T) {
	r := newAgentRig(t, true)
	dash := r.createDashboard(t)
	r.addVersion(t, dash, 1)
	_, queued := r.commentRevision(t, dash, new("rps"), "fix it")
	run := r.runFor(t, dash, queued, 2)
	if err := r.dashboards.Delete(reading(t.Context()), testUser, dash); err != nil {
		t.Fatal(err)
	}

	if err := r.resolveForRun(t, run); err != nil {
		t.Fatal(err)
	}
}

func TestRunLooksUpTheThreadOfAReplyOnItsOwnDashboard(t *testing.T) {
	r := newAgentRig(t, false)
	dash := r.createDashboard(t, "rps", "errors")
	other := r.createDashboard(t)
	root, _ := r.open(t, testUser, OpenInput{Target: target(KindDashboard, dash), Version: new(1), PanelID: new("errors"), Body: "root"})
	_, reply := r.reply(t, testUser, root.ID, "reply")
	runs := NewRuns(r.db.DB)

	thread, panel, err := runs.ForRun(reading(t.Context()), dash, mustParse(t, reply.ID))
	if err != nil || thread.String() != root.ID || panel != "errors" {
		t.Fatalf("thread %s panel %q err %v, want root %s on errors", thread, panel, err, root.ID)
	}
	if _, _, err := runs.ForRun(reading(t.Context()), other, mustParse(t, reply.ID)); !errors.Is(err, revisions.ErrCommentNotFound) {
		t.Fatalf("other dashboard err = %v, want ErrCommentNotFound", err)
	}
	if _, _, err := runs.ForRun(reading(t.Context()), dash, mustParse(t, "99999999-9999-4999-8999-999999999999")); !errors.Is(err, revisions.ErrCommentNotFound) {
		t.Fatalf("unknown comment err = %v, want ErrCommentNotFound", err)
	}
}
