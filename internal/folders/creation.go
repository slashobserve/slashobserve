package folders

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func (a *Access) CreationFolder(ctx context.Context, p auth.Principal, requested pgtype.UUID) (Folder, error) {
	levels, err := a.Levels(ctx, p)
	if err != nil {
		return Folder{}, err
	}
	var lastUsed pgtype.UUID
	if !requested.Valid {
		principalID, err := principalUUID(p)
		if err != nil {
			return Folder{}, err
		}
		lastUsed, err = a.db.Queries().LastCreatedFolder(ctx, dbsqlc.LastCreatedFolderParams{Kind: string(p.Kind), ID: principalID})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return Folder{}, err
		}
	}
	id, err := pickCreationFolder(levels.entries, lastUsed, requested)
	if err != nil {
		return Folder{}, err
	}
	e, _ := levels.find(id)
	return e.Folder, nil
}

func pickCreationFolder(levels []folderLevel, lastUsed, requested pgtype.UUID) (pgtype.UUID, error) {
	canCreate := func(id pgtype.UUID) bool {
		for _, e := range levels {
			if e.Folder.ID == id {
				return e.Abilities.Create
			}
		}
		return false
	}
	if requested.Valid {
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
	if lastUsed.Valid && canCreate(lastUsed) {
		return lastUsed, nil
	}
	for _, e := range levels {
		if e.Folder.General && e.Abilities.Create {
			return e.Folder.ID, nil
		}
	}
	for _, e := range levels {
		if e.Abilities.Create {
			return e.Folder.ID, nil
		}
	}
	return pgtype.UUID{}, ErrForbidden
}
