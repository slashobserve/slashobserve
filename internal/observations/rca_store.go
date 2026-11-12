package observations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/httpapi"
)

func (s *store) rca(ctx context.Context, observationID pgtype.UUID, revision int) (dbsqlc.ObservationRca, error) {
	if revision < 1 || revision > math.MaxInt32 {
		return dbsqlc.ObservationRca{}, ErrRevisionNotFound
	}
	row, err := s.db.Queries().RCAByRevision(ctx, dbsqlc.RCAByRevisionParams{ObservationID: observationID, Revision: httpapi.Int32(revision)})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return dbsqlc.ObservationRca{}, ErrRevisionNotFound
	case err != nil:
		return dbsqlc.ObservationRca{}, fmt.Errorf("get RCA: %w", err)
	}
	return row, nil
}

func (s *Service) insertRCA(ctx context.Context, q *dbsqlc.Queries, parent dbsqlc.Observation, live []dbsqlc.ObservationBlock, frozen map[pgtype.UUID]frozenSnapshot, who actor.Actor) (dbsqlc.ObservationRca, error) {
	current, err := q.BlocksOfObservation(ctx, parent.ID)
	if err != nil {
		return dbsqlc.ObservationRca{}, fmt.Errorf("list observation blocks: %w", err)
	}
	if !sameBlocks(publishable(current), live) {
		return dbsqlc.ObservationRca{}, ErrObservationChanged
	}
	next, err := q.NextRCARevision(ctx, parent.ID)
	if err != nil {
		return dbsqlc.ObservationRca{}, fmt.Errorf("next RCA revision: %w", err)
	}
	if next.Total >= MaxRCARevisions {
		return dbsqlc.ObservationRca{}, ErrRCALimit
	}
	cols, err := who.Columns()
	if err != nil {
		return dbsqlc.ObservationRca{}, err
	}
	refs := make(map[pgtype.UUID]snapshotRef, len(live))
	for _, b := range live {
		switch Kind(b.Kind) {
		case KindPanel:
			snap := frozen[b.ID]
			inserted, err := q.InsertSnapshot(ctx, dbsqlc.InsertSnapshotParams{
				ObservationID: parent.ID,
				Panel:         snap.panel,
				Query:         snap.query,
				Frames:        snap.frames,
				UserScoped:    snap.userScoped,
				TimeFrom:      pgtype.Timestamptz{Time: snap.from, Valid: true},
				TimeTo:        pgtype.Timestamptz{Time: snap.to, Valid: true},
				CreatedByKind: cols.Kind,
				CreatedByID:   cols.ID,
				CreatedByName: cols.Name,
				Origin:        cols.Origin,
				OriginClient:  cols.Client,
				RunID:         cols.RunID,
			})
			if err != nil {
				return dbsqlc.ObservationRca{}, fmt.Errorf("insert snapshot: %w", err)
			}
			refs[b.ID] = snapshotRef{id: inserted.ID, panel: inserted.Panel}
		case KindSnapshot:
			existing, err := q.SnapshotByID(ctx, b.SnapshotID)
			if err != nil {
				return dbsqlc.ObservationRca{}, fmt.Errorf("read snapshot of block %s: %w", b.ID, err)
			}
			refs[b.ID] = snapshotRef{id: existing.ID, panel: existing.Panel}
		}
	}
	blocks, err := rcaBlocks(live, refs)
	if err != nil {
		return dbsqlc.ObservationRca{}, err
	}
	content, err := json.Marshal(rcaContent{Blocks: blocks})
	if err != nil {
		return dbsqlc.ObservationRca{}, fmt.Errorf("encode RCA content: %w", err)
	}
	row, err := q.InsertRCA(ctx, dbsqlc.InsertRCAParams{
		ObservationID:    parent.ID,
		Revision:         next.Revision,
		Title:            parent.Title,
		Summary:          parent.Summary,
		ObservationState: parent.State,
		TimeFrom:         parent.TimeFrom,
		TimeTo:           parent.TimeTo,
		Content:          content,
		CreatedByKind:    cols.Kind,
		CreatedByID:      cols.ID,
		CreatedByName:    cols.Name,
		Origin:           cols.Origin,
		OriginClient:     cols.Client,
		RunID:            cols.RunID,
	})
	if err != nil {
		return dbsqlc.ObservationRca{}, fmt.Errorf("insert RCA: %w", err)
	}
	err = appendEvent(ctx, q, activity.TypeObservationPublished, who, parent.ID, publishedEventData{Title: parent.Title, Revision: int(row.Revision)})
	return row, err
}

func rcaOf(row dbsqlc.ObservationRca) (RCA, error) {
	var content rcaContent
	if err := json.Unmarshal(row.Content, &content); err != nil {
		return RCA{}, fmt.Errorf("decode RCA content: %w", err)
	}
	if content.Blocks == nil {
		content.Blocks = []RCABlock{}
	}
	return RCA{
		ID:            row.ID.String(),
		ObservationID: row.ObservationID.String(),
		Revision:      int(row.Revision),
		Title:         row.Title,
		Summary:       row.Summary.String,
		State:         State(row.ObservationState),
		From:          timePtr(row.TimeFrom),
		To:            timePtr(row.TimeTo),
		Blocks:        content.Blocks,
		Actor:         actorOf(row.CreatedByKind, row.CreatedByID, row.CreatedByName, row.Origin, row.OriginClient, row.RunID),
		CreatedAt:     row.CreatedAt.Time.UTC(),
	}, nil
}
