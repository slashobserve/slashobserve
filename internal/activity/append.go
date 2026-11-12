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
	Type     string
	TargetID pgtype.UUID
	Owner    Owner
	Actor    actor.Actor
	Data     any
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
	if e.Owner.locks() {
		if err := q.ShareActivityOwner(ctx, dbsqlc.ShareActivityOwnerParams{OwnerKind: string(e.Owner.kind), OwnerID: e.Owner.id}); err != nil {
			return 0, fmt.Errorf("lock activity owner: %w", err)
		}
	}
	seq, err := q.InsertActivity(ctx, dbsqlc.InsertActivityParams{
		Type:         e.Type,
		TargetKind:   targetKind(e.Type),
		TargetID:     e.TargetID,
		OwnerKind:    string(e.Owner.kind),
		OwnerID:      e.Owner.id,
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

func Move(ctx context.Context, q *dbsqlc.Queries, owner Owner, folderID pgtype.UUID) error {
	if !owner.locks() {
		return fmt.Errorf("activity of a %q owner does not move", owner.kind)
	}
	if err := q.LockActivityOwner(ctx, dbsqlc.LockActivityOwnerParams{OwnerKind: string(owner.kind), OwnerID: owner.id}); err != nil {
		return fmt.Errorf("lock activity owner: %w", err)
	}
	var err error
	switch owner.kind {
	case ownerDashboard:
		_, err = q.MoveDashboardActivity(ctx, dbsqlc.MoveDashboardActivityParams{FolderID: folderID, DashboardID: owner.id})
	case ownerSavedSearch:
		_, err = q.MoveSavedSearchActivity(ctx, dbsqlc.MoveSavedSearchActivityParams{FolderID: folderID, SavedSearchID: owner.id})
	case ownerObservation:
		_, err = q.MoveObservationActivity(ctx, dbsqlc.MoveObservationActivityParams{FolderID: folderID, ObservationID: owner.id})
	}
	if err != nil {
		return fmt.Errorf("move %s activity: %w", owner.kind, err)
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
