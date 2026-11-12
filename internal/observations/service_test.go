package observations_test

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/lifecycle"
	"github.com/slashobserve/slashobserve/internal/observations"
)

func TestCreatingNeedsCreateInTheFolder(t *testing.T) {
	r := newRig(t)
	hidden := r.hiddenFolder(t)
	cases := []struct {
		name   string
		by     auth.Principal
		folder pgtype.UUID
		want   error
	}{
		{"a viewer of the folder", viewer, r.shared, folders.ErrForbidden},
		{"a commenter of the folder", commenter, r.shared, folders.ErrForbidden},
		{"a folder that does not exist", editor, pgtype.UUID{Bytes: [16]byte{15: 9}, Valid: true}, observations.ErrFolderNotFound},
		{"a folder the caller cannot see", otherEditor, hidden, observations.ErrFolderNotFound},
		{"another person's My Dashboards", otherEditor, r.personalFolder(t, editor), observations.ErrFolderNotFound},
	}
	for _, c := range cases {
		_, err := r.service.Create(t.Context(), c.by, observations.CreateInput{FolderID: c.folder, Title: "t"})
		if !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", c.name, err, c.want)
		}
	}
	o := r.create(t, editor, r.shared)
	if o.State != observations.StateOpen || o.FolderID != r.shared.String() || o.Title != "Checkout 5xx" || o.Summary != "" || o.ThreadID != "" {
		t.Errorf("created %+v", o)
	}
	if !slices.Equal(o.AllowedActions, []observations.Action{observations.ActionUpdate, observations.ActionMove, observations.ActionResolve, observations.ActionPublish}) {
		t.Errorf("allowed actions %v", o.AllowedActions)
	}
	if o.Actor.ID != editor.ID || o.Actor.Name != "Eve" {
		t.Errorf("actor %+v", o.Actor)
	}
}

func TestCreatingWithoutAFolderFilesInTheCallersMyDashboards(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, pgtype.UUID{})
	if o.FolderID != r.personalFolder(t, editor).String() || !o.Folder.Personal {
		t.Errorf("folder %q personal %v", o.FolderID, o.Folder.Personal)
	}
	k := r.create(t, key, pgtype.UUID{})
	if !k.Folder.General {
		t.Errorf("a key files in General, got %+v", k.Folder)
	}
	if _, err := r.service.Create(t.Context(), viewer, observations.CreateInput{Title: "t"}); err != nil {
		t.Errorf("a viewer has My Dashboards: %v", err)
	}
}

func TestCreatingValidatesTheTitleAndTheRange(t *testing.T) {
	r := newRig(t)
	from, to := windowTo.Add(-time.Hour), windowTo
	cases := map[string]observations.CreateInput{
		"empty title": {FolderID: r.shared, Title: " "},
		"only from":   {FolderID: r.shared, Title: "t", From: &from},
		"reversed":    {FolderID: r.shared, Title: "t", From: &to, To: &from},
	}
	for name, in := range cases {
		var invalid *observations.InvalidInputError
		if _, err := r.service.Create(t.Context(), editor, in); !errors.As(err, &invalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	o, err := r.service.Create(t.Context(), editor, observations.CreateInput{FolderID: r.shared, Title: "  padded  ", From: &from, To: &to})
	if err != nil || o.Title != "padded" || o.From == nil || !o.From.Equal(from) || !o.To.Equal(to) {
		t.Fatalf("created %+v, %v", o, err)
	}
}

func TestCreatingWithAFirstBlockWritesBothInOneTransaction(t *testing.T) {
	r := newRig(t)
	o, err := r.service.Create(t.Context(), editor, observations.CreateInput{
		FolderID: r.shared, Title: "t", FirstBlock: &observations.BlockInput{Kind: observations.KindText, Body: textBody("Start here")},
	})
	if err != nil || len(o.Blocks) != 1 || o.Blocks[0].State != observations.BlockLive || o.Blocks[0].Position != 0 {
		t.Fatalf("created %+v, %v", o.Blocks, err)
	}
	want := []string{activity.TypeObservationCreated, activity.TypeObservationBlockAdded}
	if got := r.activityTypes(t, o); !slices.Equal(got, want) {
		t.Errorf("activity %v, want %v", got, want)
	}
	_, err = r.service.Create(t.Context(), editor, observations.CreateInput{
		FolderID: r.shared, Title: "bad", FirstBlock: &observations.BlockInput{Kind: observations.KindText, Body: textBody("")},
	})
	var invalid *observations.InvalidInputError
	if !errors.As(err, &invalid) {
		t.Fatalf("bad first block: %v", err)
	}
	list, _, err := r.service.List(t.Context(), editor, observations.ListFilter{FolderID: r.shared}, 10, "")
	if err != nil || len(list) != 1 {
		t.Errorf("the failed create left %d observations, %v", len(list), err)
	}
}

func TestACommenterAddsRequestedBlocksAnEditorAddsLiveOnesAndAViewerNone(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	id := mustUUID(t, o.ID)
	live := r.addText(t, editor, o, "From the editor")
	requested := r.addText(t, commenter, o, "From the commenter")
	if live.State != observations.BlockLive || requested.State != observations.BlockRequested {
		t.Errorf("states %s and %s", live.State, requested.State)
	}
	if live.Version != 1 || live.Position != 0 || requested.Position != 1 || live.Width != observations.WidthFull {
		t.Errorf("live %+v requested %+v", live, requested)
	}
	_, err := r.service.AddBlock(t.Context(), viewer, id, observations.BlockInput{Kind: observations.KindText, Body: textBody("x")})
	if !errors.Is(err, folders.ErrForbidden) {
		t.Errorf("viewer: %v", err)
	}
	_, err = r.service.AddBlock(t.Context(), otherEditor, mustUUID(t, r.create(t, admin, r.hiddenFolder(t)).ID), observations.BlockInput{Kind: observations.KindText, Body: textBody("x")})
	if !errors.Is(err, observations.ErrNotFound) {
		t.Errorf("a non-viewer: %v", err)
	}
}

func TestAKeptRequestBecomesLiveAndABlockCannotBeKeptByItsAuthorWhoOnlyComments(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	asked := r.addText(t, commenter, o, "Please add the 5xx rate")
	askedID := mustUUID(t, asked.ID)

	_, err := r.service.KeepBlock(t.Context(), commenter, askedID, asked.Version, pgtype.UUID{})
	var refusal *lifecycle.Refusal
	if !errors.As(err, &refusal) || refusal.State != "requested" || !slices.Equal(refusal.Allowed, []string{"discard"}) {
		t.Fatalf("author keeping: %v", err)
	}
	if _, err := r.service.KeepBlock(t.Context(), otherCommen, askedID, asked.Version, pgtype.UUID{}); !errors.Is(err, folders.ErrForbidden) {
		t.Errorf("another commenter keeping: %v", err)
	}
	kept, err := r.service.KeepBlock(t.Context(), editor, askedID, asked.Version, pgtype.UUID{})
	if err != nil || kept.State != observations.BlockLive || kept.Version != asked.Version+1 {
		t.Fatalf("editor keeping: %+v, %v", kept, err)
	}
	if kept.Actor.ID != commenter.ID {
		t.Errorf("the block stays the commenter's: %+v", kept.Actor)
	}
	if _, err := r.service.KeepBlock(t.Context(), editor, askedID, kept.Version, pgtype.UUID{}); !errors.As(err, &refusal) || refusal.State != "live" {
		t.Errorf("keeping a live block: %v", err)
	}
}

func TestTheAuthorOfARequestDiscardsItAndAnEditorDiscardsAnyoneElses(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	mine := r.addText(t, commenter, o, "mine")
	theirs := r.addText(t, commenter, o, "theirs")
	if err := r.service.DiscardBlock(t.Context(), otherCommen, mustUUID(t, mine.ID), pgtype.UUID{}); !errors.Is(err, folders.ErrForbidden) {
		t.Errorf("another commenter: %v", err)
	}
	if err := r.service.DiscardBlock(t.Context(), commenter, mustUUID(t, mine.ID), pgtype.UUID{}); err != nil {
		t.Fatalf("the author: %v", err)
	}
	if err := r.service.DiscardBlock(t.Context(), editor, mustUUID(t, theirs.ID), pgtype.UUID{}); err != nil {
		t.Fatalf("an editor: %v", err)
	}
	if got := r.positions(t, editor, o); len(got) != 0 {
		t.Errorf("blocks left: %v", got)
	}
	live := r.addText(t, editor, o, "live")
	var refusal *lifecycle.Refusal
	if err := r.service.DiscardBlock(t.Context(), editor, mustUUID(t, live.ID), pgtype.UUID{}); !errors.As(err, &refusal) || refusal.State != "live" {
		t.Errorf("discarding a live block: %v", err)
	}
	if err := r.service.DiscardBlock(t.Context(), editor, mustUUID(t, mine.ID), pgtype.UUID{}); !errors.Is(err, observations.ErrNotFound) {
		t.Errorf("discarding twice: %v", err)
	}
}

func TestAStaleVersionIsRefusedWithTheCurrentBlock(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	b := r.addText(t, editor, o, "v1")
	id := mustUUID(t, b.ID)
	updated, err := r.service.UpdateBlock(t.Context(), editor, id, observations.BlockChange{Version: 1, Body: textBody("v2")})
	if err != nil || updated.Version != 2 {
		t.Fatalf("update: %+v, %v", updated, err)
	}
	cases := map[string]func() error{
		"update": func() error {
			_, err := r.service.UpdateBlock(t.Context(), editor, id, observations.BlockChange{Version: 1, Body: textBody("v3")})
			return err
		},
		"remove": func() error { return r.service.RemoveBlock(t.Context(), editor, id, 1, pgtype.UUID{}) },
	}
	for name, call := range cases {
		var conflict *observations.ConflictError
		if err := call(); !errors.As(err, &conflict) || conflict.Current.Version != 2 || !strings.Contains(string(conflict.Current.Body), "v2") {
			t.Errorf("%s: %v", name, err)
		}
	}
	if got := r.positions(t, editor, o); !slices.Equal(got, []string{"v2"}) {
		t.Errorf("blocks %v", got)
	}
	if err := r.service.RemoveBlock(t.Context(), editor, id, 2, pgtype.UUID{}); err != nil {
		t.Errorf("remove at the current version: %v", err)
	}
}

func TestInsertingAtAPositionShiftsTheRestAndLeavesTheirVersions(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	id := mustUUID(t, o.ID)
	r.addText(t, editor, o, "a")
	r.addText(t, editor, o, "b")
	first := 0
	if _, err := r.service.AddBlock(t.Context(), editor, id, observations.BlockInput{Kind: observations.KindText, Body: textBody("front"), Position: &first}); err != nil {
		t.Fatal(err)
	}
	beyond := 99
	if _, err := r.service.AddBlock(t.Context(), editor, id, observations.BlockInput{Kind: observations.KindText, Body: textBody("end"), Position: &beyond}); err != nil {
		t.Fatal(err)
	}
	if got := r.positions(t, editor, o); !slices.Equal(got, []string{"front", "a", "b", "end"}) {
		t.Fatalf("order %v", got)
	}
	got, err := r.service.Get(t.Context(), editor, id)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range got.Blocks {
		if b.Version != 1 {
			t.Errorf("block %q version %d", b.Body, b.Version)
		}
	}
}

func TestMovingABlockReordersTheOthersAndBumpsOnlyItsVersion(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	var blocks []observations.Block
	for _, m := range []string{"a", "b", "c", "d"} {
		blocks = append(blocks, r.addText(t, editor, o, m))
	}
	to := func(i int) *int { return &i }
	steps := []struct {
		block int
		to    int
		want  []string
	}{
		{3, 0, []string{"d", "a", "b", "c"}},
		{3, 0, []string{"d", "a", "b", "c"}},
		{3, 1, []string{"a", "d", "b", "c"}},
		{0, 99, []string{"d", "b", "c", "a"}},
	}
	version := map[string]int{}
	for _, b := range blocks {
		version[b.ID] = b.Version
	}
	for _, s := range steps {
		moved, err := r.service.UpdateBlock(t.Context(), editor, mustUUID(t, blocks[s.block].ID), observations.BlockChange{Version: version[blocks[s.block].ID], Position: to(s.to)})
		if err != nil {
			t.Fatal(err)
		}
		version[moved.ID] = moved.Version
		if got := r.positions(t, editor, o); !slices.Equal(got, s.want) {
			t.Fatalf("after moving %d to %d: %v, want %v", s.block, s.to, got, s.want)
		}
	}
	current, err := r.service.Get(t.Context(), editor, mustUUID(t, o.ID))
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range current.Blocks {
		if b.Version != version[b.ID] {
			t.Errorf("block %s version %d, want %d", b.ID, b.Version, version[b.ID])
		}
	}
}

func TestRemovingABlockClosesTheGap(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	var ids []observations.Block
	for _, m := range []string{"a", "b", "c"} {
		ids = append(ids, r.addText(t, editor, o, m))
	}
	if err := r.service.RemoveBlock(t.Context(), editor, mustUUID(t, ids[1].ID), 1, pgtype.UUID{}); err != nil {
		t.Fatal(err)
	}
	if got := r.positions(t, editor, o); !slices.Equal(got, []string{"a", "c"}) {
		t.Errorf("order %v", got)
	}
	if err := r.service.RemoveBlock(t.Context(), viewer, mustUUID(t, ids[0].ID), 1, pgtype.UUID{}); !errors.Is(err, folders.ErrForbidden) {
		t.Errorf("a viewer removing: %v", err)
	}
	asked := r.addText(t, commenter, o, "asked")
	var refusal *lifecycle.Refusal
	if err := r.service.RemoveBlock(t.Context(), editor, mustUUID(t, asked.ID), 1, pgtype.UUID{}); !errors.As(err, &refusal) || refusal.State != "requested" {
		t.Errorf("removing a requested block: %v", err)
	}
}

func TestTheHundredAndFirstBlockAndTheTwentyFifthPanelAreRefused(t *testing.T) {
	r := newRig(t)
	panels := r.create(t, editor, r.shared)
	for i := range observations.MaxPanelBlocks {
		if _, err := r.service.AddBlock(t.Context(), editor, mustUUID(t, panels.ID), observations.BlockInput{Kind: observations.KindPanel, Body: panelBody()}); err != nil {
			t.Fatalf("panel %d: %v", i+1, err)
		}
	}
	_, err := r.service.AddBlock(t.Context(), editor, mustUUID(t, panels.ID), observations.BlockInput{Kind: observations.KindPanel, Body: panelBody()})
	if !errors.Is(err, observations.ErrTooManyPanels) {
		t.Errorf("25th panel: %v", err)
	}
	r.addText(t, editor, panels, "text still fits")

	texts := r.create(t, editor, r.shared)
	for i := range observations.MaxBlocks {
		if _, err := r.service.AddBlock(t.Context(), editor, mustUUID(t, texts.ID), observations.BlockInput{Kind: observations.KindText, Body: textBody("t")}); err != nil {
			t.Fatalf("block %d: %v", i+1, err)
		}
	}
	_, err = r.service.AddBlock(t.Context(), editor, mustUUID(t, texts.ID), observations.BlockInput{Kind: observations.KindText, Body: textBody("one too many")})
	if !errors.Is(err, observations.ErrTooManyBlocks) {
		t.Errorf("101st block: %v", err)
	}
}

func TestResolvingNeedsASummaryOnceAndReopeningKeepsIt(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	id := mustUUID(t, o.ID)
	var invalid *observations.InvalidInputError
	if _, err := r.service.Resolve(t.Context(), editor, id, "  "); !errors.As(err, &invalid) || invalid.Code != observations.CodeInvalidSummary {
		t.Errorf("no summary: %v", err)
	}
	if _, err := r.service.Resolve(t.Context(), viewer, id, "rolled back"); !errors.Is(err, folders.ErrForbidden) {
		t.Errorf("viewer resolving: %v", err)
	}
	resolved, err := r.service.Resolve(t.Context(), editor, id, " rolled back v42 ")
	if err != nil || resolved.State != observations.StateResolved || resolved.Summary != "rolled back v42" || resolved.ResolvedAt == nil {
		t.Fatalf("resolved %+v, %v", resolved, err)
	}
	var refusal *lifecycle.Refusal
	if _, err := r.service.Resolve(t.Context(), editor, id, "again"); !errors.As(err, &refusal) || refusal.State != "resolved" || !slices.Contains(refusal.Allowed, "reopen") {
		t.Errorf("resolving twice: %v", err)
	}
	reopened, err := r.service.Reopen(t.Context(), editor, id)
	if err != nil || reopened.State != observations.StateOpen || reopened.Summary != "rolled back v42" || reopened.ResolvedAt != nil {
		t.Fatalf("reopened %+v, %v", reopened, err)
	}
	if _, err := r.service.Reopen(t.Context(), editor, id); !errors.As(err, &refusal) || refusal.State != "open" {
		t.Errorf("reopening an open one: %v", err)
	}
}

func TestMovingNeedsEditOnBothFoldersAndRefusesMyDashboards(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	id := mustUUID(t, o.ID)
	target := r.folder(t, "Archive")
	hidden := r.hiddenFolder(t)

	if _, err := r.service.Move(t.Context(), editor, id, r.personalFolder(t, editor)); !errors.Is(err, folders.ErrMoveIntoPersonal) {
		t.Errorf("into My Dashboards: %v", err)
	}
	if _, err := r.service.Move(t.Context(), editor, id, hidden); !errors.Is(err, observations.ErrFolderNotFound) {
		t.Errorf("into a folder the caller cannot see: %v", err)
	}
	if _, err := r.service.Move(t.Context(), viewer, id, target); !errors.Is(err, folders.ErrForbidden) {
		t.Errorf("a viewer moving: %v", err)
	}
	moved, err := r.service.Move(t.Context(), editor, id, target)
	if err != nil || moved.FolderID != target.String() || moved.Folder.Title != "Archive" {
		t.Fatalf("moved %+v, %v", moved, err)
	}
	same, err := r.service.Move(t.Context(), editor, id, target)
	if err != nil || same.FolderID != target.String() {
		t.Fatalf("moving to the same folder: %v", err)
	}
	want := []string{activity.TypeObservationCreated, activity.TypeObservationMoved}
	if got := r.activityTypes(t, o); !slices.Equal(got, want) {
		t.Errorf("activity %v, want %v (a move to the same folder is no event)", got, want)
	}
	var folder string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT folder_id::text FROM activity WHERE observation_id = $1::uuid AND type = 'observation.created'`, o.ID).Scan(&folder); err != nil || folder != target.String() {
		t.Errorf("past activity follows the observation: %q, %v", folder, err)
	}
}

func TestUpdatingChangesTheTitleAndTheRangeAndClearsItOnRequest(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	id := mustUUID(t, o.ID)
	from, to := windowTo.Add(-2*time.Hour), windowTo
	title := "Renamed"
	got, err := r.service.Update(t.Context(), editor, id, observations.UpdateInput{Title: &title, From: &from, To: &to})
	if err != nil || got.Title != "Renamed" || got.From == nil || !got.To.Equal(to) {
		t.Fatalf("updated %+v, %v", got, err)
	}
	got, err = r.service.Update(t.Context(), editor, id, observations.UpdateInput{ClearRange: true})
	if err != nil || got.From != nil || got.To != nil || got.Title != "Renamed" {
		t.Fatalf("cleared %+v, %v", got, err)
	}
	if _, err := r.service.Update(t.Context(), editor, id, observations.UpdateInput{}); !errors.Is(err, observations.ErrNothingToUpdate) {
		t.Errorf("nothing: %v", err)
	}
	if _, err := r.service.Update(t.Context(), editor, id, observations.UpdateInput{ClearRange: true, From: &from, To: &to}); err == nil {
		t.Error("clearing and setting the range together was accepted")
	}
	if _, err := r.service.Update(t.Context(), commenter, id, observations.UpdateInput{Title: &title}); !errors.Is(err, folders.ErrForbidden) {
		t.Errorf("a commenter: %v", err)
	}
	if _, err := r.service.Update(t.Context(), otherEditor, mustUUID(t, r.create(t, admin, r.hiddenFolder(t)).ID), observations.UpdateInput{Title: &title}); !errors.Is(err, observations.ErrNotFound) {
		t.Errorf("a non-viewer: %v", err)
	}
}

func TestEveryWriteAppendsExactlyOneActivityRowOfTheObservationTarget(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	id := mustUUID(t, o.ID)
	text := r.addText(t, editor, o, "a")
	asked := r.addText(t, commenter, o, "b")
	if _, err := r.service.UpdateBlock(t.Context(), editor, mustUUID(t, text.ID), observations.BlockChange{Version: 1, Body: textBody("a2")}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.service.KeepBlock(t.Context(), editor, mustUUID(t, asked.ID), 1, pgtype.UUID{}); err != nil {
		t.Fatal(err)
	}
	discard := r.addText(t, commenter, o, "c")
	if err := r.service.DiscardBlock(t.Context(), commenter, mustUUID(t, discard.ID), pgtype.UUID{}); err != nil {
		t.Fatal(err)
	}
	if err := r.service.RemoveBlock(t.Context(), editor, mustUUID(t, text.ID), 2, pgtype.UUID{}); err != nil {
		t.Fatal(err)
	}
	title := "t2"
	if _, err := r.service.Update(t.Context(), editor, id, observations.UpdateInput{Title: &title}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.service.Move(t.Context(), editor, id, r.folder(t, "Elsewhere")); err != nil {
		t.Fatal(err)
	}
	if _, err := r.service.Resolve(t.Context(), editor, id, "done"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.service.Reopen(t.Context(), editor, id); err != nil {
		t.Fatal(err)
	}
	want := []string{
		activity.TypeObservationCreated,
		activity.TypeObservationBlockAdded, activity.TypeObservationBlockAdded,
		activity.TypeObservationBlockUpdated, activity.TypeObservationBlockKept,
		activity.TypeObservationBlockAdded, activity.TypeObservationBlockDiscarded,
		activity.TypeObservationBlockRemoved,
		activity.TypeObservationUpdated, activity.TypeObservationMoved,
		activity.TypeObservationResolved, activity.TypeObservationReopened,
	}
	if got := r.activityTypes(t, o); !slices.Equal(got, want) {
		t.Errorf("activity\n got %v\nwant %v", got, want)
	}
}

func TestActivityDataCarriesNoBlockTextOrSummary(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	r.addText(t, editor, o, "secret markdown words")
	r.addPanel(t, editor, o)
	if _, err := r.service.Resolve(t.Context(), editor, mustUUID(t, o.ID), "secret summary words"); err != nil {
		t.Fatal(err)
	}
	rows, err := r.db.SQLDB(t).QueryContext(t.Context(), `SELECT data::text FROM activity WHERE observation_id = $1::uuid`, o.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	n := 0
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			t.Fatal(err)
		}
		n++
		if strings.Contains(data, "secret") || strings.Contains(data, "sum(rate") {
			t.Errorf("event data %s carries content", data)
		}
	}
	if n == 0 {
		t.Fatal("no activity rows")
	}
}

func TestConcurrentAddsLeaveDensePositions(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	id := mustUUID(t, o.ID)
	const writers = 12
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := range writers {
		wg.Go(func() {
			by, position := editor, (*int)(nil)
			if i%3 == 0 {
				zero := 0
				position = &zero
			}
			_, err := r.service.AddBlock(t.Context(), by, id, observations.BlockInput{Kind: observations.KindText, Body: textBody("w"), Position: position})
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := r.positions(t, editor, o); len(got) != writers {
		t.Errorf("%d blocks, want %d", len(got), writers)
	}
}

func TestListingShowsOnlyVisibleFoldersFiltersAndPages(t *testing.T) {
	r := newRig(t)
	hidden := r.hiddenFolder(t)
	a := r.create(t, editor, r.shared)
	b := r.create(t, editor, r.shared)
	c := r.create(t, editor, r.shared)
	r.create(t, admin, hidden)
	if _, err := r.service.Resolve(t.Context(), editor, mustUUID(t, b.ID), "done"); err != nil {
		t.Fatal(err)
	}

	first, next, err := r.service.List(t.Context(), viewer, observations.ListFilter{}, 2, "")
	if err != nil || len(first) != 2 || next == "" || first[0].ID != b.ID || first[1].ID != c.ID {
		t.Fatalf("first page %v, next %q, %v", ids(first), next, err)
	}
	second, next, err := r.service.List(t.Context(), viewer, observations.ListFilter{}, 2, next)
	if err != nil || len(second) != 1 || next != "" || second[0].ID != a.ID {
		t.Fatalf("second page %v, next %q, %v", ids(second), next, err)
	}
	resolved, _, err := r.service.List(t.Context(), viewer, observations.ListFilter{State: observations.StateResolved}, 10, "")
	if err != nil || len(resolved) != 1 || resolved[0].ID != b.ID {
		t.Errorf("resolved %v, %v", ids(resolved), err)
	}
	if all, _, err := r.service.List(t.Context(), admin, observations.ListFilter{}, 10, ""); err != nil || len(all) != 4 {
		t.Errorf("an admin sees %d, %v", len(all), err)
	}
	if _, _, err := r.service.List(t.Context(), viewer, observations.ListFilter{FolderID: hidden}, 10, ""); !errors.Is(err, observations.ErrFolderNotFound) {
		t.Errorf("a folder the caller cannot see: %v", err)
	}
	if _, _, err := r.service.List(t.Context(), viewer, observations.ListFilter{}, 10, "not-a-cursor"); !errors.Is(err, observations.ErrInvalidCursor) {
		t.Errorf("a bad cursor: %v", err)
	}
	if got, _, _ := r.service.List(t.Context(), viewer, observations.ListFilter{}, 10, ""); got[0].Blocks != nil {
		t.Error("a list item carries blocks")
	}
}

func ids(list []observations.Observation) []string {
	out := make([]string, len(list))
	for i, o := range list {
		out[i] = o.ID
	}
	return out
}

func TestAnObservationInAHiddenFolderIsNotFoundForEveryRead(t *testing.T) {
	r := newRig(t)
	hidden := r.hiddenFolder(t)
	o := r.create(t, admin, hidden)
	id := mustUUID(t, o.ID)
	block := r.addText(t, admin, o, "x")
	if _, err := r.service.Get(t.Context(), editor, id); !errors.Is(err, observations.ErrNotFound) {
		t.Errorf("get: %v", err)
	}
	if _, err := r.service.Access(t.Context(), editor, id); !errors.Is(err, observations.ErrNotFound) {
		t.Errorf("access: %v", err)
	}
	if _, _, _, err := r.service.PanelDashboard(t.Context(), editor, mustUUID(t, block.ID)); !errors.Is(err, observations.ErrNotFound) {
		t.Errorf("panel dashboard: %v", err)
	}
	if err := r.service.DiscardBlock(t.Context(), editor, mustUUID(t, block.ID), pgtype.UUID{}); !errors.Is(err, observations.ErrNotFound) {
		t.Errorf("discard: %v", err)
	}
	if _, err := r.service.Get(t.Context(), admin, id); err != nil {
		t.Errorf("an admin: %v", err)
	}
}

func TestGetCarriesBlocksInOrderWithTheirAllowedActions(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	r.addText(t, editor, o, "live")
	r.addText(t, commenter, o, "asked")
	got, err := r.service.Get(t.Context(), commenter, mustUUID(t, o.ID))
	if err != nil || len(got.Blocks) != 2 {
		t.Fatalf("got %+v, %v", got.Blocks, err)
	}
	if len(got.AllowedActions) != 0 || len(got.Blocks[0].AllowedActions) != 0 || !slices.Equal(got.Blocks[1].AllowedActions, []observations.BlockAction{observations.BlockDiscard}) {
		t.Errorf("commenter: observation %v, live %v, requested %v", got.AllowedActions, got.Blocks[0].AllowedActions, got.Blocks[1].AllowedActions)
	}
	editorView, err := r.service.Get(t.Context(), editor, mustUUID(t, o.ID))
	if err != nil || !slices.Equal(editorView.Blocks[1].AllowedActions, []observations.BlockAction{observations.BlockUpdate, observations.BlockKeep, observations.BlockDiscard}) {
		t.Errorf("editor view: %v, %v", editorView.Blocks[1].AllowedActions, err)
	}
}

func TestTheDiscussionThreadIsReportedOnTheObservation(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	r.db.Exec(t, `INSERT INTO threads (observation_id) VALUES ($1::uuid)`, o.ID)
	got, err := r.service.Get(t.Context(), editor, mustUUID(t, o.ID))
	if err != nil || got.ThreadID == "" {
		t.Fatalf("thread id %q, %v", got.ThreadID, err)
	}
	list, _, err := r.service.List(t.Context(), editor, observations.ListFilter{}, 10, "")
	if err != nil || len(list) != 1 || list[0].ThreadID != got.ThreadID {
		t.Errorf("list thread id %v, %v", list, err)
	}
}

func TestAccessNamesTheFolderAbilitiesAndTheActivityOwner(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	placement, err := r.service.Access(t.Context(), commenter, mustUUID(t, o.ID))
	if err != nil || placement.Folder != r.shared || placement.Personal || !placement.Abilities.Comment || placement.Abilities.Edit {
		t.Fatalf("placement %+v, %v", placement, err)
	}
	mine := r.create(t, editor, pgtype.UUID{})
	placement, err = r.service.Access(t.Context(), editor, mustUUID(t, mine.ID))
	if err != nil || !placement.Personal {
		t.Errorf("personal placement %+v, %v", placement, err)
	}
}

func TestEditingABlockValidatesItsBodyByKind(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	text := r.addText(t, editor, o, "a")
	panel := r.addPanel(t, editor, o)
	snapshot, err := r.service.AddBlock(t.Context(), editor, mustUUID(t, o.ID), observations.BlockInput{Kind: observations.KindSnapshot, FromBlockID: mustUUID(t, panel.ID)})
	if err != nil {
		t.Fatal(err)
	}
	var invalid *observations.InvalidInputError
	if _, err := r.service.UpdateBlock(t.Context(), editor, mustUUID(t, snapshot.ID), observations.BlockChange{Version: 1, Body: textBody("x")}); !errors.As(err, &invalid) || invalid.Code != observations.CodeBodyNotEditable {
		t.Errorf("snapshot body: %v", err)
	}
	if _, err := r.service.UpdateBlock(t.Context(), editor, mustUUID(t, text.ID), observations.BlockChange{Version: 1, Body: panelBody()}); !errors.As(err, &invalid) || invalid.Code != observations.CodeInvalidBlock {
		t.Errorf("a panel body on a text block: %v", err)
	}
	if _, err := r.service.UpdateBlock(t.Context(), editor, mustUUID(t, text.ID), observations.BlockChange{Version: 1}); !errors.Is(err, observations.ErrNothingToUpdate) {
		t.Errorf("nothing: %v", err)
	}
	half := observations.WidthHalf
	got, err := r.service.UpdateBlock(t.Context(), editor, mustUUID(t, snapshot.ID), observations.BlockChange{Version: 1, Width: &half})
	if err != nil || got.Width != observations.WidthHalf || got.Version != 2 {
		t.Errorf("width of a snapshot: %+v, %v", got, err)
	}
	bad := observations.Width("wide")
	if _, err := r.service.UpdateBlock(t.Context(), editor, mustUUID(t, text.ID), observations.BlockChange{Version: 1, Width: &bad}); !errors.As(err, &invalid) {
		t.Errorf("bad width: %v", err)
	}
}

func TestAddingBlocksOfTheKindsAPersonMayNotAdd(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	id := mustUUID(t, o.ID)
	cases := map[string]struct {
		in   observations.BlockInput
		code string
	}{
		"run":                  {observations.BlockInput{Kind: observations.KindRun, Body: json.RawMessage(`{}`)}, observations.CodeKindNotAddable},
		"unknown":              {observations.BlockInput{Kind: "chart", Body: json.RawMessage(`{}`)}, observations.CodeInvalidBlock},
		"snapshot of nothing":  {observations.BlockInput{Kind: observations.KindSnapshot}, observations.CodeInvalidBlock},
		"snapshot with a body": {observations.BlockInput{Kind: observations.KindSnapshot, Body: json.RawMessage(`{}`)}, observations.CodeInvalidBlock},
		"bad width":            {observations.BlockInput{Kind: observations.KindText, Body: textBody("x"), Width: "wide"}, observations.CodeInvalidBlock},
		"text panel":           {observations.BlockInput{Kind: observations.KindPanel, Body: json.RawMessage(`{"panel":{"type":"text","title":"t","content":"x"}}`)}, observations.CodeInvalidPanel},
	}
	for name, c := range cases {
		var invalid *observations.InvalidInputError
		if _, err := r.service.AddBlock(t.Context(), editor, id, c.in); !errors.As(err, &invalid) || invalid.Code != c.code {
			t.Errorf("%s: %v, want %s", name, err, c.code)
		}
	}
	text := r.addText(t, editor, o, "a")
	var invalid *observations.InvalidInputError
	_, err := r.service.AddBlock(t.Context(), editor, id, observations.BlockInput{Kind: observations.KindSnapshot, FromBlockID: mustUUID(t, text.ID)})
	if !errors.As(err, &invalid) || invalid.Code != observations.CodeInvalidBlock {
		t.Errorf("snapshot of a text block: %v", err)
	}
	other := r.create(t, editor, r.shared)
	foreign := r.addPanel(t, editor, other)
	_, err = r.service.AddBlock(t.Context(), editor, id, observations.BlockInput{Kind: observations.KindSnapshot, FromBlockID: mustUUID(t, foreign.ID)})
	if !errors.As(err, &invalid) || invalid.Code != observations.CodeInvalidBlock {
		t.Errorf("snapshot of another observation's block: %v", err)
	}
	if r.freezer.calls != 0 {
		t.Errorf("the freezer ran %d times for refused snapshots", r.freezer.calls)
	}
}
