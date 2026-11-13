package health

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
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/health/healthrule"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/links"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

var (
	ErrInvalidCursor  = errors.New("invalid cursor")
	ErrFolderNotFound = errors.New("folder not found")
)

type Deps struct {
	DB        *db.DB
	Runner    *queryproxy.Runner
	Access    *folders.Access
	PublicURL *url.URL
	Logger    *slog.Logger
	Job       string
}

type Service struct {
	db        *db.DB
	runner    *queryproxy.Runner
	access    *folders.Access
	publicURL *url.URL
	logger    *slog.Logger
	job       string
}

func New(d Deps) *Service {
	return &Service{db: d.DB, runner: d.Runner, access: d.Access, publicURL: d.PublicURL, logger: d.Logger, job: d.Job}
}

type ListParams struct {
	FolderID     pgtype.UUID
	DashboardIDs []pgtype.UUID
	Kind         Kind
	Cursor       string
	Limit        int
}

type Item struct {
	ID, Title, URL string
	Folder         folders.Folder
	CanEdit        bool
	State          string
	CheckedVersion *int
	CheckedAt      *time.Time
	LastViewedAt   *time.Time
	Skipped        string
	Problems       []Problem
	Repair         *Repair
}

type Repair struct {
	RequestID, State string
	Outcome          string
	Version          *int
	Error            *string
	UpdatedAt        time.Time
}

type Summary struct {
	Dashboards, Failing, QueryFailed, VariableFailed, NoData, DuplicateTitles, Unopened int
}

type JobStatus struct {
	LastStartedAt  *time.Time
	LastFinishedAt *time.Time
	LastOutcome    string
	Running        bool
}

type Page struct {
	Items      []Item
	NextCursor string
	Summary    *Summary
	Job        JobStatus
}

const (
	repairSucceeded = "succeeded"
	repairFailed    = "failed"
)

func (s *Service) List(ctx context.Context, by auth.Principal, p ListParams) (Page, error) {
	levels, err := s.access.Levels(ctx, by)
	if err != nil {
		return Page{}, err
	}
	if p.FolderID.Valid && !levels.Of(p.FolderID).View {
		return Page{}, ErrFolderNotFound
	}
	cursor, ok := decodeRankCursor(p.Cursor)
	if !ok {
		return Page{}, ErrInvalidCursor
	}
	now := time.Now()
	before := pgtype.Timestamptz{Time: healthrule.UnopenedBefore(now), Valid: true}
	visible := levels.Visible()
	kind := string(p.Kind)
	if p.Kind == "" {
		kind = healthrule.AnyProblem
	}
	params := dbsqlc.ListHealthParams{
		Folders:        visible,
		Folder:         p.FolderID,
		DashboardIds:   p.DashboardIDs,
		Kind:           kind,
		UnopenedBefore: before,
		RepairPrefix:   repairPromptPrefix,
		RowLimit:       httpapi.Int32(p.Limit + 1),
	}
	if cursor != nil {
		params.CursorRank = pgtype.Int4{Int32: httpapi.Int32(cursor.Rank), Valid: true}
		params.CursorSeen = pgtype.Timestamptz{Time: cursor.Seen, Valid: true}
		params.CursorTitle = pgtype.Text{String: cursor.Title, Valid: true}
		params.CursorID = cursor.ID
	}
	rows, err := s.db.Queries().ListHealth(ctx, params)
	if err != nil {
		return Page{}, fmt.Errorf("list health: %w", err)
	}
	job, err := s.jobStatus(ctx, now)
	if err != nil {
		return Page{}, err
	}
	page := Page{Items: make([]Item, 0, min(len(rows), p.Limit)), Job: job}
	if cursor == nil && len(p.DashboardIDs) == 0 {
		row, err := s.db.Queries().HealthSummary(ctx, dbsqlc.HealthSummaryParams{UnopenedBefore: before, Folders: visible, Folder: p.FolderID})
		if err != nil {
			return Page{}, fmt.Errorf("summarize health: %w", err)
		}
		page.Summary = &Summary{
			Dashboards:      int(row.Dashboards),
			Failing:         int(row.Failing),
			QueryFailed:     int(row.QueryFailed),
			VariableFailed:  int(row.VariableFailed),
			NoData:          int(row.NoData),
			DuplicateTitles: int(row.DuplicateTitles),
			Unopened:        int(row.Unopened),
		}
	}
	if len(rows) > p.Limit {
		rows = rows[:p.Limit]
		last := rows[len(rows)-1]
		page.NextCursor = encodeRankCursor(int(last.Rank), last.SeenAt.Time, last.Title, last.ID.String())
	}
	for _, row := range rows {
		it, err := s.item(row, levels, p.Kind, now)
		if err != nil {
			return Page{}, err
		}
		page.Items = append(page.Items, it)
	}
	return page, nil
}

func (s *Service) item(row dbsqlc.ListHealthRow, levels folders.Levels, kind Kind, now time.Time) (Item, error) {
	checked, err := decodeProblems(row.Issues)
	if err != nil {
		return Item{}, err
	}
	var problems []Problem
	for _, p := range checked {
		if kind == "" || p.Kind == kind {
			problems = append(problems, p)
		}
	}
	lastView := timeOf(row.LastViewedAt)
	if row.DuplicateTitle && (kind == "" || kind == KindDuplicateTitle) {
		problems = append(problems, Problem{Kind: KindDuplicateTitle})
	}
	if unopened(lastView, row.CreatedAt.Time, now) && (kind == "" || kind == KindUnopened) {
		problems = append(problems, Problem{Kind: KindUnopened})
	}
	var checkedVersion *int
	if row.CheckedVersion.Valid {
		v := int(row.CheckedVersion.Int32)
		checkedVersion = &v
	}
	id := row.ID.String()
	return Item{
		ID:             id,
		Title:          row.Title,
		URL:            links.Dashboard(s.publicURL, id, row.Title),
		Folder:         folders.Folder{ID: row.FolderID, Title: row.FolderTitle, General: row.FolderGeneral, Personal: row.FolderPersonal},
		CanEdit:        levels.Of(row.FolderID).Edit,
		State:          itemState(checkedVersion, int(row.LatestVersion), row.Skipped, checked),
		CheckedVersion: checkedVersion,
		CheckedAt:      timeOf(row.CheckedAt),
		LastViewedAt:   lastView,
		Skipped:        row.Skipped,
		Problems:       problems,
		Repair:         repairOf(row, checkedVersion),
	}, nil
}

func repairOf(row dbsqlc.ListHealthRow, checkedVersion *int) *Repair {
	if !row.RepairID.Valid {
		return nil
	}
	r := &Repair{RequestID: row.RepairID.String(), State: row.RepairStatus, UpdatedAt: row.RepairUpdatedAt.Time}
	if r.State == repairSucceeded && row.RepairVersion.Valid {
		v := int(row.RepairVersion.Int32)
		r.Version = &v
	}
	if r.State == repairFailed && row.RepairError != "" {
		r.Error = &row.RepairError
	}
	r.Outcome = repairOutcome(row.RepairState, r.Version, checkedVersion)
	return r
}

func (s *Service) jobStatus(ctx context.Context, now time.Time) (JobStatus, error) {
	row, err := s.db.Queries().GetJob(ctx, s.job)
	if errors.Is(err, pgx.ErrNoRows) {
		return JobStatus{}, nil
	}
	if err != nil {
		return JobStatus{}, fmt.Errorf("read health job: %w", err)
	}
	return JobStatus{
		LastStartedAt:  timeOf(row.LastStartedAt),
		LastFinishedAt: timeOf(row.LastFinishedAt),
		LastOutcome:    row.LastOutcome,
		Running:        row.LeaseUntil.Valid && row.LeaseUntil.Time.After(now),
	}, nil
}

func (s *Service) RecordView(ctx context.Context, by auth.Principal, dashboardID pgtype.UUID) error {
	if by.Kind != auth.KindUser {
		return nil
	}
	userID, err := db.ParseUUID(by.ID)
	if err != nil {
		return fmt.Errorf("viewer id: %w", err)
	}
	err = s.db.Queries().RecordDashboardView(ctx, dbsqlc.RecordDashboardViewParams{UserID: userID, DashboardID: dashboardID})
	if db.IsForeignKeyViolation(err) {
		return nil
	}
	return err
}

func decodeProblems(raw []byte) ([]Problem, error) {
	var problems []Problem
	if len(raw) == 0 {
		return problems, nil
	}
	if err := json.Unmarshal(raw, &problems); err != nil {
		return nil, fmt.Errorf("decode health problems: %w", err)
	}
	return problems, nil
}

func timeOf(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}
