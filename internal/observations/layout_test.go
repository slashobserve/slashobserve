package observations_test

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/observations"
)

func layoutOf(blocks ...observations.Block) []observations.BlockLayout {
	items := make([]observations.BlockLayout, len(blocks))
	for i, b := range blocks {
		items[i] = observations.BlockLayout{ID: uuidOf(b.ID)}
	}
	return items
}

func uuidOf(raw string) pgtype.UUID {
	var id pgtype.UUID
	if err := id.Scan(raw); err != nil {
		panic(err)
	}
	return id
}

func TestArrangingOrdersWidensAndSizesTheBlocksWithoutBumpingVersions(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	a, b, c := r.addText(t, editor, o, "a"), r.addText(t, editor, o, "b"), r.addText(t, editor, o, "c")
	third, twoThirds := observations.WidthThird, observations.WidthTwoThirds
	items := layoutOf(c, a, b)
	items[0].Width = &third
	items[1].Width = &twoThirds
	got, err := r.service.Arrange(reading(t.Context()), editor, mustUUID(t, o.ID), items, pgtype.UUID{})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{c.ID, a.ID, b.ID}; !slices.Equal([]string{got[0].ID, got[1].ID, got[2].ID}, want) {
		t.Fatalf("order %v", got)
	}
	if got[0].Width != observations.WidthThird || got[1].Width != observations.WidthTwoThirds || got[2].Width != observations.WidthFull {
		t.Errorf("widths %s %s %s", got[0].Width, got[1].Width, got[2].Width)
	}
	if got[0].Height != nil || got[1].Height != nil {
		t.Errorf("heights %v %v", got[0].Height, got[1].Height)
	}
	for i, block := range got {
		if block.Position != i || block.Version != 1 {
			t.Errorf("block %d has position %d and version %d", i, block.Position, block.Version)
		}
	}
	if order := r.positions(t, editor, o); !slices.Equal(order, []string{"c", "a", "b"}) {
		t.Errorf("stored order %v", order)
	}
}

func TestArrangingKeepsAWidthOrHeightThatIsLeftOutAndClearsAHeightOnNull(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	a := r.addPanel(t, editor, o)
	half := observations.WidthHalf
	sized := []observations.BlockLayout{{ID: uuidOf(a.ID), Width: &half, Height: observations.HeightChange{Set: true, Pixels: new(240)}}}
	if _, err := r.service.Arrange(reading(t.Context()), editor, mustUUID(t, o.ID), sized, pgtype.UUID{}); err != nil {
		t.Fatal(err)
	}
	got, err := r.service.Arrange(reading(t.Context()), editor, mustUUID(t, o.ID), layoutOf(a), pgtype.UUID{})
	if err != nil || got[0].Width != observations.WidthHalf || got[0].Height == nil || *got[0].Height != 240 {
		t.Fatalf("left out: %+v, %v", got, err)
	}
	cleared := []observations.BlockLayout{{ID: uuidOf(a.ID), Height: observations.HeightChange{Set: true}}}
	got, err = r.service.Arrange(reading(t.Context()), editor, mustUUID(t, o.ID), cleared, pgtype.UUID{})
	if err != nil || got[0].Height != nil {
		t.Fatalf("cleared: %+v, %v", got, err)
	}
}

func TestAnUnchangedLayoutRecordsNothingAndAChangeRecordsOneUpdate(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	a, b := r.addText(t, editor, o, "a"), r.addText(t, editor, o, "b")
	before := len(r.activityTypes(t, o))
	if _, err := r.service.Arrange(reading(t.Context()), editor, mustUUID(t, o.ID), layoutOf(a, b), pgtype.UUID{}); err != nil {
		t.Fatal(err)
	}
	if got := len(r.activityTypes(t, o)); got != before {
		t.Fatalf("an unchanged layout appended %d events", got-before)
	}
	if _, err := r.service.Arrange(reading(t.Context()), editor, mustUUID(t, o.ID), layoutOf(b, a), pgtype.UUID{}); err != nil {
		t.Fatal(err)
	}
	types := r.activityTypes(t, o)
	if len(types) != before+1 || types[len(types)-1] != activity.TypeObservationUpdated {
		t.Fatalf("activity %v", types)
	}
}

func TestALayoutNamingABlockOfAnotherObservationIsRefusedAndChangesNothing(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	other := r.create(t, editor, r.shared)
	r.addText(t, editor, o, "a")
	b := r.addText(t, editor, o, "b")
	stranger := r.addText(t, editor, other, "x")
	items := append(layoutOf(b), layoutOf(stranger)...)
	_, err := r.service.Arrange(reading(t.Context()), editor, mustUUID(t, o.ID), items, pgtype.UUID{})
	var refused *observations.InvalidInputError
	if !errors.As(err, &refused) || refused.Code != observations.CodeInvalidBlock || !strings.Contains(refused.Message, stranger.ID) {
		t.Fatalf("got %v, want an invalid_block naming %s", err, stranger.ID)
	}
	if order := r.positions(t, editor, o); !slices.Equal(order, []string{"a", "b"}) {
		t.Errorf("stored order %v after the refusal", order)
	}
}

func TestOnlyAnEditorArranges(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	a := r.addText(t, editor, o, "a")
	if _, err := r.service.Arrange(reading(t.Context()), commenter, mustUUID(t, o.ID), layoutOf(a), pgtype.UUID{}); !errors.Is(err, folders.ErrForbidden) {
		t.Errorf("commenter: %v", err)
	}
	if _, err := r.service.Arrange(reading(t.Context()), viewer, mustUUID(t, o.ID), layoutOf(a), pgtype.UUID{}); !errors.Is(err, folders.ErrForbidden) {
		t.Errorf("viewer: %v", err)
	}
	if _, err := r.service.Arrange(reading(t.Context()), editor, mustUUID(t, o.ID), []observations.BlockLayout{{ID: uuidOf(a.ID), Height: observations.HeightChange{Set: true, Pixels: new(5)}}}, pgtype.UUID{}); err == nil {
		t.Error("a height of 5 pixels was accepted")
	}
}

func TestAnEditorArrangesRequestedBlocksToo(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	a := r.addText(t, editor, o, "a")
	asked := r.addText(t, commenter, o, "asked")
	if _, err := r.service.Arrange(reading(t.Context()), otherEditor, mustUUID(t, o.ID), layoutOf(asked, a), pgtype.UUID{}); err != nil {
		t.Fatal(err)
	}
	if order := r.positions(t, editor, o); !slices.Equal(order, []string{"asked", "a"}) {
		t.Fatalf("editor sees %v", order)
	}
	if order := r.positions(t, commenter, o); !slices.Equal(order, []string{"asked", "a"}) {
		t.Fatalf("its author sees %v", order)
	}
}

func TestALayoutWriteDoesNotMakeAnOpenContentEditStaleButAContentWriteDoes(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	a, b := r.addText(t, editor, o, "a"), r.addText(t, editor, o, "b")
	half := observations.WidthHalf
	laidOut := layoutOf(b, a)
	laidOut[1].Width = &half
	if _, err := r.service.Arrange(reading(t.Context()), otherEditor, mustUUID(t, o.ID), laidOut, pgtype.UUID{}); err != nil {
		t.Fatal(err)
	}
	edited, err := r.service.UpdateBlock(reading(t.Context()), editor, mustUUID(t, a.ID), observations.BlockChange{Version: a.Version, Body: textBody("a, edited")})
	if err != nil {
		t.Fatalf("a content edit after a layout write: %v", err)
	}
	if edited.Version != a.Version+1 || edited.Width != observations.WidthHalf {
		t.Errorf("edited block %+v kept the layout?", edited)
	}
	var conflict *observations.ConflictError
	if _, err := r.service.UpdateBlock(reading(t.Context()), otherEditor, mustUUID(t, a.ID), observations.BlockChange{Version: a.Version, Body: textBody("late")}); !errors.As(err, &conflict) {
		t.Fatalf("a second content edit on the old version: %v", err)
	}
	again, err := r.service.Arrange(reading(t.Context()), otherEditor, mustUUID(t, o.ID), layoutOf(a, b), pgtype.UUID{})
	if err != nil {
		t.Fatal(err)
	}
	for _, block := range again {
		if block.ID == a.ID && block.Version != edited.Version {
			t.Errorf("a layout write changed the content version to %d", block.Version)
		}
	}
}

func TestHeightsAreValidatedWhenABlockIsAddedAndEdited(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	id := mustUUID(t, o.ID)
	tall := 400
	b, err := r.service.AddBlock(reading(t.Context()), editor, id, observations.BlockInput{Kind: observations.KindPanel, Body: panelBody(), Width: observations.WidthTwoThirds, Height: &tall})
	if err != nil || b.Height == nil || *b.Height != 400 || b.Width != observations.WidthTwoThirds {
		t.Fatalf("added %+v, %v", b, err)
	}
	low := 10
	if _, err := r.service.AddBlock(reading(t.Context()), editor, id, observations.BlockInput{Kind: observations.KindPanel, Body: panelBody(), Height: &low}); err == nil {
		t.Error("a height of 10 was accepted on add")
	}
	cleared, err := r.service.UpdateBlock(reading(t.Context()), editor, mustUUID(t, b.ID), observations.BlockChange{Version: b.Version, Height: observations.HeightChange{Set: true}})
	if err != nil || cleared.Height != nil {
		t.Fatalf("cleared %+v, %v", cleared, err)
	}
}

func TestATextBlockTakesNoHeightOnAddUpdateOrArrange(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	id := mustUUID(t, o.ID)
	text := r.addText(t, editor, o, "a")
	var refused *observations.InvalidInputError
	if _, err := r.service.AddBlock(reading(t.Context()), editor, id, observations.BlockInput{Kind: observations.KindText, Body: textBody("b"), Height: new(300)}); !errors.As(err, &refused) || refused.Code != observations.CodeInvalidBlock || !strings.Contains(refused.Message, "panel and snapshot") {
		t.Errorf("add: %v", err)
	}
	if _, err := r.service.UpdateBlock(reading(t.Context()), editor, uuidOf(text.ID), observations.BlockChange{Version: text.Version, Height: observations.HeightChange{Set: true, Pixels: new(300)}}); !errors.As(err, &refused) || refused.Code != observations.CodeInvalidBlock {
		t.Errorf("update: %v", err)
	}
	sized := []observations.BlockLayout{{ID: uuidOf(text.ID), Height: observations.HeightChange{Set: true, Pixels: new(300)}}}
	if _, err := r.service.Arrange(reading(t.Context()), editor, id, sized, pgtype.UUID{}); !errors.As(err, &refused) || !strings.Contains(refused.Message, text.ID) {
		t.Errorf("arrange: %v", err)
	}
	unset := []observations.BlockLayout{{ID: uuidOf(text.ID), Height: observations.HeightChange{Set: true}}}
	if got, err := r.service.Arrange(reading(t.Context()), editor, id, unset, pgtype.UUID{}); err != nil || got[0].Height != nil {
		t.Errorf("a null height on a text block: %+v, %v", got, err)
	}
}

func TestALayoutChangeIsRecordedAsAnArrangementNamingWhatChanged(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	a := r.addPanel(t, editor, o)
	b := r.addText(t, editor, o, "b")
	arrange := func(items []observations.BlockLayout) {
		t.Helper()
		if _, err := r.service.Arrange(reading(t.Context()), editor, mustUUID(t, o.ID), items, pgtype.UUID{}); err != nil {
			t.Fatal(err)
		}
	}
	arrange(layoutOf(a, b))
	arrange(layoutOf(b, a))
	half := observations.WidthHalf
	sized := layoutOf(b, a)
	sized[1].Width = &half
	sized[1].Height = observations.HeightChange{Set: true, Pixels: new(240)}
	arrange(sized)
	reordered := layoutOf(a, b)
	reordered[0].Width = &half
	arrange(reordered)

	got := r.db.AuditEvents(t, "observation.arrange")
	var changed [][]any
	for _, e := range got {
		if e.Outcome != "success" || e.TargetID != o.ID || e.Data["title"] != "Checkout 5xx" {
			t.Errorf("event = %+v, want a success on %s titled Checkout 5xx", e, o.ID)
		}
		list, _ := e.Data["changed"].([]any)
		changed = append(changed, list)
	}
	want := [][]any{{"order"}, {"height", "width"}, {"order"}}
	if !reflect.DeepEqual(changed, want) {
		t.Fatalf("changed = %v, want %v (the unchanged arrangement records no event of its own)", changed, want)
	}
	if n := len(r.db.AuditEvents(t, "observation.update")); n != 0 {
		t.Fatalf("%d observation.update events, want none: an arrangement is not an update", n)
	}
}
