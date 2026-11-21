package revisions

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

type StartRequest struct {
	Prompt        *string `json:"prompt"`
	DashboardID   *string `json:"dashboard_id"`
	BaseVersion   *int    `json:"base_version"`
	Kind          *string `json:"kind"`
	PanelID       *string `json:"panel_id"`
	From          *string `json:"from"`
	To            *string `json:"to"`
	ObservationID *string `json:"observation_id"`
	FollowsID     *string `json:"follows_id"`
	ChatID        *string `json:"chat_id"`
	TimeZone      *string `json:"time_zone"`
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

func (s *Service) Start(ctx context.Context, by auth.Principal, req StartRequest) (dbsqlc.AgentRequest, error) {
	if req.Prompt == nil {
		return dbsqlc.AgentRequest{}, refused("prompt is required")
	}
	prompt, err := CheckPrompt("prompt", *req.Prompt)
	if err != nil {
		return dbsqlc.AgentRequest{}, &StartError{Message: err.Error(), Cause: err}
	}
	if req.TimeZone != nil {
		zone, err := CheckTimeZone(*req.TimeZone)
		if err != nil {
			return dbsqlc.AgentRequest{}, &StartError{Message: err.Error(), Cause: err}
		}
		ctx = WithTimeZone(ctx, zone)
	}
	switch {
	case req.Kind == nil || *req.Kind == KindBuild:
		return s.startBuild(ctx, by, req, prompt)
	case *req.Kind == KindInvestigate:
		return s.startInvestigation(ctx, by, req, prompt)
	}
	return dbsqlc.AgentRequest{}, refused("kind must be build or investigate")
}

func (s *Service) startBuild(ctx context.Context, by auth.Principal, req StartRequest, prompt string) (dbsqlc.AgentRequest, error) {
	if message := buildFieldConflict(req); message != "" {
		return dbsqlc.AgentRequest{}, refused("%s", message)
	}
	dashboardID, err := uuidField("dashboard_id", req.DashboardID)
	if err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	followsID, err := uuidField("follows_id", req.FollowsID)
	if err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	chatID, err := uuidField("chat_id", req.ChatID)
	if err != nil {
		return dbsqlc.AgentRequest{}, err
	}
	if req.BaseVersion != nil && (*req.BaseVersion < 1 || *req.BaseVersion > math.MaxInt32) {
		return dbsqlc.AgentRequest{}, refused("base_version must be a positive 32-bit integer")
	}
	if !s.Available() {
		return dbsqlc.AgentRequest{}, ErrUnavailable
	}
	switch {
	case dashboardID.Valid && chatID.Valid:
		return s.submitFollowUp(ctx, EnqueueParams{DashboardID: dashboardID, Prompt: prompt, By: by, chat: chatNamed(chatID)}, req.BaseVersion)
	case dashboardID.Valid:
		return s.SubmitFollowUp(ctx, dashboardID, req.BaseVersion, "", prompt, by)
	case followsID.Valid:
		return s.SubmitObserveFollowUp(ctx, by, prompt, followsID)
	}
	return s.submitObserve(ctx, by, prompt, observeTurn{chat: chatNamed(chatID)})
}

func buildFieldConflict(req StartRequest) string {
	switch {
	case req.PanelID != nil || req.From != nil || req.To != nil:
		return "panel_id, from and to need kind investigate"
	case req.ObservationID != nil:
		return "observation_id goes with kind investigate"
	case req.DashboardID == nil && req.BaseVersion != nil:
		return "base_version needs dashboard_id"
	case req.DashboardID != nil && req.FollowsID != nil:
		return "follows_id and dashboard_id exclude each other"
	case req.ChatID != nil && req.FollowsID != nil:
		return "follows_id and chat_id exclude each other"
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

func (s *Service) startInvestigation(ctx context.Context, by auth.Principal, req StartRequest, prompt string) (dbsqlc.AgentRequest, error) {
	if req.BaseVersion != nil {
		return dbsqlc.AgentRequest{}, refused("base_version is not allowed for kind investigate")
	}
	if req.ChatID != nil && req.FollowsID != nil {
		return dbsqlc.AgentRequest{}, refused("follows_id and chat_id exclude each other")
	}
	params := InvestigateParams{Prompt: prompt, By: by}
	var err error
	if params.chat, err = s.investigationChat(ctx, by, req); err != nil {
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

func (s *Service) investigationChat(ctx context.Context, by auth.Principal, req StartRequest) (chatChoice, error) {
	chatID, err := uuidField("chat_id", req.ChatID)
	if err != nil {
		return chatChoice{}, err
	}
	followsID, err := uuidField("follows_id", req.FollowsID)
	if err != nil || !followsID.Valid {
		return chatNamed(chatID), err
	}
	parent, err := s.GetObserve(ctx, followsID, by)
	if err != nil {
		return chatChoice{}, err
	}
	return chatChoice{mode: sameChat, id: chatRoot(parent.ID, parent.ChatID)}, nil
}
