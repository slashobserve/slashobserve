package revisions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/origin"
)

const (
	uniqueViolationCode   = "23505"
	runningDashboardIndex = "agent_requests_running_dashboard_idx"
	pinnedCursorPrefix    = "p."
	unpinnedCursorPrefix  = "u."
)

type Service struct {
	db      *db.DB
	reviser Reviser
	logger  *slog.Logger
	panels  PanelChecker
	specs   SpecReader
	builds  Builds
	threads Threads

	principals Principals
	access     *folders.Access
	leaseTTL   time.Duration
}

func NewService(database *db.DB, reviser Reviser, logger *slog.Logger, opts ...Option) *Service {
	s := &Service{db: database, reviser: reviser, logger: logger, leaseTTL: LeaseTTL}
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

	ContinuesID  pgtype.UUID
	ContinueNote string
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
	if db.IsForeignKeyViolation(err) {
		return dbsqlc.AgentRequest{}, ErrDashboardNotFound
	}
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
		ContinuesID:      p.ContinuesID,
		ContinueNote:     pgtype.Text{String: p.ContinueNote, Valid: p.ContinueNote != ""},
	})
	if err != nil {
		return dbsqlc.AgentRequest{}, fmt.Errorf("enqueue agent request: %w", err)
	}
	if err := recordRun(ctx, q, row, ActionEnqueue); err != nil {
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
		if err := s.checkPanel(ctx, dashboardID, latest, panelID); err != nil {
			return dbsqlc.AgentRequest{}, err
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
	if s.threads == nil {
		return dbsqlc.AgentRequest{}, errNoThreads
	}
	threadID, panelID, err := s.threads.ForRun(ctx, dashboardID, commentID)
	if err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	return s.Submit(ctx, EnqueueParams{
		DashboardID: dashboardID,
		Source:      SourceComment,
		CommentID:   threadID,
		Prompt:      prompt,
		PanelID:     panelID,
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
	var row dbsqlc.AgentRequest
	err := s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		var err error
		row, err = s.enqueueObserve(ctx, q, by, prompt, pgtype.UUID{}, "")
		return err
	})
	return row, err
}

func (s *Service) enqueueObserve(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, prompt string, continuesID pgtype.UUID, note string) (dbsqlc.AgentRequest, error) {
	requesterID, err := db.ParseUUID(by.ID)
	if err != nil {
		return dbsqlc.AgentRequest{}, fmt.Errorf("requester id: %w", err)
	}
	originKind, originClient := origin.From(ctx).Columns()
	if err := q.LockObserveRequests(ctx); err != nil {
		return dbsqlc.AgentRequest{}, fmt.Errorf("lock observe requests: %w", err)
	}
	mine, err := q.CountActiveObserveRequestsBy(ctx, requesterID)
	if err != nil {
		return dbsqlc.AgentRequest{}, fmt.Errorf("count active observe requests of requester: %w", err)
	}
	if mine >= MaxActiveObservePerPrincipal {
		return dbsqlc.AgentRequest{}, ErrTooManyRequests
	}
	all, err := q.CountActiveObserveRequests(ctx)
	if err != nil {
		return dbsqlc.AgentRequest{}, fmt.Errorf("count active observe requests: %w", err)
	}
	if all >= MaxActiveObserve {
		return dbsqlc.AgentRequest{}, ErrAgentBusy
	}
	row, err := q.EnqueueObserveRequest(ctx, dbsqlc.EnqueueObserveRequestParams{
		Prompt:           prompt,
		RequestedByKind:  string(by.Kind),
		RequestedByID:    requesterID,
		RequestedByName:  by.Name,
		RequestedByEmail: pgtype.Text{String: by.Email, Valid: by.Email != ""},
		Origin:           originKind,
		OriginClient:     originClient,
		ContinuesID:      continuesID,
		ContinueNote:     pgtype.Text{String: note, Valid: note != ""},
	})
	if err != nil {
		return dbsqlc.AgentRequest{}, fmt.Errorf("enqueue observe request: %w", err)
	}
	if err := recordRun(ctx, q, row, ActionEnqueue); err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	if err := q.NotifyEvent(ctx, dbsqlc.NotifyEventParams{Channel: WakeChannel, Payload: row.ID.String()}); err != nil {
		return dbsqlc.AgentRequest{}, fmt.Errorf("notify workers: %w", err)
	}
	return row, nil
}

type InvestigateParams struct {
	DashboardID pgtype.UUID
	PanelID     string
	Prompt      string
	From, To    *time.Time
	By          auth.Principal
}

func (s *Service) SubmitInvestigate(ctx context.Context, p InvestigateParams) (dbsqlc.AgentRequest, error) {
	if !s.Available() {
		return dbsqlc.AgentRequest{}, ErrUnavailable
	}
	if err := s.checkInvestigate(ctx, p); err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	requesterID, err := db.ParseUUID(p.By.ID)
	if err != nil {
		return dbsqlc.AgentRequest{}, fmt.Errorf("requester id: %w", err)
	}
	var row dbsqlc.AgentRequest
	err = s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		if err := q.LockObserveRequests(ctx); err != nil {
			return fmt.Errorf("lock observe requests: %w", err)
		}
		mine, err := q.CountActiveInvestigateRequestsBy(ctx, requesterID)
		if err != nil {
			return fmt.Errorf("count active investigations of requester: %w", err)
		}
		if mine >= MaxActiveInvestigatePerPrincipal {
			return ErrTooManyInvestigations
		}
		all, err := q.CountActiveInvestigateRequests(ctx)
		if err != nil {
			return fmt.Errorf("count active investigations: %w", err)
		}
		if all >= MaxActiveInvestigate {
			return ErrAgentBusy
		}
		originKind, originClient := origin.From(ctx).Columns()
		row, err = q.EnqueueInvestigateRequest(ctx, dbsqlc.EnqueueInvestigateRequestParams{
			DashboardID:      p.DashboardID,
			Prompt:           p.Prompt,
			PanelID:          pgtype.Text{String: p.PanelID, Valid: p.PanelID != ""},
			ContextFrom:      timestamp(p.From),
			ContextTo:        timestamp(p.To),
			RequestedByKind:  string(p.By.Kind),
			RequestedByID:    requesterID,
			RequestedByName:  p.By.Name,
			RequestedByEmail: pgtype.Text{String: p.By.Email, Valid: p.By.Email != ""},
			Origin:           originKind,
			OriginClient:     originClient,
		})
		if err != nil {
			return fmt.Errorf("enqueue investigate request: %w", err)
		}
		if err := recordRun(ctx, q, row, ActionEnqueue); err != nil {
			return err
		}
		if err := q.NotifyEvent(ctx, dbsqlc.NotifyEventParams{Channel: WakeChannel, Payload: row.ID.String()}); err != nil {
			return fmt.Errorf("notify workers: %w", err)
		}
		return nil
	})
	if db.IsForeignKeyViolation(err) {
		return dbsqlc.AgentRequest{}, ErrDashboardNotFound
	}
	return row, err
}

func (s *Service) checkInvestigate(ctx context.Context, p InvestigateParams) error {
	if err := s.checkRun(ctx, SourceInvestigate, p.DashboardID, p.By); err != nil {
		return err
	}
	if p.PanelID == "" {
		return nil
	}
	latest, _, err := s.baseVersionFor(ctx, p.DashboardID, nil)
	if err != nil {
		return err
	}
	return s.checkPanel(ctx, p.DashboardID, latest, p.PanelID)
}

func (s *Service) checkPanel(ctx context.Context, dashboardID pgtype.UUID, latest int, panelID string) error {
	if s.panels == nil {
		return fmt.Errorf("check panel: no panel checker configured")
	}
	known, err := s.panels.HasPanel(ctx, dashboardID, latest, panelID)
	if err != nil {
		return fmt.Errorf("check panel: %w", err)
	}
	if !known {
		return ErrUnknownPanel
	}
	return nil
}

func timestamp(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: t.UTC(), Valid: true}
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

type ListFilter struct {
	Source      string
	DashboardID pgtype.UUID
	Pinned      *bool
	Query       string
}

func (s *Service) ListMine(ctx context.Context, by auth.Principal, filter ListFilter, limit int, cursor string) ([]dbsqlc.ListAgentRequestsByRow, string, error) {
	afterPinned, after, ok := decodeListCursor(cursor)
	if !ok {
		return nil, "", ErrInvalidCursor
	}
	requesterID, err := db.ParseUUID(by.ID)
	if err != nil {
		return nil, "", fmt.Errorf("requester id: %w", err)
	}
	levels, err := s.access.Levels(ctx, by)
	switch {
	case errors.Is(err, folders.ErrForbidden):
		return nil, "", ErrForbidden
	case err != nil:
		return nil, "", fmt.Errorf("list agent requests of requester: %w", err)
	}
	params := dbsqlc.ListAgentRequestsByParams{
		RequestedByKind: string(by.Kind),
		RequestedByID:   requesterID,
		Folders:         levels.Visible(),
		Source:          pgtype.Text{String: filter.Source, Valid: filter.Source != ""},
		DashboardID:     filter.DashboardID,
		Pattern:         pgtype.Text{String: "%" + escapeLike(filter.Query) + "%", Valid: filter.Query != ""},
		RowLimit:        httpapi.Int32(limit + 1),
	}
	if filter.Pinned != nil {
		params.Pinned = pgtype.Bool{Bool: *filter.Pinned, Valid: true}
	}
	if after != nil {
		params.CursorCreatedAt = pgtype.Timestamptz{Time: after.Time, Valid: true}
		params.CursorID = after.ID
		params.CursorPinned = afterPinned
	}
	rows, err := s.db.Queries().ListAgentRequestsBy(ctx, params)
	if err != nil {
		return nil, "", fmt.Errorf("list agent requests of requester: %w", err)
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[limit-1]
		next = encodeListCursor(last.PinnedAt.Valid, last.CreatedAt.Time, last.ID.String())
	}
	return rows, next, nil
}

func encodeListCursor(pinned bool, createdAt time.Time, id string) string {
	prefix := unpinnedCursorPrefix
	if pinned {
		prefix = pinnedCursorPrefix
	}
	return prefix + httpapi.EncodeTimeCursor(createdAt, id)
}

func decodeListCursor(raw string) (bool, *httpapi.TimeCursor, bool) {
	pinned := false
	if rest, found := strings.CutPrefix(raw, pinnedCursorPrefix); found {
		pinned, raw = true, rest
	} else {
		raw = strings.TrimPrefix(raw, unpinnedCursorPrefix)
	}
	after, ok := httpapi.DecodeTimeCursor(raw)
	return pinned, after, ok
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func (s *Service) UpdateListFields(ctx context.Context, id pgtype.UUID, by auth.Principal, title *string, clearTitle bool, pinned *bool) (dbsqlc.AgentRequest, error) {
	if _, err := s.requesterRow(ctx, id, by); err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	params := dbsqlc.UpdateAgentRequestListFieldsParams{ID: id, SetTitle: title != nil || clearTitle}
	if title != nil {
		params.Title = pgtype.Text{String: *title, Valid: true}
	}
	if pinned != nil {
		params.Pinned = pgtype.Bool{Bool: *pinned, Valid: true}
	}
	row, err := s.db.Queries().UpdateAgentRequestListFields(ctx, params)
	if errors.Is(err, pgx.ErrNoRows) {
		return dbsqlc.AgentRequest{}, ErrRequestNotFound
	}
	if err != nil {
		return dbsqlc.AgentRequest{}, fmt.Errorf("update agent request list fields: %w", err)
	}
	return row, nil
}

func (s *Service) Archive(ctx context.Context, id pgtype.UUID, by auth.Principal) error {
	if _, err := s.requesterRow(ctx, id, by); err != nil {
		return err
	}
	if _, err := s.db.Queries().ArchiveAgentRequest(ctx, id); err != nil {
		return fmt.Errorf("archive agent request: %w", err)
	}
	return nil
}

type claimResult struct {
	request    dbsqlc.AgentRequest
	firstClaim bool
}

func (s *Service) claim(ctx context.Context, c Claimant) (claimResult, bool, error) {
	assignedID, err := c.assignedID()
	if err != nil {
		return claimResult{}, false, err
	}
	var claimed claimResult
	found := false
	err = s.db.InTxWithSavepoints(ctx, func(q *dbsqlc.Queries, savepoint db.Savepoint) error {
		if err := q.LockAgentRequestClaims(ctx); err != nil {
			return fmt.Errorf("lock agent request claims: %w", err)
		}
		if err := s.sweep(ctx, q, savepoint); err != nil {
			return err
		}
		row, err := q.ClaimAgentRequest(ctx, dbsqlc.ClaimAgentRequestParams{FromStates: RunMachine.From(ActionClaim), LeaseSeconds: s.leaseTTL.Seconds(), MaxAttempts: MaxAttempts,
			AssignedKind: string(c.Kind), AssignedID: assignedID, ClaimedBy: c.Key(),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("claim agent request: %w", err)
		}
		claimed, found = claimResult{request: row.AgentRequest, firstClaim: row.FirstClaim}, true
		return recordRun(ctx, q, row.AgentRequest, ActionClaim)
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolationCode && pgErr.ConstraintName == runningDashboardIndex {
		s.logger.WarnContext(ctx, "agent request claim hit the one-running-request-per-dashboard index; nothing claimed", logattrs.Failure("agent_request_claim_conflict", err)...)
		return claimResult{}, false, nil
	}
	return claimed, found, err
}

func (s *Service) deleteExpiredBuilds(ctx context.Context, savepoint db.Savepoint) {
	err := savepoint(func(q *dbsqlc.Queries) error {
		_, err := q.DeleteExpiredAgentRequestBuilds(ctx, BuildRetentionDays)
		return err
	})
	if err != nil {
		s.logger.WarnContext(ctx, "delete expired agent request builds", logattrs.Failure("agent_request_build_cleanup_failed", err)...)
	}
}

func (s *Service) renewLease(ctx context.Context, claimed dbsqlc.AgentRequest) (canceled bool, err error) {
	cancelRequestedAt, err := s.db.Queries().RenewAgentRequestLease(ctx, dbsqlc.RenewAgentRequestLeaseParams{
		LeaseSeconds: s.leaseTTL.Seconds(), ID: claimed.ID, Attempts: claimed.Attempts,
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return false, ErrLeaseLost
	case err != nil:
		return false, fmt.Errorf("renew agent request lease: %w", err)
	}
	return cancelRequestedAt.Valid, nil
}

type outcome struct {
	version    *int
	suggestion json.RawMessage
	finding    bool
	message    string
	code       string
	failure    FailureCode
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
		action := ActionSucceed
		settle := func() (dbsqlc.AgentRequest, error) {
			action = ActionSettle
			return q.SettleAgentRequest(ctx, dbsqlc.SettleAgentRequestParams{
				ID: claimed.ID, Attempts: claimed.Attempts, FromStates: RunMachine.From(ActionSettle), Message: CanceledMessage, RunUsage: usage,
			})
		}
		switch {
		case o.suggestion != nil:
			row, err = q.SucceedSuggestRequest(ctx, dbsqlc.SucceedSuggestRequestParams{
				ID: claimed.ID, Attempts: claimed.Attempts, FromStates: RunMachine.From(ActionSucceed), Suggestion: o.suggestion, RunUsage: usage,
			})
		case o.finding:
			row, err = q.SucceedFindingRequest(ctx, dbsqlc.SucceedFindingRequestParams{
				ID: claimed.ID, Attempts: claimed.Attempts, FromStates: RunMachine.From(ActionSucceed), RunUsage: usage,
			})
		case o.version != nil:
			row, err = q.SucceedAgentRequest(ctx, dbsqlc.SucceedAgentRequestParams{
				ID: claimed.ID, Attempts: claimed.Attempts, FromStates: RunMachine.From(ActionSucceed), ResultVersion: httpapi.Int32(*o.version), RunUsage: usage,
			})
		case o.failure == FailureCanceled:
			row, err = settle()
		default:
			action = ActionFail
			row, err = q.FailAgentRequest(ctx, dbsqlc.FailAgentRequestParams{
				ID: claimed.ID, Attempts: claimed.Attempts, FromStates: RunMachine.From(ActionFail), Error: truncateError(o.message), FailureCode: failureText(o.failure), RunUsage: usage,
			})
			if errors.Is(err, pgx.ErrNoRows) {
				row, err = settle()
			}
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("finish agent request: %w", err)
		}
		finished = true
		if err := recordRun(ctx, q, row, action); err != nil {
			return err
		}
		if State(row.State) == StateSucceeded {
			s.resolveThread(ctx, savepoint, row)
		}
		return nil
	})
	return finished, err
}

func failureText(code FailureCode) pgtype.Text {
	return pgtype.Text{String: string(code), Valid: code != ""}
}

func (s *Service) release(ctx context.Context, claimed dbsqlc.AgentRequest, checkFailed bool) (bool, error) {
	released := false
	err := s.db.InTxWithSavepoints(ctx, func(q *dbsqlc.Queries, savepoint db.Savepoint) error {
		row, err := q.ReleaseAgentRequest(ctx, dbsqlc.ReleaseAgentRequestParams{ID: claimed.ID, Attempts: claimed.Attempts, FromStates: RunMachine.From(ActionRelease), CheckFailed: checkFailed})
		if err == nil {
			released = true
			return recordRun(ctx, q, row, ActionRelease)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("release agent request: %w", err)
		}
		row, err = q.SettleAgentRequest(ctx, dbsqlc.SettleAgentRequestParams{
			ID: claimed.ID, Attempts: claimed.Attempts, FromStates: RunMachine.From(ActionSettle), Message: CanceledMessage,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("settle released agent request: %w", err)
		}
		released = true
		if err := recordRun(ctx, q, row, ActionSettle); err != nil {
			return err
		}
		if State(row.State) == StateSucceeded {
			s.resolveThread(ctx, savepoint, row)
		}
		return nil
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

func (s *Service) hasFinding(ctx context.Context, claimed dbsqlc.AgentRequest) (bool, error) {
	_, err := s.db.Queries().GetFindingByAgentRequest(ctx, claimed.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("get finding of agent request: %w", err)
	}
	return true, nil
}

func (s *Service) resolveThread(ctx context.Context, savepoint db.Savepoint, row dbsqlc.AgentRequest) {
	if s.threads == nil {
		return
	}
	err := savepoint(func(q *dbsqlc.Queries) error { return s.threads.ResolveForRun(ctx, q, row) })
	if err != nil {
		s.logger.WarnContext(ctx, "resolve comment thread of a succeeded agent request", logattrs.Failure("agent_request_thread_resolve_failed", err, slog.String("agent_request_id", row.ID.String()))...)
	}
}

func revisionFromRow(row dbsqlc.AgentRequest) Revision {
	rev := Revision{
		ID:           row.ID.String(),
		DashboardID:  row.DashboardID.String(),
		Source:       Source(row.Source),
		Prompt:       row.Prompt,
		Status:       Status(row.Status),
		State:        State(row.State),
		ResultKind:   ResultKind(row.ResultKind),
		Attempts:     int(row.Attempts),
		Error:        row.Error.String,
		RequestedBy:  requesterOf(row),
		CreatedAt:    row.CreatedAt.Time.UTC(),
		Origin:       row.Origin.String,
		OriginClient: row.OriginClient.String,
		AssignedKind: AssigneeKind(row.AssignedKind),
		AssignedID:   row.AssignedID.String(),
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
	rev.FailureCode = row.FailureCode.String
	rev.BuildSavedAt = optionalTime(row.BuildSavedAt)
	rev.ContinueNote = row.ContinueNote.String
	if row.ContinuesID.Valid {
		rev.ContinuesID = row.ContinuesID.String()
	}
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

func KeepBuild(ctx context.Context, q *dbsqlc.Queries, id, dashboardID pgtype.UUID, version int) (bool, error) {
	row, err := q.MarkAgentRequestBuildSaved(ctx, dbsqlc.MarkAgentRequestBuildSavedParams{
		FromStates:    RunMachine.From(ActionKeepBuild),
		ResultVersion: httpapi.Int32(version),
		DashboardID:   dashboardID,
		ID:            id,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("mark agent request build saved: %w", err)
	}
	if err := recordRun(ctx, q, row, ActionKeepBuild); err != nil {
		return false, err
	}
	return true, nil
}
