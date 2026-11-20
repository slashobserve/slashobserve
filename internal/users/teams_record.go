package users

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func teamTarget(id pgtype.UUID, name string) audit.Target {
	return audit.Target{Kind: "team", ID: id.String(), Name: name}
}

func recordTeam(ctx context.Context, q *dbsqlc.Queries, action audit.Action, id pgtype.UUID, name string) error {
	return audit.Record(ctx, q, audit.Event{Action: action, Target: teamTarget(id, name)})
}

func recordTeamMember(ctx context.Context, q *dbsqlc.Queries, action audit.Action, teamID, userID pgtype.UUID, reason string) error {
	team, err := q.GetTeam(ctx, teamID)
	if err != nil {
		return err
	}
	name, err := q.GetUserName(ctx, userID)
	if err != nil {
		return err
	}
	data := map[string]any{"user_id": uuid.UUID(userID.Bytes).String(), "user_name": name}
	return audit.Record(ctx, q, audit.Event{Action: action, Reason: reason, Target: teamTarget(teamID, team.Name), Data: data})
}
