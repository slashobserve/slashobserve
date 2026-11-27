package folders

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/lifecycle"
)

type Kind string

const (
	KindGeneral    Kind = "general"
	KindPersonal   Kind = "personal"
	KindAudit      Kind = "audit"
	KindAdminsOnly Kind = "admins_only"
	KindRestricted Kind = "restricted"
	KindOpen       Kind = "open"
)

var Kinds = lifecycle.Derived{Name: "folder", States: []lifecycle.DerivedState{
	{Name: string(KindGeneral), When: "It is General: everyone sees it by role, and it is never renamed, restricted or deleted."},
	{Name: string(KindPersonal), When: "It is the caller's My Dashboards: only they see it, it takes new work but no moves, and nobody renames, shares or deletes it."},
	{Name: string(KindAudit), When: "It is the Audit folder the server made for the audit starters; `admins_only` and `restricted` say who reads it."},
	{Name: string(KindAdminsOnly), When: "Only Admins read it, and it lists nobody."},
	{Name: string(KindRestricted), When: "It lists people or groups, and only they, its creator and Admins see it."},
	{Name: string(KindOpen), When: "None of the above: everyone sees it, each at the level of their role."},
}}

func Restricted(adminsOnly, listed bool) bool {
	return adminsOnly || listed
}

func KindOf(f Folder, audit bool) Kind {
	switch {
	case f.General:
		return KindGeneral
	case f.Personal:
		return KindPersonal
	case audit:
		return KindAudit
	case f.AdminsOnly:
		return KindAdminsOnly
	case f.Restricted:
		return KindRestricted
	}
	return KindOpen
}

type Action string

const (
	ActionCreateIn       Action = "create_in"
	ActionMoveInto       Action = "move_into"
	ActionRename         Action = "rename"
	ActionSetPermissions Action = "set_permissions"
	ActionDelete         Action = "delete"
)

var Actions = []Action{ActionCreateIn, ActionMoveInto, ActionRename, ActionSetPermissions, ActionDelete}

func allows(f Folder, a Abilities, action Action) bool {
	switch action {
	case ActionCreateIn:
		return a.Create
	case ActionMoveInto:
		return a.Edit && MayMoveInto(f)
	case ActionRename, ActionSetPermissions, ActionDelete:
		return a.Manage && !f.General
	}
	return false
}

func AllowedActions(f Folder, a Abilities) []Action {
	allowed := []Action{}
	for _, action := range Actions {
		if allows(f, a, action) {
			allowed = append(allowed, action)
		}
	}
	return allowed
}

func OwnPersonalActions(role auth.Role) []Action {
	return AllowedActions(Folder{Personal: true}, AbilitiesFor(LevelFor(role, Facts{Personal: true, Owner: true})))
}

func AdminsOnlyActions(role auth.Role) []Action {
	return AllowedActions(Folder{AdminsOnly: true, Restricted: true}, AbilitiesFor(LevelFor(role, Facts{AdminsOnly: true, Restricted: true})))
}

const (
	CodeGeneralFolder  = "general_folder"
	CodePersonalFolder = "personal_folder"
)

type refusalJSON struct {
	Error          string   `json:"error"`
	Code           string   `json:"code"`
	AllowedActions []Action `json:"allowed_actions"`
}

func abortRefusal(c *gin.Context, code, message string, allowed []Action) {
	c.AbortWithStatusJSON(http.StatusConflict, refusalJSON{Error: message, Code: code, AllowedActions: allowed})
}

func abortGeneralFolder(c *gin.Context, message string) {
	current := folderFrom(c)
	abortRefusal(c, CodeGeneralFolder, message, AllowedActions(current.Folder, current.Abilities))
}

func AbortPersonalFolder(c *gin.Context, message string) {
	abortRefusal(c, CodePersonalFolder, message, OwnPersonalActions(httpapi.Caller(c).Role))
}
