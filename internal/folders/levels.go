package folders

import "github.com/slashobserve/slashobserve/internal/auth"

type Level int

const (
	LevelNone Level = iota
	LevelView
	LevelComment
	LevelEdit
	LevelAdmin
)

func ParseLevel(s string) (Level, bool) {
	switch s {
	case "view":
		return LevelView, true
	case "comment":
		return LevelComment, true
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
	case LevelComment:
		return "comment"
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

func roleLevels(role auth.Role) (floor, ceiling Level) {
	switch role {
	case auth.RoleViewer:
		return LevelView, LevelView
	case auth.RoleCommenter:
		return LevelComment, LevelComment
	case auth.RoleEditor:
		return LevelEdit, LevelAdmin
	}
	return LevelNone, LevelNone
}

func LevelFor(role auth.Role, f Facts) Level {
	if role == auth.RoleAdmin {
		return LevelAdmin
	}
	floor, ceiling := roleLevels(role)
	if ceiling == LevelNone || f.Restricted && len(f.Grants) == 0 && !f.Creator {
		return LevelNone
	}
	level := floor
	for _, g := range f.Grants {
		level = max(level, g)
	}
	if f.Creator {
		level = LevelAdmin
	}
	return min(level, ceiling)
}

type Abilities struct {
	View, Comment, Edit, Manage, Create bool
}

func AbilitiesFor(l Level) Abilities {
	return Abilities{
		View:    l >= LevelView,
		Comment: l >= LevelComment,
		Edit:    l >= LevelEdit,
		Manage:  l == LevelAdmin,
		Create:  l >= LevelEdit,
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
