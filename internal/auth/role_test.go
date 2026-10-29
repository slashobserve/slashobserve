package auth

import "testing"

func TestRoleParseAcceptsOnlyTheFourRoles(t *testing.T) {
	tests := []struct {
		in   string
		want Role
		ok   bool
	}{
		{"admin", RoleAdmin, true},
		{"editor", RoleEditor, true},
		{"commenter", RoleCommenter, true},
		{"viewer", RoleViewer, true},
		{"", "", false},
		{"owner", "", false},
		{"Admin", "", false},
		{" admin", "", false},
	}
	for _, tt := range tests {
		got, ok := ParseRole(tt.in)
		if ok != tt.ok || (ok && got != tt.want) {
			t.Errorf("ParseRole(%q) = %q, %v; want %q, %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestRoleAtLeastFollowsTheRankOrder(t *testing.T) {
	order := []Role{RoleViewer, RoleCommenter, RoleEditor, RoleAdmin}
	for i, have := range order {
		for j, min := range order {
			if got, want := have.AtLeast(min), i >= j; got != want {
				t.Errorf("%s.AtLeast(%s) = %v, want %v", have, min, got, want)
			}
		}
	}
}

func TestRoleAtLeastRefusesAnUnknownRole(t *testing.T) {
	for _, have := range []Role{"", "owner"} {
		if have.AtLeast(RoleViewer) {
			t.Errorf("%q.AtLeast(viewer) = true, want false", have)
		}
	}
}

func TestRoleSourceLockedOnlyForSettingsDrivenSources(t *testing.T) {
	tests := map[RoleSource]bool{
		SourceAdminEmails: true,
		SourceFirstUser:   true,
		SourceGroup:       true,
		SourceManual:      false,
		SourceDefault:     false,
	}
	for source, want := range tests {
		if got := source.Locked(); got != want {
			t.Errorf("%s.Locked() = %v, want %v", source, got, want)
		}
	}
}
