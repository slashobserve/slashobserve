package dashboards

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
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
	if found.Refuses(n) {
		return folders.DashboardAccess{}, folders.ErrProvisioned
	}
	return found, nil
}

func (d Dashboard) WithAccess(a folders.DashboardAccess) Dashboard {
	d.Folder = a.Folder
	d.Abilities = a.Shown()
	d.Provisioned = a.Provisioned
	d.ManagedBy = a.ManagedBy()
	return d
}

func (s *Service) placed(ctx context.Context, by auth.Principal, d Dashboard) (Dashboard, error) {
	id, err := db.ParseUUID(d.ID)
	if err != nil {
		return Dashboard{}, fmt.Errorf("dashboard id: %w", err)
	}
	access, err := s.access.Dashboard(ctx, by, id)
	if errors.Is(err, folders.ErrNotFound) || errors.Is(err, folders.ErrNoAccess) {
		return d, nil
	}
	if err != nil {
		return Dashboard{}, err
	}
	return d.WithAccess(access), nil
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
