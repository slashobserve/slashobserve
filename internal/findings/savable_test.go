package findings

import (
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
)

func TestOnlyTheAskerMaySaveAnAnswerNotSavedYet(t *testing.T) {
	asker := auth.Principal{Kind: auth.KindUser, ID: "11111111-1111-4111-8111-111111111111", Role: auth.RoleEditor}
	other := auth.Principal{Kind: auth.KindUser, ID: "22222222-2222-4222-8222-222222222222", Role: auth.RoleAdmin}
	unsaved := Finding{RequestedBy: Author{Kind: string(auth.KindUser), ID: asker.ID}}
	saved := unsaved
	saved.ObservationID = "33333333-3333-4333-8333-333333333333"
	hidden := unsaved
	hidden.observationHidden = true

	for _, tt := range []struct {
		name string
		f    Finding
		by   auth.Principal
		want bool
	}{
		{"the asker, unsaved", unsaved, asker, true},
		{"someone else, unsaved", unsaved, other, false},
		{"the asker, saved", saved, asker, false},
		{"the asker, saved where they cannot read", hidden, asker, false},
	} {
		if got := tt.f.SavableBy(tt.by); got != tt.want {
			t.Errorf("%s: SavableBy = %v, want %v", tt.name, got, tt.want)
		}
	}
}
