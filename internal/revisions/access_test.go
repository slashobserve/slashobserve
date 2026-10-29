package revisions

import (
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/folders"
)

func TestRunAllowed(t *testing.T) {
	roles := []auth.Role{auth.RoleViewer, auth.RoleCommenter, auth.RoleEditor, auth.RoleAdmin}
	withEdit := folders.Abilities{View: true, Comment: true, Edit: true}
	withoutEdit := folders.Abilities{View: true, Comment: true}
	always := func(auth.Role) bool { return true }
	never := func(auth.Role) bool { return false }
	editorOrAbove := func(r auth.Role) bool { return r.AtLeast(auth.RoleEditor) }

	cases := []struct {
		name         string
		source       Source
		hasDashboard bool
		abilities    folders.Abilities
		want         func(role auth.Role) bool
	}{
		{"Viewer with an Edit grant may send to the agent", SourcePrompt, true, withEdit, always},
		{"Commenter without a grant may not", SourcePrompt, true, withoutEdit, never},
		{"comment request needs Edit", SourceComment, true, withoutEdit, never},
		{"comment request with Edit", SourceComment, true, withEdit, always},
		{"suggest needs Edit", SourceSuggest, true, withoutEdit, never},
		{"suggest with Edit", SourceSuggest, true, withEdit, always},
		{"observe follow-up on a dashboard needs Edit", SourceObserve, true, withoutEdit, never},
		{"observe follow-up on a dashboard with Edit", SourceObserve, true, withEdit, always},
		{"observe without a dashboard needs Editor", SourceObserve, false, folders.Abilities{}, editorOrAbove},
		{"abilities do not help an observe without a dashboard", SourceObserve, false, withEdit, editorOrAbove},
		{"no other source runs without a dashboard", SourcePrompt, false, withEdit, never},
	}
	for _, tc := range cases {
		for _, role := range roles {
			t.Run(tc.name+"/"+string(role), func(t *testing.T) {
				if got := runAllowed(tc.source, tc.hasDashboard, role, tc.abilities); got != tc.want(role) {
					t.Fatalf("runAllowed(%s, dashboard=%v, %s, %+v) = %v, want %v", tc.source, tc.hasDashboard, role, tc.abilities, got, tc.want(role))
				}
			})
		}
	}
}
