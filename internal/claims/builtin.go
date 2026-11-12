package claims

import (
	"context"
	"fmt"
	"net/url"

	"github.com/slashobserve/slashobserve/internal/revisions"
)

type ModelSettings struct {
	Provider              string `json:"provider"`
	Model                 string `json:"model"`
	APIKey                string `json:"api_key"`
	BaseURL               string `json:"base_url"`
	Effort                string `json:"effort"`
	MaxTokens             int    `json:"max_tokens"`
	RequestTimeoutSeconds int    `json:"request_timeout_seconds"`
	IdleTimeoutSeconds    int    `json:"idle_timeout_seconds"`
	ReplayDir             string `json:"replay_dir"`
	LogProviderErrors     bool   `json:"log_provider_errors"`
	SessionRenewBytes     int    `json:"session_renew_bytes"`
}

func (s *Service) BuiltIn() Claimer {
	return Claimer{Kind: revisions.AssignBuiltin, ID: s.instanceID}
}

func (s *Service) modelFor(c Claimer) *ModelSettings {
	if c.Kind != revisions.AssignBuiltin || s.model == nil {
		return nil
	}
	return s.model()
}

func (s *Service) sessionBase(c Claimer) *url.URL {
	if c.Kind == revisions.AssignBuiltin && s.internalURL != nil {
		return s.internalURL
	}
	return s.publicURL
}

func (s *Service) ReleaseHeld(ctx context.Context, c Claimer) error {
	held, err := s.db.Queries().ListHeldAgentRequests(ctx, textOf(c.claimant().Key()))
	if err != nil {
		return fmt.Errorf("list the runs a claimer holds: %w", err)
	}
	for _, row := range held {
		s.runs.Release(ctx, row)
	}
	return nil
}
