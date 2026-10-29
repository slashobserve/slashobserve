package folders

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

const generalTitle = "General"

type ImportFolder struct {
	Folder  Folder
	Created bool
	Note    string
}

type importAction int

const (
	importDefault importAction = iota
	importUse
	importCreate
	importFallback
)

type importChoice struct {
	action importAction
	title  string
	id     pgtype.UUID
}

func pickImportFolder(rawTitle string, levels []folderLevel, existing pgtype.UUID, role auth.Role) importChoice {
	title, err := cleanTitle(rawTitle)
	if err != nil || strings.EqualFold(title, generalTitle) {
		return importChoice{action: importDefault}
	}
	if existing.Valid {
		for _, e := range levels {
			if e.Folder.ID == existing && e.Abilities.Create {
				return importChoice{action: importUse, title: title, id: existing}
			}
		}
		return importChoice{action: importFallback, title: title}
	}
	if role.AtLeast(auth.RoleEditor) {
		return importChoice{action: importCreate, title: title}
	}
	return importChoice{action: importFallback, title: title}
}

func (a *Access) ForImport(ctx context.Context, p auth.Principal, rawTitle string) (ImportFolder, error) {
	out, raced, err := a.resolveImportFolder(ctx, p, rawTitle)
	if raced {
		out, _, err = a.resolveImportFolder(ctx, p, rawTitle)
	}
	return out, err
}

func (a *Access) resolveImportFolder(ctx context.Context, p auth.Principal, rawTitle string) (ImportFolder, bool, error) {
	levels, err := a.Levels(ctx, p)
	if err != nil {
		return ImportFolder{}, false, err
	}
	var existing pgtype.UUID
	if title, cleanErr := cleanTitle(rawTitle); cleanErr == nil {
		existing, err = a.db.Queries().FolderByTitle(ctx, title)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return ImportFolder{}, false, err
		}
	}
	choice := pickImportFolder(rawTitle, levels.entries, existing, p.Role)
	switch choice.action {
	case importUse:
		folder, err := a.CreationFolder(ctx, p, choice.id)
		return ImportFolder{Folder: folder}, false, err
	case importCreate:
		return a.createImportFolder(ctx, p, choice.title)
	case importFallback:
		folder, err := a.CreationFolder(ctx, p, pgtype.UUID{})
		if err != nil {
			return ImportFolder{}, false, err
		}
		note := fmt.Sprintf("Folder %q is not available to you, so the dashboard went to %q.", choice.title, folder.Title)
		return ImportFolder{Folder: folder, Note: note}, false, nil
	}
	folder, err := a.CreationFolder(ctx, p, pgtype.UUID{})
	return ImportFolder{Folder: folder}, false, err
}

func (a *Access) createImportFolder(ctx context.Context, p auth.Principal, title string) (ImportFolder, bool, error) {
	principalID, err := principalUUID(p)
	if err != nil {
		return ImportFolder{}, false, err
	}
	row, err := a.db.Queries().CreateFolder(ctx, dbsqlc.CreateFolderParams{
		Title:         title,
		CreatedByKind: pgtype.Text{String: string(p.Kind), Valid: true},
		CreatedByID:   principalID,
		CreatedByName: pgtype.Text{String: callerName(p), Valid: true},
	})
	if isPgError(err, uniqueViolation) {
		return ImportFolder{}, true, nil
	}
	if err != nil {
		return ImportFolder{}, false, err
	}
	a.logger.InfoContext(ctx, "created a folder", slog.String("actor_id", p.ID), slog.String("folder_id", row.ID.String()))
	folder, err := a.CreationFolder(ctx, p, row.ID)
	return ImportFolder{Folder: folder, Created: true}, false, err
}
