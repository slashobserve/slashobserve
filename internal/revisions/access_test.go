package revisions

import (
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/folders"
)

func TestRunAllowed(t *testing.T) {
	withEdit := folders.Abilities{View: true, Comment: true, Edit: true}
	withoutEdit := folders.Abilities{View: true, Comment: true}

	cases := []struct {
		name         string
		source       Source
		hasDashboard bool
		canCreate    bool
		role         auth.Role
		abilities    folders.Abilities
		want         bool
	}{
		{"prompt on a dashboard with Edit", SourcePrompt, true, false, auth.RoleEditor, withEdit, true},
		{"prompt on a dashboard with Comment only", SourcePrompt, true, true, auth.RoleEditor, withoutEdit, true},
		{"prompt on a dashboard with View only", SourcePrompt, true, true, auth.RoleEditor, folders.Abilities{View: true}, false},
		{"comment request with Edit", SourceComment, true, false, auth.RoleEditor, withEdit, true},
		{"comment request with Comment only", SourceComment, true, true, auth.RoleEditor, withoutEdit, true},
		{"comment request with View only", SourceComment, true, true, auth.RoleEditor, folders.Abilities{View: true}, false},
		{"suggest with Edit", SourceSuggest, true, false, auth.RoleEditor, withEdit, true},
		{"suggest without Edit", SourceSuggest, true, true, auth.RoleEditor, withoutEdit, false},
		{"observe follow-up with Edit", SourceObserve, true, false, auth.RoleEditor, withEdit, true},
		{"observe follow-up with Comment only", SourceObserve, true, true, auth.RoleEditor, withoutEdit, true},
		{"observe follow-up with View only", SourceObserve, true, true, auth.RoleEditor, folders.Abilities{View: true}, false},
		{"observe without a dashboard when the requester can create", SourceObserve, false, true, auth.RoleEditor, folders.Abilities{}, true},
		{"observe without a dashboard when the requester cannot create", SourceObserve, false, false, auth.RoleEditor, folders.Abilities{}, false},
		{"Edit abilities do not help an observe without a dashboard", SourceObserve, false, false, auth.RoleEditor, withEdit, false},
		{"prompt without a dashboard even when the requester can create", SourcePrompt, false, true, auth.RoleEditor, folders.Abilities{}, false},
		{"investigate by a Viewer", SourceInvestigate, false, true, auth.RoleViewer, folders.Abilities{}, false},
		{"investigate by a Viewer of the dashboard", SourceInvestigate, true, false, auth.RoleViewer, withoutEdit, false},
		{"investigate without a dashboard by an Editor", SourceInvestigate, false, false, auth.RoleEditor, folders.Abilities{}, true},
		{"investigate by an Editor who can view the dashboard", SourceInvestigate, true, false, auth.RoleEditor, folders.Abilities{View: true}, true},
		{"investigate by an Editor with no access to the dashboard", SourceInvestigate, true, false, auth.RoleEditor, folders.Abilities{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := runAllowed(tc.source, tc.hasDashboard, tc.canCreate, tc.role, tc.abilities); got != tc.want {
				t.Fatalf("runAllowed(%s, dashboard=%v, canCreate=%v, %+v) = %v, want %v", tc.source, tc.hasDashboard, tc.canCreate, tc.abilities, got, tc.want)
			}
		})
	}
}
