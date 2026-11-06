package stars

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
)

var ErrNotFound = errors.New("dashboard not found")

type Star struct {
	DashboardID string
	Title       string
	FolderID    string
	FolderTitle string
	StarredAt   time.Time
}

type Service struct {
	db     *db.DB
	access *folders.Access
}

func New(database *db.DB, access *folders.Access) *Service {
	return &Service{db: database, access: access}
}

func (s *Service) Star(ctx context.Context, by auth.Principal, dashboardID pgtype.UUID) error {
	userID, err := userOf(by)
	if err != nil {
		return err
	}
	err = s.db.Queries().StarDashboard(ctx, dbsqlc.StarDashboardParams{UserID: userID, DashboardID: dashboardID})
	if db.IsForeignKeyViolation(err) {
		return ErrNotFound
	}
	return err
}

func (s *Service) Unstar(ctx context.Context, by auth.Principal, dashboardID pgtype.UUID) error {
	userID, err := userOf(by)
	if err != nil {
		return err
	}
	return s.db.Queries().UnstarDashboard(ctx, dbsqlc.UnstarDashboardParams{UserID: userID, DashboardID: dashboardID})
}

func (s *Service) List(ctx context.Context, by auth.Principal) ([]Star, error) {
	userID, err := userOf(by)
	if err != nil {
		return nil, err
	}
	levels, err := s.access.Levels(ctx, by)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Queries().ListStars(ctx, dbsqlc.ListStarsParams{UserID: userID, Folders: levels.Visible()})
	if err != nil {
		return nil, err
	}
	out := make([]Star, 0, len(rows))
	for _, r := range rows {
		out = append(out, Star{
			DashboardID: r.ID.String(),
			Title:       r.Title,
			FolderID:    r.FolderID.String(),
			FolderTitle: r.FolderTitle,
			StarredAt:   r.StarredAt.Time,
		})
	}
	return out, nil
}

func userOf(by auth.Principal) (pgtype.UUID, error) {
	return db.ParseUUID(by.ID)
}
