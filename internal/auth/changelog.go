package auth

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

type RoleChange struct {
	UserID pgtype.UUID
	Name   string
	From   Role
	To     Role
	Manual bool
	Source RoleSource
}

type ChangeLog interface {
	RoleChanged(ctx context.Context, q *dbsqlc.Queries, by Principal, change RoleChange) error
	ServiceAccountCreated(ctx context.Context, q *dbsqlc.Queries, by Principal, account dbsqlc.ServiceAccount) error
	ServiceAccountUpdated(ctx context.Context, q *dbsqlc.Queries, by Principal, before, after dbsqlc.ServiceAccount) error
	KeyCreated(ctx context.Context, q *dbsqlc.Queries, by Principal, account dbsqlc.ServiceAccount, key dbsqlc.CreateServiceAccountKeyRow) error
	KeyRevoked(ctx context.Context, q *dbsqlc.Queries, by Principal, account dbsqlc.ServiceAccount, keyID pgtype.UUID, keyName string) error
}

type unrecorded struct{}

func (unrecorded) RoleChanged(context.Context, *dbsqlc.Queries, Principal, RoleChange) error {
	return nil
}

func (unrecorded) ServiceAccountCreated(context.Context, *dbsqlc.Queries, Principal, dbsqlc.ServiceAccount) error {
	return nil
}

func (unrecorded) ServiceAccountUpdated(context.Context, *dbsqlc.Queries, Principal, dbsqlc.ServiceAccount, dbsqlc.ServiceAccount) error {
	return nil
}

func (unrecorded) KeyCreated(context.Context, *dbsqlc.Queries, Principal, dbsqlc.ServiceAccount, dbsqlc.CreateServiceAccountKeyRow) error {
	return nil
}

func (unrecorded) KeyRevoked(context.Context, *dbsqlc.Queries, Principal, dbsqlc.ServiceAccount, pgtype.UUID, string) error {
	return nil
}

func (s *Service) RecordChanges(database *db.DB, log ChangeLog) {
	s.database = database
	s.changes = log
}

func (s *Service) inTx(ctx context.Context, fn func(q *dbsqlc.Queries) error) error {
	if s.database == nil {
		return fn(s.queries)
	}
	return s.database.InTx(ctx, fn)
}
