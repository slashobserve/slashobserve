package folders

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
)

func (a *Access) CreationFolder(ctx context.Context, p auth.Principal, requested pgtype.UUID) (Folder, error) {
	levels, err := a.Levels(ctx, p)
	if err != nil {
		return Folder{}, err
	}
	id, err := pickCreationFolder(levels.entries, levels.personal, requested)
	if err != nil {
		return Folder{}, err
	}
	e, _ := levels.find(id)
	return e.Folder, nil
}

func pickCreationFolder(levels []folderLevel, personal, requested pgtype.UUID) (pgtype.UUID, error) {
	if requested.Valid {
		return requestedCreationFolder(levels, requested)
	}
	for _, e := range levels {
		if personal.Valid && e.Folder.ID == personal && e.Abilities.Create {
			return personal, nil
		}
	}
	for _, e := range levels {
		if e.Folder.General && e.Abilities.Create {
			return e.Folder.ID, nil
		}
	}
	return pgtype.UUID{}, ErrForbidden
}

func requestedCreationFolder(levels []folderLevel, requested pgtype.UUID) (pgtype.UUID, error) {
	for _, e := range levels {
		if e.Folder.ID != requested {
			continue
		}
		if !e.Abilities.Create {
			return pgtype.UUID{}, ErrForbidden
		}
		return requested, nil
	}
	return pgtype.UUID{}, ErrNotFound
}
