package folders

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func uuidOf(n byte) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{15: n}, Valid: true}
}

func TestPickCreationFolder(t *testing.T) {
	general, checkout, payments, viewOnly, mine := uuidOf(1), uuidOf(2), uuidOf(3), uuidOf(4), uuidOf(5)
	missing := uuidOf(9)
	can := Abilities{View: true, Comment: true, Edit: true, Create: true}
	cannot := Abilities{View: true, Comment: true}
	levels := func(generalAbilities Abilities) []folderLevel {
		return []folderLevel{
			{Folder: Folder{ID: checkout, Title: "Checkout"}, Abilities: can},
			{Folder: Folder{ID: viewOnly, Title: "Reports"}, Abilities: cannot},
			{Folder: Folder{ID: payments, Title: "Payments"}, Abilities: can},
			{Folder: Folder{ID: general, Title: "General", General: true}, Abilities: generalAbilities},
		}
	}
	withPersonal := func(generalAbilities Abilities) []folderLevel {
		return append(levels(generalAbilities), folderLevel{Folder: Folder{ID: mine, Title: "My Dashboards", Personal: true}, Abilities: can})
	}
	none := pgtype.UUID{}
	cases := []struct {
		name      string
		levels    []folderLevel
		personal  pgtype.UUID
		lastUsed  pgtype.UUID
		requested pgtype.UUID
		want      pgtype.UUID
		wantErr   error
	}{
		{"requested folder where the caller can create", levels(can), none, pgtype.UUID{}, payments, payments, nil},
		{"requested folder that is visible but not creatable", levels(can), none, pgtype.UUID{}, viewOnly, pgtype.UUID{}, ErrForbidden},
		{"requested folder that is invisible", levels(can), none, pgtype.UUID{}, missing, pgtype.UUID{}, ErrNotFound},
		{"requested folder wins over the last used one", levels(can), none, checkout, payments, payments, nil},
		{"last used folder where the caller can create", levels(can), none, payments, pgtype.UUID{}, payments, nil},
		{"last used folder the caller cannot create in falls to General", levels(can), none, viewOnly, pgtype.UUID{}, general, nil},
		{"last used folder that is no longer visible falls to General", levels(can), none, missing, pgtype.UUID{}, general, nil},
		{"no last used folder uses General", levels(can), none, pgtype.UUID{}, pgtype.UUID{}, general, nil},
		{"General not creatable falls to the first creatable folder", levels(cannot), none, pgtype.UUID{}, pgtype.UUID{}, checkout, nil},
		{"nothing creatable", []folderLevel{
			{Folder: Folder{ID: general, General: true}, Abilities: cannot},
			{Folder: Folder{ID: viewOnly}, Abilities: cannot},
		}, none, none, none, none, ErrForbidden},
		{"General not creatable falls to the personal folder before another creatable folder", withPersonal(cannot), mine, none, none, mine, nil},
		{"General creatable wins over the personal folder", withPersonal(can), mine, none, none, general, nil},
		{"last used personal folder is picked", withPersonal(can), mine, mine, none, mine, nil},
		{"requested personal folder is honoured", withPersonal(can), mine, none, mine, mine, nil},
		{"requested personal folder wins over the last used one", withPersonal(can), mine, checkout, mine, mine, nil},
		{"a personal folder that is not visible is not picked", levels(cannot), mine, none, none, checkout, nil},
		{"no visible folders", nil, none, pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, ErrForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := pickCreationFolder(tc.levels, tc.personal, tc.lastUsed, tc.requested)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("folder = %v, want %v", got, tc.want)
			}
		})
	}
}
