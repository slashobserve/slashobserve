package threads

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/annotations"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/folders"
)

type scope struct {
	target    Target
	owner     pgtype.UUID
	global    bool
	abilities folders.Abilities
}

func (s *Service) scopeOf(ctx context.Context, by auth.Principal, t Target) (scope, error) {
	var owner pgtype.UUID
	var abilities folders.Abilities
	var err error
	switch t.Kind {
	case KindDashboard:
		var found folders.DashboardAccess
		found, err = s.access.Dashboard(ctx, by, t.ID)
		owner, abilities = t.ID, found.Abilities
	case KindFinding:
		owner, abilities, err = s.findings.Access(ctx, by, t.ID)
	case KindAnnotation:
		owner, abilities, err = s.annotations.Access(ctx, by, t.ID)
	default:
		return scope{}, invalidTarget("target kind must be dashboard, finding or annotation")
	}
	switch {
	case isHidden(err):
		return scope{}, ErrTargetNotFound
	case err != nil:
		return scope{}, err
	case !abilities.View:
		return scope{}, ErrTargetNotFound
	}
	return scope{target: t, owner: owner, global: t.Kind == KindAnnotation && !owner.Valid, abilities: abilities}, nil
}

func isHidden(err error) bool {
	return errors.Is(err, folders.ErrNotFound) || errors.Is(err, folders.ErrNoAccess) || errors.Is(err, folders.ErrForbidden) ||
		errors.Is(err, findings.ErrNotFound) || errors.Is(err, annotations.ErrNotFound)
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
	case row.FindingID.Valid:
		return Target{Kind: KindFinding, ID: row.FindingID}
	}
	return Target{Kind: KindAnnotation, ID: row.AnnotationID}
}

func lockTarget(ctx context.Context, q *dbsqlc.Queries, sc scope) error {
	if sc.owner.Valid {
		if _, err := q.LockDashboardKeyShare(ctx, sc.owner); err != nil {
			return lockFailure(err)
		}
	}
	var err error
	switch sc.target.Kind {
	case KindFinding:
		_, err = q.LockFindingKeyShare(ctx, sc.target.ID)
	case KindAnnotation:
		_, err = q.LockAnnotationKeyShare(ctx, sc.target.ID)
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
