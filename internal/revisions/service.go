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
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/origin"
)

const (
	uniqueViolationCode     = "23505"
	runningDashboardIndex   = "agent_requests_running_dashboard_idx"
	maxEventAuthorNameRunes = 256
)

type Service struct {
	db      *db.DB
	reviser Reviser
	logger  *slog.Logger
	panels  PanelChecker
	specs   SpecReader

	principals Principals
	access     *folders.Access
}

func NewService(database *db.DB, reviser Reviser, logger *slog.Logger, opts ...Option) *Service {
	s := &Service{db: database, reviser: reviser, logger: logger}
	for _, opt := range opts {
		opt(s)
	}
	return s
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
	if err := s.checkRun(ctx, p.Source, p.DashboardID, p.By); err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	return s.submit(ctx, p)
}

func (s *Service) submit(ctx context.Context, p EnqueueParams) (dbsqlc.AgentRequest, error) {
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
	originKind, originClient := origin.From(ctx).Columns()
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

func (s *Service) baseVersionFor(ctx context.Context, dashboardID pgtype.UUID, baseVersion *int) (latest, base int, err error) {
	v, err := s.db.Queries().GetDashboardLatestVersion(ctx, dashboardID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return 0, 0, ErrDashboardNotFound
	case err != nil:
		return 0, 0, fmt.Errorf("read latest dashboard version: %w", err)
	case baseVersion != nil && *baseVersion > int(v):
		return 0, 0, ErrDashboardVersionNotFound
	}
	latest = int(v)
	base = latest
	if baseVersion != nil {
		base = *baseVersion
	}
	return latest, base, nil
}

func (s *Service) SubmitFollowUp(ctx context.Context, dashboardID pgtype.UUID, baseVersion *int, panelID string, prompt string, by auth.Principal) (dbsqlc.AgentRequest, error) {
	if err := s.checkRun(ctx, SourcePrompt, dashboardID, by); err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	latest, base, err := s.baseVersionFor(ctx, dashboardID, baseVersion)
	if err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	if panelID != "" {
		if s.panels == nil {
			return dbsqlc.AgentRequest{}, fmt.Errorf("check panel: no panel checker configured")
		}
		known, err := s.panels.HasPanel(ctx, dashboardID, latest, panelID)
		if err != nil {
			return dbsqlc.AgentRequest{}, fmt.Errorf("check panel: %w", err)
		}
		if !known {
			return dbsqlc.AgentRequest{}, ErrUnknownPanel
		}
	}
	return s.submit(ctx, EnqueueParams{
		DashboardID: dashboardID,
		Source:      SourcePrompt,
		Prompt:      prompt,
		PanelID:     panelID,
		BaseVersion: base,
		By:          by,
	})
}

func (s *Service) SubmitForThread(ctx context.Context, dashboardID, commentID pgtype.UUID, baseVersion *int, prompt string, by auth.Principal) (dbsqlc.AgentRequest, error) {
	_, base, err := s.baseVersionFor(ctx, dashboardID, baseVersion)
	if err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	root, err := s.db.Queries().GetCommentThreadRoot(ctx, dbsqlc.GetCommentThreadRootParams{ID: commentID, DashboardID: dashboardID})
	if errors.Is(err, pgx.ErrNoRows) {
		return dbsqlc.AgentRequest{}, ErrCommentNotFound
	}
	if err != nil {
		return dbsqlc.AgentRequest{}, fmt.Errorf("read comment thread: %w", err)
	}
	return s.Submit(ctx, EnqueueParams{
		DashboardID: dashboardID,
		Source:      SourceComment,
		CommentID:   root.ID,
		Prompt:      prompt,
		PanelID:     root.PanelID.String,
		BaseVersion: base,
		By:          by,
	})
}

func (s *Service) SubmitObserve(ctx context.Context, by auth.Principal, prompt string) (dbsqlc.AgentRequest, error) {
	if !s.Available() {
		return dbsqlc.AgentRequest{}, ErrUnavailable
	}
	if err := s.checkRun(ctx, SourceObserve, pgtype.UUID{}, by); err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	requesterID, err := db.ParseUUID(by.ID)
	if err != nil {
		return dbsqlc.AgentRequest{}, fmt.Errorf("requester id: %w", err)
	}
	originKind, originClient := origin.From(ctx).Columns()
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

func (s *Service) GetObserve(ctx context.Context, id pgtype.UUID, by auth.Principal) (dbsqlc.AgentRequest, error) {
	row, err := s.GetByID(ctx, id)
	if err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	if row.Source == string(SourceSuggest) {
		return dbsqlc.AgentRequest{}, ErrRequestNotFound
	}
	if !row.DashboardID.Valid && (row.RequestedByKind != string(by.Kind) || row.RequestedByID.String() != by.ID) {
		return dbsqlc.AgentRequest{}, ErrRequestNotFound
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

func (s *Service) ListMine(ctx context.Context, by auth.Principal, source string, limit int, cursor string) ([]dbsqlc.ListAgentRequestsByRow, string, error) {
	after, ok := httpapi.DecodeTimeCursor(cursor)
	if !ok {
		return nil, "", ErrInvalidCursor
	}
	requesterID, err := db.ParseUUID(by.ID)
	if err != nil {
		return nil, "", fmt.Errorf("requester id: %w", err)
	}
	params := dbsqlc.ListAgentRequestsByParams{
		RequestedByKind: string(by.Kind),
		RequestedByID:   requesterID,
		Source:          pgtype.Text{String: source, Valid: source != ""},
		RowLimit:        httpapi.Int32(limit + 1),
	}
	if after != nil {
		params.CursorCreatedAt = pgtype.Timestamptz{Time: after.Time, Valid: true}
		params.CursorID = after.ID
	}
	rows, err := s.db.Queries().ListAgentRequestsBy(ctx, params)
	if err != nil {
		return nil, "", fmt.Errorf("list agent requests of requester: %w", err)
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		next = httpapi.EncodeTimeCursor(rows[limit-1].CreatedAt.Time, rows[limit-1].ID.String())
	}
	return rows, next, nil
}

func (s *Service) claim(ctx context.Context) (dbsqlc.AgentRequest, bool, error) {
	var claimed dbsqlc.AgentRequest
	found := false
	err := s.db.InTxWithSavepoints(ctx, func(q *dbsqlc.Queries, savepoint db.Savepoint) error {
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
			if row.Status == string(StatusSucceeded) {
				s.resolveThread(ctx, savepoint, row)
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
	version    *int
	suggestion json.RawMessage
	message    string
	code       string
	cause      error
	usage      *Usage
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
	err := s.db.InTxWithSavepoints(ctx, func(q *dbsqlc.Queries, savepoint db.Savepoint) error {
		var row dbsqlc.AgentRequest
		var err error
		switch {
		case o.suggestion != nil:
			row, err = q.SucceedSuggestRequest(ctx, dbsqlc.SucceedSuggestRequestParams{
				ID: claimed.ID, Attempts: claimed.Attempts, Suggestion: o.suggestion, RunUsage: usage,
			})
		case o.version != nil:
			row, err = q.SucceedAgentRequest(ctx, dbsqlc.SucceedAgentRequestParams{
				ID: claimed.ID, Attempts: claimed.Attempts, ResultVersion: httpapi.Int32(*o.version), RunUsage: usage,
			})
		default:
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
		if err := notifyRevision(ctx, q, row); err != nil {
			return err
		}
		if o.version != nil {
			s.resolveThread(ctx, savepoint, row)
		}
		return nil
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
	if !row.DashboardID.Valid || row.Source == string(SourceSuggest) {
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

func (s *Service) resolveThread(ctx context.Context, savepoint db.Savepoint, row dbsqlc.AgentRequest) {
	err := savepoint(func(q *dbsqlc.Queries) error { return resolveThreadOf(ctx, q, row) })
	if err != nil {
		s.logger.WarnContext(ctx, "resolve comment thread of a succeeded agent request", logattrs.Failure("agent_request_thread_resolve_failed", err, slog.String("agent_request_id", row.ID.String()))...)
	}
}

func resolveThreadOf(ctx context.Context, q *dbsqlc.Queries, row dbsqlc.AgentRequest) error {
	if !row.CommentID.Valid || !row.DashboardID.Valid || !row.ResultVersion.Valid {
		return nil
	}
	root, err := q.ResolveThreadForAgentRequest(ctx, dbsqlc.ResolveThreadForAgentRequestParams{
		ID:                row.CommentID,
		DashboardID:       row.DashboardID,
		ResolvedByKind:    row.RequestedByKind,
		ResolvedByID:      row.RequestedByID,
		ResolvedByName:    row.RequestedByName,
		ResolvedByVersion: row.ResultVersion.Int32,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("resolve comment thread: %w", err)
	}
	ev := events.Event{
		Type:        events.TypeCommentUpdated,
		DashboardID: root.DashboardID.String(),
		CommentID:   root.ID.String(),
		PanelID:     root.PanelID.String,
		Status:      root.Status,
		Author:      &events.Author{Kind: row.RequestedByKind, ID: row.RequestedByID.String(), Name: capEventAuthorName(row.RequestedByName)},
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

func capEventAuthorName(name string) string {
	if r := []rune(name); len(r) > maxEventAuthorNameRunes {
		return string(r[:maxEventAuthorNameRunes])
	}
	return name
}

func revisionFromRow(row dbsqlc.AgentRequest) Revision {
	rev := Revision{
		ID:           row.ID.String(),
		DashboardID:  row.DashboardID.String(),
		Source:       Source(row.Source),
		Prompt:       row.Prompt,
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
	if row.BaseVersion.Valid {
		n := int(row.BaseVersion.Int32)
		rev.BaseVersion = &n
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
