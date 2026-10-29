package auth

import (
	"context"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

var (
	ErrRoleLocked = errors.New("role is locked")
	ErrLastAdmin  = errors.New("last admin")
	ErrNoUser     = errors.New("user not found")
)

type RoleLockedError struct {
	Source RoleSource
}

func (e *RoleLockedError) Error() string { return ErrRoleLocked.Error() }

func (e *RoleLockedError) Is(target error) bool { return target == ErrRoleLocked }

func (s *Service) SetManualRole(ctx context.Context, q *dbsqlc.Queries, by Principal, userID pgtype.UUID, role *Role) error {
	admins, err := q.LockAdmins(ctx)
	if err != nil {
		return err
	}
	user, err := q.GetUserForRole(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNoUser
	}
	if err != nil {
		return err
	}
	if source := RoleSource(user.RoleSource); source.Locked() {
		return &RoleLockedError{Source: source}
	}
	if err := q.SetManualRole(ctx, manualRoleParams(userID, by, role)); err != nil {
		return err
	}
	first, err := q.IsFirstUser(ctx, userID)
	if err != nil {
		return err
	}
	decision := ResolveRole(RoleInputs{Email: user.Email, Groups: user.Groups, FirstUser: first, Manual: role}, s.cfg.RolePolicy)
	if leavesNoAdmin(admins, userID, decision.Role) {
		return ErrLastAdmin
	}
	return q.SetUserRole(ctx, setUserRoleParams(userID, decision))
}

func (s *Service) SetServiceAccountRole(ctx context.Context, id pgtype.UUID, role Role) (dbsqlc.ServiceAccount, error) {
	return s.queries.SetServiceAccountRole(ctx, dbsqlc.SetServiceAccountRoleParams{ID: id, Role: string(role)})
}

func leavesNoAdmin(admins []pgtype.UUID, target pgtype.UUID, next Role) bool {
	return next != RoleAdmin && len(admins) == 1 && slices.Contains(admins, target)
}

func manualRoleParams(userID pgtype.UUID, by Principal, role *Role) dbsqlc.SetManualRoleParams {
	if role == nil {
		return dbsqlc.SetManualRoleParams{ID: userID}
	}
	var byID pgtype.UUID
	_ = byID.Scan(by.ID)
	return dbsqlc.SetManualRoleParams{
		ID:         userID,
		ManualRole: pgtype.Text{String: string(*role), Valid: true},
		ByID:       byID,
		ByName:     pgtype.Text{String: by.Name, Valid: by.Name != ""},
	}
}
