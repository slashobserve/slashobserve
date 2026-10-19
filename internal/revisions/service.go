package revisions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/events"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/origin"
)

const (
	uniqueViolationCode   = "23505"
	runningDashboardIndex = "agent_requests_running_dashboard_idx"
)

type Service struct {
	db      *db.DB
	reviser Reviser
	logger  *slog.Logger
}

func NewService(database *db.DB, reviser Reviser, logger *slog.Logger) *Service {
	return &Service{db: database, reviser: reviser, logger: logger}
}

func (s *Service) Available() bool {
	return s != nil && s.reviser != nil
}

type EnqueueParams struct {
	DashboardID pgtype.UUID
	Source      Source
	CommentID   pgtype.UUID
	Prompt      string
	PanelID     string
	BaseVersion int
	By          auth.Principal
}

func (s *Service) Submit(ctx context.Context, p EnqueueParams) (dbsqlc.AgentRequest, error) {
	var row dbsqlc.AgentRequest
	err := s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		var err error
		row, err = s.Enqueue(ctx, q, p)
		return err
	})
	return row, err
}

func (s *Service) Enqueue(ctx context.Context, q *dbsqlc.Queries, p EnqueueParams) (dbsqlc.AgentRequest, error) {
	if !s.Available() {
		return dbsqlc.AgentRequest{}, ErrUnavailable
	}
	if !p.DashboardID.Valid {
		return dbsqlc.AgentRequest{}, fmt.Errorf("enqueue agent request: dashboard id is required")
	}
	requesterID, err := db.ParseUUID(p.By.ID)
	if err != nil {
		return dbsqlc.AgentRequest{}, fmt.Errorf("requester id: %w", err)
	}
	if err := q.LockAgentRequestsForDashboard(ctx, p.DashboardID); err != nil {
		return dbsqlc.AgentRequest{}, fmt.Errorf("lock agent requests: %w", err)
	}
	active, err := q.CountActiveAgentRequests(ctx, p.DashboardID)
	if err != nil {
		return dbsqlc.AgentRequest{}, fmt.Errorf("count active agent requests: %w", err)
	}
	if active >= MaxActive {
		return dbsqlc.AgentRequest{}, ErrTooManyRevisions
	}
	originKind, originClient := originColumns(ctx)
	row, err := q.EnqueueAgentRequest(ctx, dbsqlc.EnqueueAgentRequestParams{
		DashboardID:      p.DashboardID,
		Source:           string(p.Source),
		CommentID:        p.CommentID,
		Prompt:           p.Prompt,
		PanelID:          pgtype.Text{String: p.PanelID, Valid: p.PanelID != ""},
		BaseVersion:      httpapi.Int32(p.BaseVersion),
		RequestedByKind:  string(p.By.Kind),
		RequestedByID:    requesterID,
		RequestedByName:  p.By.Name,
		RequestedByEmail: pgtype.Text{String: p.By.Email, Valid: p.By.Email != ""},
		Origin:           originKind,
		OriginClient:     originClient,
	})
	if err != nil {
		return dbsqlc.AgentRequest{}, fmt.Errorf("enqueue agent request: %w", err)
	}
	if err := notifyRevision(ctx, q, row); err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	if err := q.NotifyEvent(ctx, dbsqlc.NotifyEventParams{Channel: WakeChannel, Payload: row.ID.String()}); err != nil {
		return dbsqlc.AgentRequest{}, fmt.Errorf("notify workers: %w", err)
	}
	return row, nil
}

func (s *Service) SubmitFollowUp(ctx context.Context, dashboardID pgtype.UUID, baseVersion *int, prompt string, by auth.Principal) (dbsqlc.AgentRequest, error) {
	latest, err := s.db.Queries().GetDashboardLatestVersion(ctx, dashboardID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return dbsqlc.AgentRequest{}, ErrDashboardNotFound
	case err != nil:
		return dbsqlc.AgentRequest{}, fmt.Errorf("read latest dashboard version: %w", err)
	case baseVersion != nil && *baseVersion > int(latest):
		return dbsqlc.AgentRequest{}, ErrDashboardVersionNotFound
	}
	base := int(latest)
	if baseVersion != nil {
		base = *baseVersion
	}
	return s.Submit(ctx, EnqueueParams{
		DashboardID: dashboardID,
		Source:      SourcePrompt,
		Prompt:      prompt,
		BaseVersion: base,
		By:          by,
	})
}

func (s *Service) SubmitObserve(ctx context.Context, by auth.Principal, prompt string) (dbsqlc.AgentRequest, error) {
	if !s.Available() {
		return dbsqlc.AgentRequest{}, ErrUnavailable
	}
	requesterID, err := db.ParseUUID(by.ID)
	if err != nil {
		return dbsqlc.AgentRequest{}, fmt.Errorf("requester id: %w", err)
	}
	originKind, originClient := originColumns(ctx)
	var row dbsqlc.AgentRequest
	err = s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		if err := q.LockObserveRequests(ctx); err != nil {
			return fmt.Errorf("lock observe requests: %w", err)
		}
		mine, err := q.CountActiveObserveRequestsBy(ctx, requesterID)
		if err != nil {
			return fmt.Errorf("count active observe requests of requester: %w", err)
		}
		if mine >= MaxActiveObservePerPrincipal {
			return ErrTooManyRequests
		}
		all, err := q.CountActiveObserveRequests(ctx)
		if err != nil {
			return fmt.Errorf("count active observe requests: %w", err)
		}
		if all >= MaxActiveObserve {
			return ErrAgentBusy
		}
		row, err = q.EnqueueObserveRequest(ctx, dbsqlc.EnqueueObserveRequestParams{
			Prompt:           prompt,
			RequestedByKind:  string(by.Kind),
			RequestedByID:    requesterID,
			RequestedByName:  by.Name,
			RequestedByEmail: pgtype.Text{String: by.Email, Valid: by.Email != ""},
			Origin:           originKind,
			OriginClient:     originClient,
		})
		if err != nil {
			return fmt.Errorf("enqueue observe request: %w", err)
		}
		if err := q.NotifyEvent(ctx, dbsqlc.NotifyEventParams{Channel: WakeChannel, Payload: row.ID.String()}); err != nil {
			return fmt.Errorf("notify workers: %w", err)
		}
		return nil
	})
	return row, err
}

func (s *Service) GetByID(ctx context.Context, id pgtype.UUID) (dbsqlc.AgentRequest, error) {
	row, err := s.db.Queries().GetAgentRequestByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return dbsqlc.AgentRequest{}, ErrRequestNotFound
	}
	if err != nil {
		return dbsqlc.AgentRequest{}, fmt.Errorf("get agent request: %w", err)
	}
	return row, nil
}

func (s *Service) Get(ctx context.Context, dashboardID, id pgtype.UUID) (Revision, error) {
	row, err := s.db.Queries().GetAgentRequest(ctx, dbsqlc.GetAgentRequestParams{ID: id, DashboardID: dashboardID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Revision{}, ErrNotFound
	}
	if err != nil {
		return Revision{}, fmt.Errorf("get agent request: %w", err)
	}
	return revisionFromRow(row), nil
}

func (s *Service) List(ctx context.Context, dashboardID pgtype.UUID, limit int, cursor string) ([]Revision, string, error) {
	after, ok := httpapi.DecodeTimeCursor(cursor)
	if !ok {
		return nil, "", ErrInvalidCursor
	}
	params := dbsqlc.ListAgentRequestsParams{DashboardID: dashboardID, RowLimit: httpapi.Int32(limit + 1)}
	if after != nil {
		params.CursorCreatedAt = pgtype.Timestamptz{Time: after.Time, Valid: true}
		params.CursorID = after.ID
	}
	rows, err := s.db.Queries().ListAgentRequests(ctx, params)
	if err != nil {
		return nil, "", fmt.Errorf("list agent requests: %w", err)
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		next = httpapi.EncodeTimeCursor(rows[limit-1].CreatedAt.Time, rows[limit-1].ID.String())
	}
	items := make([]Revision, len(rows))
	for i, r := range rows {
		items[i] = revisionFromRow(r)
	}
	return items, next, nil
}

func (s *Service) claim(ctx context.Context) (dbsqlc.AgentRequest, bool, error) {
	var claimed dbsqlc.AgentRequest
	found := false
	err := s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		if err := q.LockAgentRequestClaims(ctx); err != nil {
			return fmt.Errorf("lock agent request claims: %w", err)
		}
		exhausted, err := q.FailExhaustedAgentRequests(ctx, MaxAttempts)
		if err != nil {
			return fmt.Errorf("fail exhausted agent requests: %w", err)
		}
		for _, row := range exhausted {
			if err := notifyRevision(ctx, q, row); err != nil {
				return err
			}
		}
		row, err := q.ClaimAgentRequest(ctx, MaxAttempts)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("claim agent request: %w", err)
		}
		claimed, found = row, true
		return notifyRevision(ctx, q, row)
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolationCode && pgErr.ConstraintName == runningDashboardIndex {
		s.logger.WarnContext(ctx, "agent request claim hit the one-running-request-per-dashboard index; nothing claimed", logattrs.Failure("agent_request_claim_conflict", err)...)
		return dbsqlc.AgentRequest{}, false, nil
	}
	return claimed, found, err
}

type outcome struct {
	version *int
	message string
	code    string
	cause   error
	usage   *Usage
}

func (s *Service) finish(ctx context.Context, claimed dbsqlc.AgentRequest, o outcome) (bool, error) {
	var usage []byte
	if o.usage != nil {
		var err error
		if usage, err = json.Marshal(o.usage); err != nil {
			s.logger.WarnContext(ctx, "marshal agent run usage", logattrs.Failure("agent_usage_encode_failed", err, slog.String("agent_request_id", claimed.ID.String()))...)
			usage = nil
		}
	}
	finished := false
	err := s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		var row dbsqlc.AgentRequest
		var err error
		if o.version != nil {
			row, err = q.SucceedAgentRequest(ctx, dbsqlc.SucceedAgentRequestParams{
				ID: claimed.ID, Attempts: claimed.Attempts, ResultVersion: httpapi.Int32(*o.version), RunUsage: usage,
			})
		} else {
			row, err = q.FailAgentRequest(ctx, dbsqlc.FailAgentRequestParams{
				ID: claimed.ID, Attempts: claimed.Attempts, Error: truncateError(o.message), RunUsage: usage,
			})
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("finish agent request: %w", err)
		}
		finished = true
		return notifyRevision(ctx, q, row)
	})
	return finished, err
}

func (s *Service) release(ctx context.Context, claimed dbsqlc.AgentRequest, checkFailed bool) (bool, error) {
	released := false
	err := s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		row, err := q.ReleaseAgentRequest(ctx, dbsqlc.ReleaseAgentRequestParams{ID: claimed.ID, Attempts: claimed.Attempts, CheckFailed: checkFailed})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("release agent request: %w", err)
		}
		released = true
		return notifyRevision(ctx, q, row)
	})
	return released, err
}

func (s *Service) existingVersion(ctx context.Context, claimed dbsqlc.AgentRequest) (int, bool, error) {
	saved, err := s.db.Queries().GetAgentRequestSavedVersion(ctx, dbsqlc.GetAgentRequestSavedVersionParams{AgentRequestID: claimed.ID, DashboardID: claimed.DashboardID})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("get saved version of agent request: %w", err)
	}
	return int(saved.Version), true, nil
}

func notifyRevision(ctx context.Context, q *dbsqlc.Queries, row dbsqlc.AgentRequest) error {
	if row.Source == string(SourceObserve) {
		return nil
	}
	ev := events.Event{
		Type:        events.TypeRevisionUpdated,
		DashboardID: row.DashboardID.String(),
		RevisionID:  row.ID.String(),
		Status:      row.Status,
	}
	if row.ResultVersion.Valid {
		ev.Version = int(row.ResultVersion.Int32)
	}
	if row.CommentID.Valid {
		ev.CommentID = row.CommentID.String()
	}
	payload, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	if err := q.NotifyEvent(ctx, dbsqlc.NotifyEventParams{Channel: events.Channel, Payload: string(payload)}); err != nil {
		return fmt.Errorf("notify event: %w", err)
	}
	return nil
}

func originColumns(ctx context.Context) (kind, client pgtype.Text) {
	o := origin.From(ctx)
	kind = pgtype.Text{String: string(o.Kind), Valid: o.Kind != ""}
	client = pgtype.Text{String: o.Client, Valid: o.Kind == origin.MCP && o.Client != ""}
	return kind, client
}

func revisionFromRow(row dbsqlc.AgentRequest) Revision {
	rev := Revision{
		ID:           row.ID.String(),
		DashboardID:  row.DashboardID.String(),
		Source:       Source(row.Source),
		Prompt:       row.Prompt,
		BaseVersion:  int(row.BaseVersion.Int32),
		Status:       Status(row.Status),
		Attempts:     int(row.Attempts),
		Error:        row.Error.String,
		RequestedBy:  requesterOf(row),
		CreatedAt:    row.CreatedAt.Time.UTC(),
		Origin:       row.Origin.String,
		OriginClient: row.OriginClient.String,
	}
	if row.CommentID.Valid {
		rev.CommentID = row.CommentID.String()
	}
	if row.PanelID.Valid {
		rev.PanelID = row.PanelID.String
	}
	if row.ResultVersion.Valid {
		n := int(row.ResultVersion.Int32)
		rev.ResultVersion = &n
	}
	rev.StartedAt = optionalTime(row.StartedAt)
	rev.FinishedAt = optionalTime(row.FinishedAt)
	return rev
}

func requesterOf(row dbsqlc.AgentRequest) auth.Principal {
	return auth.Principal{
		Kind:  auth.PrincipalKind(row.RequestedByKind),
		ID:    row.RequestedByID.String(),
		Name:  row.RequestedByName,
		Email: row.RequestedByEmail.String,
	}
}

func optionalTime(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}
