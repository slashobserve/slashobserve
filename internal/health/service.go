package health

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/links"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/internal/repairs"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

var (
	ErrInvalidCursor  = errors.New("invalid cursor")
	ErrFolderNotFound = errors.New("folder not found")
)

type Deps struct {
	DB        *db.DB
	Runner    *queryproxy.Runner
	Access    *folders.Access
	Revisions *revisions.Service
	PublicURL *url.URL
	Logger    *slog.Logger
	Job       string
}

type Service struct {
	db        *db.DB
	runner    *queryproxy.Runner
	access    *folders.Access
	revisions *revisions.Service
	publicURL *url.URL
	logger    *slog.Logger
	job       string
}

func New(d Deps) *Service {
	return &Service{db: d.DB, runner: d.Runner, access: d.Access, revisions: d.Revisions, publicURL: d.PublicURL, logger: d.Logger, job: d.Job}
}

type ListParams struct {
	FolderID pgtype.UUID
	Kind     Kind
	Cursor   string
	Limit    int
}

type Item struct {
	ID, Title, URL string
	Folder         folders.Folder
	CanEdit        bool
	CheckedVersion *int
	CheckedAt      *time.Time
	LastViewedAt   *time.Time
	Skipped        string
	Problems       []Problem
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
	Job        JobStatus
}

func (s *Service) List(ctx context.Context, by auth.Principal, p ListParams) (Page, error) {
	levels, err := s.access.Levels(ctx, by)
	if err != nil {
		return Page{}, err
	}
	if p.FolderID.Valid && !levels.Of(p.FolderID).View {
		return Page{}, ErrFolderNotFound
	}
	cursor, ok := httpapi.DecodeTimeCursor(p.Cursor)
	if !ok {
		return Page{}, ErrInvalidCursor
	}
	now := time.Now()
	params := dbsqlc.ListHealthParams{
		Folders:        levels.Visible(),
		Folder:         p.FolderID,
		WantChecked:    p.Kind == "" || p.Kind.checked(),
		WantDuplicate:  p.Kind == "" || p.Kind == KindDuplicateTitle,
		WantUnopened:   p.Kind == "" || p.Kind == KindUnopened,
		UnopenedBefore: pgtype.Timestamptz{Time: unopenedBefore(now), Valid: true},
		RowLimit:       httpapi.Int32(p.Limit + 1),
	}
	if p.Kind.checked() {
		params.CheckedKind = string(p.Kind)
	}
	if cursor != nil {
		params.CursorUpdatedAt = pgtype.Timestamptz{Time: cursor.Time, Valid: true}
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
	if len(rows) > p.Limit {
		rows = rows[:p.Limit]
		last := rows[len(rows)-1]
		page.NextCursor = httpapi.EncodeTimeCursor(last.UpdatedAt.Time, last.ID.String())
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
	id := row.ID.String()
	it := Item{
		ID:           id,
		Title:        row.Title,
		URL:          links.Dashboard(s.publicURL, id, row.Title),
		Folder:       folders.Folder{ID: row.FolderID, Title: row.FolderTitle, General: row.FolderGeneral, Personal: row.FolderPersonal},
		CanEdit:      levels.Of(row.FolderID).Edit,
		CheckedAt:    timeOf(row.CheckedAt),
		LastViewedAt: lastView,
		Skipped:      row.Skipped,
		Problems:     problems,
	}
	if row.CheckedVersion.Valid {
		v := int(row.CheckedVersion.Int32)
		it.CheckedVersion = &v
	}
	return it, nil
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

func (s *Service) Repair(ctx context.Context, by auth.Principal, ids []pgtype.UUID) (repairs.Outcome, error) {
	if !s.revisions.Available() {
		return repairs.Outcome{}, revisions.ErrUnavailable
	}
	requester, err := db.ParseUUID(by.ID)
	if err != nil {
		return repairs.Outcome{}, fmt.Errorf("requester id: %w", err)
	}
	levels, err := s.access.Levels(ctx, by)
	if err != nil {
		return repairs.Outcome{}, err
	}
	out := repairs.Outcome{Queued: []repairs.Queued{}, Skipped: []repairs.Skipped{}}
	seen := make(map[pgtype.UUID]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		if err := s.repairOne(ctx, by, requester, levels, id, &out); err != nil {
			return repairs.Outcome{}, err
		}
	}
	return out, nil
}

func (s *Service) repairOne(ctx context.Context, by auth.Principal, requester pgtype.UUID, levels folders.Levels, id pgtype.UUID, out *repairs.Outcome) error {
	var reason string
	var queued dbsqlc.AgentRequest
	err := s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		latest, prompt, why, err := repairTarget(ctx, q, requester, levels, id)
		if err != nil || why != "" {
			reason = why
			return err
		}
		queued, err = s.revisions.Enqueue(ctx, q, revisions.EnqueueParams{
			DashboardID: id,
			Source:      revisions.SourcePrompt,
			Prompt:      prompt,
			BaseVersion: latest,
			By:          by,
		})
		return err
	})
	switch {
	case db.IsForeignKeyViolation(err):
		reason = repairs.ReasonNotFound
	case err != nil:
		return err
	}
	if reason != "" {
		out.Skipped = append(out.Skipped, repairs.Skipped{DashboardID: id.String(), Reason: reason})
		return nil
	}
	out.Queued = append(out.Queued, repairs.Queued{DashboardID: id.String(), RequestID: queued.ID.String()})
	return nil
}

func repairTarget(ctx context.Context, q *dbsqlc.Queries, requester pgtype.UUID, levels folders.Levels, id pgtype.UUID) (int, string, string, error) {
	if err := q.LockAgentRequestsForDashboard(ctx, id); err != nil {
		return 0, "", "", fmt.Errorf("lock agent requests: %w", err)
	}
	target, err := q.HealthRepairTarget(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", repairs.ReasonNotFound, nil
	}
	if err != nil {
		return 0, "", "", fmt.Errorf("read health repair target: %w", err)
	}
	abilities := levels.Of(target.FolderID)
	if !abilities.View {
		return 0, "", repairs.ReasonNotFound, nil
	}
	if !abilities.Edit {
		return 0, "", repairs.ReasonForbidden, nil
	}
	active, err := q.CountActiveAgentRequests(ctx, id)
	if err != nil {
		return 0, "", "", fmt.Errorf("count active agent requests: %w", err)
	}
	if active > 0 {
		return 0, "", repairs.ReasonActive, nil
	}
	problems, err := decodeProblems(target.Issues)
	if err != nil {
		return 0, "", "", err
	}
	prompt, ok := repairPrompt(problems)
	if !ok {
		return 0, "", repairs.ReasonNothingToRepair, nil
	}
	mine, err := q.CountActiveHealthRepairsBy(ctx, dbsqlc.CountActiveHealthRepairsByParams{RequesterID: requester, PromptPrefix: repairPromptPrefix})
	if err != nil {
		return 0, "", "", fmt.Errorf("count active health repairs: %w", err)
	}
	if mine >= repairs.MaxActivePerRequester {
		return 0, "", repairs.ReasonQueueFull, nil
	}
	return int(target.LatestVersion), prompt, "", nil
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

func ValidKind(raw string) (Kind, bool) {
	k := Kind(raw)
	return k, slices.Contains(Kinds, k)
}

func timeOf(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}
