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
		requested pgtype.UUID
		want      pgtype.UUID
		wantErr   error
	}{
		{"requested folder where the caller can create", levels(can), none, payments, payments, nil},
		{"requested folder that is visible but not creatable", levels(can), none, viewOnly, none, ErrForbidden},
		{"requested folder that is invisible", levels(can), none, missing, none, ErrNotFound},
		{"requested folder wins over the personal folder", withPersonal(can), mine, payments, payments, nil},
		{"requested personal folder is honoured", withPersonal(can), mine, mine, mine, nil},
		{"personal folder wins over a creatable General", withPersonal(can), mine, none, mine, nil},
		{"no personal folder falls to General", levels(can), none, none, general, nil},
		{"a personal folder that is not visible falls to General", levels(can), mine, none, general, nil},
		{"General not creatable and no personal folder is forbidden", levels(cannot), none, none, none, ErrForbidden},
		{"nothing creatable", []folderLevel{
			{Folder: Folder{ID: general, General: true}, Abilities: cannot},
			{Folder: Folder{ID: viewOnly}, Abilities: cannot},
		}, none, none, none, ErrForbidden},
		{"no visible folders", nil, none, none, none, ErrForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := pickCreationFolder(tc.levels, tc.personal, tc.requested)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("folder = %v, want %v", got, tc.want)
			}
		})
	}
}
