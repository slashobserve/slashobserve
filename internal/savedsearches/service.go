package savedsearches

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/placement"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const maxFolderRetries = 5

var errFolderMoved = errors.New("the saved search changed folders after its access was checked")

type Service struct {
	store     *store
	access    *folders.Access
	sources   *datasource.Registry
	publicURL *url.URL
	logger    *slog.Logger
}

func New(database *db.DB, access *folders.Access, sources *datasource.Registry, publicURL *url.URL, logger *slog.Logger) *Service {
	return &Service{store: &store{db: database, guard: placement.New(sources)}, access: access, sources: sources, publicURL: publicURL, logger: logger}
}

func (s *Service) Create(ctx context.Context, by auth.Principal, in CreateInput) (SavedSearch, error) {
	checked, err := validate(in, time.Now())
	if err != nil {
		return SavedSearch{}, err
	}
	if err := s.checkDatasource(ctx, checked); err != nil {
		return SavedSearch{}, err
	}
	folder, err := s.access.CreationFolder(ctx, by, in.FolderID)
	if errors.Is(err, folders.ErrNotFound) {
		return SavedSearch{}, ErrFolderNotFound
	}
	if err != nil {
		return SavedSearch{}, err
	}
	row, err := s.store.insert(ctx, actor.In(ctx, by, ""), checked, folder.ID)
	if err != nil {
		return SavedSearch{}, err
	}
	s.logger.InfoContext(ctx, "saved search created",
		slog.String("actor_id", by.ID), slog.String("saved_search_id", row.ID.String()), slog.String("folder_id", folder.ID.String()))
	return s.render(ctx, by, row)
}

func (s *Service) checkDatasource(ctx context.Context, in validated) error {
	inst, err := s.sources.For(ctx, in.DatasourceUID)
	if err != nil {
		return &InvalidInputError{Code: CodeUnknownDatasource, Message: "datasource " + in.DatasourceUID + " does not exist"}
	}
	return fitDatasource(in.Kind, spec.DatasourceType(inst.Datasource.Type()))
}

func (s *Service) List(ctx context.Context, by auth.Principal, filter ListFilter, limit int, cursor string) ([]SavedSearch, string, error) {
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
	rows, err := s.store.list(ctx, ids, filter, after, limit+1)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[limit-1]
		next = httpapi.EncodeTimeCursor(last.UpdatedAt.Time, last.ID.String())
	}
	out := make([]SavedSearch, len(rows))
	for i, row := range rows {
		if out[i], err = s.item(row, byID[row.FolderID], levels.Of(row.FolderID)); err != nil {
			return nil, "", err
		}
	}
	return out, next, nil
}

func (s *Service) Update(ctx context.Context, by auth.Principal, id pgtype.UUID, in UpdateInput) (SavedSearch, error) {
	if in.Title == nil && !in.FolderID.Valid && !in.changesContent() {
		return SavedSearch{}, ErrNothingToUpdate
	}
	var title *string
	if in.Title != nil {
		cleaned, err := cleanTitle(*in.Title)
		if err != nil {
			return SavedSearch{}, err
		}
		title = &cleaned
	}
	for range maxFolderRetries {
		updated, err := s.updateOnce(ctx, by, id, title, in)
		if !errors.Is(err, errFolderMoved) {
			return updated, err
		}
	}
	return SavedSearch{}, fmt.Errorf("saved search %s changed folders %d times in a row: %w", id, maxFolderRetries, errFolderMoved)
}

func (s *Service) updateOnce(ctx context.Context, by auth.Principal, id pgtype.UUID, title *string, in UpdateInput) (SavedSearch, error) {
	current, err := s.store.get(ctx, id)
	if err != nil {
		return SavedSearch{}, err
	}
	if err := s.mayEdit(ctx, by, current.FolderID); err != nil {
		return SavedSearch{}, err
	}
	var changed change
	if title != nil && *title != current.Title {
		changed.title = title
	}
	if in.FolderID.Valid && in.FolderID != current.FolderID {
		if err := s.mayMoveInto(ctx, by, in.FolderID); err != nil {
			return SavedSearch{}, err
		}
		changed.moveTo = in.FolderID
	}
	if in.changesContent() {
		if changed.content, err = s.mergeContent(ctx, current, in, time.Now()); err != nil {
			return SavedSearch{}, err
		}
	}
	row := current
	if changed.title != nil || changed.moveTo.Valid || changed.content != nil {
		if row, err = s.store.update(ctx, actor.In(ctx, by, ""), id, current.FolderID, changed); err != nil {
			return SavedSearch{}, err
		}
		s.logger.InfoContext(ctx, "saved search updated",
			slog.String("actor_id", by.ID), slog.String("saved_search_id", id.String()), slog.String("folder_id", row.FolderID.String()))
	}
	return s.render(ctx, by, row)
}

func (s *Service) Delete(ctx context.Context, by auth.Principal, id pgtype.UUID) error {
	var folder pgtype.UUID
	err := s.store.delete(ctx, actor.In(ctx, by, ""), id, func(folderID pgtype.UUID) error {
		folder = folderID
		return s.mayEdit(ctx, by, folderID)
	})
	if err != nil {
		return err
	}
	s.logger.InfoContext(ctx, "saved search deleted",
		slog.String("actor_id", by.ID), slog.String("saved_search_id", id.String()), slog.String("folder_id", folder.String()))
	return nil
}

func (s *Service) mayEdit(ctx context.Context, by auth.Principal, folderID pgtype.UUID) error {
	_, abilities, err := s.access.Folder(ctx, by, folderID)
	switch {
	case errors.Is(err, folders.ErrNotFound), errors.Is(err, folders.ErrForbidden), err == nil && !abilities.View:
		return ErrNotFound
	case err != nil:
		return err
	case !abilities.Edit:
		return folders.ErrForbidden
	}
	return nil
}

func (s *Service) mayMoveInto(ctx context.Context, by auth.Principal, folderID pgtype.UUID) error {
	target, abilities, err := s.access.Folder(ctx, by, folderID)
	switch {
	case errors.Is(err, folders.ErrNotFound):
		return ErrFolderNotFound
	case err != nil:
		return err
	case !abilities.Edit:
		return folders.ErrForbidden
	case !folders.MayMoveInto(target):
		return folders.ErrMoveIntoPersonal
	}
	return nil
}

func (s *Service) render(ctx context.Context, by auth.Principal, row dbsqlc.SavedSearch) (SavedSearch, error) {
	folder, abilities, err := s.access.Folder(ctx, by, row.FolderID)
	if err != nil {
		return SavedSearch{}, err
	}
	return s.item(row, folder, abilities)
}

func (s *Service) item(row dbsqlc.SavedSearch, folder folders.Folder, abilities folders.Abilities) (SavedSearch, error) {
	item, err := savedSearchOf(row)
	if err != nil {
		return SavedSearch{}, err
	}
	item.Folder, item.Abilities = folder, abilities
	item.URL = URL(s.publicURL, item)
	return item, nil
}
