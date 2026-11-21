package observations_test

import (
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/observations"
)

func (r rig) runAskedOn(t *testing.T, by auth.Principal, block observations.Block) dbsqlc.AgentRequest {
	t.Helper()
	run := r.run(t, by, pgtype.UUID{}, "Why?", nil, nil)
	r.db.Exec(t, `UPDATE agent_requests SET block_id = $2::uuid WHERE id = $1::uuid`, run.ID.String(), block.ID)
	run.BlockID = mustUUID(t, block.ID)
	return run
}

func (r rig) kindsOf(t *testing.T, by auth.Principal, id string) []observations.Kind {
	t.Helper()
	got, err := r.service.Get(reading(t.Context()), by, mustUUID(t, id))
	if err != nil {
		t.Fatal(err)
	}
	kinds := make([]observations.Kind, len(got.Blocks))
	for i, b := range got.Blocks {
		if b.Position != i {
			t.Fatalf("block %d has position %d", i, b.Position)
		}
		kinds[i] = b.Kind
	}
	return kinds
}

func (r rig) widthsOf(t *testing.T, by auth.Principal, id string) []observations.Width {
	t.Helper()
	got, err := r.service.Get(reading(t.Context()), by, mustUUID(t, id))
	if err != nil {
		t.Fatal(err)
	}
	widths := make([]observations.Width, len(got.Blocks))
	for i, b := range got.Blocks {
		widths[i] = b.Width
	}
	return widths
}

func TestARunAskedOnABlockPutsItsRunBlockAndAnswerDirectlyUnderIt(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	r.addText(t, editor, o, "a")
	asked := r.addText(t, editor, o, "b")
	r.addText(t, editor, o, "c")
	half := observations.WidthHalf
	got, err := r.service.Get(reading(t.Context()), editor, mustUUID(t, o.ID))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.service.Arrange(reading(t.Context()), editor, mustUUID(t, o.ID), []observations.BlockLayout{
		{ID: mustUUID(t, got.Blocks[0].ID), Width: &half},
		{ID: mustUUID(t, got.Blocks[2].ID), Width: &half, Height: observations.HeightChange{Set: true, Pixels: new(300)}},
	}, pgtype.UUID{}); err != nil {
		t.Fatal(err)
	}

	run := r.runAskedOn(t, editor, asked)
	r.mustOpenForRun(t, observations.RunOpen{Run: run, By: editor, ObservationID: mustUUID(t, o.ID), Strict: true})
	r.link(t, run, o)
	if _, _, err := r.conclude(t, editor, recordInput(t, run, evidencePanel, evidencePanel)); err != nil {
		t.Fatal(err)
	}

	want := []observations.Kind{observations.KindText, observations.KindText, observations.KindRun, observations.KindText, observations.KindPanel, observations.KindPanel, observations.KindText}
	if kinds := r.kindsOf(t, editor, o.ID); !slices.Equal(kinds, want) {
		t.Fatalf("kinds %v, want %v", kinds, want)
	}
	widths := r.widthsOf(t, editor, o.ID)
	wantWidths := []observations.Width{half, observations.WidthFull, observations.WidthFull, observations.WidthFull, observations.WidthFull, observations.WidthFull, half}
	if !slices.Equal(widths, wantWidths) {
		t.Errorf("widths %v, want %v", widths, wantWidths)
	}
	final, err := r.service.Get(reading(t.Context()), editor, mustUUID(t, o.ID))
	if err != nil {
		t.Fatal(err)
	}
	if final.Blocks[6].Height == nil || *final.Blocks[6].Height != 300 {
		t.Errorf("a hand-set height was lost: %+v", final.Blocks[6].Height)
	}
	if cited := cites(t, final.Blocks[2]); len(cited) != 3 {
		t.Errorf("run block cites %v", cited)
	}
}

func TestARunAskedOnTheWholePageStillAppends(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	r.addText(t, editor, o, "a")
	run := r.run(t, editor, pgtype.UUID{}, "Why?", nil, nil)
	r.mustOpenForRun(t, observations.RunOpen{Run: run, By: editor, ObservationID: mustUUID(t, o.ID), Strict: true})
	r.link(t, run, o)
	r.addText(t, editor, o, "added while the run worked")
	if _, _, err := r.conclude(t, editor, recordInput(t, run, evidencePanel)); err != nil {
		t.Fatal(err)
	}
	want := []observations.Kind{observations.KindText, observations.KindRun, observations.KindText, observations.KindText, observations.KindPanel}
	if kinds := r.kindsOf(t, editor, o.ID); !slices.Equal(kinds, want) {
		t.Errorf("kinds %v, want %v", kinds, want)
	}
}

func TestAnAnswerWhoseBlockWasDeletedMeanwhileAppends(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	r.addText(t, editor, o, "a")
	asked := r.addText(t, editor, o, "b")
	r.addText(t, editor, o, "c")
	run := r.runAskedOn(t, editor, asked)
	r.link(t, run, o)
	if err := r.service.RemoveBlock(reading(t.Context()), editor, mustUUID(t, asked.ID), asked.Version, pgtype.UUID{}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.conclude(t, editor, recordInput(t, run, evidencePanel)); err != nil {
		t.Fatal(err)
	}
	want := []observations.Kind{observations.KindText, observations.KindText, observations.KindText, observations.KindPanel}
	if kinds := r.kindsOf(t, editor, o.ID); !slices.Equal(kinds, want) {
		t.Errorf("kinds %v, want %v", kinds, want)
	}
}

func TestACommentersRunAskedOnABlockArrivesRequestedUnderIt(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	asked := r.addText(t, editor, o, "a")
	r.addText(t, editor, o, "b")
	run := r.runAskedOn(t, commenter, asked)
	r.mustOpenForRun(t, observations.RunOpen{Run: run, By: commenter, ObservationID: mustUUID(t, o.ID), Strict: true})
	r.link(t, run, o)
	if _, _, err := r.conclude(t, commenter, recordInput(t, run, evidencePanel)); err != nil {
		t.Fatal(err)
	}
	got, err := r.service.Get(reading(t.Context()), editor, mustUUID(t, o.ID))
	if err != nil {
		t.Fatal(err)
	}
	want := []observations.Kind{observations.KindText, observations.KindRun, observations.KindText, observations.KindPanel, observations.KindText}
	kinds := make([]observations.Kind, len(got.Blocks))
	for i, b := range got.Blocks {
		kinds[i] = b.Kind
		if i >= 1 && i <= 3 && b.State != observations.BlockRequested {
			t.Errorf("block %d is %s", i, b.State)
		}
	}
	if !slices.Equal(kinds, want) {
		t.Errorf("kinds %v, want %v", kinds, want)
	}
}

func TestAnAnswerShownOnThePageGoesUnderTheBlockItWasAskedOn(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	r.addText(t, editor, o, "a")
	asked := r.addText(t, editor, o, "b")
	r.addText(t, editor, o, "c")
	run := r.runAskedOn(t, editor, asked)
	c := recordInput(t, run, evidencePanel)
	c.Into = mustUUID(t, o.ID)
	if _, _, err := r.conclude(t, editor, c); err != nil {
		t.Fatal(err)
	}
	want := []observations.Kind{observations.KindText, observations.KindText, observations.KindText, observations.KindPanel, observations.KindText}
	if kinds := r.kindsOf(t, editor, o.ID); !slices.Equal(kinds, want) {
		t.Errorf("kinds %v, want %v", kinds, want)
	}
	if texts := r.positions(t, editor, o); texts[0] != "a" || texts[1] != "b" || texts[4] != "c" {
		t.Errorf("order %v", texts)
	}
}
