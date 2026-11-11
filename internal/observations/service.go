package observations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/links"
	"github.com/slashobserve/slashobserve/internal/origin"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const maxFolderRetries = 5

var errFolderMoved = errors.New("the observation moved to another folder")

type Frozen struct {
	Data       json.RawMessage
	UserScoped bool
}

type Freezer interface {
	Freeze(ctx context.Context, by auth.Principal, d *spec.Dashboard, panelID string, from, to time.Time) (Frozen, error)
}

type Service struct {
	store     *store
	access    *folders.Access
	freezer   Freezer
	publicURL *url.URL
	logger    *slog.Logger
}

func New(database *db.DB, access *folders.Access, freezer Freezer, publicURL *url.URL, logger *slog.Logger) *Service {
	return &Service{store: &store{db: database}, access: access, freezer: freezer, publicURL: publicURL, logger: logger}
}

func (s *Service) URL(id string) string {
	return links.Observation(s.publicURL, id)
}

func (s *Service) locate(ctx context.Context, by auth.Principal, folderID pgtype.UUID) (folders.Folder, folders.Abilities, error) {
	folder, abilities, err := s.access.Folder(ctx, by, folderID)
	switch {
	case errors.Is(err, folders.ErrNotFound), errors.Is(err, folders.ErrForbidden), err == nil && !abilities.View:
		return folders.Folder{}, folders.Abilities{}, ErrNotFound
	case err != nil:
		return folders.Folder{}, folders.Abilities{}, err
	}
	return folder, abilities, nil
}

func (s *Service) open(ctx context.Context, by auth.Principal, id pgtype.UUID) (dbsqlc.Observation, folders.Folder, folders.Abilities, error) {
	row, err := s.store.observation(ctx, id)
	if err != nil {
		return dbsqlc.Observation{}, folders.Folder{}, folders.Abilities{}, err
	}
	folder, abilities, err := s.locate(ctx, by, row.FolderID)
	return row, folder, abilities, err
}

type writeFunc func(q *dbsqlc.Queries, row dbsqlc.Observation, abilities folders.Abilities, by actor.Actor) error

func (s *Service) write(ctx context.Context, by auth.Principal, id, runID pgtype.UUID, fn writeFunc) error {
	for range maxFolderRetries {
		seen, _, abilities, err := s.open(ctx, by, id)
		if err != nil {
			return err
		}
		err = s.store.db.InTx(ctx, func(q *dbsqlc.Queries) error {
			locked, err := q.LockObservation(ctx, id)
			switch {
			case errors.Is(err, pgx.ErrNoRows):
				return ErrNotFound
			case err != nil:
				return fmt.Errorf("lock observation: %w", err)
			case locked.FolderID != seen.FolderID:
				return errFolderMoved
			}
			return fn(q, locked, abilities, actor.Of(by, origin.From(ctx), runID.String()))
		})
		if !errors.Is(err, errFolderMoved) {
			return err
		}
	}
	return fmt.Errorf("observation %s changed folders %d times in a row: %w", id, maxFolderRetries, errFolderMoved)
}

func (s *Service) Create(ctx context.Context, by auth.Principal, in CreateInput) (Observation, error) {
	title, err := validateTitle(in.Title)
	if err != nil {
		return Observation{}, err
	}
	if err := validateRange(in.From, in.To); err != nil {
		return Observation{}, err
	}
	var first *preparedBlock
	if in.FirstBlock != nil {
		prepared, err := s.prepare(ctx, by, pgtype.UUID{}, *in.FirstBlock)
		if err != nil {
			return Observation{}, err
		}
		first = &prepared
	}
	folder, err := s.access.CreationFolder(ctx, by, in.FolderID)
	switch {
	case errors.Is(err, folders.ErrNotFound):
		return Observation{}, ErrFolderNotFound
	case err != nil:
		return Observation{}, err
	}
	who := actor.Of(by, origin.From(ctx), in.RunID.String())
	cols, err := who.Columns()
	if err != nil {
		return Observation{}, err
	}
	var row dbsqlc.Observation
	err = s.store.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		var err error
		row, err = q.CreateObservation(ctx, dbsqlc.CreateObservationParams{
			FolderID:      folder.ID,
			Title:         title,
			TimeFrom:      timestamptz(in.From),
			TimeTo:        timestamptz(in.To),
			CreatedByKind: cols.Kind,
			CreatedByID:   cols.ID,
			CreatedByName: cols.Name,
			Origin:        cols.Origin,
			OriginClient:  cols.Client,
			RunID:         cols.RunID,
		})
		if err != nil {
			return err
		}
		if err := appendEvent(ctx, q, activity.TypeObservationCreated, who, row.ID, eventData{Title: row.Title}); err != nil {
			return err
		}
		if first == nil {
			return nil
		}
		_, err = insertBlock(ctx, q, row, *first, BlockLive, who)
		return err
	})
	switch {
	case db.IsForeignKeyViolation(err):
		return Observation{}, ErrFolderNotFound
	case err != nil:
		return Observation{}, fmt.Errorf("create observation: %w", err)
	}
	s.logger.InfoContext(ctx, "observation created",
		slog.String("actor_id", by.ID), slog.String("observation_id", row.ID.String()), slog.String("folder_id", folder.ID.String()))
	return s.Get(ctx, by, row.ID)
}

func (s *Service) Get(ctx context.Context, by auth.Principal, id pgtype.UUID) (Observation, error) {
	row, folder, abilities, err := s.open(ctx, by, id)
	if err != nil {
		return Observation{}, err
	}
	thread, err := s.store.thread(ctx, id)
	if err != nil {
		return Observation{}, err
	}
	blocks, err := s.store.blocks(ctx, id)
	if err != nil {
		return Observation{}, err
	}
	o := observationOf(row, folder, abilities, thread)
	viewer := actor.Of(by, origin.Origin{}, "")
	o.Blocks = make([]Block, len(blocks))
	for i, b := range blocks {
		o.Blocks[i] = blockOf(b, abilities, viewer)
	}
	return o, nil
}

func (s *Service) List(ctx context.Context, by auth.Principal, filter ListFilter, limit int, cursor string) ([]Observation, string, error) {
	after, ok := httpapi.DecodeTimeCursor(cursor)
	if !ok {
		return nil, "", ErrInvalidCursor
	}
	levels, err := s.access.Levels(ctx, by)
	if errors.Is(err, folders.ErrForbidden) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	visible := levels.Folders()
	byID := make(map[pgtype.UUID]folders.Folder, len(visible))
	ids := make([]pgtype.UUID, len(visible))
	for i, f := range visible {
		byID[f.ID] = f
		ids[i] = f.ID
	}
	if _, seen := byID[filter.FolderID]; filter.FolderID.Valid && !seen {
		return nil, "", ErrFolderNotFound
	}
	params := dbsqlc.ListObservationsParams{Folders: ids, FolderID: filter.FolderID, RowLimit: httpapi.Int32(limit + 1)}
	if filter.State != "" {
		params.State = pgtype.Text{String: string(filter.State), Valid: true}
	}
	if after != nil {
		params.CursorUpdatedAt = pgtype.Timestamptz{Time: after.Time, Valid: true}
		params.CursorID = after.ID
	}
	rows, err := s.store.db.Queries().ListObservations(ctx, params)
	if err != nil {
		return nil, "", fmt.Errorf("list observations: %w", err)
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[limit-1]
		next = httpapi.EncodeTimeCursor(last.UpdatedAt.Time, last.ID.String())
	}
	rowIDs := make([]pgtype.UUID, len(rows))
	for i, row := range rows {
		rowIDs[i] = row.ID
	}
	threads, err := s.store.threads(ctx, rowIDs)
	if err != nil {
		return nil, "", err
	}
	out := make([]Observation, len(rows))
	for i, row := range rows {
		out[i] = observationOf(row, byID[row.FolderID], levels.Of(row.FolderID), threads[row.ID])
	}
	return out, next, nil
}

func (s *Service) Update(ctx context.Context, by auth.Principal, id pgtype.UUID, in UpdateInput) (Observation, error) {
	if in.Title == nil && in.From == nil && in.To == nil && !in.ClearRange {
		return Observation{}, ErrNothingToUpdate
	}
	title, err := checkedUpdate(in)
	if err != nil {
		return Observation{}, err
	}
	err = s.write(ctx, by, id, in.RunID, func(q *dbsqlc.Queries, row dbsqlc.Observation, abilities folders.Abilities, who actor.Actor) error {
		if err := permit(Machine, State(row.State), ActionUpdate, can(abilities, false)); err != nil {
			return err
		}
		params := dbsqlc.UpdateObservationParams{ID: id, Title: row.Title, TimeFrom: row.TimeFrom, TimeTo: row.TimeTo}
		if in.Title != nil {
			params.Title = title
		}
		switch {
		case in.ClearRange:
			params.TimeFrom, params.TimeTo = pgtype.Timestamptz{}, pgtype.Timestamptz{}
		case in.From != nil:
			params.TimeFrom, params.TimeTo = timestamptz(in.From), timestamptz(in.To)
		}
		updated, err := q.UpdateObservation(ctx, params)
		if err != nil {
			return fmt.Errorf("update observation: %w", err)
		}
		return appendEvent(ctx, q, activity.TypeObservationUpdated, who, id, eventData{Title: updated.Title})
	})
	if err != nil {
		return Observation{}, err
	}
	return s.Get(ctx, by, id)
}

func checkedUpdate(in UpdateInput) (string, error) {
	var title string
	if in.Title != nil {
		var err error
		if title, err = validateTitle(*in.Title); err != nil {
			return "", err
		}
	}
	if in.ClearRange && (in.From != nil || in.To != nil) {
		return "", invalid(CodeInvalidRange, "a range is cleared or set, not both")
	}
	return title, validateRange(in.From, in.To)
}

func (s *Service) Move(ctx context.Context, by auth.Principal, id, folderID pgtype.UUID) (Observation, error) {
	current, _, abilities, err := s.open(ctx, by, id)
	if err != nil {
		return Observation{}, err
	}
	if err := permit(Machine, State(current.State), ActionMove, can(abilities, false)); err != nil {
		return Observation{}, err
	}
	target, targetAbilities, err := s.access.Folder(ctx, by, folderID)
	switch {
	case errors.Is(err, folders.ErrNotFound):
		return Observation{}, ErrFolderNotFound
	case err != nil:
		return Observation{}, err
	case !targetAbilities.Edit:
		return Observation{}, folders.ErrForbidden
	case !folders.MayMoveInto(target):
		return Observation{}, folders.ErrMoveIntoPersonal
	case current.FolderID == target.ID:
		return s.Get(ctx, by, id)
	}
	err = s.write(ctx, by, id, pgtype.UUID{}, func(q *dbsqlc.Queries, row dbsqlc.Observation, abilities folders.Abilities, who actor.Actor) error {
		if err := permit(Machine, State(row.State), ActionMove, can(abilities, false)); err != nil {
			return err
		}
		moved, err := q.MoveObservation(ctx, dbsqlc.MoveObservationParams{ID: id, FolderID: target.ID})
		switch {
		case db.IsForeignKeyViolation(err):
			return ErrFolderNotFound
		case err != nil:
			return fmt.Errorf("move observation: %w", err)
		}
		if err := activity.Move(ctx, q, activity.OnObservation(id), target.ID); err != nil {
			return err
		}
		return appendEvent(ctx, q, activity.TypeObservationMoved, who, id,
			movedEventData{Title: moved.Title, FromFolderID: row.FolderID.String(), ToFolderID: target.ID.String()})
	})
	if err != nil {
		return Observation{}, err
	}
	s.logger.InfoContext(ctx, "observation moved",
		slog.String("actor_id", by.ID), slog.String("observation_id", id.String()), slog.String("folder_id", target.ID.String()))
	return s.Get(ctx, by, id)
}

func (s *Service) Resolve(ctx context.Context, by auth.Principal, id pgtype.UUID, summary string) (Observation, error) {
	cleaned, err := validateSummary(summary)
	if err != nil {
		return Observation{}, err
	}
	err = s.write(ctx, by, id, pgtype.UUID{}, func(q *dbsqlc.Queries, row dbsqlc.Observation, abilities folders.Abilities, who actor.Actor) error {
		if err := permit(Machine, State(row.State), ActionResolve, can(abilities, false)); err != nil {
			return err
		}
		resolved, err := q.ResolveObservation(ctx, dbsqlc.ResolveObservationParams{
			ID:         id,
			Summary:    pgtype.Text{String: cleaned, Valid: true},
			FromStates: Machine.From(ActionResolve),
		})
		if err != nil {
			return fmt.Errorf("resolve observation: %w", err)
		}
		return appendEvent(ctx, q, activity.TypeObservationResolved, who, id, eventData{Title: resolved.Title})
	})
	if err != nil {
		return Observation{}, err
	}
	return s.Get(ctx, by, id)
}

func (s *Service) Reopen(ctx context.Context, by auth.Principal, id pgtype.UUID) (Observation, error) {
	err := s.write(ctx, by, id, pgtype.UUID{}, func(q *dbsqlc.Queries, row dbsqlc.Observation, abilities folders.Abilities, who actor.Actor) error {
		if err := permit(Machine, State(row.State), ActionReopen, can(abilities, false)); err != nil {
			return err
		}
		reopened, err := q.ReopenObservation(ctx, dbsqlc.ReopenObservationParams{ID: id, FromStates: Machine.From(ActionReopen)})
		if err != nil {
			return fmt.Errorf("reopen observation: %w", err)
		}
		return appendEvent(ctx, q, activity.TypeObservationReopened, who, id, eventData{Title: reopened.Title})
	})
	if err != nil {
		return Observation{}, err
	}
	return s.Get(ctx, by, id)
}

func (s *Service) Access(ctx context.Context, by auth.Principal, id pgtype.UUID) (Placement, error) {
	row, folder, abilities, err := s.open(ctx, by, id)
	if err != nil {
		return Placement{}, err
	}
	return Placement{Folder: row.FolderID, Personal: folder.Personal, Abilities: abilities, Activity: activity.OnObservation(id)}, nil
}

func (s *Service) Snapshot(ctx context.Context, by auth.Principal, id pgtype.UUID) (Snapshot, error) {
	row, err := s.store.snapshot(ctx, id)
	if err != nil {
		return Snapshot{}, err
	}
	if _, _, _, err := s.open(ctx, by, row.ObservationID); err != nil {
		return Snapshot{}, err
	}
	return snapshotOf(row), nil
}
