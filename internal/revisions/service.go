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

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/origin"
	"github.com/slashobserve/slashobserve/internal/placement"
)

const (
	uniqueViolationCode   = "23505"
	runningDashboardIndex = "agent_requests_running_dashboard_idx"
	pinnedCursorPrefix    = "p."
	unpinnedCursorPrefix  = "u."
)

type Service struct {
	db           *db.DB
	builtInAgent bool
	metrics      Metrics
	logger       *slog.Logger
	panels       PanelChecker
	specs        SpecReader
	builds       Builds
	threads      Threads
	threadReads  ThreadReader

	observations RunObservations
	principals   Principals
	access       *folders.Access
	leaseTTL     time.Duration
	reads        *audit.Repeats
}

func NewService(database *db.DB, logger *slog.Logger, opts ...Option) *Service {
	s := &Service{db: database, logger: logger, leaseTTL: LeaseTTL, metrics: noMetrics{}, reads: audit.NewRepeats()}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *Service) Available() bool {
	return s != nil && s.builtInAgent
}

type EnqueueParams struct {
	DashboardID pgtype.UUID
	CommentID   pgtype.UUID
	Prompt      string
	PanelID     string
	BaseVersion int
	By          auth.Principal

	ContinuesID  pgtype.UUID
	ContinueNote string
	RetriesID    pgtype.UUID

	Assignee        Assignee
	Trigger         string
	ThreadMessageID pgtype.UUID
	ThreadContext   string
	Point           Point

	AppliesFindingID pgtype.UUID

	chat     chatChoice
	clientID pgtype.UUID
}

type Point struct {
	Anchor  []byte
	BlockID pgtype.UUID
}

func (s *Service) Submit(ctx context.Context, p EnqueueParams) (dbsqlc.AgentRequest, error) {
	if err := s.checkRun(ctx, RunTarget{Intent: IntentApply, DashboardID: p.DashboardID}, p.By); err != nil {
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
	if p.Assignee.builtin() && !s.Available() {
		return dbsqlc.AgentRequest{}, ErrUnavailable
	}
	return s.enqueue(ctx, q, p)
}

func (s *Service) EnqueueAssigned(ctx context.Context, q *dbsqlc.Queries, p EnqueueParams) (dbsqlc.AgentRequest, error) {
	return s.enqueue(ctx, q, p)
}

func (s *Service) enqueue(ctx context.Context, q *dbsqlc.Queries, p EnqueueParams) (dbsqlc.AgentRequest, error) {
	if !p.DashboardID.Valid {
		return dbsqlc.AgentRequest{}, fmt.Errorf("enqueue agent request: dashboard id is required")
	}
	requesterID, err := db.ParseUUID(p.By.ID)
	if err != nil {
		return dbsqlc.AgentRequest{}, fmt.Errorf("requester id: %w", err)
	}
	slot, err := takeChat(ctx, q, p.By, p.chat, chatSubject{dashboard: p.DashboardID})
	if err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	if err := q.LockAgentRequestsForDashboard(ctx, p.DashboardID); err != nil {
		return dbsqlc.AgentRequest{}, fmt.Errorf("lock agent requests: %w", err)
	}
	if err := placement.AppliedAnswer(ctx, q, p.AppliesFindingID, p.DashboardID); err != nil {
		return dbsqlc.AgentRequest{}, err
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
		ID:               slot.turn,
		ChatID:           slot.chat,
		DashboardID:      p.DashboardID,
		Intent:           string(IntentApply),
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
		RetriesID:        p.RetriesID,
		AssignedKind:     p.Assignee.kindText(),
		AssignedID:       p.Assignee.ID,
		Trigger:          pgtype.Text{String: p.Trigger, Valid: p.Trigger != ""},
		ThreadMessageID:  p.ThreadMessageID,
		ThreadContext:    pgtype.Text{String: p.ThreadContext, Valid: p.ThreadContext != ""},
		TimeZone:         timeZoneColumn(ctx),
		AsOf:             asOfColumn(ctx),
		Anchor:           p.Point.Anchor,
		BlockID:          p.Point.BlockID,
		ClientID:         p.clientID,
		AppliesFindingID: p.AppliesFindingID,
	})
	if db.ViolatedUnique(err) == appliesFindingIndex {
		return dbsqlc.AgentRequest{}, ErrFindingApplied
	}
	if err != nil {
		return dbsqlc.AgentRequest{}, clientIDTaken(err, p.clientID, "enqueue agent request")
	}
	if row, err = recordEnqueued(ctx, q, row); err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	if err := recordChatTurn(ctx, q, slot, row); err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	if err := q.NotifyEvent(ctx, dbsqlc.NotifyEventParams{Channel: WakeChannel, Payload: row.ID.String()}); err != nil {
		return dbsqlc.AgentRequest{}, fmt.Errorf("notify workers: %w", err)
	}
	return row, nil
}

func clientIDTaken(err error, clientID pgtype.UUID, what string) error {
	if clientID.Valid && errors.Is(err, pgx.ErrNoRows) {
		return errClientIDTaken
	}
	return fmt.Errorf("%s: %w", what, err)
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
	return s.submitFollowUp(ctx, EnqueueParams{DashboardID: dashboardID, PanelID: panelID, Prompt: prompt, By: by, chat: chatChoice{mode: dashboardChat}}, baseVersion)
}

func (s *Service) submitFollowUp(ctx context.Context, p EnqueueParams, baseVersion *int) (dbsqlc.AgentRequest, error) {
	if err := s.checkRun(ctx, RunTarget{Intent: IntentApply, DashboardID: p.DashboardID}, p.By); err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	latest, base, err := s.baseVersionFor(ctx, p.DashboardID, baseVersion)
	if err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	if p.PanelID != "" {
		if err := s.checkPanel(ctx, p.DashboardID, latest, p.PanelID); err != nil {
			return dbsqlc.AgentRequest{}, err
		}
	}
	p.BaseVersion = base
	return s.submit(ctx, p)
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
		CommentID:   threadID,
		Prompt:      prompt,
		PanelID:     panelID,
		BaseVersion: base,
		By:          by,
	})
}

func (s *Service) SubmitObserve(ctx context.Context, by auth.Principal, prompt string) (dbsqlc.AgentRequest, error) {
	return s.submitObserve(ctx, by, prompt, observeTurn{chat: chatChoice{mode: newChat}})
}

type observeTurn struct {
	chat     chatChoice
	retries  pgtype.UUID
	clientID pgtype.UUID
}

func (s *Service) submitObserve(ctx context.Context, by auth.Principal, prompt string, turn observeTurn) (dbsqlc.AgentRequest, error) {
	if !s.Available() {
		return dbsqlc.AgentRequest{}, ErrUnavailable
	}
	if err := s.checkRun(ctx, RunTarget{Intent: IntentAsk}, by); err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	var row dbsqlc.AgentRequest
	err := s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		var err error
		row, err = s.enqueueObserve(ctx, q, by, prompt, pgtype.UUID{}, "", turn)
		return err
	})
	return row, err
}

func (s *Service) enqueueObserve(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, prompt string, continuesID pgtype.UUID, note string, turn observeTurn) (dbsqlc.AgentRequest, error) {
	requesterID, err := db.ParseUUID(by.ID)
	if err != nil {
		return dbsqlc.AgentRequest{}, fmt.Errorf("requester id: %w", err)
	}
	originKind, originClient := origin.From(ctx).Columns()
	slot, err := takeChat(ctx, q, by, turn.chat, chatSubject{})
	if err != nil {
		return dbsqlc.AgentRequest{}, err
	}
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
		ID:               slot.turn,
		Prompt:           prompt,
		RequestedByKind:  string(by.Kind),
		RequestedByID:    requesterID,
		RequestedByName:  by.Name,
		RequestedByEmail: pgtype.Text{String: by.Email, Valid: by.Email != ""},
		Origin:           originKind,
		OriginClient:     originClient,
		ContinuesID:      continuesID,
		ContinueNote:     pgtype.Text{String: note, Valid: note != ""},
		RetriesID:        turn.retries,
		ChatID:           slot.chat,
		TimeZone:         timeZoneColumn(ctx),
		AsOf:             asOfColumn(ctx),
		ClientID:         turn.clientID,
	})
	if err != nil {
		return dbsqlc.AgentRequest{}, clientIDTaken(err, turn.clientID, "enqueue observe request")
	}
	if row, err = recordEnqueued(ctx, q, row); err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	if err := recordChatTurn(ctx, q, slot, row); err != nil {
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

	ObservationID    pgtype.UUID
	ReuseObservation bool
	RetriesID        pgtype.UUID

	Assignee        Assignee
	Trigger         string
	ThreadMessageID pgtype.UUID
	ThreadContext   string
	Point           Point
	Ask             bool
	Private         bool

	chat     chatChoice
	clientID pgtype.UUID
}

func (s *Service) SubmitInvestigate(ctx context.Context, p InvestigateParams) (dbsqlc.AgentRequest, error) {
	if p.Assignee.builtin() && !s.Available() {
		return dbsqlc.AgentRequest{}, ErrUnavailable
	}
	if err := s.checkInvestigate(ctx, p); err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	var row dbsqlc.AgentRequest
	err := s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		var err error
		row, err = s.enqueueInvestigate(ctx, q, p)
		return err
	})
	switch db.ViolatedForeignKey(err) {
	case "":
		return row, err
	case anchoredBlockForeignKey:
		return dbsqlc.AgentRequest{}, ErrObservationNotFound
	}
	return dbsqlc.AgentRequest{}, ErrDashboardNotFound
}

func (s *Service) EnqueueAssignedInvestigate(ctx context.Context, q *dbsqlc.Queries, p InvestigateParams) (dbsqlc.AgentRequest, error) {
	return s.enqueueInvestigate(ctx, q, p)
}

func (s *Service) enqueueInvestigate(ctx context.Context, q *dbsqlc.Queries, p InvestigateParams) (dbsqlc.AgentRequest, error) {
	requesterID, err := db.ParseUUID(p.By.ID)
	if err != nil {
		return dbsqlc.AgentRequest{}, fmt.Errorf("requester id: %w", err)
	}
	subject := chatSubject{dashboard: p.DashboardID}
	if p.ObservationID.Valid {
		subject = chatSubject{observation: p.ObservationID}
	}
	slot, err := takeChat(ctx, q, p.By, p.chat, subject)
	if err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	if !p.Ask && !p.Private {
		if err := checkInvestigateCaps(ctx, q, requesterID); err != nil {
			return dbsqlc.AgentRequest{}, err
		}
	}
	originKind, originClient := origin.From(ctx).Columns()
	row, err := q.EnqueueInvestigateRequest(ctx, dbsqlc.EnqueueInvestigateRequestParams{
		ID:               slot.turn,
		ChatID:           slot.chat,
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
		RetriesID:        p.RetriesID,
		AssignedKind:     p.Assignee.kindText(),
		AssignedID:       p.Assignee.ID,
		Trigger:          pgtype.Text{String: p.Trigger, Valid: p.Trigger != ""},
		ThreadMessageID:  p.ThreadMessageID,
		ThreadContext:    pgtype.Text{String: p.ThreadContext, Valid: p.ThreadContext != ""},
		TimeZone:         timeZoneColumn(ctx),
		AsOf:             asOfColumn(ctx),
		Anchor:           p.Point.Anchor,
		BlockID:          p.Point.BlockID,
		ClientID:         p.clientID,

		SubjectObservationID: p.subject(),
		Investigation:        p.opensObservation(),
	})
	if err != nil {
		return dbsqlc.AgentRequest{}, clientIDTaken(err, p.clientID, "enqueue investigate request")
	}
	if row, err = recordEnqueued(ctx, q, row); err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	if err := recordChatTurn(ctx, q, slot, row); err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	if p.opensObservation() {
		if row, err = s.openObservation(ctx, q, row, p); err != nil {
			return dbsqlc.AgentRequest{}, err
		}
		if err := seedNoObservationStep(ctx, q, row); err != nil {
			return dbsqlc.AgentRequest{}, err
		}
	}
	if err := lockAnchoredBlock(ctx, q, p); err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	if err := q.NotifyEvent(ctx, dbsqlc.NotifyEventParams{Channel: WakeChannel, Payload: row.ID.String()}); err != nil {
		return dbsqlc.AgentRequest{}, fmt.Errorf("notify workers: %w", err)
	}
	return row, nil
}

func checkInvestigateCaps(ctx context.Context, q *dbsqlc.Queries, requesterID pgtype.UUID) error {
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
	return nil
}

func (s *Service) openObservation(ctx context.Context, q *dbsqlc.Queries, row dbsqlc.AgentRequest, p InvestigateParams) (dbsqlc.AgentRequest, error) {
	if s.observations == nil {
		return row, nil
	}
	id, err := s.observations.OpenForRun(ctx, q, RunObservationOpen{Run: row, By: p.By, ObservationID: p.ObservationID, Strict: !p.ReuseObservation})
	switch {
	case errors.Is(err, folders.ErrForbidden):
		return row, ErrForbidden
	case err != nil:
		return row, err
	case !id.Valid:
		return row, nil
	}
	linked, err := q.SetRequestObservation(ctx, dbsqlc.SetRequestObservationParams{ID: row.ID, ObservationID: id})
	if err != nil {
		return row, fmt.Errorf("link run to observation: %w", err)
	}
	return linked, nil
}

const anchoredBlockForeignKey = "agent_requests_block_id_fkey"

func lockAnchoredBlock(ctx context.Context, q *dbsqlc.Queries, p InvestigateParams) error {
	if !p.Point.BlockID.Valid {
		return nil
	}
	_, err := q.LockObservationBlock(ctx, dbsqlc.LockObservationBlockParams{ID: p.Point.BlockID, ObservationID: p.ObservationID})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ErrObservationNotFound
	case err != nil:
		return fmt.Errorf("lock the anchored block: %w", err)
	}
	return nil
}

func (p InvestigateParams) opensObservation() bool {
	return !p.Private && !p.ThreadMessageID.Valid
}

func (p InvestigateParams) subject() pgtype.UUID {
	if p.opensObservation() {
		return pgtype.UUID{}
	}
	return p.ObservationID
}

func (p InvestigateParams) target() RunTarget {
	return RunTarget{Intent: IntentAsk, DashboardID: p.DashboardID, ObservationID: p.ObservationID, Files: !p.Ask && !p.Private}
}

func (s *Service) checkInvestigate(ctx context.Context, p InvestigateParams) error {
	if err := s.checkRun(ctx, p.target(), p.By); err != nil {
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
	if SourceOf(row) == SourceSuggest {
		return dbsqlc.AgentRequest{}, ErrRequestNotFound
	}
	if sameActor(requesterOf(row), by) {
		return row, nil
	}
	hidden, err := s.hidesRun(ctx, row, by)
	if err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	switch {
	case PrivateAsk(row), hidden:
		return dbsqlc.AgentRequest{}, ErrRequestNotFound
	case AskedPrivately(row):
		return row, s.checkObservationVisible(ctx, row.ObservationID, by)
	case row.DashboardID.Valid:
		return row, nil
	case row.ObservationID.Valid:
		err := s.checkObservationVisible(ctx, row.ObservationID, by)
		if errors.Is(err, ErrRequestNotFound) && row.SubjectObservationID.Valid {
			err = s.checkObservationVisible(ctx, row.SubjectObservationID, by)
		}
		return row, err
	case row.SubjectObservationID.Valid:
		return row, s.checkObservationVisible(ctx, row.SubjectObservationID, by)
	}
	return dbsqlc.AgentRequest{}, ErrRequestNotFound
}

func (s *Service) ReadObserve(ctx context.Context, id pgtype.UUID, by auth.Principal) (dbsqlc.AgentRequest, bool, error) {
	row, readOnly, err := s.resolveObserve(ctx, id, by)
	if err == nil {
		s.recordRunView(ctx, row)
	}
	return row, readOnly, err
}

func (s *Service) resolveObserve(ctx context.Context, id pgtype.UUID, by auth.Principal) (dbsqlc.AgentRequest, bool, error) {
	row, err := s.GetObserve(ctx, id, by)
	if err == nil {
		err = s.checkVisible(ctx, row.DashboardID, by)
	}
	if err == nil {
		return row, false, nil
	}
	if !errors.Is(err, ErrRequestNotFound) {
		return dbsqlc.AgentRequest{}, false, err
	}
	if shared, ok, sharedErr := s.sharedWith(ctx, id, by); sharedErr != nil || ok {
		return shared, ok, sharedErr
	}
	if !by.Role.AtLeast(auth.RoleAdmin) {
		return dbsqlc.AgentRequest{}, false, err
	}
	row, err = s.GetByID(ctx, id)
	if err != nil {
		return dbsqlc.AgentRequest{}, false, err
	}
	if SourceOf(row) == SourceSuggest {
		return dbsqlc.AgentRequest{}, false, ErrRequestNotFound
	}
	return row, true, nil
}

func (s *Service) hidesRun(ctx context.Context, row dbsqlc.AgentRequest, by auth.Principal) (bool, error) {
	if sameActor(requesterOf(row), by) || by.Role.AtLeast(auth.RoleAdmin) {
		return false, nil
	}
	if row.ReadRestricted {
		return true, nil
	}
	kept, err := s.db.Queries().RunAnswerKeptWithAdmins(ctx, row.ID)
	if err != nil {
		return false, fmt.Errorf("read whether the run's answer is kept with Admins: %w", err)
	}
	return kept, nil
}

func (s *Service) Get(ctx context.Context, by auth.Principal, dashboardID, id pgtype.UUID) (Revision, error) {
	row, err := s.db.Queries().GetAgentRequest(ctx, dbsqlc.GetAgentRequestParams{ID: id, DashboardID: dashboardID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Revision{}, ErrNotFound
	}
	if err != nil {
		return Revision{}, fmt.Errorf("get agent request: %w", err)
	}
	if hidden, err := s.hidesRun(ctx, row, by); err != nil || hidden {
		if err == nil {
			err = ErrNotFound
		}
		return Revision{}, err
	}
	s.recordRunView(ctx, row)
	return revisionFromRow(row), nil
}

func (s *Service) List(ctx context.Context, by auth.Principal, dashboardID pgtype.UUID, limit int, cursor string) ([]Revision, string, error) {
	after, ok := httpapi.DecodeTimeCursor(cursor)
	if !ok {
		return nil, "", ErrInvalidCursor
	}
	reader, err := readerOf(by)
	if err != nil {
		return nil, "", err
	}
	params := dbsqlc.ListAgentRequestsParams{DashboardID: dashboardID, RowLimit: httpapi.Int32(limit + 1), ReaderAdmin: reader.admin, ReaderKind: reader.kind, ReaderID: reader.id}
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
	s.recordDashboardRunList(ctx, dashboardID)
	return items, next, nil
}

type ListScope string

const (
	ScopeMine ListScope = "mine"
	ScopeAll  ListScope = "all"
)

type ListFilter struct {
	Scope         ListScope
	DashboardID   pgtype.UUID
	ObservationID pgtype.UUID
	ChatID        pgtype.UUID
	Pinned        *bool
	Query         string
	States        []State
	Anchored      bool
}

func (f ListFilter) check() error {
	if f.Anchored && f.Scope != ScopeMine {
		return ErrAnchoredNeedsMine
	}
	return nil
}

func (f ListFilter) stateNames() []string {
	if len(f.States) == 0 {
		return nil
	}
	names := make([]string, len(f.States))
	for i, state := range f.States {
		names[i] = string(state)
	}
	return names
}

type listMatch struct {
	pattern pgtype.Text
	pinned  pgtype.Bool
	states  []string
}

func (f ListFilter) match() listMatch {
	m := listMatch{
		pattern: pgtype.Text{String: "%" + escapeLike(f.Query) + "%", Valid: f.Query != ""},
		states:  f.stateNames(),
	}
	if f.Pinned != nil {
		m.pinned = pgtype.Bool{Bool: *f.Pinned, Valid: true}
	}
	return m
}

type RequestCount struct {
	Total  int64
	People *int64
}

func (s *Service) CountRequests(ctx context.Context, by auth.Principal, filter ListFilter) (RequestCount, error) {
	if err := filter.check(); err != nil {
		return RequestCount{}, err
	}
	if filter.Scope == ScopeAll && !by.Role.AtLeast(auth.RoleAdmin) {
		return RequestCount{}, ErrForbidden
	}
	requesterID, err := db.ParseUUID(by.ID)
	if err != nil {
		return RequestCount{}, fmt.Errorf("requester id: %w", err)
	}
	levels, err := s.access.Levels(ctx, by)
	switch {
	case errors.Is(err, folders.ErrForbidden):
		return RequestCount{}, ErrForbidden
	case err != nil:
		return RequestCount{}, fmt.Errorf("count agent requests: %w", err)
	}
	match := filter.match()
	if filter.Scope == ScopeAll {
		row, err := s.db.Queries().CountAgentRequestsAll(ctx, dbsqlc.CountAgentRequestsAllParams{
			Folders:       levels.Visible(),
			DashboardID:   filter.DashboardID,
			ObservationID: filter.ObservationID,
			ChatID:        filter.ChatID,
			Pinned:        match.pinned,
			Pattern:       match.pattern,
			States:        match.states,
		})
		if err != nil {
			return RequestCount{}, fmt.Errorf("count agent requests: %w", err)
		}
		return RequestCount{Total: row.Total, People: &row.People}, nil
	}
	total, err := s.db.Queries().CountAgentRequestsBy(ctx, dbsqlc.CountAgentRequestsByParams{
		RequestedByKind: string(by.Kind),
		RequestedByID:   requesterID,
		Folders:         levels.Visible(),
		DashboardID:     filter.DashboardID,
		ObservationID:   filter.ObservationID,
		ChatID:          filter.ChatID,
		Pinned:          match.pinned,
		Pattern:         match.pattern,
		States:          match.states,
		Anchored:        filter.Anchored,
	})
	if err != nil {
		return RequestCount{}, fmt.Errorf("count agent requests: %w", err)
	}
	return RequestCount{Total: total}, nil
}

func (s *Service) ListRequests(ctx context.Context, by auth.Principal, filter ListFilter, limit int, cursor string) ([]dbsqlc.ListAgentRequestsByRow, string, error) {
	if err := filter.check(); err != nil {
		return nil, "", err
	}
	afterPinned, after, ok := decodeListCursor(cursor)
	if !ok {
		return nil, "", ErrInvalidCursor
	}
	if filter.Scope == ScopeAll && !by.Role.AtLeast(auth.RoleAdmin) {
		return nil, "", ErrForbidden
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
		return nil, "", fmt.Errorf("list agent requests: %w", err)
	}
	match := filter.match()
	var cursorAt pgtype.Timestamptz
	var cursorID pgtype.UUID
	if after != nil {
		cursorAt, cursorID = pgtype.Timestamptz{Time: after.Time, Valid: true}, after.ID
	}
	var rows []dbsqlc.ListAgentRequestsByRow
	if filter.Scope == ScopeAll {
		all, err := s.db.Queries().ListAgentRequestsAll(ctx, dbsqlc.ListAgentRequestsAllParams{
			Folders:         levels.Visible(),
			DashboardID:     filter.DashboardID,
			ObservationID:   filter.ObservationID,
			ChatID:          filter.ChatID,
			Pinned:          match.pinned,
			Pattern:         match.pattern,
			States:          match.states,
			CursorCreatedAt: cursorAt,
			CursorID:        cursorID,
			RowLimit:        httpapi.Int32(limit + 1),
		})
		if err != nil {
			return nil, "", fmt.Errorf("list agent requests: %w", err)
		}
		rows = make([]dbsqlc.ListAgentRequestsByRow, len(all))
		for i, row := range all {
			rows[i] = dbsqlc.ListAgentRequestsByRow(row)
		}
		s.recordList(ctx, actionRunList, ScopeAll)
	} else {
		rows, err = s.db.Queries().ListAgentRequestsBy(ctx, dbsqlc.ListAgentRequestsByParams{
			RequestedByKind: string(by.Kind),
			RequestedByID:   requesterID,
			Folders:         levels.Visible(),
			DashboardID:     filter.DashboardID,
			ObservationID:   filter.ObservationID,
			ChatID:          filter.ChatID,
			Pinned:          match.pinned,
			Pattern:         match.pattern,
			States:          match.states,
			Anchored:        filter.Anchored,
			CursorCreatedAt: cursorAt,
			CursorID:        cursorID,
			CursorPinned:    afterPinned,
			RowLimit:        httpapi.Int32(limit + 1),
		})
		if err != nil {
			return nil, "", fmt.Errorf("list agent requests: %w", err)
		}
		s.recordList(ctx, actionRunList, ScopeMine)
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[limit-1]
		next = encodeListCursor(filter.Scope != ScopeAll && last.PinnedAt.Valid, last.CreatedAt.Time, last.ID.String())
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
		request := row.AgentRequest
		if row.FirstClaim {
			if request, err = withChatTranscript(ctx, q, request); err != nil {
				return err
			}
		}
		claimed, found = claimResult{request: request, firstClaim: row.FirstClaim}, true
		return recordRun(ctx, q, request, ActionClaim)
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

type outcome struct {
	version    *int
	suggestion json.RawMessage
	finding    bool
	proposal   bool
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
				ID: claimed.ID, ClaimEpoch: claimed.ClaimEpoch, FromStates: RunMachine.From(ActionSettle), Message: CanceledMessage, RunUsage: usage,
			})
		}
		switch {
		case o.suggestion != nil:
			row, err = q.SucceedSuggestRequest(ctx, dbsqlc.SucceedSuggestRequestParams{
				ID: claimed.ID, ClaimEpoch: claimed.ClaimEpoch, FromStates: RunMachine.From(ActionSucceed), Suggestion: o.suggestion, RunUsage: usage,
			})
		case o.finding:
			row, err = q.SucceedFindingRequest(ctx, dbsqlc.SucceedFindingRequestParams{
				ID: claimed.ID, ClaimEpoch: claimed.ClaimEpoch, FromStates: RunMachine.From(ActionSucceed), RunUsage: usage,
			})
		case o.proposal:
			row, err = q.SucceedProposalRequest(ctx, dbsqlc.SucceedProposalRequestParams{
				ID: claimed.ID, ClaimEpoch: claimed.ClaimEpoch, FromStates: RunMachine.From(ActionSucceed), RunUsage: usage,
			})
		case o.version != nil:
			row, err = q.SucceedAgentRequest(ctx, dbsqlc.SucceedAgentRequestParams{
				ID: claimed.ID, ClaimEpoch: claimed.ClaimEpoch, FromStates: RunMachine.From(ActionSucceed), ResultVersion: httpapi.Int32(*o.version), RunUsage: usage,
			})
		case o.failure == FailureCanceled:
			row, err = settle()
		default:
			action = ActionFail
			row, err = q.FailAgentRequest(ctx, dbsqlc.FailAgentRequestParams{
				ID: claimed.ID, ClaimEpoch: claimed.ClaimEpoch, FromStates: RunMachine.From(ActionFail), Error: truncateError(o.message), FailureCode: failureText(o.failure), RunUsage: usage,
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
		return s.recordEnded(ctx, q, savepoint, row, action)
	})
	if finished && err == nil {
		s.metrics.AgentRun(runKind(claimed), runOutcome(o))
	}
	return finished, err
}

func runKind(row dbsqlc.AgentRequest) string {
	switch {
	case SourceOf(row) == SourceInvestigate:
		return "investigate"
	case SourceOf(row) == SourceSuggest:
		return "suggest"
	case !row.DashboardID.Valid:
		return "create"
	}
	return "revise"
}

func runOutcome(o outcome) string {
	switch {
	case o.suggestion != nil, o.finding, o.proposal, o.version != nil:
		return "succeeded"
	case o.failure != "":
		return string(o.failure)
	}
	return string(FailureInternal)
}

func failureText(code FailureCode) pgtype.Text {
	return pgtype.Text{String: string(code), Valid: code != ""}
}

func (s *Service) release(ctx context.Context, claimed dbsqlc.AgentRequest, checkFailed bool) (bool, error) {
	released := false
	err := s.db.InTxWithSavepoints(ctx, func(q *dbsqlc.Queries, savepoint db.Savepoint) error {
		row, err := q.ReleaseAgentRequest(ctx, dbsqlc.ReleaseAgentRequestParams{ID: claimed.ID, ClaimEpoch: claimed.ClaimEpoch, FromStates: RunMachine.From(ActionRelease), CheckFailed: checkFailed})
		if err == nil {
			released = true
			return recordRun(ctx, q, row, ActionRelease)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("release agent request: %w", err)
		}
		row, err = q.SettleAgentRequest(ctx, dbsqlc.SettleAgentRequestParams{
			ID: claimed.ID, ClaimEpoch: claimed.ClaimEpoch, FromStates: RunMachine.From(ActionSettle), Message: CanceledMessage,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("settle released agent request: %w", err)
		}
		released = true
		return s.recordEnded(ctx, q, savepoint, row, ActionSettle)
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

func (s *Service) hasProposal(ctx context.Context, claimed dbsqlc.AgentRequest) (bool, error) {
	found, err := s.db.Queries().RunHasProposal(ctx, claimed.ID)
	if err != nil {
		return false, fmt.Errorf("check proposal of agent request: %w", err)
	}
	return found, nil
}

func (s *Service) recordEnded(ctx context.Context, q *dbsqlc.Queries, savepoint db.Savepoint, row dbsqlc.AgentRequest, action Action) error {
	if err := recordRun(ctx, q, row, action); err != nil {
		return err
	}
	s.runEnded(ctx, savepoint, row)
	return nil
}

func (s *Service) runEnded(ctx context.Context, savepoint db.Savepoint, row dbsqlc.AgentRequest) {
	if s.threads == nil || !State(row.State).Terminal() {
		return
	}
	err := savepoint(func(q *dbsqlc.Queries) error { return s.threads.RunEnded(ctx, q, row) })
	if err != nil {
		s.logger.WarnContext(ctx, "answer in the thread of an ended agent request", logattrs.Failure("agent_request_thread_end_failed", err, slog.String("agent_request_id", row.ID.String()))...)
	}
}

func revisionFromRow(row dbsqlc.AgentRequest) Revision {
	rev := Revision{
		ID:            row.ID.String(),
		DashboardID:   row.DashboardID.String(),
		Intent:        Intent(row.Intent),
		Investigation: row.Investigation,
		Source:        SourceOf(row),
		Prompt:        row.Prompt,
		Status:        State(row.State).Status(),
		State:         State(row.State),
		ResultKind:    ReportedResultKind(row.ResultKind, row.ObservationID),
		Attempts:      int(row.Attempts),
		Error:         row.Error.String,
		RequestedBy:   requesterOf(row),
		CreatedAt:     row.CreatedAt.Time.UTC(),
		Origin:        row.Origin.String,
		OriginClient:  row.OriginClient.String,
		AssignedKind:  AssigneeKind(row.AssignedKind),
		AssignedID:    row.AssignedID.String(),
		audience:      audienceOf(row),
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
