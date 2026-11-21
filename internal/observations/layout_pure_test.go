package observations

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
)

func TestArrangedOrderPlacesTheNamedBlocksThenKeepsTheRestAfterTheirPredecessor(t *testing.T) {
	stored := []string{"a", "b", "c", "d", "e"}
	cases := []struct {
		name      string
		requested []string
		want      []string
	}{
		{"nothing asked", nil, stored},
		{"a full reorder", []string{"e", "d", "c", "b", "a"}, []string{"e", "d", "c", "b", "a"}},
		{"a partial move keeps the rest behind their predecessor", []string{"c", "a"}, []string{"c", "d", "e", "a", "b"}},
		{"a block named alone stays behind the ones before it", []string{"e"}, stored},
		{"an unknown id is dropped", []string{"x", "b"}, stored},
		{"a repeated id counts once", []string{"b", "b", "a"}, []string{"b", "c", "d", "e", "a"}},
	}
	for _, tc := range cases {
		if got := arrangedOrder(stored, tc.requested); !slices.Equal(got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestHeightChangeTellsNullFromAbsentFromANumber(t *testing.T) {
	var body struct {
		Height HeightChange `json:"height"`
	}
	for raw, want := range map[string]struct {
		set    bool
		pixels *int
	}{
		`{}`:                {false, nil},
		`{"height":null}`:   {true, nil},
		`{"height":360}`:    {true, new(360)},
		`{"height":"tall"}`: {true, nil},
	} {
		body.Height = HeightChange{}
		err := json.Unmarshal([]byte(raw), &body)
		if raw == `{"height":"tall"}` {
			if err == nil {
				t.Errorf("%s: want a decode error", raw)
			}
			continue
		}
		if err != nil || body.Height.Set != want.set || !equalHeights(body.Height.Pixels, want.pixels) {
			t.Errorf("%s: got %+v, %v", raw, body.Height, err)
		}
	}
}

func TestValidateLayoutRefusesTooManyRepeatsAndOutOfRangeValues(t *testing.T) {
	id := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	other := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	third, bogus := WidthThird, Width("quarter")
	cases := []struct {
		name  string
		items []BlockLayout
		ok    bool
	}{
		{"empty", nil, true},
		{"widths and heights", []BlockLayout{{ID: id, Width: &third, Height: HeightChange{Set: true, Pixels: new(120)}}, {ID: other, Height: HeightChange{Set: true, Pixels: new(960)}}}, true},
		{"automatic height", []BlockLayout{{ID: id, Height: HeightChange{Set: true}}}, true},
		{"a repeated block", []BlockLayout{{ID: id}, {ID: id}}, false},
		{"a width that is not one", []BlockLayout{{ID: id, Width: &bogus}}, false},
		{"a height under the floor", []BlockLayout{{ID: id, Height: HeightChange{Set: true, Pixels: new(119)}}}, false},
		{"a height over the ceiling", []BlockLayout{{ID: id, Height: HeightChange{Set: true, Pixels: new(961)}}}, false},
	}
	for _, tc := range cases {
		err := validateLayout(tc.items)
		if tc.ok && err != nil {
			t.Errorf("%s: %v", tc.name, err)
		}
		var invalidErr *InvalidInputError
		if !tc.ok && !errors.As(err, &invalidErr) {
			t.Errorf("%s: got %v, want an invalid error", tc.name, err)
		}
	}
}

func TestALayoutRefusesABlockThatIsNotOnThePageOrIsNotVisibleToTheCaller(t *testing.T) {
	asked := block(2, KindText, BlockRequested, 1, WidthFull, `{}`)
	asked.CreatedByKind, asked.CreatedByID = "user", uuidOf(9)
	rows := []dbsqlc.ObservationBlock{block(1, KindText, BlockLive, 0, WidthFull, `{}`), asked}
	caller := actor.Actor{Kind: auth.KindUser, ID: uuidOf(7).String()}
	cases := []struct {
		name      string
		items     []BlockLayout
		abilities folders.Abilities
		unknown   pgtype.UUID
	}{
		{"every block is on the page", []BlockLayout{{ID: uuidOf(1)}}, folders.Abilities{}, pgtype.UUID{}},
		{"a block of another observation", []BlockLayout{{ID: uuidOf(1)}, {ID: uuidOf(5)}}, folders.Abilities{}, uuidOf(5)},
		{"a request someone else made, seen by a caller who cannot edit", []BlockLayout{{ID: uuidOf(2)}}, folders.Abilities{}, uuidOf(2)},
		{"a request someone else made, seen by an editor", []BlockLayout{{ID: uuidOf(2)}}, folders.Abilities{Edit: true}, pgtype.UUID{}},
	}
	for _, tc := range cases {
		err := unknownLayoutBlock(rows, tc.items, tc.abilities, caller)
		switch {
		case !tc.unknown.Valid && err != nil:
			t.Errorf("%s: refused: %v", tc.name, err)
		case tc.unknown.Valid && (err == nil || err.Code != CodeInvalidBlock || !strings.Contains(err.Message, tc.unknown.String())):
			t.Errorf("%s: got %v, want a refusal naming %s", tc.name, err, tc.unknown)
		}
	}
}
