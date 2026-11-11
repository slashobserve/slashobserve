package folders

import "github.com/slashobserve/slashobserve/internal/auth"

func Unfiled(p auth.Principal, creator bool) Abilities {
	switch {
	case p.Role == auth.RoleAdmin:
		return AbilitiesFor(LevelAdmin)
	case creator:
		return AbilitiesFor(LevelEdit)
	}
	return Abilities{}
}
