package auth

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

var ErrDeactivateSelf = errors.New("cannot deactivate self")

type Deactivation struct {
	Name     string
	Changed  bool
	Sessions int64
	Grants   int64
}

func Deactivate(ctx context.Context, q *dbsqlc.Queries, by Principal, userID pgtype.UUID) (Deactivation, error) {
	admins, err := q.LockAdmins(ctx)
	if err != nil {
		return Deactivation{}, err
	}
	user, err := q.GetUserForRole(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Deactivation{}, ErrNoUser
	}
	if err != nil {
		return Deactivation{}, err
	}
	if user.DeactivatedAt.Valid {
		return Deactivation{}, nil
	}
	if by.ID == userID.String() {
		return Deactivation{}, ErrDeactivateSelf
	}
	if leavesNoAdmin(admins, userID, "") {
		return Deactivation{}, ErrLastAdmin
	}
	var byID pgtype.UUID
	_ = byID.Scan(by.ID)
	if err := q.DeactivateUser(ctx, dbsqlc.DeactivateUserParams{ID: userID, ByID: byID, ByName: pgtype.Text{String: by.Name, Valid: by.Name != ""}}); err != nil {
		return Deactivation{}, err
	}
	d := Deactivation{Name: user.Name, Changed: true}
	if d.Sessions, err = q.DeleteUserSessions(ctx, userID); err != nil {
		return Deactivation{}, err
	}
	if _, err := q.DeleteUnusedUserOAuthCodes(ctx, userID); err != nil {
		return Deactivation{}, err
	}
	if d.Grants, err = q.RevokeUserOAuthGrants(ctx, dbsqlc.RevokeUserOAuthGrantsParams{By: byID, UserID: userID}); err != nil {
		return Deactivation{}, err
	}
	return d, nil
}

type Reactivation struct {
	Name    string
	Changed bool
}

func Reactivate(ctx context.Context, q *dbsqlc.Queries, userID pgtype.UUID) (Reactivation, error) {
	user, err := q.GetUserForRole(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Reactivation{}, ErrNoUser
	}
	if err != nil {
		return Reactivation{}, err
	}
	if !user.DeactivatedAt.Valid {
		return Reactivation{Name: user.Name}, nil
	}
	if err := q.ReactivateUser(ctx, userID); err != nil {
		return Reactivation{}, err
	}
	return Reactivation{Name: user.Name, Changed: true}, nil
}
