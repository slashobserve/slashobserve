package auth

type Role string

const (
	RoleViewer    Role = "viewer"
	RoleCommenter Role = "commenter"
	RoleEditor    Role = "editor"
	RoleAdmin     Role = "admin"
)

type RoleSource string

const (
	SourceAdminEmails RoleSource = "admin_emails"
	SourceFirstUser   RoleSource = "first_user"
	SourceGroup       RoleSource = "group"
	SourceManual      RoleSource = "manual"
	SourceDefault     RoleSource = "default"
)

func (r Role) rank() int {
	switch r {
	case RoleViewer:
		return 1
	case RoleCommenter:
		return 2
	case RoleEditor:
		return 3
	case RoleAdmin:
		return 4
	}
	return 0
}

func ParseRole(s string) (Role, bool) {
	r := Role(s)
	return r, r.rank() > 0
}

func (r Role) AtLeast(min Role) bool {
	return r.rank() > 0 && r.rank() >= min.rank()
}

func (s RoleSource) Locked() bool {
	return s == SourceAdminEmails || s == SourceFirstUser || s == SourceGroup
}
