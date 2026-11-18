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

type NeedError struct{ Body forbidden.Body }

func (e *NeedError) Error() string { return forbidden.RoleMessage }

func (e *NeedError) Is(target error) bool { return target == ErrForbidden }

func refusal(source Source, hasDashboard bool, role auth.Role, folderTitle string) error {
	switch {
	case source == SourceInvestigate && !role.AtLeast(auth.RoleEditor):
		return &NeedError{Body: forbidden.NeedsRole(string(auth.RoleEditor))}
	case source == SourceInvestigate:
		return ErrForbidden
	case hasDashboard && source == SourceSuggest:
		return &NeedError{Body: forbidden.NeedsAccess("edit", folderTitle)}
	case hasDashboard:
		return &NeedError{Body: forbidden.NeedsAccess("comment", folderTitle)}
	}
	return &NeedError{Body: forbidden.NeedsAccess("edit", "")}
}

const requesterForbiddenMessage = "the person or key that asked no longer has access to change this dashboard"

func runAllowed(source Source, hasDashboard, canCreate bool, role auth.Role, a folders.Abilities) bool {
	if source == SourceInvestigate {
		return role.AtLeast(auth.RoleEditor) && (!hasDashboard || a.View)
	}
	if hasDashboard {
		if source == SourceSuggest {
			return a.Edit
		}
		return a.Comment
	}
	return source == SourceObserve && canCreate
}

func (s *Service) CheckRun(ctx context.Context, source Source, dashboardID pgtype.UUID, by auth.Principal) error {
	return s.checkRun(ctx, source, dashboardID, by)
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
			return refusal(source, false, by.Role, "")
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
		if !access.Abilities.View {
			return ErrForbidden
		}
		return refusal(source, true, by.Role, access.Folder.Title)
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
