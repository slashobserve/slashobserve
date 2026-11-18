package homedash

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
)

var ErrNotFound = errors.New("dashboard not found")

type Status string

const (
	StatusNone       Status = "none"
	StatusOK         Status = "ok"
	StatusGone       Status = "gone"
	StatusUnreadable Status = "unreadable"
)

type Home struct {
	Status      Status
	DashboardID string
	Title       string
	FolderID    string
	FolderTitle string
	SetAt       time.Time
}

type Service struct {
	db     *db.DB
	access *folders.Access
}

func New(database *db.DB, access *folders.Access) *Service {
	return &Service{db: database, access: access}
}

func (s *Service) Get(ctx context.Context, by auth.Principal) (Home, error) {
	userID, err := db.ParseUUID(by.ID)
	if err != nil {
		return Home{}, err
	}
	row, err := s.db.Queries().GetHomeDashboard(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Home{Status: StatusNone}, nil
	}
	if err != nil {
		return Home{}, err
	}
	home := Home{Status: StatusGone, DashboardID: row.DashboardID.String(), SetAt: row.SetAt.Time}
	if !row.DashboardTitle.Valid {
		return home, nil
	}
	access, err := s.access.Dashboard(ctx, by, row.DashboardID)
	switch {
	case errors.Is(err, folders.ErrNotFound):
		return home, nil
	case errors.Is(err, folders.ErrNoAccess):
		home.Status = StatusUnreadable
		return home, nil
	case err != nil:
		return Home{}, err
	}
	home.Status = StatusOK
	home.Title = row.DashboardTitle.String
	home.FolderID = access.Folder.ID.String()
	home.FolderTitle = access.Folder.Title
	return home, nil
}

func (s *Service) Set(ctx context.Context, by auth.Principal, dashboardID pgtype.UUID) error {
	userID, err := db.ParseUUID(by.ID)
	if err != nil {
		return err
	}
	if _, err := s.access.Dashboard(ctx, by, dashboardID); errors.Is(err, folders.ErrNotFound) || errors.Is(err, folders.ErrNoAccess) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	return s.db.Queries().SetHomeDashboard(ctx, dbsqlc.SetHomeDashboardParams{UserID: userID, DashboardID: dashboardID})
}

func (s *Service) Clear(ctx context.Context, by auth.Principal) error {
	userID, err := db.ParseUUID(by.ID)
	if err != nil {
		return err
	}
	return s.db.Queries().ClearHomeDashboard(ctx, userID)
}
