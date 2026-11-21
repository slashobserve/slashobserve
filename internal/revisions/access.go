package revisions

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/forbidden"
)

var ErrForbidden = errors.New(forbidden.RoleMessage)

type RunTarget struct {
	Intent        Intent
	DashboardID   pgtype.UUID
	ObservationID pgtype.UUID
	Files         bool
}

func targetOf(row dbsqlc.AgentRequest) RunTarget {
	return shapeOfRow(row).target()
}

func (r RunShape) target() RunTarget {
	if r.Investigation {
		return RunTarget{Intent: r.Intent, DashboardID: r.DashboardID, ObservationID: r.ObservationID, Files: true}
	}
	return RunTarget{Intent: r.Intent, DashboardID: r.DashboardID, ObservationID: r.SubjectID}
}

func retryTarget(source Source, ran RunTarget) RunTarget {
	if source == SourceObserve {
		return RunTarget{Intent: IntentAsk}
	}
	return ran
}

type runRule struct {
	target      RunTarget
	role        auth.Role
	canCreate   bool
	dashboard   folders.Abilities
	observation folders.Abilities
}

func runAllowed(r runRule) bool {
	t := r.target
	switch t.Intent {
	case IntentSuggest:
		return t.DashboardID.Valid && r.dashboard.Edit
	case IntentApply:
		return t.DashboardID.Valid && r.dashboard.Comment
	case IntentAsk:
	default:
		return false
	}
	if t.DashboardID.Valid && !r.dashboard.View {
		return false
	}
	switch {
	case t.Files:
		return r.role.AtLeast(auth.RoleEditor)
	case t.ObservationID.Valid:
		return r.observation.Comment
	case t.DashboardID.Valid:
		return r.dashboard.Comment
	}
	return r.canCreate
}

func refusal(r runRule, folderTitle string) error {
	t := r.target
	switch {
	case t.Intent == IntentAsk && t.Files && !r.role.AtLeast(auth.RoleEditor):
		return forbidden.Need(ErrForbidden, forbidden.NeedsRole(string(auth.RoleEditor)))
	case t.Intent == IntentAsk && (t.Files || t.ObservationID.Valid && !t.DashboardID.Valid):
		return forbidden.Need(ErrForbidden, forbidden.NeedsAccess("comment", folderTitle))
	case t.DashboardID.Valid && t.Intent == IntentSuggest:
		return forbidden.Need(ErrForbidden, forbidden.NeedsAccess("edit", folderTitle))
	case t.DashboardID.Valid:
		return forbidden.Need(ErrForbidden, forbidden.NeedsAccess("comment", folderTitle))
	}
	return forbidden.Need(ErrForbidden, forbidden.NeedsAccess("edit", ""))
}

const requesterForbiddenMessage = "the person or key that asked no longer has access to change this dashboard"

func (s *Service) CheckRun(ctx context.Context, target RunTarget, by auth.Principal) error {
	return s.checkRun(ctx, target, by)
}

func (s *Service) checkRun(ctx context.Context, target RunTarget, by auth.Principal) error {
	rule := runRule{target: target, role: by.Role}
	title, starter := "", false
	if target.DashboardID.Valid {
		access, err := s.dashboardAccess(ctx, target.DashboardID, by)
		if err != nil {
			return err
		}
		rule.dashboard, title = access.Abilities, access.Folder.Title
		starter = access.Provisioned && target.Intent != IntentAsk
	}
	if target.Intent == IntentAsk && target.ObservationID.Valid && !target.Files {
		folder, abilities, err := s.observationAccess(ctx, target.ObservationID, by)
		if err != nil {
			return err
		}
		rule.observation, title = abilities, folder
	}
	if !target.DashboardID.Valid && !target.ObservationID.Valid && !target.Files {
		levels, err := s.access.Levels(ctx, by)
		switch {
		case errors.Is(err, folders.ErrForbidden):
			return ErrForbidden
		case err != nil:
			return err
		}
		rule.canCreate = levels.CanCreate()
	}
	switch {
	case !runAllowed(rule):
		return refusal(rule, title)
	case starter:
		return folders.ErrProvisioned
	}
	return nil
}

func (s *Service) dashboardAccess(ctx context.Context, dashboardID pgtype.UUID, by auth.Principal) (folders.DashboardAccess, error) {
	access, err := s.access.Dashboard(ctx, by, dashboardID)
	switch {
	case errors.Is(err, folders.ErrNotFound), errors.Is(err, folders.ErrNoAccess):
		return folders.DashboardAccess{}, ErrDashboardNotFound
	case errors.Is(err, folders.ErrForbidden):
		return folders.DashboardAccess{}, ErrForbidden
	case err != nil:
		return folders.DashboardAccess{}, err
	case !access.Abilities.View:
		return folders.DashboardAccess{}, ErrForbidden
	}
	return access, nil
}

func (s *Service) observationAccess(ctx context.Context, observationID pgtype.UUID, by auth.Principal) (string, folders.Abilities, error) {
	observation, err := s.db.Queries().ObservationByID(ctx, observationID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return "", folders.Abilities{}, ErrObservationNotFound
	case err != nil:
		return "", folders.Abilities{}, fmt.Errorf("read the run's observation: %w", err)
	}
	folder, abilities, err := s.access.Folder(ctx, by, observation.FolderID)
	switch {
	case errors.Is(err, folders.ErrNotFound), errors.Is(err, folders.ErrNoAccess):
		return "", folders.Abilities{}, ErrObservationNotFound
	case errors.Is(err, folders.ErrForbidden):
		return "", folders.Abilities{}, ErrForbidden
	case err != nil:
		return "", folders.Abilities{}, err
	case !abilities.View:
		return "", folders.Abilities{}, ErrObservationNotFound
	}
	return folder.Title, abilities, nil
}

func (s *Service) mayComment(ctx context.Context, observationID pgtype.UUID, by auth.Principal) (bool, error) {
	_, abilities, err := s.observationAccess(ctx, observationID, by)
	switch {
	case errors.Is(err, ErrObservationNotFound), errors.Is(err, ErrForbidden):
		return false, nil
	case err != nil:
		return false, err
	}
	return abilities.Comment, nil
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

func (s *Service) checkObservationVisible(ctx context.Context, observationID pgtype.UUID, by auth.Principal) error {
	observation, err := s.db.Queries().ObservationByID(ctx, observationID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ErrRequestNotFound
	case err != nil:
		return fmt.Errorf("read the run's observation: %w", err)
	}
	_, _, err = s.access.Folder(ctx, by, observation.FolderID)
	if errors.Is(err, folders.ErrNotFound) || errors.Is(err, folders.ErrNoAccess) || errors.Is(err, folders.ErrForbidden) {
		return ErrRequestNotFound
	}
	return err
}
