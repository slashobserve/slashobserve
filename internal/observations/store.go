package observations

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
)

type store struct {
	db *db.DB
}

func (s *store) observation(ctx context.Context, id pgtype.UUID) (dbsqlc.Observation, error) {
	row, err := s.db.Queries().ObservationByID(ctx, id)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return dbsqlc.Observation{}, ErrNotFound
	case err != nil:
		return dbsqlc.Observation{}, fmt.Errorf("get observation: %w", err)
	}
	return row, nil
}

func (s *store) block(ctx context.Context, id pgtype.UUID) (dbsqlc.ObservationBlock, error) {
	row, err := s.db.Queries().BlockByID(ctx, id)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return dbsqlc.ObservationBlock{}, ErrNotFound
	case err != nil:
		return dbsqlc.ObservationBlock{}, fmt.Errorf("get observation block: %w", err)
	}
	return row, nil
}

func (s *store) snapshot(ctx context.Context, id pgtype.UUID) (dbsqlc.Snapshot, error) {
	row, err := s.db.Queries().SnapshotByID(ctx, id)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return dbsqlc.Snapshot{}, ErrNotFound
	case err != nil:
		return dbsqlc.Snapshot{}, fmt.Errorf("get snapshot: %w", err)
	}
	return row, nil
}

func (s *store) blocks(ctx context.Context, observationID pgtype.UUID) ([]dbsqlc.ObservationBlock, error) {
	rows, err := s.db.Queries().BlocksOfObservation(ctx, observationID)
	if err != nil {
		return nil, fmt.Errorf("list observation blocks: %w", err)
	}
	return rows, nil
}

func (s *store) thread(ctx context.Context, observationID pgtype.UUID) (pgtype.UUID, error) {
	id, err := s.db.Queries().ObservationThread(ctx, observationID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return pgtype.UUID{}, nil
	case err != nil:
		return pgtype.UUID{}, fmt.Errorf("get observation thread: %w", err)
	}
	return id, nil
}

func (s *store) threads(ctx context.Context, observationIDs []pgtype.UUID) (map[pgtype.UUID]pgtype.UUID, error) {
	rows, err := s.db.Queries().ObservationThreads(ctx, observationIDs)
	if err != nil {
		return nil, fmt.Errorf("list observation threads: %w", err)
	}
	out := make(map[pgtype.UUID]pgtype.UUID, len(rows))
	for _, r := range rows {
		out[r.ObservationID] = r.ID
	}
	return out, nil
}

func appendEvent(ctx context.Context, q *dbsqlc.Queries, typ string, by actor.Actor, id pgtype.UUID, data any) error {
	_, err := activity.Append(ctx, q, activity.Entry{
		Type:     typ,
		TargetID: id,
		Owner:    activity.OnObservation(id),
		Actor:    by,
		Data:     data,
	}, nil)
	return err
}

type eventData struct {
	Title string `json:"title"`
}

type movedEventData struct {
	Title        string `json:"title"`
	FromFolderID string `json:"from_folder_id"`
	ToFolderID   string `json:"to_folder_id"`
}

type blockEventData struct {
	Title     string     `json:"title"`
	BlockID   string     `json:"block_id"`
	BlockKind Kind       `json:"block_kind"`
	State     BlockState `json:"state,omitempty"`
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	utc := t.Time.UTC()
	return &utc
}

func timestamptz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

func actorOf(kind string, id pgtype.UUID, name string, origin, client pgtype.Text, runID pgtype.UUID) actor.Actor {
	return actor.FromColumns(actor.Columns{Kind: kind, ID: id, Name: name, Origin: origin, Client: client, RunID: runID})
}

func observationOf(row dbsqlc.Observation, folder folders.Folder, abilities folders.Abilities, thread pgtype.UUID) Observation {
	return Observation{
		ID:             row.ID.String(),
		FolderID:       row.FolderID.String(),
		Title:          row.Title,
		State:          State(row.State),
		Summary:        row.Summary.String,
		From:           timePtr(row.TimeFrom),
		To:             timePtr(row.TimeTo),
		ThreadID:       thread.String(),
		RunID:          row.RunID.String(),
		Actor:          actorOf(row.CreatedByKind, row.CreatedByID, row.CreatedByName, row.Origin, row.OriginClient, row.RunID),
		CreatedAt:      row.CreatedAt.Time.UTC(),
		UpdatedAt:      row.UpdatedAt.Time.UTC(),
		ResolvedAt:     timePtr(row.ResolvedAt),
		Folder:         folder,
		Abilities:      abilities,
		AllowedActions: allowedActions(State(row.State), abilities),
	}
}

func blockOf(row dbsqlc.ObservationBlock, abilities folders.Abilities, viewer actor.Actor) Block {
	author := actorOf(row.CreatedByKind, row.CreatedByID, row.CreatedByName, row.Origin, row.OriginClient, row.RunID)
	mine := author.Kind == viewer.Kind && author.ID == viewer.ID
	return Block{
		ID:             row.ID.String(),
		ObservationID:  row.ObservationID.String(),
		Kind:           Kind(row.Kind),
		State:          BlockState(row.State),
		Position:       int(row.Position),
		Width:          Width(row.Width),
		Version:        int(row.Version),
		Body:           row.Body,
		SnapshotID:     row.SnapshotID.String(),
		RunID:          row.RunID.String(),
		Actor:          author,
		CreatedAt:      row.CreatedAt.Time.UTC(),
		UpdatedAt:      row.UpdatedAt.Time.UTC(),
		AllowedActions: allowedBlockActions(BlockState(row.State), abilities, mine),
	}
}

func snapshotOf(row dbsqlc.Snapshot) Snapshot {
	return Snapshot{
		ID:            row.ID.String(),
		ObservationID: row.ObservationID.String(),
		Panel:         row.Panel,
		Query:         row.Query,
		Data:          row.Frames,
		UserScoped:    row.UserScoped,
		From:          row.TimeFrom.Time.UTC(),
		To:            row.TimeTo.Time.UTC(),
		Actor:         actorOf(row.CreatedByKind, row.CreatedByID, row.CreatedByName, row.Origin, row.OriginClient, row.RunID),
		CreatedAt:     row.CreatedAt.Time.UTC(),
	}
}
