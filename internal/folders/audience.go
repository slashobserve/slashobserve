package folders

import (
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
)

type Audience struct {
	Folders    []pgtype.UUID
	Admin      bool
	ReaderKind string
	ReaderID   pgtype.UUID
}

func AudienceOf(p auth.Principal, levels Levels) (Audience, error) {
	id, err := db.ParseUUID(p.ID)
	if err != nil {
		return Audience{}, ErrForbidden
	}
	return Audience{
		Folders:    levels.Visible(),
		Admin:      p.Role == auth.RoleAdmin,
		ReaderKind: string(p.Kind),
		ReaderID:   id,
	}, nil
}
