package threads

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func (r *rig) runFor(t *testing.T, dash pgtype.UUID, comment Comment, resultVersion int32) dbsqlc.AgentRequest {
	t.Helper()
	if comment.Revision == nil {
		t.Fatal("the comment queued no revision")
	}
	row, err := r.db.Queries().GetAgentRequest(t.Context(), dbsqlc.GetAgentRequestParams{ID: mustParse(t, comment.Revision.ID), DashboardID: dash})
	if err != nil {
		t.Fatal(err)
	}
	row.ResultVersion = pgtype.Int4{Int32: resultVersion, Valid: true}
	return row
}

func (r *rig) resolveForRun(t *testing.T, run dbsqlc.AgentRequest) error {
	t.Helper()
	return r.db.InTx(t.Context(), func(q *dbsqlc.Queries) error {
		return NewRuns(r.db.DB).ResolveForRun(t.Context(), q, run)
	})
}

func (r *rig) threadRow(t *testing.T, id string) dbsqlc.Thread {
	t.Helper()
	row, err := r.db.Queries().GetThread(t.Context(), mustParse(t, id))
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func TestRunResolvesItsThreadAsTheRequester(t *testing.T) {
	r := newAgentRig(t, true)
	dash := r.createDashboard(t, "rps", "errors")
	r.addVersion(t, dash, 1)
	comment := r.createComment(t, dash, NewComment{Version: 1, PanelID: new("errors"), Body: "show this per route", RequestRevision: true})
	run := r.runFor(t, dash, comment, 2)

	if err := r.resolveForRun(t, run); err != nil {
		t.Fatal(err)
	}

	row := r.threadRow(t, comment.ID)
	if row.Status != StatusResolved || row.ResolvedByName.String != "Ada" || row.ResolvedRunID != run.ID || row.ResolvedByVersion.Int32 != 2 {
		t.Fatalf("thread = %+v", row)
	}
	listed, _, err := r.comments.List(t.Context(), testUser, dash, StatusResolved, "", 10, "")
	if err != nil || len(listed) != 1 || listed[0].ResolvedBy == nil || listed[0].ResolvedBy.Name != "Ada" {
		t.Fatalf("listed = %+v, err %v", listed, err)
	}

	var runID, actorName string
	var data []byte
	err = r.db.SQLDB(t).QueryRowContext(t.Context(),
		`SELECT coalesce(run_id::text, ''), actor_name, data::text FROM activity WHERE type = 'thread.resolved' AND target_id = $1`, comment.ID,
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
	comment := r.createComment(t, dash, NewComment{Version: 1, PanelID: new("rps"), Body: "fix it", RequestRevision: true})
	if _, _, err := r.comments.SetStatus(t.Context(), testUser, dash, mustParse(t, comment.ID), StatusResolved, new(1)); err != nil {
		t.Fatal(err)
	}
	before := r.threadRow(t, comment.ID)

	if err := r.resolveForRun(t, r.runFor(t, dash, comment, 2)); err != nil {
		t.Fatal(err)
	}

	after := r.threadRow(t, comment.ID)
	if after.ResolvedByVersion.Int32 != 1 || after.ResolvedRunID.Valid || !after.UpdatedAt.Time.Equal(before.UpdatedAt.Time) {
		t.Fatalf("thread = %+v, was %+v", after, before)
	}
	if got := activityTypes(r.activity(t)); len(got) != 2 || got[1] != "thread.resolved" {
		t.Fatalf("activity = %v, want the opening and the user's resolve only", got)
	}
}

func TestRunWithoutACommentOrResultResolvesNothing(t *testing.T) {
	r := newAgentRig(t, true)
	dash := r.createDashboard(t)
	r.addVersion(t, dash, 1)
	comment := r.createComment(t, dash, NewComment{Version: 1, PanelID: new("rps"), Body: "fix it", RequestRevision: true})
	run := r.runFor(t, dash, comment, 2)

	noComment := run
	noComment.CommentID = pgtype.UUID{}
	noResult := run
	noResult.ResultVersion = pgtype.Int4{}
	for name, row := range map[string]dbsqlc.AgentRequest{"no comment": noComment, "no result version": noResult} {
		if err := r.resolveForRun(t, row); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if got := r.threadRow(t, comment.ID).Status; got != StatusOpen {
		t.Fatalf("status = %s, want open", got)
	}
}

func TestRunSkipsAThreadOfADeletedDashboard(t *testing.T) {
	r := newAgentRig(t, true)
	dash := r.createDashboard(t)
	r.addVersion(t, dash, 1)
	comment := r.createComment(t, dash, NewComment{Version: 1, PanelID: new("rps"), Body: "fix it", RequestRevision: true})
	run := r.runFor(t, dash, comment, 2)
	if err := r.dashboards.Delete(t.Context(), testUser, dash); err != nil {
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
	root := r.createComment(t, dash, NewComment{Version: 1, PanelID: new("errors"), Body: "root"})
	reply := r.createComment(t, dash, NewComment{ParentID: mustParse(t, root.ID), Body: "reply"})
	runs := NewRuns(r.db.DB)

	thread, panel, err := runs.ForRun(t.Context(), dash, mustParse(t, reply.ID))
	if err != nil || thread.String() != root.ID || panel != "errors" {
		t.Fatalf("thread %s panel %q err %v, want root %s on errors", thread, panel, err, root.ID)
	}
	if _, _, err := runs.ForRun(t.Context(), other, mustParse(t, reply.ID)); !errors.Is(err, revisions.ErrCommentNotFound) {
		t.Fatalf("other dashboard err = %v, want ErrCommentNotFound", err)
	}
	if _, _, err := runs.ForRun(t.Context(), dash, mustParse(t, "99999999-9999-4999-8999-999999999999")); !errors.Is(err, revisions.ErrCommentNotFound) {
		t.Fatalf("unknown comment err = %v, want ErrCommentNotFound", err)
	}
}
