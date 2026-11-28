package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/netip"
	"net/url"
	"strings"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/claims"
	"github.com/slashobserve/slashobserve/internal/config"
	"github.com/slashobserve/slashobserve/internal/drafts"
	"github.com/slashobserve/slashobserve/internal/harness"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/mcpauth"
	"github.com/slashobserve/slashobserve/internal/server/mcp"
)

var errSidecarAddr = errors.New("the built-in agent needs SLASHOBSERVE_HTTP_ADDR to listen on all addresses or on a loopback address")

func loopbackURL(addr string) (*url.URL, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ip, parseErr := netip.ParseAddr(host)
	switch {
	case host == "" || (parseErr == nil && ip.IsUnspecified()):
		host = "127.0.0.1"
	case host == "localhost" || (parseErr == nil && ip.IsLoopback()):
	default:
		return nil, errSidecarAddr
	}
	return &url.URL{Scheme: "http", Host: net.JoinHostPort(host, port)}, nil
}

type sidecarSetup struct {
	path   string
	url    *url.URL
	drafts *drafts.Queue
}

func prepareAgentRuntime(cfg config.Config, providerConfigured bool, modelCalls drafts.Metrics, logger *slog.Logger) (*sidecarSetup, error) {
	if !providerConfigured {
		return nil, nil
	}
	path, err := harness.FindBinary()
	if err != nil {
		return nil, described("agent_runtime_unavailable", err)
	}
	serverURL, err := loopbackURL(cfg.HTTPAddr)
	if err != nil {
		return nil, described("invalid_config", err)
	}
	model := modelSettingsOf(cfg.LLM, cfg.Agent.SessionRenewBytes)
	queue := drafts.New(drafts.Config{Model: func() *claims.ModelSettings { return model(false) }, Timeout: tools.DraftDeadline, Metrics: modelCalls})
	return &sidecarSetup{path: path, url: serverURL, drafts: queue}, nil
}

func (s *sidecarSetup) internalURL() *url.URL {
	if s == nil {
		return nil
	}
	return s.url
}

func (s *sidecarSetup) draftQueue() tools.DraftQueue {
	if s == nil {
		return nil
	}
	return s.drafts
}

func (s *sidecarSetup) draftModel() tools.DraftModel {
	if s == nil {
		return nil
	}
	return s.drafts
}

func (s *sidecarSetup) modelSettings(cfg config.Config) func(question bool) *claims.ModelSettings {
	if s == nil {
		return nil
	}
	return modelSettingsOf(cfg.LLM, cfg.Agent.SessionRenewBytes)
}

func (s *sidecarSetup) agentSettings(cfg config.Config) *claims.AgentSettings {
	if s == nil {
		return nil
	}
	return agentSettingsOf(cfg.Agent)
}

const (
	defaultInvestigationCalls = 30
	defaultChatCalls          = 12
	callsBeforeSave           = 6
	callsBeforeProposal       = 12
)

func investigationCallsByTask() map[string]int {
	return map[string]int{"measure": 16, "logs": 20, "traces": 20, "incident": 30, "inventory": 12, "explain": 12}
}

func agentSettingsOf(a config.Agent) *claims.AgentSettings {
	return &claims.AgentSettings{
		InvestigationCalls:    a.InvestigationCalls,
		InvestigationCallsMax: a.InvestigationCallsMax,
		DefaultCalls:          defaultInvestigationCalls,
		TaskCalls:             investigationCallsByTask(),
		ChatCalls:             defaultChatCalls,
		SaveCalls:             callsBeforeSave,
		SuggestCalls:          callsBeforeProposal,
		Loop:                  a.InvestigationLoop,
		Grounding:             a.FindingGrounding,
		BriefSweep:            a.InvestigationBrief,
		SessionRenewBytes:     a.SessionRenewBytes,
	}
}

func (s *sidecarSetup) start(ctx context.Context, cfg config.Config, service *claims.Service, logger *slog.Logger) (*harness.Supervisor, func()) {
	if s == nil {
		return nil, func() {}
	}
	supervisor := harness.NewSupervisor(harness.SupervisorConfig{
		Path: s.path, ServerURL: s.url.String(), Slots: cfg.Agent.Workers,
		LogLevel: cfg.LogLevel, Logger: logger, OnExit: observeAgentExit(service, s.drafts, logger),
	})
	return supervisor, startBackground(ctx, "harness supervisor", supervisor.Run, logger)
}

func modelSettingsOf(llm config.LLM, sessionRenewBytes int) func(question bool) *claims.ModelSettings {
	settings := claims.ModelSettings{
		Provider:              llm.Provider,
		Model:                 llm.Model,
		APIKey:                llm.APIKey.Reveal(),
		Effort:                llm.Effort,
		MaxTokens:             llm.MaxTokens,
		RequestTimeoutSeconds: int(llm.RequestTimeout.Seconds()),
		IdleTimeoutSeconds:    int(llm.IdleTimeout.Seconds()),
		ReplayDir:             llm.ReplayDir,
		LogProviderErrors:     llm.LogProviderErrors,
		SessionRenewBytes:     sessionRenewBytes,
	}
	if llm.BaseURL != nil {
		settings.BaseURL = llm.BaseURL.String()
	}
	return func(question bool) *claims.ModelSettings {
		kept := settings
		if question {
			kept.Effort = llm.QuestionEffort
		}
		return &kept
	}
}

type mcpCredentials struct {
	keys       *auth.Service
	runs       *claims.Service
	oauth      *mcpauth.Server
	supervisor *harness.Supervisor
}

func (m mcpCredentials) observeAgent(header string) (mcp.Credential, bool, bool) {
	scheme, token, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") || !strings.HasPrefix(token, harness.TokenPrefix) {
		return mcp.Credential{}, false, false
	}
	if m.supervisor == nil || !m.supervisor.Authenticate(token) {
		return mcp.Credential{}, false, true
	}
	builtIn := m.runs.BuiltIn()
	return mcp.Credential{Claimer: &builtIn, Loopback: true, ClaimOnly: true}, true, true
}

func observeAgentExit(service *claims.Service, queue *drafts.Queue, logger *slog.Logger) func(context.Context) {
	return func(ctx context.Context) {
		queue.Lost()
		if err := service.ReleaseHeld(ctx, service.BuiltIn()); err != nil {
			logger.Error("release the runs the harness held", logattrs.Failure("harness_release_failed", err)...)
		}
	}
}
