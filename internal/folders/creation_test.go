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
	general, checkout, payments, viewOnly := uuidOf(1), uuidOf(2), uuidOf(3), uuidOf(4)
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
	cases := []struct {
		name      string
		levels    []folderLevel
		lastUsed  pgtype.UUID
		requested pgtype.UUID
		want      pgtype.UUID
		wantErr   error
	}{
		{"requested folder where the caller can create", levels(can), pgtype.UUID{}, payments, payments, nil},
		{"requested folder that is visible but not creatable", levels(can), pgtype.UUID{}, viewOnly, pgtype.UUID{}, ErrForbidden},
		{"requested folder that is invisible", levels(can), pgtype.UUID{}, missing, pgtype.UUID{}, ErrNotFound},
		{"requested folder wins over the last used one", levels(can), checkout, payments, payments, nil},
		{"last used folder where the caller can create", levels(can), payments, pgtype.UUID{}, payments, nil},
		{"last used folder the caller cannot create in falls to General", levels(can), viewOnly, pgtype.UUID{}, general, nil},
		{"last used folder that is no longer visible falls to General", levels(can), missing, pgtype.UUID{}, general, nil},
		{"no last used folder uses General", levels(can), pgtype.UUID{}, pgtype.UUID{}, general, nil},
		{"General not creatable falls to the first creatable folder", levels(cannot), pgtype.UUID{}, pgtype.UUID{}, checkout, nil},
		{"nothing creatable", []folderLevel{
			{Folder: Folder{ID: general, General: true}, Abilities: cannot},
			{Folder: Folder{ID: viewOnly}, Abilities: cannot},
		}, pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, ErrForbidden},
		{"no visible folders", nil, pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, ErrForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := pickCreationFolder(tc.levels, tc.lastUsed, tc.requested)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("folder = %v, want %v", got, tc.want)
			}
		})
	}
}
