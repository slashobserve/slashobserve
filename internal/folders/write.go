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
	row, err := a.insertFolder(ctx, caller, callerID, title, false)
	if isPgError(err, uniqueViolation) {
		return Folder{}, Abilities{}, ErrFolderExists
	}
	if err != nil {
		return Folder{}, Abilities{}, err
	}
	a.logger.InfoContext(ctx, "created a folder", slog.String("actor_id", caller.ID), slog.String("folder_id", row.ID.String()))
	return a.Folder(ctx, caller, row.ID)
}

func (a *Access) CreateAdminsOnlyFolder(ctx context.Context, by auth.Principal, rawTitle string) (pgtype.UUID, error) {
	title, err := titleOf(rawTitle)
	if err != nil {
		return pgtype.UUID{}, err
	}
	if by.Role != auth.RoleAdmin {
		return pgtype.UUID{}, ErrForbidden
	}
	byID, err := principalUUID(by)
	if err != nil {
		return pgtype.UUID{}, err
	}
	row, err := a.insertFolder(ctx, by, byID, title, true)
	if isPgError(err, uniqueViolation) {
		return pgtype.UUID{}, ErrFolderExists
	}
	if err != nil {
		return pgtype.UUID{}, err
	}
	a.logger.InfoContext(ctx, "created a folder only Admins can read", slog.String("actor_id", by.ID), slog.String("folder_id", row.ID.String()))
	return row.ID, nil
}

func (a *Access) insertFolder(ctx context.Context, by auth.Principal, byID pgtype.UUID, title string, adminsOnly bool) (dbsqlc.Folder, error) {
	var row dbsqlc.Folder
	err := a.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		if err := runWriteInto(ctx, q, adminsOnly); err != nil {
			return err
		}
		var err error
		row, err = q.CreateFolder(ctx, dbsqlc.CreateFolderParams{
			Title:         title,
			CreatedByKind: pgtype.Text{String: string(by.Kind), Valid: true},
			CreatedByID:   byID,
			CreatedByName: pgtype.Text{String: callerName(by), Valid: true},
		})
		if err != nil {
			return err
		}
		if adminsOnly {
			if err := q.SetFolderAdminsOnly(ctx, dbsqlc.SetFolderAdminsOnlyParams{ID: row.ID, AdminsOnly: true}); err != nil {
				return err
			}
			row.AdminsOnly = true
		}
		return recordFolder(ctx, q, "folder.create", row.ID, row.Title, "", nil)
	})
	return row, err
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
	row, err := a.renameInTx(ctx, current.ID, title)
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

func (a *Access) renameInTx(ctx context.Context, id pgtype.UUID, title string) (dbsqlc.Folder, error) {
	var row dbsqlc.Folder
	err := a.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		before, err := q.LockFolder(ctx, id)
		if err != nil {
			return err
		}
		if err := runWriteInto(ctx, q, before.AdminsOnly); err != nil {
			return err
		}
		row, err = q.RenameFolder(ctx, dbsqlc.RenameFolderParams{ID: id, Title: title})
		if err != nil {
			return err
		}
		return recordFolder(ctx, q, "folder.rename", row.ID, row.Title, noChangeReason(before.Title == row.Title), map[string]any{"from_title": before.Title, "title": row.Title})
	})
	return row, err
}

func isPgError(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}
