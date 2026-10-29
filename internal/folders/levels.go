package folders

import "github.com/slashobserve/slashobserve/internal/auth"

type Level int

const (
	LevelNone Level = iota
	LevelView
	LevelEdit
	LevelAdmin
)

func ParseLevel(s string) (Level, bool) {
	switch s {
	case "view":
		return LevelView, true
	case "edit":
		return LevelEdit, true
	case "admin":
		return LevelAdmin, true
	}
	return LevelNone, false
}

func (l Level) String() string {
	switch l {
	case LevelView:
		return "view"
	case LevelEdit:
		return "edit"
	case LevelAdmin:
		return "admin"
	}
	return "none"
}

type Facts struct {
	Restricted bool
	Grants     []Level
	Creator    bool
}

func baseLevel(role auth.Role) Level {
	switch role {
	case auth.RoleEditor:
		return LevelEdit
	case auth.RoleCommenter, auth.RoleViewer:
		return LevelView
	}
	return LevelNone
}

func LevelFor(role auth.Role, f Facts) Level {
	if role == auth.RoleAdmin {
		return LevelAdmin
	}
	base := baseLevel(role)
	if base == LevelNone || f.Restricted && len(f.Grants) == 0 && !f.Creator {
		return LevelNone
	}
	level := base
	for _, g := range f.Grants {
		level = max(level, g)
	}
	if f.Creator {
		level = LevelAdmin
	}
	if !role.AtLeast(auth.RoleEditor) {
		level = min(level, LevelEdit)
	}
	return level
}

type Abilities struct {
	View, Comment, Edit, Manage, Create bool
}

func AbilitiesFor(role auth.Role, l Level) Abilities {
	canView := l >= LevelView
	canEdit := l >= LevelEdit
	return Abilities{
		View:    canView,
		Comment: canView && (role.AtLeast(auth.RoleCommenter) || canEdit),
		Edit:    canEdit,
		Manage:  l == LevelAdmin,
		Create:  canEdit && role.AtLeast(auth.RoleEditor),
	}
}

type Need int

const (
	NeedView Need = iota
	NeedComment
	NeedEdit
	NeedManage
	NeedCreate
)

func (a Abilities) Has(n Need) bool {
	switch n {
	case NeedView:
		return a.View
	case NeedComment:
		return a.Comment
	case NeedEdit:
		return a.Edit
	case NeedManage:
		return a.Manage
	case NeedCreate:
		return a.Create
	}
	return false
}
