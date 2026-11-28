package revisions

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func (s *Service) SubmitFollowUp(ctx context.Context, dashboardID pgtype.UUID, baseVersion *int, panelID string, prompt string, by auth.Principal) (dbsqlc.AgentRequest, error) {
	return s.submitFollowUp(ctx, EnqueueParams{DashboardID: dashboardID, PanelID: panelID, Prompt: prompt, By: by, chat: chatChoice{mode: dashboardChat}}, baseVersion)
}

func (s *Service) SetLeaseTTL(d time.Duration) { s.leaseTTL = d }

func (s *Service) ClaimRow(ctx context.Context) (dbsqlc.AgentRequest, bool, error) {
	claimed, found, err := s.claim(ctx, Claimant{Kind: AssignBuiltin})
	return claimed.request, found, err
}

func (s *Service) ReleaseRow(ctx context.Context, claimed dbsqlc.AgentRequest, checkFailed bool) (bool, error) {
	return s.release(ctx, claimed, checkFailed)
}
