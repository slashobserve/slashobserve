package folders

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

var (
	ErrFolderExists  = errors.New("a folder with that name already exists")
	ErrGeneralFolder = errors.New("the General folder cannot be renamed")
)

type InvalidTitleError struct {
	Message string
	Cause   error
}

func (e *InvalidTitleError) Error() string { return e.Message }

func (e *InvalidTitleError) Unwrap() error { return e.Cause }

func titleOf(raw string) (string, error) {
	title, err := cleanTitle(raw)
	if err != nil {
		return "", &InvalidTitleError{Message: err.Error(), Cause: err}
	}
	if reservedTitle(title) {
		return "", ErrFolderExists
	}
	return title, nil
}

func callerName(p auth.Principal) string {
	if p.Name != "" {
		return p.Name
	}
	return p.Email
}

func (a *Access) CreateFolder(ctx context.Context, caller auth.Principal, rawTitle string) (Folder, Abilities, error) {
	title, err := titleOf(rawTitle)
	if err != nil {
		return Folder{}, Abilities{}, err
	}
	if !caller.Role.AtLeast(auth.RoleEditor) {
		return Folder{}, Abilities{}, ErrForbidden
	}
	callerID, err := db.ParseUUID(caller.ID)
	if err != nil {
		return Folder{}, Abilities{}, ErrForbidden
	}
	row, err := a.db.Queries().CreateFolder(ctx, dbsqlc.CreateFolderParams{
		Title:         title,
		CreatedByKind: pgtype.Text{String: string(caller.Kind), Valid: true},
		CreatedByID:   callerID,
		CreatedByName: pgtype.Text{String: callerName(caller), Valid: true},
	})
	if isPgError(err, uniqueViolation) {
		return Folder{}, Abilities{}, ErrFolderExists
	}
	if err != nil {
		return Folder{}, Abilities{}, err
	}
	a.logger.InfoContext(ctx, "created a folder", slog.String("actor_id", caller.ID), slog.String("folder_id", row.ID.String()))
	return a.Folder(ctx, caller, row.ID)
}

func (a *Access) RenameFolder(ctx context.Context, caller auth.Principal, id pgtype.UUID, rawTitle string) (Folder, Abilities, error) {
	title, err := titleOf(rawTitle)
	if err != nil {
		return Folder{}, Abilities{}, err
	}
	current, abilities, err := a.Folder(ctx, caller, id)
	if err != nil {
		return Folder{}, Abilities{}, err
	}
	if !abilities.Manage {
		return Folder{}, Abilities{}, ErrForbidden
	}
	if current.General {
		return Folder{}, Abilities{}, ErrGeneralFolder
	}
	row, err := a.db.Queries().RenameFolder(ctx, dbsqlc.RenameFolderParams{ID: current.ID, Title: title})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Folder{}, Abilities{}, ErrNotFound
	case isPgError(err, uniqueViolation):
		return Folder{}, Abilities{}, ErrFolderExists
	case err != nil:
		return Folder{}, Abilities{}, err
	}
	a.logger.InfoContext(ctx, "renamed a folder", slog.String("actor_id", caller.ID), slog.String("folder_id", row.ID.String()))
	current.Title = row.Title
	return current, abilities, nil
}

func isPgError(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}
