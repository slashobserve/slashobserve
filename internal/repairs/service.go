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

const MaxActivePerRequester = 20

const (
	ReasonActive          = "active"
	ReasonNothingToRepair = "nothing_to_repair"
	ReasonNotFound        = "not_found"
	ReasonForbidden       = "forbidden_role"
	ReasonQueueFull       = "queue_full"
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
	revisions *revisions.Service
	publicURL *url.URL
	logger    *slog.Logger
}

func New(d Deps) *Service {
	return &Service{db: d.DB, access: d.Access, revisions: d.Revisions, publicURL: d.PublicURL, logger: d.Logger}
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
	if !s.revisions.Available() {
		return Outcome{}, revisions.ErrUnavailable
	}
	requester, err := db.ParseUUID(by.ID)
	if err != nil {
		return Outcome{}, fmt.Errorf("requester id: %w", err)
	}
	levels, err := s.access.Levels(ctx, by)
	if err != nil {
		return Outcome{}, err
	}
	out := Outcome{Queued: []Queued{}, Skipped: []Skipped{}}
	handle := func(id pgtype.UUID) error {
		return s.handle(ctx, by, requester, levels, id, &out)
	}
	if !t.All {
		seen := make(map[pgtype.UUID]bool, len(t.DashboardIDs))
		for _, id := range t.DashboardIDs {
			if seen[id] {
				continue
			}
			seen[id] = true
			if err := handle(id); err != nil {
				return Outcome{}, err
			}
		}
		return out, nil
	}
	if err := checkFolder(levels, t.FolderID); err != nil {
		return Outcome{}, err
	}
	var cursor *httpapi.TimeCursor
	for {
		rows, err := s.rows(ctx, levels, t.FolderID, cursor, walkPageSize)
		if err != nil {
			return Outcome{}, err
		}
		for _, row := range rows {
			if err := handle(row.ID); err != nil {
				return Outcome{}, err
			}
		}
		if len(rows) < walkPageSize {
			return out, nil
		}
		last := rows[len(rows)-1]
		cursor = &httpapi.TimeCursor{Time: last.UpdatedAt.Time, ID: last.ID}
	}
}

func (s *Service) handle(ctx context.Context, by auth.Principal, requester pgtype.UUID, levels folders.Levels, id pgtype.UUID, out *Outcome) error {
	var reason string
	var queued dbsqlc.AgentRequest
	err := s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		latest, why, err := checkTarget(ctx, q, requester, levels, id)
		if err != nil || why != "" {
			reason = why
			return err
		}
		queued, err = s.revisions.Enqueue(ctx, q, revisions.EnqueueParams{
			DashboardID: id,
			Source:      revisions.SourcePrompt,
			Prompt:      Prompt,
			BaseVersion: latest,
			By:          by,
		})
		return err
	})
	switch {
	case db.IsForeignKeyViolation(err):
		reason = ReasonNotFound
	case err != nil:
		return err
	}
	if reason != "" {
		out.Skipped = append(out.Skipped, Skipped{DashboardID: id.String(), Reason: reason})
		return nil
	}
	out.Queued = append(out.Queued, Queued{DashboardID: id.String(), RequestID: queued.ID.String()})
	return nil
}

func checkTarget(ctx context.Context, q *dbsqlc.Queries, requester pgtype.UUID, levels folders.Levels, id pgtype.UUID) (int, string, error) {
	if err := q.LockAgentRequestsForDashboard(ctx, id); err != nil {
		return 0, "", fmt.Errorf("lock agent requests: %w", err)
	}
	target, err := q.RepairTarget(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ReasonNotFound, nil
	}
	if err != nil {
		return 0, "", fmt.Errorf("read repair target: %w", err)
	}
	abilities := levels.Of(target.FolderID)
	if !abilities.View {
		return 0, ReasonNotFound, nil
	}
	if !abilities.Edit {
		return 0, ReasonForbidden, nil
	}
	active, err := q.CountActiveAgentRequests(ctx, id)
	if err != nil {
		return 0, "", fmt.Errorf("count active agent requests: %w", err)
	}
	if active > 0 {
		return 0, ReasonActive, nil
	}
	var unverified dashboards.Unverified
	if len(target.Unverified) > 0 {
		if err := json.Unmarshal(target.Unverified, &unverified); err != nil {
			return 0, "", fmt.Errorf("decode unverified: %w", err)
		}
	}
	if unverified.Empty() {
		return 0, ReasonNothingToRepair, nil
	}
	mine, err := q.CountActiveRepairsBy(ctx, dbsqlc.CountActiveRepairsByParams{RequesterID: requester, RepairPrompt: Prompt})
	if err != nil {
		return 0, "", fmt.Errorf("count active repairs: %w", err)
	}
	if mine >= MaxActivePerRequester {
		return 0, ReasonQueueFull, nil
	}
	return int(target.LatestVersion), "", nil
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
