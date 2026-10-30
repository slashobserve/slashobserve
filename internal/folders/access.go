package folders

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

var (
	ErrNotFound  = errors.New("not found")
	ErrNoAccess  = errors.New("no access")
	ErrForbidden = errors.New("forbidden")
)

type Folder struct {
	ID         pgtype.UUID
	Title      string
	General    bool
	Restricted bool
}

type folderLevel struct {
	Folder    Folder
	Abilities Abilities
}

type Levels struct {
	entries []folderLevel
}

func (l Levels) find(id pgtype.UUID) (folderLevel, bool) {
	for _, e := range l.entries {
		if e.Folder.ID == id {
			return e, true
		}
	}
	return folderLevel{}, false
}

func (l Levels) Of(folderID pgtype.UUID) Abilities {
	e, _ := l.find(folderID)
	return e.Abilities
}

func (l Levels) Visible() []pgtype.UUID {
	ids := make([]pgtype.UUID, 0, len(l.entries))
	for _, e := range l.entries {
		ids = append(ids, e.Folder.ID)
	}
	return ids
}

func (l Levels) Folders() []Folder {
	out := make([]Folder, 0, len(l.entries))
	for _, e := range l.entries {
		out = append(out, e.Folder)
	}
	return out
}

type DashboardAccess struct {
	DashboardID pgtype.UUID
	Folder      Folder
	Abilities   Abilities
}

type Access struct {
	db     *db.DB
	logger *slog.Logger
}

func NewAccess(database *db.DB) *Access {
	return &Access{db: database, logger: slog.Default()}
}

func principalUUID(p auth.Principal) (pgtype.UUID, error) {
	id, err := db.ParseUUID(p.ID)
	if err != nil {
		return pgtype.UUID{}, ErrForbidden
	}
	return id, nil
}

func (a *Access) Levels(ctx context.Context, p auth.Principal) (Levels, error) {
	principalID, err := principalUUID(p)
	if err != nil {
		return Levels{}, err
	}
	params := dbsqlc.FolderFactsParams{Groups: p.Groups, Kind: string(p.Kind), PrincipalID: principalID}
	if params.Groups == nil {
		params.Groups = []string{}
	}
	if p.Kind == auth.KindUser {
		params.UserID = principalID
	}
	rows, err := a.db.Queries().FolderFacts(ctx, params)
	if err != nil {
		return Levels{}, err
	}
	entries := make([]folderLevel, 0, len(rows))
	for _, r := range rows {
		facts := Facts{Restricted: r.Restricted, Creator: r.Creator}
		for _, g := range r.Grants {
			if level, ok := ParseLevel(g); ok {
				facts.Grants = append(facts.Grants, level)
			}
		}
		level := LevelFor(p.Role, facts)
		if level < LevelView {
			continue
		}
		entries = append(entries, folderLevel{
			Folder:    Folder{ID: r.ID, Title: r.Title, General: r.General, Restricted: r.Restricted},
			Abilities: AbilitiesFor(level),
		})
	}
	return Levels{entries: entries}, nil
}

func (a *Access) Dashboard(ctx context.Context, p auth.Principal, id pgtype.UUID) (DashboardAccess, error) {
	folderID, err := a.db.Queries().DashboardFolder(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return DashboardAccess{}, ErrNotFound
	}
	if err != nil {
		return DashboardAccess{}, err
	}
	levels, err := a.Levels(ctx, p)
	if err != nil {
		return DashboardAccess{}, err
	}
	e, ok := levels.find(folderID)
	if !ok {
		return DashboardAccess{}, ErrNoAccess
	}
	return DashboardAccess{DashboardID: id, Folder: e.Folder, Abilities: e.Abilities}, nil
}

func (a *Access) Folder(ctx context.Context, p auth.Principal, id pgtype.UUID) (Folder, Abilities, error) {
	levels, err := a.Levels(ctx, p)
	if err != nil {
		return Folder{}, Abilities{}, err
	}
	e, ok := levels.find(id)
	if !ok {
		return Folder{}, Abilities{}, ErrNotFound
	}
	return e.Folder, e.Abilities, nil
}
