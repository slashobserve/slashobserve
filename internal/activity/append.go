package activity

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/events"
)

type Entry struct {
	Type        string
	TargetID    pgtype.UUID
	DashboardID pgtype.UUID
	Global      bool
	Actor       actor.Actor
	Data        any
}

func Append(ctx context.Context, q *dbsqlc.Queries, e Entry, live *events.Event) (int64, error) {
	if !Known(e.Type) {
		return 0, fmt.Errorf("unknown activity type %q", e.Type)
	}
	columns, err := e.Actor.Columns()
	if err != nil {
		return 0, fmt.Errorf("activity actor: %w", err)
	}
	data, err := marshalData(e.Data)
	if err != nil {
		return 0, err
	}
	if e.DashboardID.Valid {
		if err := q.ShareDashboardActivity(ctx, e.DashboardID); err != nil {
			return 0, fmt.Errorf("lock dashboard activity: %w", err)
		}
	}
	seq, err := q.InsertActivity(ctx, dbsqlc.InsertActivityParams{
		Type:         e.Type,
		TargetKind:   targetKind(e.Type),
		TargetID:     e.TargetID,
		DashboardID:  e.DashboardID,
		Global:       e.Global,
		ActorKind:    columns.Kind,
		ActorID:      columns.ID,
		ActorName:    columns.Name,
		Origin:       columns.Origin,
		OriginClient: columns.Client,
		RunID:        columns.RunID,
		Data:         data,
	})
	if err != nil {
		return 0, fmt.Errorf("insert activity: %w", err)
	}
	if err := events.Notify(ctx, q, seq, live); err != nil {
		return 0, err
	}
	return seq, nil
}

func MoveDashboard(ctx context.Context, q *dbsqlc.Queries, dashboardID, folderID pgtype.UUID) error {
	if err := q.LockDashboardActivity(ctx, dashboardID); err != nil {
		return fmt.Errorf("lock dashboard activity: %w", err)
	}
	if _, err := q.MoveDashboardActivity(ctx, dbsqlc.MoveDashboardActivityParams{FolderID: folderID, DashboardID: dashboardID}); err != nil {
		return fmt.Errorf("move dashboard activity: %w", err)
	}
	return nil
}

func marshalData(data any) ([]byte, error) {
	if data == nil {
		return []byte("{}"), nil
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshal activity data: %w", err)
	}
	if bytes.Equal(raw, []byte("null")) {
		return []byte("{}"), nil
	}
	return raw, nil
}
