package main

import (
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/config"
	"github.com/slashobserve/slashobserve/internal/observeagent"
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
		Provider: "anthropic", Model: "m", APIKey: "k", BaseURL: base, Effort: "high", MaxTokens: 4096,
		RequestTimeout: 90 * time.Second, IdleTimeout: 30 * time.Second, ReplayDir: "/replay", LogProviderErrors: true,
	}, 262144)
	got := settings()
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
	if settings().Model != "m" {
		t.Error("a caller changed the shared settings")
	}
}

func TestModelSettingsWithoutABaseURLSendAnEmptyOne(t *testing.T) {
	if got := modelSettingsOf(config.LLM{Provider: "replay"}, 0)(); got.BaseURL != "" || got.Provider != "replay" {
		t.Fatalf("settings = %+v", got)
	}
}

func TestObserveAgentTokenIsNeverAcceptedWithoutARunningChild(t *testing.T) {
	supervisor := observeagent.NewSupervisor(observeagent.SupervisorConfig{})
	for name, m := range map[string]mcpCredentials{
		"go runtime":     {},
		"no child yet":   {supervisor: supervisor},
		"empty token":    {supervisor: supervisor},
		"lowercase auth": {supervisor: supervisor},
	} {
		header := "Bearer " + observeagent.TokenPrefix + "guess"
		if name == "empty token" {
			header = "Bearer " + observeagent.TokenPrefix
		}
		if name == "lowercase auth" {
			header = "bearer " + observeagent.TokenPrefix + "guess"
		}
		cred, ok, owned := m.observeAgent(header)
		if ok || !owned || cred.Claimer != nil {
			t.Errorf("%s: ok=%v owned=%v cred=%+v", name, ok, owned, cred)
		}
	}
}

func TestOtherBearerTokensFallThroughToTheNormalCredentials(t *testing.T) {
	m := mcpCredentials{supervisor: observeagent.NewSupervisor(observeagent.SupervisorConfig{})}
	for _, header := range []string{"", "Bearer sok_abc", "Bearer sot_abc", "Basic soa_abc", "soa_abc"} {
		if _, ok, owned := m.observeAgent(header); ok || owned {
			t.Errorf("%q: ok=%v owned=%v, want it left to the other credentials", header, ok, owned)
		}
	}
}
