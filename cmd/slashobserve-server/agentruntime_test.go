package main

import (
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/agent/guard"
	"github.com/slashobserve/slashobserve/internal/config"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/harness"
)

func TestLoopbackURLPointsTheSidecarAtThisServer(t *testing.T) {
	cases := map[string]string{
		":8080":          "http://127.0.0.1:8080",
		"0.0.0.0:8080":   "http://127.0.0.1:8080",
		"[::]:9000":      "http://127.0.0.1:9000",
		"127.0.0.1:8081": "http://127.0.0.1:8081",
		"localhost:8080": "http://localhost:8080",
		"[::1]:8080":     "http://[::1]:8080",
	}
	for addr, want := range cases {
		got, err := loopbackURL(addr)
		if err != nil {
			t.Errorf("%s: %v", addr, err)
			continue
		}
		if got.String() != want {
			t.Errorf("%s: got %s, want %s", addr, got, want)
		}
	}
}

func TestLoopbackURLRefusesAHostTheSidecarCannotBeSureToReach(t *testing.T) {
	for _, addr := range []string{"10.0.0.5:8080", "observe.internal:8080", "[2001:db8::1]:8080"} {
		if _, err := loopbackURL(addr); !errors.Is(err, errSidecarAddr) {
			t.Errorf("%s: err = %v, want errSidecarAddr", addr, err)
		}
	}
	if _, err := loopbackURL("8080"); err == nil {
		t.Error("an address without a port was accepted")
	}
}

func TestModelSettingsCarryTheConfiguredModelAndAFreshCopyEachCall(t *testing.T) {
	base, err := url.Parse("https://llm.example/v1")
	if err != nil {
		t.Fatal(err)
	}
	settings := modelSettingsOf(config.LLM{
		Provider: "anthropic", Model: "m", APIKey: datasource.NewSecret("k"), BaseURL: base, Effort: "high", MaxTokens: 4096,
		RequestTimeout: 90 * time.Second, IdleTimeout: 30 * time.Second, ReplayDir: "/replay", LogProviderErrors: true,
	}, 262144)
	got := settings(false)
	want := struct {
		provider, model, key, baseURL, effort, replay string
		maxTokens, request, idle, renew               int
		logErrors                                     bool
	}{"anthropic", "m", "k", "https://llm.example/v1", "high", "/replay", 4096, 90, 30, 262144, true}
	if got.Provider != want.provider || got.Model != want.model || got.APIKey != want.key || got.BaseURL != want.baseURL ||
		got.Effort != want.effort || got.ReplayDir != want.replay || got.MaxTokens != want.maxTokens ||
		got.RequestTimeoutSeconds != want.request || got.IdleTimeoutSeconds != want.idle ||
		got.SessionRenewBytes != want.renew || got.LogProviderErrors != want.logErrors {
		t.Fatalf("settings = %+v", got)
	}
	got.Model = "changed"
	if settings(false).Model != "m" {
		t.Error("a caller changed the shared settings")
	}
}

func TestAgentSettingsCarryTheConfigAndTheGuardDefaults(t *testing.T) {
	got := agentSettingsOf(config.Agent{
		InvestigationCalls: 20, InvestigationCallsMax: 50, InvestigationLoop: true, FindingGrounding: "enforce",
		InvestigationBrief: true, SessionRenewBytes: 65536,
	})
	if got.InvestigationCalls != 20 || got.InvestigationCallsMax != 50 || !got.Loop || got.Grounding != "enforce" || !got.BriefSweep || got.SessionRenewBytes != 65536 {
		t.Fatalf("settings = %+v", got)
	}
	if got.DefaultCalls != guard.DefaultInvestigationCalls || got.ChatCalls != guard.DefaultChatCalls || got.SaveCalls != 6 || got.SuggestCalls != 12 {
		t.Fatalf("call limits = %+v", got)
	}
	if got.TaskCalls["measure"] != 16 || got.TaskCalls["incident"] != 30 || len(got.TaskCalls) != 6 {
		t.Fatalf("task calls = %v", got.TaskCalls)
	}
}

func TestModelSettingsWithoutABaseURLSendAnEmptyOne(t *testing.T) {
	if got := modelSettingsOf(config.LLM{Provider: "replay"}, 0)(false); got.BaseURL != "" || got.Provider != "replay" {
		t.Fatalf("settings = %+v", got)
	}
}

func TestAQuestionRunsAtTheQuestionEffort(t *testing.T) {
	settings := modelSettingsOf(config.LLM{Provider: "anthropic", Effort: "", QuestionEffort: "high"}, 0)
	if got := settings(true).Effort; got != "high" {
		t.Errorf("a question runs at %q", got)
	}
	if got := settings(false).Effort; got != "" {
		t.Errorf("a build runs at %q, want the provider default", got)
	}
}

func TestObserveAgentTokenIsNeverAcceptedWithoutARunningChild(t *testing.T) {
	supervisor := harness.NewSupervisor(harness.SupervisorConfig{})
	for name, m := range map[string]mcpCredentials{
		"no supervisor":  {},
		"no child yet":   {supervisor: supervisor},
		"empty token":    {supervisor: supervisor},
		"lowercase auth": {supervisor: supervisor},
	} {
		header := "Bearer " + harness.TokenPrefix + "guess"
		if name == "empty token" {
			header = "Bearer " + harness.TokenPrefix
		}
		if name == "lowercase auth" {
			header = "bearer " + harness.TokenPrefix + "guess"
		}
		cred, ok, owned := m.observeAgent(header)
		if ok || !owned || cred.Claimer != nil {
			t.Errorf("%s: ok=%v owned=%v cred=%+v", name, ok, owned, cred)
		}
	}
}

func TestOtherBearerTokensFallThroughToTheNormalCredentials(t *testing.T) {
	m := mcpCredentials{supervisor: harness.NewSupervisor(harness.SupervisorConfig{})}
	for _, header := range []string{"", "Bearer sok_abc", "Bearer sot_abc", "Basic sob_abc", "sob_abc"} {
		if _, ok, owned := m.observeAgent(header); ok || owned {
			t.Errorf("%q: ok=%v owned=%v, want it left to the other credentials", header, ok, owned)
		}
	}
}

func TestOAuthAccessTokensAreNotClaimedByTheBuiltInAgent(t *testing.T) {
	m := mcpCredentials{supervisor: harness.NewSupervisor(harness.SupervisorConfig{})}
	header := "Bearer soa_" + strings.Repeat("a", 43)
	if _, ok, owned := m.observeAgent(header); ok || owned {
		t.Fatalf("ok=%v owned=%v, want an OAuth access token left to the OAuth check", ok, owned)
	}
}
