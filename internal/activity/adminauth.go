package activity

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/origin"
)

type Change[T any] struct {
	From T `json:"from"`
	To   T `json:"to"`
}

func Changed[T comparable](from, to T) *Change[T] {
	if from == to {
		return nil
	}
	return &Change[T]{From: from, To: to}
}

func By(ctx context.Context, p auth.Principal) actor.Actor {
	return actor.Of(p, origin.From(ctx), "")
}

type AuthChanges struct{}

var _ auth.ChangeLog = AuthChanges{}

type roleChangedData struct {
	Name     string    `json:"name"`
	FromRole auth.Role `json:"from_role"`
	ToRole   auth.Role `json:"to_role"`
	Manual   bool      `json:"manual"`
}

func (AuthChanges) RoleChanged(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, c auth.RoleChange) error {
	return AppendAdmin(ctx, q, By(ctx, by), TypeUserRoleChanged, c.UserID, roleChangedData{Name: c.Name, FromRole: c.From, ToRole: c.To, Manual: c.Manual})
}

type serviceAccountCreatedData struct {
	Name      string    `json:"name"`
	Role      auth.Role `json:"role"`
	TakesRuns bool      `json:"takes_runs"`
}

func (AuthChanges) ServiceAccountCreated(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, a dbsqlc.ServiceAccount) error {
	return AppendAdmin(ctx, q, By(ctx, by), TypeServiceAccountCreated, a.ID, serviceAccountCreatedData{Name: a.Name, Role: auth.Role(a.Role), TakesRuns: a.TakesRuns})
}

type serviceAccountUpdatedData struct {
	Name      string             `json:"name"`
	Role      *Change[auth.Role] `json:"role,omitempty"`
	TakesRuns *Change[bool]      `json:"takes_runs,omitempty"`
}

func (AuthChanges) ServiceAccountUpdated(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, before, after dbsqlc.ServiceAccount) error {
	data := serviceAccountUpdatedData{
		Name:      after.Name,
		Role:      Changed(auth.Role(before.Role), auth.Role(after.Role)),
		TakesRuns: Changed(before.TakesRuns, after.TakesRuns),
	}
	if data.Role == nil && data.TakesRuns == nil {
		return nil
	}
	return AppendAdmin(ctx, q, By(ctx, by), TypeServiceAccountUpdated, after.ID, data)
}

type keyData struct {
	Name      string  `json:"name"`
	KeyID     string  `json:"key_id"`
	KeyName   string  `json:"key_name"`
	ExpiresAt *string `json:"expires_at,omitempty"`
}

func (AuthChanges) KeyCreated(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, a dbsqlc.ServiceAccount, key dbsqlc.CreateServiceAccountKeyRow) error {
	data := keyData{Name: a.Name, KeyID: key.ID.String(), KeyName: key.Name}
	if key.ExpiresAt.Valid {
		expires := httpapi.FormatTime(key.ExpiresAt.Time)
		data.ExpiresAt = &expires
	}
	return AppendAdmin(ctx, q, By(ctx, by), TypeServiceAccountKeyCreated, a.ID, data)
}

func (AuthChanges) KeyRevoked(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, a dbsqlc.ServiceAccount, keyID pgtype.UUID, keyName string) error {
	return AppendAdmin(ctx, q, By(ctx, by), TypeServiceAccountKeyRevoked, a.ID, keyData{Name: a.Name, KeyID: keyID.String(), KeyName: keyName})
}
