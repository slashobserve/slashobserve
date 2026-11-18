package mcpauth

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/origin"
)

type recordedGrant struct {
	Client     string `json:"client"`
	PersonID   string `json:"person_id"`
	PersonName string `json:"person_name"`
	Mode       string `json:"mode"`
	Reason     string `json:"reason,omitempty"`
}

func recordGrant(ctx context.Context, q *dbsqlc.Queries, typ string, grantID pgtype.UUID, reason string, by *actor.Actor) error {
	subject, err := q.GetOAuthGrantSubject(ctx, grantID)
	if err != nil {
		return err
	}
	who := actor.Actor{Kind: auth.KindUser, ID: subject.UserID.String(), Name: subject.UserName, Origin: origin.From(ctx)}
	if by != nil {
		who = *by
	}
	return activity.AppendAdmin(ctx, q, who, typ, grantID, recordedGrant{
		Client: subject.ClientName.String, PersonID: subject.UserID.String(), PersonName: subject.UserName,
		Mode: subject.Mode, Reason: reason,
	})
}

func recordConnected(ctx context.Context, q *dbsqlc.Queries, grantID pgtype.UUID) error {
	return recordGrant(ctx, q, activity.TypeMCPClientConnected, grantID, "", nil)
}

func recordDisconnected(ctx context.Context, q *dbsqlc.Queries, grantID pgtype.UUID, reason string, by *actor.Actor) error {
	return recordGrant(ctx, q, activity.TypeMCPClientDisconnected, grantID, reason, by)
}

func (s *Server) revokeRecorded(ctx context.Context, grantID pgtype.UUID, reason string, by *actor.Actor, revoke func(q *dbsqlc.Queries) (int64, error)) (revoked int64, err error) {
	err = s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		revoked, err = revoke(q)
		if err != nil || revoked == 0 {
			return err
		}
		return recordDisconnected(ctx, q, grantID, reason, by)
	})
	return revoked, err
}
