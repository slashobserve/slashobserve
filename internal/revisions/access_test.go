package revisions

import (
	"testing"

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
		abilities    folders.Abilities
		want         bool
	}{
		{"prompt on a dashboard with Edit", SourcePrompt, true, false, withEdit, true},
		{"prompt on a dashboard without Edit", SourcePrompt, true, true, withoutEdit, false},
		{"comment request with Edit", SourceComment, true, false, withEdit, true},
		{"comment request without Edit", SourceComment, true, true, withoutEdit, false},
		{"suggest with Edit", SourceSuggest, true, false, withEdit, true},
		{"suggest without Edit", SourceSuggest, true, true, withoutEdit, false},
		{"observe follow-up with Edit", SourceObserve, true, false, withEdit, true},
		{"observe follow-up without Edit", SourceObserve, true, true, withoutEdit, false},
		{"observe without a dashboard when the requester can create", SourceObserve, false, true, folders.Abilities{}, true},
		{"observe without a dashboard when the requester cannot create", SourceObserve, false, false, folders.Abilities{}, false},
		{"Edit abilities do not help an observe without a dashboard", SourceObserve, false, false, withEdit, false},
		{"prompt without a dashboard even when the requester can create", SourcePrompt, false, true, folders.Abilities{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := runAllowed(tc.source, tc.hasDashboard, tc.canCreate, tc.abilities); got != tc.want {
				t.Fatalf("runAllowed(%s, dashboard=%v, canCreate=%v, %+v) = %v, want %v", tc.source, tc.hasDashboard, tc.canCreate, tc.abilities, got, tc.want)
			}
		})
	}
}
