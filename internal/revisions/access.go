package revisions

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/forbidden"
)

var ErrForbidden = errors.New(forbidden.RoleMessage)

const requesterForbiddenMessage = "the person or key that asked no longer has access to change this dashboard"

func runAllowed(source Source, hasDashboard, canCreate bool, role auth.Role, a folders.Abilities) bool {
	if source == SourceInvestigate {
		return role.AtLeast(auth.RoleEditor) && (!hasDashboard || a.View)
	}
	if hasDashboard {
		return a.Edit
	}
	return source == SourceObserve && canCreate
}

func (s *Service) checkRun(ctx context.Context, source Source, dashboardID pgtype.UUID, by auth.Principal) error {
	if !dashboardID.Valid {
		levels, err := s.access.Levels(ctx, by)
		switch {
		case errors.Is(err, folders.ErrForbidden):
			return ErrForbidden
		case err != nil:
			return err
		case !runAllowed(source, false, levels.CanCreate(), by.Role, folders.Abilities{}):
			return ErrForbidden
		}
		return nil
	}
	access, err := s.access.Dashboard(ctx, by, dashboardID)
	switch {
	case errors.Is(err, folders.ErrNotFound), errors.Is(err, folders.ErrNoAccess):
		return ErrDashboardNotFound
	case errors.Is(err, folders.ErrForbidden):
		return ErrForbidden
	case err != nil:
		return err
	case !runAllowed(source, true, false, by.Role, access.Abilities):
		return ErrForbidden
	}
	return nil
}

func (s *Service) checkVisible(ctx context.Context, dashboardID pgtype.UUID, by auth.Principal) error {
	if !dashboardID.Valid {
		return nil
	}
	_, err := s.access.Dashboard(ctx, by, dashboardID)
	if errors.Is(err, folders.ErrNotFound) || errors.Is(err, folders.ErrNoAccess) || errors.Is(err, folders.ErrForbidden) {
		return ErrRequestNotFound
	}
	return err
}
