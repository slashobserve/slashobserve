package threads

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/annotations"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/observations"
	"github.com/slashobserve/slashobserve/internal/proposals"
)

type scope struct {
	target    Target
	dashboard pgtype.UUID
	activity  activity.Owner
	abilities folders.Abilities
}

func (s *Service) scopeOf(ctx context.Context, by auth.Principal, t Target) (scope, error) {
	var err error
	var dashboard pgtype.UUID
	var owner activity.Owner
	var abilities folders.Abilities
	switch t.Kind {
	case KindDashboard:
		var found folders.DashboardAccess
		found, err = s.access.Dashboard(ctx, by, t.ID)
		dashboard, owner, abilities = t.ID, activity.OnDashboard(t.ID), found.Abilities
	case KindAnnotation:
		dashboard, abilities, err = s.annotations.Access(ctx, by, t.ID)
		owner = activity.General()
		if dashboard.Valid {
			owner = activity.OnDashboard(dashboard)
		}
	case KindObservation:
		var placement observations.Placement
		placement, err = s.observations.Access(ctx, by, t.ID)
		owner, abilities = placement.Activity, placement.Abilities
	case KindProposal:
		dashboard, abilities, err = s.proposals.Access(ctx, by, t.ID)
		owner = activity.OnDashboard(dashboard)
	default:
		return scope{}, invalidTarget("target kind must be dashboard, annotation, observation or proposal")
	}
	switch {
	case isHidden(err):
		return scope{}, ErrTargetNotFound
	case err != nil:
		return scope{}, err
	case !abilities.View:
		return scope{}, ErrTargetNotFound
	}
	return scope{target: t, dashboard: dashboard, activity: owner, abilities: abilities}, nil
}

func isHidden(err error) bool {
	return errors.Is(err, folders.ErrNotFound) || errors.Is(err, folders.ErrNoAccess) || errors.Is(err, folders.ErrForbidden) ||
		errors.Is(err, annotations.ErrNotFound) || errors.Is(err, observations.ErrNotFound) || errors.Is(err, proposals.ErrNotFound)
}

func (s *Service) scopeOfThread(ctx context.Context, by auth.Principal, row dbsqlc.Thread) (scope, error) {
	sc, err := s.scopeOf(ctx, by, targetOf(row))
	if errors.Is(err, ErrTargetNotFound) {
		return scope{}, ErrNotFound
	}
	return sc, err
}

func targetOf(row dbsqlc.Thread) Target {
	switch {
	case row.DashboardID.Valid:
		return Target{Kind: KindDashboard, ID: row.DashboardID}
	case row.ObservationID.Valid:
		return Target{Kind: KindObservation, ID: row.ObservationID}
	case row.ProposalID.Valid:
		return Target{Kind: KindProposal, ID: row.ProposalID}
	}
	return Target{Kind: KindAnnotation, ID: row.AnnotationID}
}

func lockTarget(ctx context.Context, q *dbsqlc.Queries, sc scope) error {
	if sc.dashboard.Valid {
		if _, err := q.LockDashboardKeyShare(ctx, sc.dashboard); err != nil {
			return lockFailure(err)
		}
	}
	var err error
	switch sc.target.Kind {
	case KindAnnotation:
		_, err = q.LockAnnotationKeyShare(ctx, sc.target.ID)
	case KindObservation:
		_, err = q.LockObservationKeyShare(ctx, sc.target.ID)
	case KindProposal:
		_, err = q.LockProposalKeyShare(ctx, sc.target.ID)
	}
	return lockFailure(err)
}

func lockFailure(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, pgx.ErrNoRows):
		return ErrTargetNotFound
	}
	return fmt.Errorf("lock thread target: %w", err)
}
