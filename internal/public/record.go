package public

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

type recordedObject struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type recordedLink struct {
	Object    recordedObject `json:"object"`
	Title     string         `json:"title"`
	Prefix    string         `json:"prefix"`
	ExpiresAt *time.Time     `json:"expires_at,omitempty"`
}

func recordLink(ctx context.Context, q *dbsqlc.Queries, typ string, by auth.Principal, linkID pgtype.UUID) error {
	row, err := q.GetPublicLinkEvent(ctx, linkID)
	if err != nil {
		return err
	}
	data := recordedLink{Title: row.Title, Prefix: row.DisplayPrefix, ExpiresAt: timeOf(row.ExpiresAt)}
	owner := activity.OnDashboard(row.DashboardID)
	data.Object = recordedObject{Kind: "dashboard", ID: row.DashboardID.String()}
	if row.ObservationID.Valid {
		owner = activity.OnObservation(row.ObservationID)
		data.Object = recordedObject{Kind: "observation", ID: row.ObservationID.String()}
	}
	_, err = activity.Append(ctx, q, activity.Entry{Type: typ, TargetID: linkID, Owner: owner, Actor: activity.By(ctx, by), Data: data}, nil)
	return err
}
