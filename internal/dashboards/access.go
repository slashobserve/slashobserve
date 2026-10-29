package dashboards

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/folders"
)

type createTarget struct {
	folder    folders.Folder
	abilities folders.Abilities
}

func (s *Service) need(ctx context.Context, by auth.Principal, id pgtype.UUID, n folders.Need) (folders.DashboardAccess, error) {
	found, err := s.access.Dashboard(ctx, by, id)
	if errors.Is(err, folders.ErrNotFound) {
		return folders.DashboardAccess{}, ErrNotFound
	}
	if err != nil {
		return folders.DashboardAccess{}, err
	}
	if !found.Abilities.Has(n) {
		return folders.DashboardAccess{}, folders.ErrForbidden
	}
	return found, nil
}

func withAccess(d Dashboard, a folders.DashboardAccess) Dashboard {
	d.Folder = a.Folder
	d.Abilities = a.Abilities
	return d
}

func (s *Service) creationTarget(ctx context.Context, by auth.Principal, requested pgtype.UUID) (createTarget, error) {
	folder, err := s.access.CreationFolder(ctx, by, requested)
	if errors.Is(err, folders.ErrNotFound) {
		return createTarget{}, ErrFolderNotFound
	}
	if err != nil {
		return createTarget{}, err
	}
	levels, err := s.access.Levels(ctx, by)
	if err != nil {
		return createTarget{}, err
	}
	return createTarget{folder: folder, abilities: levels.Of(folder.ID)}, nil
}
