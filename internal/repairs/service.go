package repairs

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

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/links"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const Prompt = "Repair every panel and variable listed as unverified so its query runs and returns data; keep every other panel unchanged and never delete an unverified panel."

const (
	ReasonActive          = "active"
	ReasonNothingToRepair = "nothing_to_repair"
	ReasonNotFound        = "not_found"
	ReasonForbidden       = "forbidden_role"
	ReasonProvisioned     = "provisioned"
)

const walkPageSize = 100

var (
	ErrInvalidCursor  = errors.New("invalid cursor")
	ErrFolderNotFound = errors.New("folder not found")
)

type Deps struct {
	DB        *db.DB
	Access    *folders.Access
	Revisions *revisions.Service
	PublicURL *url.URL
	Logger    *slog.Logger
}

type Service struct {
	db        *db.DB
	access    *folders.Access
	queue     *Queue
	publicURL *url.URL
	logger    *slog.Logger
}

func New(d Deps) *Service {
	return &Service{db: d.DB, access: d.Access, queue: NewQueue(d.DB, d.Access, d.Revisions), publicURL: d.PublicURL, logger: d.Logger}
}

type ListParams struct {
	FolderID pgtype.UUID
	Cursor   string
	Limit    int
}

type Item struct {
	ID, Title, URL                                           string
	Folder                                                   folders.Folder
	UnverifiedPanels, UnverifiedVariables, PlaceholderPanels int
	LastImportAt                                             *time.Time
	Repair                                                   *Repair
}

type Repair struct {
	ID         string
	Status     string
	FinishedAt *time.Time
}

type Page struct {
	Items      []Item
	NextCursor string
}

type Target struct {
	DashboardIDs []pgtype.UUID
	All          bool
	FolderID     pgtype.UUID
}

type Queued struct{ DashboardID, RequestID string }

type Skipped struct{ DashboardID, Reason string }

type Outcome struct {
	Queued  []Queued
	Skipped []Skipped
}

func (s *Service) List(ctx context.Context, by auth.Principal, p ListParams) (Page, error) {
	levels, err := s.access.Levels(ctx, by)
	if err != nil {
		return Page{}, err
	}
	if err := checkFolder(levels, p.FolderID); err != nil {
		return Page{}, err
	}
	cursor, ok := httpapi.DecodeTimeCursor(p.Cursor)
	if !ok {
		return Page{}, ErrInvalidCursor
	}
	rows, err := s.rows(ctx, levels, p.FolderID, cursor, p.Limit+1)
	if err != nil {
		return Page{}, err
	}
	page := Page{Items: make([]Item, 0, min(len(rows), p.Limit))}
	if len(rows) > p.Limit {
		rows = rows[:p.Limit]
		last := rows[len(rows)-1]
		page.NextCursor = httpapi.EncodeTimeCursor(last.UpdatedAt.Time, last.ID.String())
	}
	for _, row := range rows {
		page.Items = append(page.Items, s.item(row))
	}
	return page, nil
}

func (s *Service) Request(ctx context.Context, by auth.Principal, t Target) (Outcome, error) {
	batch, err := s.queue.Begin(ctx, by, importPlan)
	if err != nil {
		return Outcome{}, err
	}
	if !t.All {
		if err := batch.Each(ctx, t.DashboardIDs); err != nil {
			return Outcome{}, err
		}
		return batch.Outcome(), nil
	}
	if err := checkFolder(batch.Levels, t.FolderID); err != nil {
		return Outcome{}, err
	}
	var cursor *httpapi.TimeCursor
	for {
		rows, err := s.rows(ctx, batch.Levels, t.FolderID, cursor, walkPageSize)
		if err != nil {
			return Outcome{}, err
		}
		for _, row := range rows {
			if err := batch.Add(ctx, row.ID); err != nil {
				return Outcome{}, err
			}
		}
		if len(rows) < walkPageSize {
			return batch.Outcome(), nil
		}
		last := rows[len(rows)-1]
		cursor = &httpapi.TimeCursor{Time: last.UpdatedAt.Time, ID: last.ID}
	}
}

func importPlan(ctx context.Context, q *dbsqlc.Queries, levels folders.Levels, id pgtype.UUID) (int, string, string, error) {
	target, err := q.RepairTarget(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", ReasonNotFound, nil
	}
	if err != nil {
		return 0, "", "", fmt.Errorf("read repair target: %w", err)
	}
	if reason, err := Admit(ctx, q, levels, id, target.FolderID, target.Provisioned); err != nil || reason != "" {
		return 0, "", reason, err
	}
	var unverified dashboards.Unverified
	if len(target.Unverified) > 0 {
		if err := json.Unmarshal(target.Unverified, &unverified); err != nil {
			return 0, "", "", fmt.Errorf("decode unverified: %w", err)
		}
	}
	if unverified.Empty() {
		return 0, "", ReasonNothingToRepair, nil
	}
	return int(target.LatestVersion), Prompt, "", nil
}

func checkFolder(levels folders.Levels, folder pgtype.UUID) error {
	if folder.Valid && !levels.Of(folder).View {
		return ErrFolderNotFound
	}
	return nil
}

func (s *Service) rows(ctx context.Context, levels folders.Levels, folder pgtype.UUID, cursor *httpapi.TimeCursor, limit int) ([]dbsqlc.ListRepairsRow, error) {
	params := dbsqlc.ListRepairsParams{RepairPrompt: Prompt, Folders: levels.Visible(), Folder: folder, RowLimit: httpapi.Int32(limit)}
	if cursor != nil {
		params.CursorUpdatedAt = pgtype.Timestamptz{Time: cursor.Time, Valid: true}
		params.CursorID = cursor.ID
	}
	rows, err := s.db.Queries().ListRepairs(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list repairs: %w", err)
	}
	return rows, nil
}

func (s *Service) item(row dbsqlc.ListRepairsRow) Item {
	id := row.ID.String()
	it := Item{
		ID:                  id,
		Title:               row.Title,
		URL:                 links.Dashboard(s.publicURL, id, row.Title),
		Folder:              folders.Folder{ID: row.FolderID, Title: row.FolderTitle, General: row.FolderGeneral, Personal: row.FolderPersonal},
		UnverifiedPanels:    int(row.UnverifiedPanels),
		UnverifiedVariables: int(row.UnverifiedVariables),
		PlaceholderPanels:   int(row.PlaceholderPanels),
		LastImportAt:        timeOf(row.LastImportAt),
	}
	if row.RepairID.Valid {
		it.Repair = &Repair{ID: row.RepairID.String(), Status: row.RepairStatus, FinishedAt: timeOf(row.RepairFinishedAt)}
	}
	return it
}

func timeOf(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}
