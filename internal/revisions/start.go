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
}

type StartError struct {
	Message string
}

func (e *StartError) Error() string { return e.Message }

func refused(format string, args ...any) error {
	return &StartError{Message: fmt.Sprintf(format, args...)}
}

func (s *Service) Start(ctx context.Context, by auth.Principal, req StartRequest) (dbsqlc.AgentRequest, error) {
	if req.Prompt == nil {
		return dbsqlc.AgentRequest{}, refused("prompt is required")
	}
	prompt, ok := CleanPrompt(*req.Prompt)
	if !ok {
		return dbsqlc.AgentRequest{}, refused("prompt must be 1 to %d characters without control or bidirectional override characters", MaxPromptRunes)
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
	if req.PanelID != nil || req.From != nil || req.To != nil {
		return dbsqlc.AgentRequest{}, refused("panel_id, from and to need kind investigate")
	}
	if req.ObservationID != nil {
		return dbsqlc.AgentRequest{}, refused("observation_id goes with kind investigate")
	}
	if req.DashboardID == nil && req.BaseVersion != nil {
		return dbsqlc.AgentRequest{}, refused("base_version needs dashboard_id")
	}
	var dashboardID pgtype.UUID
	if req.DashboardID != nil {
		id, err := db.ParseUUID(*req.DashboardID)
		if err != nil {
			return dbsqlc.AgentRequest{}, refused("dashboard_id must be a UUID")
		}
		dashboardID = id
	}
	if req.BaseVersion != nil && (*req.BaseVersion < 1 || *req.BaseVersion > math.MaxInt32) {
		return dbsqlc.AgentRequest{}, refused("base_version must be a positive 32-bit integer")
	}
	if !s.Available() {
		return dbsqlc.AgentRequest{}, ErrUnavailable
	}
	if dashboardID.Valid {
		return s.SubmitFollowUp(ctx, dashboardID, req.BaseVersion, "", prompt, by)
	}
	return s.SubmitObserve(ctx, by, prompt)
}

func (s *Service) startInvestigation(ctx context.Context, by auth.Principal, req StartRequest, prompt string) (dbsqlc.AgentRequest, error) {
	if req.BaseVersion != nil {
		return dbsqlc.AgentRequest{}, refused("base_version is not allowed for kind investigate")
	}
	params := InvestigateParams{Prompt: prompt, By: by}
	if req.DashboardID != nil {
		id, err := db.ParseUUID(*req.DashboardID)
		if err != nil {
			return dbsqlc.AgentRequest{}, refused("dashboard_id must be a UUID")
		}
		params.DashboardID = id
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
	if req.ObservationID != nil {
		id, err := db.ParseUUID(*req.ObservationID)
		if err != nil {
			return dbsqlc.AgentRequest{}, refused("observation_id must be a UUID")
		}
		params.ObservationID = id
	}
	var err error
	if params.From, params.To, err = investigateWindow(req.From, req.To, time.Now()); err != nil {
		return dbsqlc.AgentRequest{}, refused("%s", err.Error())
	}
	if !s.Available() {
		return dbsqlc.AgentRequest{}, ErrUnavailable
	}
	return s.SubmitInvestigate(ctx, params)
}
