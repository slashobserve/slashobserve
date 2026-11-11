package folders

import (
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
)

func TestUnfiledGivesAnAdminAdminTheCreatorEditAndNobodyElseNothing(t *testing.T) {
	tests := []struct {
		name    string
		role    auth.Role
		creator bool
		want    Abilities
	}{
		{"an admin who did not make it", auth.RoleAdmin, false, AbilitiesFor(LevelAdmin)},
		{"an admin who made it", auth.RoleAdmin, true, AbilitiesFor(LevelAdmin)},
		{"the creator", auth.RoleEditor, true, AbilitiesFor(LevelEdit)},
		{"a viewer who made it", auth.RoleViewer, true, AbilitiesFor(LevelEdit)},
		{"someone else", auth.RoleEditor, false, Abilities{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Unfiled(auth.Principal{Role: tt.role}, tt.creator); got != tt.want {
				t.Errorf("Unfiled = %+v, want %+v", got, tt.want)
			}
		})
	}
}
