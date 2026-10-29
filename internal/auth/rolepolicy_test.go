package auth

import "testing"

func TestResolveRolePrecedence(t *testing.T) {
	viewer, commenter, admin := RoleViewer, RoleCommenter, RoleAdmin
	groupRoles := []GroupRole{
		{Group: "eng", Role: RoleEditor},
		{Group: "ops", Role: RoleAdmin},
		{Group: "support", Role: RoleCommenter},
		{Group: "guests", Role: RoleViewer},
		{Group: "other-editors", Role: RoleEditor},
	}
	tests := []struct {
		name   string
		in     RoleInputs
		policy RolePolicy
		want   RoleDecision
	}{
		{
			name:   "admin email wins",
			in:     RoleInputs{Email: "boss@example.com"},
			policy: RolePolicy{AdminEmails: []string{"boss@example.com"}},
			want:   RoleDecision{Role: RoleAdmin, Source: SourceAdminEmails},
		},
		{
			name:   "admin email match ignores case",
			in:     RoleInputs{Email: "Boss@Example.COM"},
			policy: RolePolicy{AdminEmails: []string{"boss@example.com"}},
			want:   RoleDecision{Role: RoleAdmin, Source: SourceAdminEmails},
		},
		{
			name:   "admin email beats a lower group",
			in:     RoleInputs{Email: "boss@example.com", Groups: []string{"guests"}},
			policy: RolePolicy{AdminEmails: []string{"boss@example.com"}, GroupRoles: groupRoles},
			want:   RoleDecision{Role: RoleAdmin, Source: SourceAdminEmails},
		},
		{
			name:   "admin email beats a manual role",
			in:     RoleInputs{Email: "boss@example.com", Manual: &viewer},
			policy: RolePolicy{AdminEmails: []string{"boss@example.com"}},
			want:   RoleDecision{Role: RoleAdmin, Source: SourceAdminEmails},
		},
		{
			name:   "admin email must match exactly",
			in:     RoleInputs{Email: "boss@example.com.evil.test"},
			policy: RolePolicy{AdminEmails: []string{"boss@example.com"}},
			want:   RoleDecision{Role: RoleEditor, Source: SourceDefault},
		},
		{
			name: "first user is admin when no admin emails are set",
			in:   RoleInputs{Email: "a@example.com", FirstUser: true},
			want: RoleDecision{Role: RoleAdmin, Source: SourceFirstUser},
		},
		{
			name:   "first user is not admin when admin emails are set",
			in:     RoleInputs{Email: "a@example.com", FirstUser: true},
			policy: RolePolicy{AdminEmails: []string{"boss@example.com"}},
			want:   RoleDecision{Role: RoleEditor, Source: SourceDefault},
		},
		{
			name:   "first user beats a group",
			in:     RoleInputs{Email: "a@example.com", FirstUser: true, Groups: []string{"guests"}},
			policy: RolePolicy{GroupRoles: groupRoles},
			want:   RoleDecision{Role: RoleAdmin, Source: SourceFirstUser},
		},
		{
			name:   "group maps to its role",
			in:     RoleInputs{Email: "a@example.com", Groups: []string{"support"}},
			policy: RolePolicy{GroupRoles: groupRoles},
			want:   RoleDecision{Role: RoleCommenter, Source: SourceGroup, Group: "support"},
		},
		{
			name:   "highest mapped group wins",
			in:     RoleInputs{Email: "a@example.com", Groups: []string{"guests", "ops", "eng"}},
			policy: RolePolicy{GroupRoles: groupRoles},
			want:   RoleDecision{Role: RoleAdmin, Source: SourceGroup, Group: "ops"},
		},
		{
			name:   "tie keeps the first entry in setting order",
			in:     RoleInputs{Email: "a@example.com", Groups: []string{"other-editors", "eng"}},
			policy: RolePolicy{GroupRoles: groupRoles},
			want:   RoleDecision{Role: RoleEditor, Source: SourceGroup, Group: "eng"},
		},
		{
			name:   "group beats a manual role",
			in:     RoleInputs{Email: "a@example.com", Groups: []string{"guests"}, Manual: &admin},
			policy: RolePolicy{GroupRoles: groupRoles},
			want:   RoleDecision{Role: RoleViewer, Source: SourceGroup, Group: "guests"},
		},
		{
			name:   "unmapped group falls through to the manual role",
			in:     RoleInputs{Email: "a@example.com", Groups: []string{"unknown"}, Manual: &commenter},
			policy: RolePolicy{GroupRoles: groupRoles},
			want:   RoleDecision{Role: RoleCommenter, Source: SourceManual},
		},
		{
			name:   "group names match exactly",
			in:     RoleInputs{Email: "a@example.com", Groups: []string{"OPS"}},
			policy: RolePolicy{GroupRoles: groupRoles},
			want:   RoleDecision{Role: RoleEditor, Source: SourceDefault},
		},
		{
			name: "manual role applies without settings",
			in:   RoleInputs{Email: "a@example.com", Manual: &viewer},
			want: RoleDecision{Role: RoleViewer, Source: SourceManual},
		},
		{
			name: "default is editor",
			in:   RoleInputs{Email: "a@example.com"},
			want: RoleDecision{Role: RoleEditor, Source: SourceDefault},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveRole(tt.in, tt.policy); got != tt.want {
				t.Errorf("ResolveRole() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
