package observations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/origin"
)

var _ findings.Observations = (*Service)(nil)

func (s *Service) WriteConclusion(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, c findings.Conclusion) (pgtype.UUID, bool, error) {
	evidence := make([]json.RawMessage, len(c.Evidence))
	for i, panel := range c.Evidence {
		raw, err := json.Marshal(panel)
		if err != nil {
			return pgtype.UUID{}, false, fmt.Errorf("encode evidence panel: %w", err)
		}
		evidence[i] = raw
	}
	in := conclusion{runID: c.RunID, title: c.Title, markdown: c.Markdown, evidence: evidence, from: c.From, to: c.To}
	who := actor.Of(by, origin.From(ctx), c.RunID.String())
	if c.RunID.Valid {
		id, err := s.store.db.Queries().RunObservation(ctx, c.RunID)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
		case err != nil:
			return pgtype.UUID{}, false, fmt.Errorf("read run observation: %w", err)
		case id.Valid:
			return id, false, s.writeIntoObservation(ctx, q, by, who, id, in)
		}
	}
	id, err := s.createForConclusion(ctx, q, by, who, c, in)
	return id, err == nil, err
}

func (s *Service) writeIntoObservation(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, who actor.Actor, id pgtype.UUID, in conclusion) error {
	locked, err := q.LockObservation(ctx, id)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ErrNotFound
	case err != nil:
		return fmt.Errorf("lock observation: %w", err)
	}
	_, abilities, err := s.locate(ctx, by, locked.FolderID)
	if err != nil {
		return err
	}
	return s.writeConclusionBlocks(ctx, q, locked, abilities, who, in)
}

func (s *Service) createForConclusion(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, who actor.Actor, c findings.Conclusion, in conclusion) (pgtype.UUID, error) {
	place, err := s.conclusionPlace(ctx, by, c)
	if err != nil {
		return pgtype.UUID{}, err
	}
	cols, err := who.Columns()
	if err != nil {
		return pgtype.UUID{}, err
	}
	title, err := validateTitle(cutRunes(c.Title, MaxTitleRunes))
	if err != nil {
		title = defaultRunTitle
	}
	params := dbsqlc.CreateObservationParams{
		FolderID:      place.folder,
		Title:         title,
		CreatedByKind: cols.Kind,
		CreatedByID:   cols.ID,
		CreatedByName: cols.Name,
		Origin:        cols.Origin,
		OriginClient:  cols.Client,
		RunID:         cols.RunID,
	}
	if validateRange(&c.From, &c.To) == nil {
		params.TimeFrom, params.TimeTo = timestamptz(&c.From), timestamptz(&c.To)
	}
	created, err := q.CreateObservation(ctx, params)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("create observation: %w", err)
	}
	if err := appendEvent(ctx, q, activity.TypeObservationCreated, who, created.ID, eventData{Title: created.Title}); err != nil {
		return pgtype.UUID{}, err
	}
	if err := s.writeConclusionBlocks(ctx, q, created, place.abilities, who, in); err != nil {
		return pgtype.UUID{}, err
	}
	if c.RunID.Valid {
		if _, err := q.SetRequestObservation(ctx, dbsqlc.SetRequestObservationParams{ID: c.RunID, ObservationID: created.ID}); err != nil {
			return pgtype.UUID{}, fmt.Errorf("link run to observation: %w", err)
		}
	}
	return created.ID, nil
}

func (s *Service) conclusionPlace(ctx context.Context, by auth.Principal, c findings.Conclusion) (runPlace, error) {
	if !c.FolderID.Valid && c.DashboardID.Valid {
		place, ok, err := s.dashboardPlace(ctx, by, c.DashboardID)
		if err != nil || ok {
			return place, err
		}
	}
	folder, err := s.access.CreationFolder(ctx, by, c.FolderID)
	switch {
	case errors.Is(err, folders.ErrNotFound):
		return runPlace{}, findings.ErrFolderNotFound
	case err != nil:
		return runPlace{}, err
	}
	_, abilities, err := s.access.Folder(ctx, by, folder.ID)
	if err != nil {
		return runPlace{}, err
	}
	return runPlace{folder: folder.ID, abilities: abilities}, nil
}

func (s *Service) Placement(ctx context.Context, by auth.Principal, id pgtype.UUID) (folders.Folder, folders.Abilities, error) {
	_, folder, abilities, err := s.open(ctx, by, id)
	if errors.Is(err, ErrNotFound) {
		return folders.Folder{}, folders.Abilities{}, findings.ErrNotFound
	}
	return folder, abilities, err
}
