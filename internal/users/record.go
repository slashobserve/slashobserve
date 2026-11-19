package users

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

type invitedData struct {
	Email string    `json:"email"`
	Role  auth.Role `json:"role"`
}

type inviteUpdatedData struct {
	Email string                      `json:"email"`
	Role  *activity.Change[auth.Role] `json:"role"`
}

type inviteWithdrawnData struct {
	Email string `json:"email"`
}

type deactivatedData struct {
	Name     string `json:"name"`
	Sessions int64  `json:"sessions"`
	Grants   int64  `json:"grants"`
}

type personData struct {
	Name string `json:"name"`
}

func recordInvite(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, row dbsqlc.UpsertInviteRow, previous auth.Role) error {
	if row.Inserted {
		return activity.AppendAdmin(ctx, q, activity.By(ctx, by), activity.TypeUserInvited, row.ID, invitedData{Email: row.Email, Role: auth.Role(row.Role)})
	}
	change := activity.Changed(previous, auth.Role(row.Role))
	if change == nil {
		return nil
	}
	return activity.AppendAdmin(ctx, q, activity.By(ctx, by), activity.TypeUserInviteUpdated, row.ID, inviteUpdatedData{Email: row.Email, Role: change})
}

func recordInviteWithdrawn(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, id pgtype.UUID, email string) error {
	return activity.AppendAdmin(ctx, q, activity.By(ctx, by), activity.TypeUserInviteWithdrawn, id, inviteWithdrawnData{Email: email})
}

func recordDeactivated(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, id pgtype.UUID, d auth.Deactivation) error {
	if !d.Changed {
		return nil
	}
	return activity.AppendAdmin(ctx, q, activity.By(ctx, by), activity.TypeUserDeactivated, id, deactivatedData{Name: d.Name, Sessions: d.Sessions, Grants: d.Grants})
}

func recordReactivated(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, id pgtype.UUID, r auth.Reactivation) error {
	if !r.Changed {
		return nil
	}
	return activity.AppendAdmin(ctx, q, activity.By(ctx, by), activity.TypeUserReactivated, id, personData{Name: r.Name})
}

func recordRemoved(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, id pgtype.UUID, name string) error {
	return activity.AppendAdmin(ctx, q, activity.By(ctx, by), activity.TypeUserRemoved, id, personData{Name: name})
}
