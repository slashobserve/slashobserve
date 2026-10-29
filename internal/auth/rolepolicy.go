package auth

import (
	"slices"
	"strings"
)

type GroupRole struct {
	Group string
	Role  Role
}

type RolePolicy struct {
	AdminEmails []string
	GroupRoles  []GroupRole
}

type RoleInputs struct {
	Email     string
	Groups    []string
	FirstUser bool
	Manual    *Role
}

type RoleDecision struct {
	Role   Role
	Source RoleSource
	Group  string
}

func ResolveRole(in RoleInputs, policy RolePolicy) RoleDecision {
	for _, e := range policy.AdminEmails {
		if strings.EqualFold(e, in.Email) {
			return RoleDecision{Role: RoleAdmin, Source: SourceAdminEmails}
		}
	}
	if len(policy.AdminEmails) == 0 && in.FirstUser {
		return RoleDecision{Role: RoleAdmin, Source: SourceFirstUser}
	}
	if d, ok := groupDecision(in.Groups, policy.GroupRoles); ok {
		return d
	}
	if in.Manual != nil {
		return RoleDecision{Role: *in.Manual, Source: SourceManual}
	}
	return RoleDecision{Role: RoleEditor, Source: SourceDefault}
}

func groupDecision(groups []string, mapped []GroupRole) (RoleDecision, bool) {
	var best RoleDecision
	for _, gr := range mapped {
		if gr.Role.rank() <= best.Role.rank() || !slices.Contains(groups, gr.Group) {
			continue
		}
		best = RoleDecision{Role: gr.Role, Source: SourceGroup, Group: gr.Group}
	}
	return best, best.Source == SourceGroup
}
