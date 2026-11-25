package savedsearches

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/placement"
)

type store struct {
	db    *db.DB
	guard *placement.Guard
}

func (s *store) place(ctx context.Context, q *dbsqlc.Queries, row dbsqlc.SavedSearch, moved bool) error {
	refs := []string{row.DatasourceUid}
	if moved {
		return s.guard.Move(ctx, q, folders.ContentSavedSearch, row.FolderID, refs)
	}
	return s.guard.Save(ctx, q, folders.ContentSavedSearch, row.FolderID, refs)
}

type eventData struct {
	Title   string   `json:"title"`
	Kind    Kind     `json:"kind"`
	Changed []string `json:"changed,omitempty"`
}

type movedEventData struct {
	eventData
	FromFolderID string `json:"from_folder_id"`
	ToFolderID   string `json:"to_folder_id"`
}

func appendEvent(ctx context.Context, q *dbsqlc.Queries, typ string, by actor.Actor, row dbsqlc.SavedSearch, data any) error {
	_, err := activity.Append(ctx, q, activity.Entry{
		Type:     typ,
		TargetID: row.ID,
		Owner:    activity.OnSavedSearch(row.ID),
		Actor:    by,
		Data:     data,
	}, nil)
	return err
}

func (s *store) insert(ctx context.Context, by actor.Actor, in validated, folderID pgtype.UUID) (dbsqlc.SavedSearch, error) {
	cols, err := by.Columns()
	if err != nil {
		return dbsqlc.SavedSearch{}, err
	}
	filters, err := json.Marshal(in.filters)
	if err != nil {
		return dbsqlc.SavedSearch{}, fmt.Errorf("encode filters: %w", err)
	}
	params := dbsqlc.InsertSavedSearchParams{
		Kind:          string(in.Kind),
		Title:         in.Title,
		DatasourceUid: in.DatasourceUID,
		Filters:       filters,
		TimeFrom:      in.From,
		TimeTo:        in.To,
		FolderID:      folderID,
		CreatedByKind: cols.Kind,
		CreatedByID:   cols.ID,
		CreatedByName: cols.Name,
		Origin:        cols.Origin,
		OriginClient:  cols.Client,
	}
	if in.Query != nil {
		params.Query = pgtype.Text{String: *in.Query, Valid: true}
	}
	var row dbsqlc.SavedSearch
	err = s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		if err := placement.RunWrite(ctx, q, by, folderID); err != nil {
			return err
		}
		if err := s.guard.Save(ctx, q, folders.ContentSavedSearch, folderID, []string{in.DatasourceUID}); err != nil {
			return err
		}
		var err error
		if row, err = q.InsertSavedSearch(ctx, params); err != nil {
			return err
		}
		return appendEvent(ctx, q, activity.TypeSavedSearchCreated, by, row, eventData{Title: row.Title, Kind: in.Kind})
	})
	switch {
	case db.IsForeignKeyViolation(err):
		return dbsqlc.SavedSearch{}, ErrFolderNotFound
	case err != nil:
		return dbsqlc.SavedSearch{}, fmt.Errorf("insert saved search: %w", err)
	}
	return row, nil
}

func (s *store) get(ctx context.Context, id pgtype.UUID) (dbsqlc.SavedSearch, error) {
	row, err := s.db.Queries().GetSavedSearch(ctx, id)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return dbsqlc.SavedSearch{}, ErrNotFound
	case err != nil:
		return dbsqlc.SavedSearch{}, fmt.Errorf("get saved search: %w", err)
	}
	return row, nil
}

func (s *store) list(ctx context.Context, visible []pgtype.UUID, filter ListFilter, after *httpapi.TimeCursor, rows int) ([]dbsqlc.SavedSearch, error) {
	params := dbsqlc.ListSavedSearchesParams{Folders: visible, FolderID: filter.FolderID, RowLimit: httpapi.Int32(rows)}
	if filter.Kind != "" {
		params.Kind = pgtype.Text{String: string(filter.Kind), Valid: true}
	}
	if after != nil {
		params.CursorUpdatedAt = pgtype.Timestamptz{Time: after.Time, Valid: true}
		params.CursorID = after.ID
	}
	found, err := s.db.Queries().ListSavedSearches(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list saved searches: %w", err)
	}
	return found, nil
}

type change struct {
	title   *string
	moveTo  pgtype.UUID
	content *contentChange
}

func (c change) target(before dbsqlc.SavedSearch) pgtype.UUID {
	if c.moveTo.Valid {
		return c.moveTo
	}
	return before.FolderID
}

func (c change) fields() []string {
	var fields []string
	if c.title != nil {
		fields = append(fields, "title")
	}
	if c.content != nil {
		fields = append(fields, c.content.parts...)
	}
	return fields
}

func (s *store) update(ctx context.Context, by actor.Actor, id, checkedFolder pgtype.UUID, c change) (dbsqlc.SavedSearch, error) {
	var row dbsqlc.SavedSearch
	err := s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		before, err := q.LockSavedSearch(ctx, id)
		if err != nil {
			return err
		}
		if before.FolderID != checkedFolder {
			return errFolderMoved
		}
		if err := placement.RunWrite(ctx, q, by, c.target(before)); err != nil {
			return err
		}
		if row, err = applyChange(ctx, q, by, before, c); err != nil {
			return err
		}
		if c.content == nil && !c.moveTo.Valid {
			return nil
		}
		return s.place(ctx, q, row, c.moveTo.Valid)
	})
	switch {
	case errors.Is(err, errFolderMoved):
		return dbsqlc.SavedSearch{}, err
	case errors.Is(err, pgx.ErrNoRows):
		return dbsqlc.SavedSearch{}, ErrNotFound
	case db.IsForeignKeyViolation(err):
		return dbsqlc.SavedSearch{}, ErrFolderNotFound
	case err != nil:
		return dbsqlc.SavedSearch{}, fmt.Errorf("update saved search: %w", err)
	}
	return row, nil
}

func applyChange(ctx context.Context, q *dbsqlc.Queries, by actor.Actor, before dbsqlc.SavedSearch, c change) (dbsqlc.SavedSearch, error) {
	row, err := writeChange(ctx, q, before, c)
	if err != nil {
		return row, err
	}
	if fields := c.fields(); len(fields) > 0 {
		if err := appendEvent(ctx, q, activity.TypeSavedSearchUpdated, by, row, eventData{Title: row.Title, Kind: Kind(row.Kind), Changed: fields}); err != nil {
			return row, err
		}
	}
	if c.moveTo.Valid {
		data := movedEventData{eventData: eventData{Title: row.Title, Kind: Kind(row.Kind)}, FromFolderID: before.FolderID.String(), ToFolderID: c.moveTo.String()}
		return row, appendEvent(ctx, q, activity.TypeSavedSearchMoved, by, row, data)
	}
	return row, nil
}

func writeChange(ctx context.Context, q *dbsqlc.Queries, before dbsqlc.SavedSearch, c change) (dbsqlc.SavedSearch, error) {
	row, err := before, error(nil)
	if c.content != nil {
		if row, err = q.ChangeSavedSearchContent(ctx, c.content.params(before.ID)); err != nil {
			return row, err
		}
	}
	if c.title != nil {
		if row, err = q.RenameSavedSearch(ctx, dbsqlc.RenameSavedSearchParams{ID: before.ID, Title: *c.title}); err != nil {
			return row, err
		}
	}
	if c.moveTo.Valid {
		if row, err = q.MoveSavedSearch(ctx, dbsqlc.MoveSavedSearchParams{ID: before.ID, FolderID: c.moveTo}); err != nil {
			return row, err
		}
		err = activity.Move(ctx, q, activity.OnSavedSearch(before.ID), c.moveTo)
	}
	return row, err
}

func (s *store) delete(ctx context.Context, by actor.Actor, id pgtype.UUID, authorize func(folderID pgtype.UUID) error) error {
	err := s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		row, err := q.LockSavedSearch(ctx, id)
		if err != nil {
			return err
		}
		if err := authorize(row.FolderID); err != nil {
			return err
		}
		if err := appendEvent(ctx, q, activity.TypeSavedSearchDeleted, by, row, eventData{Title: row.Title, Kind: Kind(row.Kind)}); err != nil {
			return err
		}
		_, err = q.DeleteSavedSearch(ctx, id)
		return err
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ErrNotFound
	case err != nil:
		return fmt.Errorf("delete saved search: %w", err)
	}
	return nil
}

func savedSearchOf(r dbsqlc.SavedSearch) (SavedSearch, error) {
	s := SavedSearch{
		ID:            r.ID.String(),
		Kind:          Kind(r.Kind),
		Title:         r.Title,
		DatasourceUID: r.DatasourceUid,
		From:          r.TimeFrom,
		To:            r.TimeTo,
		FolderID:      r.FolderID.String(),
		CreatedAt:     r.CreatedAt.Time.UTC(),
		UpdatedAt:     r.UpdatedAt.Time.UTC(),
		Actor: actor.FromColumns(actor.Columns{
			Kind: r.CreatedByKind, ID: r.CreatedByID, Name: r.CreatedByName, Origin: r.Origin, Client: r.OriginClient,
		}),
	}
	if r.Query.Valid {
		s.Query = &r.Query.String
	}
	if err := json.Unmarshal(r.Filters, &s.Filters); err != nil {
		return SavedSearch{}, fmt.Errorf("decode filters of saved search %s: %w", s.ID, err)
	}
	return s, nil
}
