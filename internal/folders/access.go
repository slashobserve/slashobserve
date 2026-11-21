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

	ErrProvisioned = errors.New("this dashboard is a starter the server keeps current with each release, so it cannot be changed; duplicate it to change a copy")

	ErrMoveIntoPersonal = errors.New("a dashboard cannot move into My Dashboards; duplicate it there instead")
)

const AuditStarterKey = "audit/folder"

type Folder struct {
	ID         pgtype.UUID
	Title      string
	General    bool
	Personal   bool
	Restricted bool
	AdminsOnly bool
}

type folderLevel struct {
	Folder    Folder
	Abilities Abilities
}

type Levels struct {
	entries  []folderLevel
	personal pgtype.UUID
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

func (l Levels) General() Abilities {
	for _, e := range l.entries {
		if e.Folder.General {
			return e.Abilities
		}
	}
	return Abilities{}
}

func (l Levels) CanCreate() bool {
	for _, e := range l.entries {
		if e.Abilities.Create {
			return true
		}
	}
	return false
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
	Provisioned bool
}

func (a DashboardAccess) Shown() Abilities {
	shown := a.Abilities
	if a.Provisioned {
		shown.Edit = false
	}
	return shown
}

func (a DashboardAccess) Refuses(n Need) bool {
	return a.Provisioned && n == NeedContent
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
	withheld := withholdsAdminsOnly(ctx, p)
	entries := make([]folderLevel, 0, len(rows))
	var personal pgtype.UUID
	for _, r := range rows {
		facts := Facts{Restricted: r.Restricted, AdminsOnly: r.AdminsOnly, Creator: r.Creator, Personal: r.Personal, Owner: r.Owner}
		for _, g := range r.Grants {
			if level, ok := ParseLevel(g); ok {
				facts.Grants = append(facts.Grants, level)
			}
		}
		level := LevelFor(p.Role, facts)
		if level < LevelView || withheld && r.AdminsOnly {
			continue
		}
		if r.Owner {
			personal = r.ID
		}
		entries = append(entries, folderLevel{
			Folder:    Folder{ID: r.ID, Title: r.Title, General: r.General, Personal: r.Personal, Restricted: r.Restricted, AdminsOnly: r.AdminsOnly},
			Abilities: AbilitiesFor(level),
		})
	}
	return Levels{entries: entries, personal: personal}, nil
}

func (a *Access) Dashboard(ctx context.Context, p auth.Principal, id pgtype.UUID) (DashboardAccess, error) {
	placement, err := a.db.Queries().DashboardPlacement(ctx, id)
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
	e, ok := levels.find(placement.FolderID)
	if !ok {
		return DashboardAccess{}, ErrNoAccess
	}
	return DashboardAccess{DashboardID: id, Folder: e.Folder, Abilities: e.Abilities, Provisioned: placement.Provisioned}, nil
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

func (a *Access) AuditFolder(ctx context.Context) (pgtype.UUID, error) {
	id, err := a.db.Queries().StarterContent(ctx, AuditStarterKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return pgtype.UUID{}, nil
	}
	return id, err
}
