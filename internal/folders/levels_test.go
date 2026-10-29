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
		{"unrestricted folder", Facts{}, want{LevelView, LevelView, LevelEdit, LevelAdmin}},
		{"restricted with no matching row", Facts{Restricted: true}, want{LevelNone, LevelNone, LevelNone, LevelAdmin}},
		{"restricted with a view row", Facts{Restricted: true, Grants: []Level{LevelView}}, want{LevelView, LevelView, LevelEdit, LevelAdmin}},
		{"restricted with an edit row", Facts{Restricted: true, Grants: []Level{LevelEdit}}, want{LevelEdit, LevelEdit, LevelEdit, LevelAdmin}},
		{"restricted with an admin row", Facts{Restricted: true, Grants: []Level{LevelAdmin}}, want{LevelEdit, LevelEdit, LevelAdmin, LevelAdmin}},
		{"several rows, the highest wins", Facts{Restricted: true, Grants: []Level{LevelView, LevelAdmin, LevelEdit}}, want{LevelEdit, LevelEdit, LevelAdmin, LevelAdmin}},
		{"several rows without admin", Facts{Restricted: true, Grants: []Level{LevelView, LevelEdit}}, want{LevelEdit, LevelEdit, LevelEdit, LevelAdmin}},
		{"creator of a restricted folder with no row", Facts{Restricted: true, Creator: true}, want{LevelEdit, LevelEdit, LevelAdmin, LevelAdmin}},
		{"creator of an unrestricted folder", Facts{Creator: true}, want{LevelEdit, LevelEdit, LevelAdmin, LevelAdmin}},
		{"creator with a view row", Facts{Restricted: true, Creator: true, Grants: []Level{LevelView}}, want{LevelEdit, LevelEdit, LevelAdmin, LevelAdmin}},
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
	none := Abilities{}
	view := Abilities{View: true}
	viewComment := Abilities{View: true, Comment: true}
	edit := Abilities{View: true, Comment: true, Edit: true}
	editCreate := Abilities{View: true, Comment: true, Edit: true, Create: true}
	everything := Abilities{View: true, Comment: true, Edit: true, Manage: true, Create: true}
	cases := []struct {
		role  auth.Role
		level Level
		want  Abilities
	}{
		{auth.RoleViewer, LevelNone, none},
		{auth.RoleViewer, LevelView, view},
		{auth.RoleViewer, LevelEdit, edit},
		{auth.RoleViewer, LevelAdmin, Abilities{View: true, Comment: true, Edit: true, Manage: true}},
		{auth.RoleCommenter, LevelNone, none},
		{auth.RoleCommenter, LevelView, viewComment},
		{auth.RoleCommenter, LevelEdit, edit},
		{auth.RoleCommenter, LevelAdmin, Abilities{View: true, Comment: true, Edit: true, Manage: true}},
		{auth.RoleEditor, LevelNone, none},
		{auth.RoleEditor, LevelView, viewComment},
		{auth.RoleEditor, LevelEdit, editCreate},
		{auth.RoleEditor, LevelAdmin, everything},
		{auth.RoleAdmin, LevelNone, none},
		{auth.RoleAdmin, LevelView, viewComment},
		{auth.RoleAdmin, LevelEdit, editCreate},
		{auth.RoleAdmin, LevelAdmin, everything},
	}
	for _, tc := range cases {
		t.Run(string(tc.role)+" at "+tc.level.String(), func(t *testing.T) {
			if got := AbilitiesFor(tc.role, tc.level); got != tc.want {
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
			"Viewer with an Edit row can Edit but not Create or Manage",
			auth.RoleViewer, Facts{Restricted: true, Grants: []Level{LevelEdit}},
			LevelEdit, Abilities{View: true, Comment: true, Edit: true},
		},
		{
			"Commenter with an Admin row gets Edit",
			auth.RoleCommenter, Facts{Restricted: true, Grants: []Level{LevelAdmin}},
			LevelEdit, Abilities{View: true, Comment: true, Edit: true},
		},
		{
			"Editor with a View row on a restricted folder gets Edit",
			auth.RoleEditor, Facts{Restricted: true, Grants: []Level{LevelView}},
			LevelEdit, Abilities{View: true, Comment: true, Edit: true, Create: true},
		},
		{
			"Admin with no row on a restricted folder gets Manage",
			auth.RoleAdmin, Facts{Restricted: true},
			LevelAdmin, Abilities{View: true, Comment: true, Edit: true, Manage: true, Create: true},
		},
		{
			"Viewer with a View row can View but not Comment",
			auth.RoleViewer, Facts{Restricted: true, Grants: []Level{LevelView}},
			LevelView, Abilities{View: true},
		},
		{
			"Commenter with a View row can Comment",
			auth.RoleCommenter, Facts{Restricted: true, Grants: []Level{LevelView}},
			LevelView, Abilities{View: true, Comment: true},
		},
		{
			"Viewer on a restricted folder with no row sees nothing",
			auth.RoleViewer, Facts{Restricted: true},
			LevelNone, Abilities{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			level := LevelFor(tc.role, tc.facts)
			if level != tc.wantLevel {
				t.Fatalf("level = %s, want %s", level, tc.wantLevel)
			}
			if got := AbilitiesFor(tc.role, level); got != tc.want {
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
