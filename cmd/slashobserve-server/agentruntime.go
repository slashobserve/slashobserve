package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/netip"
	"net/url"
	"strings"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/claims"
	"github.com/slashobserve/slashobserve/internal/config"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/mcpauth"
	"github.com/slashobserve/slashobserve/internal/observeagent"
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
	path string
	url  *url.URL
}

func prepareAgentRuntime(cfg config.Config, providerConfigured bool, logger *slog.Logger) (*sidecarSetup, error) {
	if !providerConfigured {
		return nil, nil
	}
	path, err := observeagent.FindBinary()
	if err != nil {
		return nil, described("agent_runtime_unavailable", err)
	}
	serverURL, err := loopbackURL(cfg.HTTPAddr)
	if err != nil {
		return nil, described("invalid_config", err)
	}
	return &sidecarSetup{path: path, url: serverURL}, nil
}

func (s *sidecarSetup) internalURL() *url.URL {
	if s == nil {
		return nil
	}
	return s.url
}

func (s *sidecarSetup) modelSettings(cfg config.Config) func() *claims.ModelSettings {
	if s == nil {
		return nil
	}
	return modelSettingsOf(cfg.LLM, cfg.Agent.SessionRenewBytes)
}

func (s *sidecarSetup) start(ctx context.Context, cfg config.Config, service *claims.Service, logger *slog.Logger) (*observeagent.Supervisor, func()) {
	if s == nil {
		return nil, func() {}
	}
	supervisor := observeagent.NewSupervisor(observeagent.SupervisorConfig{
		Path: s.path, ServerURL: s.url.String(), Slots: cfg.Agent.Workers,
		LogLevel: cfg.LogLevel, Logger: logger, OnExit: observeAgentExit(service, logger),
	})
	return supervisor, startBackground(ctx, "observe-agent supervisor", supervisor.Run, logger)
}

func modelSettingsOf(llm config.LLM, sessionRenewBytes int) func() *claims.ModelSettings {
	settings := claims.ModelSettings{
		Provider:              llm.Provider,
		Model:                 llm.Model,
		APIKey:                llm.APIKey,
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
	return func() *claims.ModelSettings {
		kept := settings
		return &kept
	}
}

type mcpCredentials struct {
	keys       *auth.Service
	runs       *claims.Service
	oauth      *mcpauth.Server
	supervisor *observeagent.Supervisor
}

func (m mcpCredentials) observeAgent(header string) (mcp.Credential, bool, bool) {
	scheme, token, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") || !strings.HasPrefix(token, observeagent.TokenPrefix) {
		return mcp.Credential{}, false, false
	}
	if m.supervisor == nil || !m.supervisor.Authenticate(token) {
		return mcp.Credential{}, false, true
	}
	builtIn := m.runs.BuiltIn()
	return mcp.Credential{Claimer: &builtIn, Loopback: true, ClaimOnly: true}, true, true
}

func observeAgentExit(service *claims.Service, logger *slog.Logger) func(context.Context) {
	return func(ctx context.Context) {
		if err := service.ReleaseHeld(ctx, service.BuiltIn()); err != nil {
			logger.Error("release the runs observe-agent held", logattrs.Failure("observe_agent_release_failed", err)...)
		}
	}
}
