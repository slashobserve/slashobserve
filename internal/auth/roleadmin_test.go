package auth

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestLeavesNoAdmin(t *testing.T) {
	id := func(b byte) pgtype.UUID { return pgtype.UUID{Bytes: [16]byte{b}, Valid: true} }
	tests := []struct {
		name   string
		admins []pgtype.UUID
		target pgtype.UUID
		next   Role
		want   bool
	}{
		{"the only admin becomes an editor", []pgtype.UUID{id(1)}, id(1), RoleEditor, true},
		{"the only admin becomes a viewer", []pgtype.UUID{id(1)}, id(1), RoleViewer, true},
		{"the only admin stays an admin", []pgtype.UUID{id(1)}, id(1), RoleAdmin, false},
		{"one of two admins is lowered", []pgtype.UUID{id(1), id(2)}, id(1), RoleEditor, false},
		{"a non-admin is lowered while one admin remains", []pgtype.UUID{id(1)}, id(2), RoleViewer, false},
		{"a non-admin is raised", []pgtype.UUID{id(1)}, id(2), RoleAdmin, false},
		{"no admins exist", nil, id(2), RoleViewer, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := leavesNoAdmin(tt.admins, tt.target, tt.next); got != tt.want {
				t.Fatalf("leavesNoAdmin = %v, want %v", got, tt.want)
			}
		})
	}
}
