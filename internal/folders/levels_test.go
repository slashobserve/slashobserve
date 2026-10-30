package folders

import (
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
)

var allRoles = []auth.Role{auth.RoleViewer, auth.RoleCommenter, auth.RoleEditor, auth.RoleAdmin}

func TestLevelFor(t *testing.T) {
	type want struct{ viewer, commenter, editor, admin Level }
	cases := []struct {
		name  string
		facts Facts
		want  want
	}{
		{"unrestricted folder", Facts{}, want{LevelView, LevelComment, LevelEdit, LevelAdmin}},
		{"restricted with no matching row", Facts{Restricted: true}, want{LevelNone, LevelNone, LevelNone, LevelAdmin}},
		{"restricted with a view row", Facts{Restricted: true, Grants: []Level{LevelView}}, want{LevelView, LevelComment, LevelEdit, LevelAdmin}},
		{"restricted with a comment row", Facts{Restricted: true, Grants: []Level{LevelComment}}, want{LevelView, LevelComment, LevelEdit, LevelAdmin}},
		{"restricted with an edit row", Facts{Restricted: true, Grants: []Level{LevelEdit}}, want{LevelView, LevelComment, LevelEdit, LevelAdmin}},
		{"restricted with an admin row", Facts{Restricted: true, Grants: []Level{LevelAdmin}}, want{LevelView, LevelComment, LevelAdmin, LevelAdmin}},
		{"several rows, the highest wins", Facts{Restricted: true, Grants: []Level{LevelView, LevelAdmin, LevelEdit}}, want{LevelView, LevelComment, LevelAdmin, LevelAdmin}},
		{"several rows without admin", Facts{Restricted: true, Grants: []Level{LevelView, LevelEdit}}, want{LevelView, LevelComment, LevelEdit, LevelAdmin}},
		{"creator of a restricted folder with no row", Facts{Restricted: true, Creator: true}, want{LevelView, LevelComment, LevelAdmin, LevelAdmin}},
		{"creator of an unrestricted folder", Facts{Creator: true}, want{LevelView, LevelComment, LevelAdmin, LevelAdmin}},
		{"creator with a view row", Facts{Restricted: true, Creator: true, Grants: []Level{LevelView}}, want{LevelView, LevelComment, LevelAdmin, LevelAdmin}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expected := map[auth.Role]Level{
				auth.RoleViewer:    tc.want.viewer,
				auth.RoleCommenter: tc.want.commenter,
				auth.RoleEditor:    tc.want.editor,
				auth.RoleAdmin:     tc.want.admin,
			}
			for _, role := range allRoles {
				if got := LevelFor(role, tc.facts); got != expected[role] {
					t.Errorf("%s: level = %s, want %s", role, got, expected[role])
				}
			}
		})
	}
}

func TestLevelForRefusesAnUnknownRole(t *testing.T) {
	for _, role := range []auth.Role{"", "owner"} {
		if got := LevelFor(role, Facts{Creator: true}); got != LevelNone {
			t.Errorf("role %q: level = %s, want none", role, got)
		}
	}
}

func TestAbilitiesFor(t *testing.T) {
	cases := []struct {
		level Level
		want  Abilities
	}{
		{LevelNone, Abilities{}},
		{LevelView, Abilities{View: true}},
		{LevelComment, Abilities{View: true, Comment: true}},
		{LevelEdit, Abilities{View: true, Comment: true, Edit: true, Create: true}},
		{LevelAdmin, Abilities{View: true, Comment: true, Edit: true, Manage: true, Create: true}},
	}
	for _, tc := range cases {
		t.Run(tc.level.String(), func(t *testing.T) {
			if got := AbilitiesFor(tc.level); got != tc.want {
				t.Errorf("abilities = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestLevelForNamedCases(t *testing.T) {
	cases := []struct {
		name      string
		role      auth.Role
		facts     Facts
		wantLevel Level
		want      Abilities
	}{
		{
			"Viewer given Edit still only views",
			auth.RoleViewer, Facts{Restricted: true, Grants: []Level{LevelEdit}},
			LevelView, Abilities{View: true},
		},
		{
			"Commenter given Edit only comments",
			auth.RoleCommenter, Facts{Restricted: true, Grants: []Level{LevelEdit}},
			LevelComment, Abilities{View: true, Comment: true},
		},
		{
			"Commenter given Admin only comments",
			auth.RoleCommenter, Facts{Restricted: true, Grants: []Level{LevelAdmin}},
			LevelComment, Abilities{View: true, Comment: true},
		},
		{
			"Editor given View on a restricted folder edits",
			auth.RoleEditor, Facts{Restricted: true, Grants: []Level{LevelView}},
			LevelEdit, Abilities{View: true, Comment: true, Edit: true, Create: true},
		},
		{
			"Editor given Admin manages",
			auth.RoleEditor, Facts{Restricted: true, Grants: []Level{LevelAdmin}},
			LevelAdmin, Abilities{View: true, Comment: true, Edit: true, Manage: true, Create: true},
		},
		{
			"Viewer on a restricted folder with no row sees nothing",
			auth.RoleViewer, Facts{Restricted: true},
			LevelNone, Abilities{},
		},
		{
			"Admin with no row on a restricted folder manages",
			auth.RoleAdmin, Facts{Restricted: true},
			LevelAdmin, Abilities{View: true, Comment: true, Edit: true, Manage: true, Create: true},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			level := LevelFor(tc.role, tc.facts)
			if level != tc.wantLevel {
				t.Fatalf("level = %s, want %s", level, tc.wantLevel)
			}
			if got := AbilitiesFor(level); got != tc.want {
				t.Errorf("abilities = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestAbilitiesHas(t *testing.T) {
	a := Abilities{View: true, Edit: true}
	cases := []struct {
		need Need
		want bool
	}{
		{NeedView, true},
		{NeedComment, false},
		{NeedEdit, true},
		{NeedManage, false},
		{NeedCreate, false},
		{Need(99), false},
	}
	for _, tc := range cases {
		if got := a.Has(tc.need); got != tc.want {
			t.Errorf("Has(%d) = %v, want %v", tc.need, got, tc.want)
		}
	}
}

func TestParseLevel(t *testing.T) {
	cases := []struct {
		in   string
		want Level
		ok   bool
	}{
		{"view", LevelView, true},
		{"comment", LevelComment, true},
		{"edit", LevelEdit, true},
		{"admin", LevelAdmin, true},
		{"none", LevelNone, false},
		{"", LevelNone, false},
		{"View", LevelNone, false},
		{"owner", LevelNone, false},
	}
	for _, tc := range cases {
		got, ok := ParseLevel(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("ParseLevel(%q) = %s, %v; want %s, %v", tc.in, got, ok, tc.want, tc.ok)
		}
		if ok && got.String() != tc.in {
			t.Errorf("String() of ParseLevel(%q) = %q", tc.in, got.String())
		}
	}
}
