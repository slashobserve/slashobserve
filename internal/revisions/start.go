package revisions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/anchor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

type StartRequest struct {
	Prompt        *string        `json:"prompt"`
	DashboardID   *string        `json:"dashboard_id"`
	BaseVersion   *int           `json:"base_version"`
	Kind          *string        `json:"kind"`
	PanelID       *string        `json:"panel_id"`
	From          *string        `json:"from"`
	To            *string        `json:"to"`
	ObservationID *string        `json:"observation_id"`
	ChatID        *string        `json:"chat_id"`
	NewChat       *bool          `json:"new_chat"`
	ClientID      *string        `json:"client_id"`
	Anchor        *anchor.Anchor `json:"anchor"`
	BlockID       *string        `json:"block_id"`
	TimeZone      *string        `json:"time_zone"`

	AppliesFindingID *string `json:"applies_finding_id"`
}

type Started struct {
	dbsqlc.AgentRequest
	Replayed bool
}

type StartError struct {
	Message string
	Cause   error
}

func (e *StartError) Error() string { return e.Message }

func (e *StartError) Unwrap() error { return e.Cause }

func refused(format string, args ...any) error {
	return &StartError{Message: fmt.Sprintf(format, args...)}
}

var (
	ErrClientIDReused = errors.New("client_id was already used for another request; send a new one")
	errClientIDTaken  = errors.New("client_id taken by a concurrent request")
)

type turnInput struct {
	clientID pgtype.UUID
	point    Point
	applies  pgtype.UUID
}

func (s *Service) Start(ctx context.Context, by auth.Principal, req StartRequest) (Started, error) {
	ctx, prompt, turn, err := s.turnOf(ctx, by, req)
	if err != nil {
		return Started{}, err
	}
	if replay, found, err := s.startedBefore(ctx, by, turn, req, prompt); err != nil || found {
		return replay, err
	}
	row, err := s.startKind(ctx, by, req, prompt, turn)
	if errors.Is(err, ErrFindingApplied) {
		if replay, found, err := s.applyingRun(ctx, by, turn.applies); err != nil || found {
			return replay, err
		}
	}
	if errors.Is(err, errClientIDTaken) {
		replay, found, err := s.replayTurn(ctx, by, turn.clientID, req, prompt)
		if err == nil && !found {
			err = fmt.Errorf("read the request of a taken client_id: %w", pgx.ErrNoRows)
		}
		return replay, err
	}
	return Started{AgentRequest: row}, err
}

func (s *Service) turnOf(ctx context.Context, by auth.Principal, req StartRequest) (context.Context, string, turnInput, error) {
	ctx, prompt, err := checkStartText(ctx, req)
	if err != nil {
		return ctx, "", turnInput{}, err
	}
	var turn turnInput
	if turn.clientID, err = uuidField("client_id", req.ClientID); err != nil {
		return ctx, "", turnInput{}, err
	}
	if turn.point, err = pointOf(req); err != nil {
		return ctx, "", turnInput{}, err
	}
	if msg := newChatConflict(req); msg != "" {
		return ctx, "", turnInput{}, refused("%s", msg)
	}
	if req.AppliesFindingID != nil {
		if turn.applies, prompt, err = s.suggestedChange(ctx, by, req); err != nil {
			return ctx, "", turnInput{}, err
		}
	}
	return ctx, prompt, turn, nil
}

func (s *Service) startedBefore(ctx context.Context, by auth.Principal, turn turnInput, req StartRequest, prompt string) (Started, bool, error) {
	if turn.clientID.Valid {
		if replay, found, err := s.replayTurn(ctx, by, turn.clientID, req, prompt); err != nil || found {
			return replay, found, err
		}
	}
	if turn.applies.Valid {
		return s.applyingRun(ctx, by, turn.applies)
	}
	return Started{}, false, nil
}

func (r StartRequest) newChat() bool { return r.NewChat != nil && *r.NewChat }

func newChatConflict(req StartRequest) string {
	if req.newChat() && req.ChatID != nil {
		return "new_chat and chat_id exclude each other"
	}
	return ""
}

func checkStartText(ctx context.Context, req StartRequest) (context.Context, string, error) {
	var prompt string
	switch {
	case req.AppliesFindingID != nil && req.Prompt != nil:
		return ctx, "", refused("applies_finding_id and prompt exclude each other: the prompt is the answer's suggested change")
	case req.AppliesFindingID == nil && req.Prompt == nil:
		return ctx, "", refused("prompt is required")
	case req.Prompt != nil:
		checked, err := CheckPrompt("prompt", *req.Prompt)
		if err != nil {
			return ctx, "", &StartError{Message: err.Error(), Cause: err}
		}
		prompt = checked
	}
	if req.TimeZone != nil {
		zone, err := CheckTimeZone(*req.TimeZone)
		if err != nil {
			return ctx, "", &StartError{Message: err.Error(), Cause: err}
		}
		ctx = WithTimeZone(ctx, zone)
	}
	return ctx, prompt, nil
}

func (s *Service) startKind(ctx context.Context, by auth.Principal, req StartRequest, prompt string, turn turnInput) (dbsqlc.AgentRequest, error) {
	kind := KindAsk
	if req.Kind != nil {
		kind = *req.Kind
	}
	switch {
	case kind == KindAsk && req.DashboardID == nil && req.ObservationID == nil:
		return s.startBuild(ctx, by, req, prompt, turn)
	case kind == KindAsk:
		return s.startInvestigation(ctx, by, req, prompt, turn, true)
	case kind == KindRevise:
		return s.startBuild(ctx, by, req, prompt, turn)
	case kind == KindInvestigate:
		return s.startInvestigation(ctx, by, req, prompt, turn, false)
	}
	return dbsqlc.AgentRequest{}, refused("kind must be ask, revise or investigate")
}

func (s *Service) replayTurn(ctx context.Context, by auth.Principal, clientID pgtype.UUID, req StartRequest, prompt string) (Started, bool, error) {
	requesterID, err := db.ParseUUID(by.ID)
	if err != nil {
		return Started{}, false, fmt.Errorf("requester id: %w", err)
	}
	row, err := s.db.Queries().GetAgentRequestByClient(ctx, dbsqlc.GetAgentRequestByClientParams{RequestedByKind: string(by.Kind), RequestedByID: requesterID, ClientID: clientID})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Started{}, false, nil
	case err != nil:
		return Started{}, false, fmt.Errorf("read the request of a client_id: %w", err)
	}
	chatID, err := uuidField("chat_id", req.ChatID)
	if err != nil {
		return Started{}, false, err
	}
	if row.Prompt != prompt || (chatID.Valid && row.ChatID != chatID) {
		return Started{}, false, ErrClientIDReused
	}
	return Started{AgentRequest: row, Replayed: true}, true, nil
}

func pointOf(req StartRequest) (Point, error) {
	if req.BlockID != nil && req.ObservationID == nil {
		return Point{}, refused("block_id needs observation_id")
	}
	var point Point
	if req.Anchor != nil {
		if req.DashboardID == nil && req.ObservationID == nil {
			return Point{}, refused("anchor needs dashboard_id or observation_id")
		}
		if err := req.Anchor.Validate(); err != nil {
			return Point{}, refused("anchor: %s", err.Error())
		}
		raw, err := json.Marshal(req.Anchor)
		if err != nil {
			return Point{}, fmt.Errorf("encode anchor: %w", err)
		}
		point.Anchor = raw
	}
	if req.BlockID == nil {
		return point, nil
	}
	if req.Anchor != nil {
		if err := req.Anchor.CheckOnBlock(); err != nil {
			return Point{}, refused("%s", err.Error())
		}
	}
	var err error
	if point.BlockID, err = uuidField("block_id", req.BlockID); err != nil {
		return Point{}, err
	}
	return point, nil
}

func (s *Service) startBuild(ctx context.Context, by auth.Principal, req StartRequest, prompt string, turn turnInput) (dbsqlc.AgentRequest, error) {
	if message := buildShapeConflict(req); message != "" {
		return dbsqlc.AgentRequest{}, refused("%s", message)
	}
	dashboardID, err := uuidField("dashboard_id", req.DashboardID)
	if err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	if req.BaseVersion != nil && (*req.BaseVersion < 1 || *req.BaseVersion > math.MaxInt32) {
		return dbsqlc.AgentRequest{}, refused("base_version must be a positive 32-bit integer")
	}
	chat, err := buildChat(req)
	if err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	if !s.Available() {
		return dbsqlc.AgentRequest{}, ErrUnavailable
	}
	if dashboardID.Valid {
		return s.submitFollowUp(ctx, EnqueueParams{DashboardID: dashboardID, Prompt: prompt, By: by, Point: turn.point, AppliesFindingID: turn.applies, chat: chat, clientID: turn.clientID}, req.BaseVersion)
	}
	return s.submitObserve(ctx, by, prompt, observeTurn{chat: chat, clientID: turn.clientID})
}

func buildChat(req StartRequest) (chatChoice, error) {
	chatID, err := uuidField("chat_id", req.ChatID)
	switch {
	case err != nil:
		return chatChoice{}, err
	case !chatID.Valid && req.DashboardID != nil && !req.newChat():
		return chatChoice{mode: dashboardChat}, nil
	}
	return chatNamed(chatID), nil
}

func buildShapeConflict(req StartRequest) string {
	if req.Kind == nil || *req.Kind != KindRevise {
		return createFieldConflict(req)
	}
	switch {
	case req.DashboardID == nil:
		return "kind revise needs dashboard_id"
	case req.PanelID != nil || req.From != nil || req.To != nil:
		return "panel_id, from and to are not allowed for kind revise"
	case req.ObservationID != nil:
		return "observation_id is not allowed for kind revise"
	}
	return ""
}

func createFieldConflict(req StartRequest) string {
	switch {
	case req.PanelID != nil:
		return "panel_id needs dashboard_id"
	case req.From != nil || req.To != nil:
		return "from and to need dashboard_id, observation_id or kind investigate"
	case req.BaseVersion != nil:
		return "base_version needs kind revise"
	}
	return ""
}

func uuidField(name string, raw *string) (pgtype.UUID, error) {
	if raw == nil {
		return pgtype.UUID{}, nil
	}
	id, err := db.ParseUUID(*raw)
	if err != nil {
		return pgtype.UUID{}, refused("%s must be a UUID", name)
	}
	return id, nil
}

func askFieldConflict(req StartRequest, private bool) string {
	switch {
	case req.BaseVersion != nil:
		return "base_version needs kind revise"
	case private && req.DashboardID != nil && req.ObservationID != nil:
		return "kind ask takes dashboard_id or observation_id, not both"
	}
	return ""
}

func (s *Service) startInvestigation(ctx context.Context, by auth.Principal, req StartRequest, prompt string, turn turnInput, private bool) (dbsqlc.AgentRequest, error) {
	if msg := askFieldConflict(req, private); msg != "" {
		return dbsqlc.AgentRequest{}, refused("%s", msg)
	}
	params := InvestigateParams{Prompt: prompt, By: by, Point: turn.point, Private: private, clientID: turn.clientID}
	var err error
	if params.chat, err = investigationChat(req); err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	if params.DashboardID, err = uuidField("dashboard_id", req.DashboardID); err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	if req.PanelID != nil {
		if !params.DashboardID.Valid {
			return dbsqlc.AgentRequest{}, refused("panel_id needs dashboard_id")
		}
		if err := ValidPanelID(*req.PanelID); err != nil {
			return dbsqlc.AgentRequest{}, refused("%s", err.Error())
		}
		params.PanelID = *req.PanelID
	}
	if params.ObservationID, err = uuidField("observation_id", req.ObservationID); err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	if params.From, params.To, err = investigateWindow(req.From, req.To, time.Now()); err != nil {
		return dbsqlc.AgentRequest{}, refused("%s", err.Error())
	}
	if !s.Available() {
		return dbsqlc.AgentRequest{}, ErrUnavailable
	}
	return s.SubmitInvestigate(ctx, params)
}

func investigationChat(req StartRequest) (chatChoice, error) {
	chatID, err := uuidField("chat_id", req.ChatID)
	return chatNamed(chatID), err
}
