package threads

import (
	"bytes"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/anchor"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

var retrySpot = &anchor.Anchor{Kind: anchor.Position, Pos: anchor.Pos{X: 0.4, Y: 0.3}}

func (r *rig) anchoredThreadAnswer(t *testing.T) (about, block, run pgtype.UUID) {
	t.Helper()
	about = r.filedObservationIn(t, testUser, foldertest.Open(t, r.db, "Shared"))
	blockID := r.addBlock(t, about)
	_, _, started, refusal := r.openWithRun(t, testUser, OpenInput{Target: target(KindObservation, about), Body: "@observe look here", BlockID: &blockID, Anchor: retrySpot})
	if started == nil || refusal != nil {
		t.Fatalf("started = %+v, refusal = %+v", started, refusal)
	}
	run = mustParse(t, started.ID)
	r.answer(t, testUser, findings.SaveInput{AgentRequestID: run})
	r.finishWithAnswer(t, run)
	return about, mustParse(t, blockID), run
}

func samePoint(a, b dbsqlc.AgentRequest) bool {
	return a.BlockID == b.BlockID && bytes.Equal(a.Anchor, b.Anchor)
}

func TestARetryByItsRequesterKeepsTheAnchorOfAPrivateAsk(t *testing.T) {
	r := newShareRig(t)
	about := r.filedObservationIn(t, testUser, pgtype.UUID{})
	id, block := about.String(), r.addBlock(t, about)
	run := r.privateAnswer(t, revisions.StartRequest{ObservationID: &id, Anchor: retrySpot, BlockID: &block}, "prom", "")
	asked := r.runRow(t, run)
	if !asked.BlockID.Valid || len(asked.Anchor) == 0 {
		t.Fatalf("the ask stored block %v and anchor %s", asked.BlockID, asked.Anchor)
	}

	next := r.runRow(t, retriedOverHTTP(t, r.observeEngine(testUser), run))

	if !samePoint(next, asked) {
		t.Fatalf("the retry points at block %v anchor %s, want block %v anchor %s", next.BlockID, next.Anchor, asked.BlockID, asked.Anchor)
	}
}

func TestARetryInTheThreadKeepsTheThreadsAnchor(t *testing.T) {
	r := newShareRig(t)
	_, _, run := r.anchoredThreadAnswer(t)

	next := r.runRow(t, retriedOverHTTP(t, r.observeEngine(commenter), run))

	if !next.ThreadMessageID.Valid || !samePoint(next, r.runRow(t, run)) {
		t.Fatalf("the retry in the thread points at block %v anchor %s, want the thread's", next.BlockID, next.Anchor)
	}
}

func TestARetryOutsideTheThreadTakesNoAnchorFromIt(t *testing.T) {
	r := newShareRig(t)
	about, _, run := r.anchoredThreadAnswer(t)
	saved, _, err := r.findings.SaveAsObservation(reading(t.Context()), testUser, run)
	if err != nil {
		t.Fatal(err)
	}
	r.db.Exec(t, `UPDATE observations SET folder_id = $2 WHERE id = $1`, mustParse(t, saved.ObservationID), foldertest.Open(t, r.db, "Public"))
	r.db.Exec(t, `UPDATE observations SET folder_id = $2 WHERE id = $1`, about, foldertest.AdminsOnly(t, r.db, "Admins"))

	next := r.runRow(t, retriedOverHTTP(t, r.observeEngine(commenter), run))

	if next.ThreadMessageID.Valid || next.BlockID.Valid || len(next.Anchor) != 0 {
		t.Fatalf("the retry outside the thread carries message %v, block %v and anchor %s", next.ThreadMessageID, next.BlockID, next.Anchor)
	}
}
