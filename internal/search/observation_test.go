package search

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func (r *rig) observation(t *testing.T, title, summary string, folder pgtype.UUID) string {
	t.Helper()
	state, resolvedAt := "open", "NULL"
	if summary != "" {
		state, resolvedAt = "resolved", "now()"
	}
	return r.text(t, `INSERT INTO observations (folder_id, title, state, summary, resolved_at, created_by_kind, created_by_id, created_by_name)
VALUES (coalesce($1::uuid, general_folder_id()), $2, $3, nullif($4, ''), `+resolvedAt+`, 'user', $5::uuid, 'Eve') RETURNING id::text`,
		folder, title, state, summary, editor.ID)
}

func (r *rig) block(t *testing.T, observation, kind, state, body string) {
	t.Helper()
	r.db.Exec(t, `INSERT INTO observation_blocks (observation_id, kind, state, position, body, snapshot_id, created_by_kind, created_by_id, created_by_name)
VALUES ($1::uuid, $2, $3, (SELECT count(*) FROM observation_blocks WHERE observation_id = $1::uuid), $4::jsonb, NULL, 'user', $5::uuid, 'Eve')`,
		observation, kind, state, body, editor.ID)
}

func TestSearchFindsAnObservationByItsTitleSummaryAndLiveTextBlocksOnce(t *testing.T) {
	r := newRig(t)
	byTitle := r.observation(t, "Heron outage", "", pgtype.UUID{})
	bySummary := r.observation(t, "Checkout slow", "Heron pool exhausted", pgtype.UUID{})
	byBlock := r.observation(t, "Latency", "", pgtype.UUID{})
	r.block(t, byBlock, "text", "live", `{"markdown":"the heron cache was cold"}`)
	r.block(t, byBlock, "text", "live", `{"markdown":"a second heron remark"}`)
	byAsk := r.observation(t, "Errors", "", pgtype.UUID{})
	r.block(t, byAsk, "text", "requested", `{"markdown":"was it a heron deploy"}`)
	byPanel := r.observation(t, "Traffic", "", pgtype.UUID{})
	r.block(t, byPanel, "panel", "live", `{"markdown":"heron","panel":{}}`)

	got := r.search(t, admin, "heron", KindObservation)
	have := ids(got)
	for name, id := range map[string]string{"title": byTitle, "summary": bySummary, "live text block": byBlock} {
		if !have["observation:"+id] {
			t.Errorf("%s did not find its observation: %v", name, titles(got))
		}
	}
	for name, id := range map[string]string{"requested block": byAsk, "panel block": byPanel} {
		if have["observation:"+id] {
			t.Errorf("%s found its observation: %v", name, titles(got))
		}
	}
	if len(got) != 3 {
		t.Errorf("results %v, want each observation once", titles(got))
	}
	if all := r.search(t, admin, "heron"); len(all) != 3 {
		t.Errorf("an unfiltered search finds %v", titles(all))
	}
}

func TestSearchShowsTheObservationItsFolderAndItsPage(t *testing.T) {
	r := newRig(t)
	id := r.observation(t, "Heron outage", "", pgtype.UUID{})
	r.block(t, id, "text", "live", `{"markdown":"the heron cache was cold"}`)

	got := r.search(t, admin, "heron")
	if len(got) != 1 {
		t.Fatalf("results %v", titles(got))
	}
	res := got[0]
	if res.Kind != KindObservation || res.Title != "Heron outage" || res.URL != "https://observe.example.com/o/"+id || res.Folder == nil || res.Folder.Title != "General" {
		t.Errorf("result %+v", res)
	}
	if res.Target != nil || res.ThreadID != "" {
		t.Errorf("an observation is no comment: %+v", res)
	}
	var marked bool
	for _, segment := range res.Snippet {
		marked = marked || segment.Match
	}
	if !marked {
		t.Errorf("snippet %+v marks no match", res.Snippet)
	}
}

func TestSearchNeverReturnsAnObservationOfAFolderTheReaderCannotSee(t *testing.T) {
	r := newRig(t)
	hidden := r.restricted(t, "Payroll")
	id := r.observation(t, "Heron payroll outage", "", hidden)
	r.block(t, id, "text", "live", `{"markdown":"heron details"}`)
	_, message := r.comment(t, KindObservation, id, "heron comment")
	own := r.observation(t, "Heron mine", "", r.personal(t, ada))

	if got := r.search(t, editor, "heron"); len(got) != 0 {
		t.Errorf("an editor outside the group finds %v", titles(got))
	}
	got := r.search(t, payroll, "heron")
	if have := ids(got); !have["observation:"+id] || !have["comment:"+message] {
		t.Errorf("a member finds %v", titles(got))
	}
	if have := ids(r.search(t, admin, "heron")); have["observation:"+own] {
		t.Error("an admin finds a personal folder's observation")
	}
	if have := ids(r.search(t, ada, "heron")); !have["observation:"+own] || have["observation:"+id] {
		t.Errorf("the owner finds %v", have)
	}
}

func TestSearchReturnsACommentOnAnObservationWithItsSubject(t *testing.T) {
	r := newRig(t)
	id := r.observation(t, "Latency spike", "", pgtype.UUID{})
	thread, message := r.comment(t, KindObservation, id, "was the heron cache cold")

	got := r.search(t, admin, "heron", KindComment)
	if len(got) != 1 {
		t.Fatalf("results %v", titles(got))
	}
	res := got[0]
	want := &Target{Kind: KindObservation, ID: id, Title: "Latency spike"}
	if res.ID != message || res.ThreadID != thread || res.Target == nil || *res.Target != *want || res.URL != "https://observe.example.com/o/"+id {
		t.Errorf("comment %+v target %+v", res, res.Target)
	}
	if len(r.search(t, admin, "heron", KindObservation)) != 0 {
		t.Error("a comment is not an observation result")
	}
}

func TestSearchFollowsAnObservationMovedOrEditedAtOnce(t *testing.T) {
	r := newRig(t)
	id := r.observation(t, "Quiet", "", pgtype.UUID{})
	if got := r.search(t, admin, "heron"); len(got) != 0 {
		t.Fatalf("before: %v", titles(got))
	}
	r.db.Exec(t, `UPDATE observations SET title = 'Heron outage' WHERE id = $1::uuid`, id)
	if got := r.search(t, admin, "heron"); len(got) != 1 {
		t.Fatalf("after a rename: %v", titles(got))
	}
	r.db.Exec(t, `UPDATE observations SET folder_id = $2::uuid WHERE id = $1::uuid`, id, r.restricted(t, "Payroll"))
	if got := r.search(t, editor, "heron"); len(got) != 0 {
		t.Fatalf("after a move into a hidden folder: %v", titles(got))
	}
	r.db.Exec(t, `DELETE FROM observations WHERE id = $1::uuid`, id)
	if got := r.search(t, payroll, "heron"); len(got) != 0 {
		t.Fatalf("after a delete: %v", titles(got))
	}
}
