package observations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strings"
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
	"github.com/slashobserve/slashobserve/internal/placement"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const maxFolderRetries = 5

var errFolderMoved = errors.New("the observation moved to another folder")

type Frozen struct {
	Data        json.RawMessage
	UserScoped  bool
	Datasources []string
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

	dashboards  Dashboards
	annotations Annotations
	guard       *placement.Guard
}

func New(database *db.DB, access *folders.Access, freezer Freezer, publicURL *url.URL, logger *slog.Logger, options ...Option) *Service {
	s := &Service{store: &store{db: database}, access: access, freezer: freezer, publicURL: publicURL, logger: logger, guard: placement.New(nil)}
	for _, option := range options {
		option(s)
	}
	return s
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
			return fn(q, locked, abilities, actor.In(ctx, by, runID.String()))
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
	prepared := make([]preparedBlock, len(in.Blocks))
	for i, b := range in.Blocks {
		block, err := s.prepare(ctx, by, folders.Folder{}, pgtype.UUID{}, b, anyBlock)
		if err != nil {
			return Observation{}, &BlockError{Index: i, Err: err}
		}
		prepared[i] = block
	}
	folder, err := s.access.CreationFolder(ctx, by, in.FolderID)
	switch {
	case errors.Is(err, folders.ErrNotFound):
		return Observation{}, ErrFolderNotFound
	case err != nil:
		return Observation{}, err
	}
	who := actor.In(ctx, by, in.RunID.String())
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
		for i, block := range prepared {
			if _, err := s.insertBlock(ctx, q, row, block, BlockLive, who); err != nil {
				return &BlockError{Index: i, Err: err}
			}
		}
		return nil
	})
	var refused *BlockError
	switch {
	case errors.As(err, &refused):
		return Observation{}, err
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
	counts, err := s.store.messageCounts(ctx, []pgtype.UUID{id})
	if err != nil {
		return Observation{}, err
	}
	blocks, err := s.store.blocks(ctx, id)
	if err != nil {
		return Observation{}, err
	}
	viewer := actor.Of(by, origin.Origin{}, "")
	o := observationOf(row, folder, abilities, viewer, thread, counts[id])
	about, err := s.runDashboard(ctx, id)
	if err != nil {
		return Observation{}, err
	}
	if about.Valid {
		o.DashboardID = about.String()
		if o.Pinned, err = s.pinned(ctx, id, about); err != nil {
			return Observation{}, err
		}
		if o.CanPin, err = s.mayEdit(ctx, by, about); err != nil {
			return Observation{}, err
		}
	}
	o.Blocks = shownBlocks(blocks, abilities, viewer)
	return o, nil
}

func (s *Service) List(ctx context.Context, by auth.Principal, filter ListFilter, limit int, cursor string) ([]Observation, string, error) {
	after, ok := httpapi.DecodeTimeCursor(cursor)
	if !ok {
		return nil, "", ErrInvalidCursor
	}
	if after != nil && !filter.newestFirst() {
		return nil, "", ErrCursorOrder
	}
	rows, _, err := s.list(ctx, by, filter, limit+1, 0, after, false)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		if filter.newestFirst() {
			last := rows[limit-1]
			next = httpapi.EncodeTimeCursor(last.UpdatedAt, last.ID)
		}
	}
	return rows, next, nil
}

func (s *Service) Page(ctx context.Context, by auth.Principal, filter ListFilter, limit, offset int) (Listing, error) {
	rows, total, err := s.list(ctx, by, filter, limit, offset, nil, true)
	if err != nil {
		return Listing{}, err
	}
	return Listing{Observations: rows, Total: total}, nil
}

func (f ListFilter) newestFirst() bool {
	return (f.Sort == "" || f.Sort == SortUpdated) && !f.Ascending
}

func likePattern(query string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.TrimSpace(query))
}

func (f ListFilter) where(folderIDs []pgtype.UUID, viewer actor.Actor) (dbsqlc.CountObservationsParams, error) {
	where := dbsqlc.CountObservationsParams{Folders: folderIDs, FolderID: f.FolderID}
	if f.State != "" {
		where.State = pgtype.Text{String: string(f.State), Valid: true}
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		where.TitleQuery = pgtype.Text{String: likePattern(q), Valid: true}
	}
	if f.CreatedByMe {
		cols, err := viewer.Columns()
		if err != nil {
			return where, err
		}
		where.CreatedByID = cols.ID
		where.CreatedByKind = pgtype.Text{String: cols.Kind, Valid: true}
	}
	return where, nil
}

func (s *Service) list(ctx context.Context, by auth.Principal, filter ListFilter, limit, offset int, after *httpapi.TimeCursor, counted bool) ([]Observation, int, error) {
	levels, err := s.access.Levels(ctx, by)
	if errors.Is(err, folders.ErrForbidden) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	visible := levels.Folders()
	byID := make(map[pgtype.UUID]folders.Folder, len(visible))
	ids := make([]pgtype.UUID, len(visible))
	for i, f := range visible {
		byID[f.ID] = f
		ids[i] = f.ID
	}
	if _, seen := byID[filter.FolderID]; filter.FolderID.Valid && !seen {
		return nil, 0, ErrFolderNotFound
	}
	viewer := actor.Of(by, origin.Origin{}, "")
	sort := filter.Sort
	if sort == "" {
		sort = SortUpdated
	}
	where, err := filter.where(ids, viewer)
	if err != nil {
		return nil, 0, err
	}
	params := dbsqlc.ListObservationsParams{
		Folders: where.Folders, FolderID: where.FolderID, State: where.State, TitleQuery: where.TitleQuery,
		CreatedByID: where.CreatedByID, CreatedByKind: where.CreatedByKind,
		Sort: string(sort), Descending: !filter.Ascending, RowLimit: httpapi.Int32(limit), RowOffset: httpapi.Int32(offset),
	}
	if after != nil {
		params.CursorUpdatedAt = pgtype.Timestamptz{Time: after.Time, Valid: true}
		params.CursorID = after.ID
	}
	queries := s.store.db.Queries()
	rows, err := queries.ListObservations(ctx, params)
	if err != nil {
		return nil, 0, fmt.Errorf("list observations: %w", err)
	}
	total := 0
	if counted {
		n, err := queries.CountObservations(ctx, where)
		if err != nil {
			return nil, 0, fmt.Errorf("count observations: %w", err)
		}
		total = int(n)
	}
	out, err := s.render(ctx, rows, byID, levels, viewer)
	return out, total, err
}

func (s *Service) render(ctx context.Context, rows []dbsqlc.Observation, byID map[pgtype.UUID]folders.Folder, levels folders.Levels, viewer actor.Actor) ([]Observation, error) {
	rowIDs := make([]pgtype.UUID, len(rows))
	for i, row := range rows {
		rowIDs[i] = row.ID
	}
	threads, err := s.store.threads(ctx, rowIDs)
	if err != nil {
		return nil, err
	}
	counts, err := s.store.messageCounts(ctx, rowIDs)
	if err != nil {
		return nil, err
	}
	out := make([]Observation, len(rows))
	for i, row := range rows {
		out[i] = observationOf(row, byID[row.FolderID], levels.Of(row.FolderID), viewer, threads[row.ID], counts[row.ID])
	}
	return out, nil
}

func (s *Service) Delete(ctx context.Context, by auth.Principal, id pgtype.UUID) error {
	var pinned []pgtype.UUID
	err := s.write(ctx, by, id, pgtype.UUID{}, func(q *dbsqlc.Queries, row dbsqlc.Observation, abilities folders.Abilities, who actor.Actor) error {
		author := string(who.Kind) == row.CreatedByKind && who.ID == row.CreatedByID.String()
		if !slices.Contains(allowedActions(State(row.State), abilities, author), ActionDelete) {
			return folders.ErrForbidden
		}
		var err error
		if pinned, err = q.ListObservationPinAnnotations(ctx, id); err != nil {
			return fmt.Errorf("list the pins of an observation: %w", err)
		}
		active, err := q.ObservationHasActiveRun(ctx, id)
		switch {
		case err != nil:
			return fmt.Errorf("check the observation's runs: %w", err)
		case active:
			return ErrRunInProgress
		}
		if err := appendEvent(ctx, q, activity.TypeObservationDeleted, who, id, eventData{Title: row.Title}); err != nil {
			return err
		}
		n, err := q.DeleteObservation(ctx, id)
		switch {
		case err != nil:
			return fmt.Errorf("delete observation: %w", err)
		case n != 1:
			return ErrNotFound
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.unpin(ctx, by, id, pinned)
	s.logger.InfoContext(ctx, "observation deleted", slog.String("actor_id", by.ID), slog.String("observation_id", id.String()))
	return nil
}

func updatedEvent(before, after string) any {
	if before == after {
		return eventData{Title: after}
	}
	return renamedEventData{Title: after, FromTitle: before}
}

func (s *Service) Update(ctx context.Context, by auth.Principal, id pgtype.UUID, in UpdateInput) (Observation, error) {
	if in.Title == nil && in.From == nil && in.To == nil && !in.ClearRange {
		return Observation{}, ErrNothingToUpdate
	}
	title, err := checkedUpdate(in)
	if err != nil {
		return Observation{}, err
	}
	var chat revisions.SharedTitle
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
		if err := s.saveUpdate(ctx, q, row, who, params); err != nil {
			return err
		}
		if in.Title == nil {
			return nil
		}
		var err error
		chat, err = renameChat(ctx, q, by, id, title)
		return err
	})
	if err != nil {
		return Observation{}, err
	}
	o, err := s.Get(ctx, by, id)
	o.ChatRename = chat
	return o, err
}

func (s *Service) saveUpdate(ctx context.Context, q *dbsqlc.Queries, row dbsqlc.Observation, who actor.Actor, params dbsqlc.UpdateObservationParams) error {
	updated, err := q.UpdateObservation(ctx, params)
	if err != nil {
		return fmt.Errorf("update observation: %w", err)
	}
	return appendEvent(ctx, q, activity.TypeObservationUpdated, who, row.ID, updatedEvent(row.Title, updated.Title))
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
	moved, err := s.move(ctx, by, id, folderID)
	if err != nil {
		return Observation{}, err
	}
	if moved {
		s.logger.InfoContext(ctx, "observation moved",
			slog.String("actor_id", by.ID), slog.String("observation_id", id.String()), slog.String("folder_id", folderID.String()))
	}
	return s.Get(ctx, by, id)
}

func (s *Service) move(ctx context.Context, by auth.Principal, id, folderID pgtype.UUID) (bool, error) {
	current, _, abilities, err := s.open(ctx, by, id)
	if err != nil {
		return false, err
	}
	if err := permit(Machine, State(current.State), ActionMove, can(abilities, false)); err != nil {
		return false, err
	}
	target, targetAbilities, err := s.access.Folder(ctx, by, folderID)
	switch {
	case errors.Is(err, folders.ErrNotFound):
		return false, ErrFolderNotFound
	case err != nil:
		return false, err
	case !targetAbilities.Edit:
		return false, folders.ErrForbidden
	case !folders.MayMoveInto(target):
		return false, folders.ErrMoveIntoPersonal
	case current.FolderID == target.ID:
		return false, nil
	}
	err = s.write(ctx, by, id, pgtype.UUID{}, func(q *dbsqlc.Queries, row dbsqlc.Observation, abilities folders.Abilities, who actor.Actor) error {
		if err := permit(Machine, State(row.State), ActionMove, can(abilities, false)); err != nil {
			return err
		}
		if err := s.guard.MoveObservation(ctx, q, id, target.ID); err != nil {
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
	return err == nil, err
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
	return Placement{Folder: row.FolderID, Personal: folder.Personal, AdminsOnly: folder.AdminsOnly, Title: folder.Title, Abilities: abilities, Activity: activity.OnObservation(id)}, nil
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
