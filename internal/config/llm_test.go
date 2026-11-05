package config

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func llmEnv(extra map[string]string) map[string]string {
	env := map[string]string{
		envDatabaseURL:        "postgres://localhost/db",
		envPublicURL:          "http://localhost:8080",
		envOIDCIssuerURL:      "http://dex/dex",
		envOIDCClientID:       "id",
		envOIDCClientSecret:   "secret",
		envAllowedEmailDomain: "example.com",
	}
	for k, v := range extra {
		env[k] = v
	}
	return env
}

func TestLLMAPIKeyFileIsReadOnlyForKeyedProviders(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	for _, provider := range []string{"", LLMProviderReplay} {
		t.Run("ignored for provider "+provider, func(t *testing.T) {
			cfg, err := Load(envOf(llmEnv(map[string]string{
				envLLMProvider:   provider,
				envLLMReplayDir:  "fixtures",
				envLLMAPIKeyFile: missing,
			})))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.LLM.APIKey != "" {
				t.Fatal("the key file must not be read")
			}
		})
	}
	for _, provider := range []string{LLMProviderAnthropic, LLMProviderOpenAI} {
		t.Run("read for provider "+provider, func(t *testing.T) {
			_, err := Load(envOf(llmEnv(map[string]string{
				envLLMProvider:   provider,
				envLLMModel:      "m",
				envLLMAPIKeyFile: missing,
			})))
			if err == nil || !strings.Contains(err.Error(), envLLMAPIKeyFile+": cannot read the file") {
				t.Fatalf("expected a key file read error, got %v", err)
			}
		})
	}
}

func TestLLMAPIKeyFileLoadsForAnthropic(t *testing.T) {
	cfg, err := Load(envOf(llmEnv(map[string]string{
		envLLMProvider:   LLMProviderAnthropic,
		envLLMAPIKeyFile: writeFile(t, "key", "sk-secret\n"),
	})))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LLM.APIKey != "sk-secret" {
		t.Fatal("the key file was not loaded")
	}
}

func TestLLMLogProviderErrorsSetting(t *testing.T) {
	tests := []struct {
		name  string
		set   map[string]string
		want  bool
		fails bool
	}{
		{name: "unset is off", set: nil},
		{name: "empty is off", set: map[string]string{envLLMLogProviderErrors: ""}},
		{name: "false", set: map[string]string{envLLMLogProviderErrors: "false"}},
		{name: "true", set: map[string]string{envLLMLogProviderErrors: "true"}, want: true},
		{name: "true with spaces", set: map[string]string{envLLMLogProviderErrors: " true "}, want: true},
		{name: "yes is refused", set: map[string]string{envLLMLogProviderErrors: "yes"}, fails: true},
		{name: "1 is refused", set: map[string]string{envLLMLogProviderErrors: "1"}, fails: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Load(envOf(llmEnv(tt.set)))
			if tt.fails {
				if err == nil || !strings.Contains(err.Error(), envLLMLogProviderErrors+": must be true or false") {
					t.Fatalf("expected a boolean error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.LLM.LogProviderErrors != tt.want {
				t.Fatalf("LogProviderErrors = %v, want %v", cfg.LLM.LogProviderErrors, tt.want)
			}
		})
	}
}

func TestAgentLogToolInputsSetting(t *testing.T) {
	tests := []struct {
		name  string
		set   map[string]string
		want  bool
		fails bool
	}{
		{name: "unset is off", set: nil},
		{name: "empty is off", set: map[string]string{envAgentLogToolInputs: ""}},
		{name: "false", set: map[string]string{envAgentLogToolInputs: "false"}},
		{name: "true", set: map[string]string{envAgentLogToolInputs: "true"}, want: true},
		{name: "yes is refused", set: map[string]string{envAgentLogToolInputs: "yes"}, fails: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Load(envOf(llmEnv(tt.set)))
			if tt.fails {
				if err == nil || !strings.Contains(err.Error(), envAgentLogToolInputs+": must be true or false") {
					t.Fatalf("expected a boolean error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.Agent.LogToolInputs != tt.want {
				t.Fatalf("LogToolInputs = %v, want %v", cfg.Agent.LogToolInputs, tt.want)
			}
		})
	}
}

func TestLLMAPIKeyAndFileAreMutuallyExclusiveForEveryProvider(t *testing.T) {
	for _, provider := range []string{"", LLMProviderReplay, LLMProviderAnthropic} {
		_, err := Load(envOf(llmEnv(map[string]string{
			envLLMProvider:   provider,
			envLLMReplayDir:  "fixtures",
			envLLMAPIKey:     "sk-secret",
			envLLMAPIKeyFile: writeFile(t, "key", "other\n"),
		})))
		if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
			t.Fatalf("provider %q: expected a mutual exclusion error, got %v", provider, err)
		}
		if strings.Contains(err.Error(), "sk-secret") {
			t.Fatalf("provider %q: the error echoes the key", provider)
		}
	}
}

func TestLLMEffortValuesDependOnTheProvider(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		effort   string
		want     string
		wantErr  string
	}{
		{name: "unset stays empty for anthropic", provider: LLMProviderAnthropic},
		{name: "unset stays empty for openai", provider: LLMProviderOpenAI},
		{name: "openai accepts none", provider: LLMProviderOpenAI, effort: "none", want: "none"},
		{name: "openai accepts minimal", provider: LLMProviderOpenAI, effort: "minimal", want: "minimal"},
		{name: "openai is case insensitive", provider: LLMProviderOpenAI, effort: "LOW", want: "low"},
		{name: "openai refuses max", provider: LLMProviderOpenAI, effort: "max", wantErr: envLLMEffort + ": for provider openai must be one of none, minimal, low, medium, high, xhigh"},
		{name: "anthropic accepts max", provider: LLMProviderAnthropic, effort: "max", want: "max"},
		{name: "anthropic refuses none", provider: LLMProviderAnthropic, effort: "none", wantErr: envLLMEffort + ": for provider anthropic must be one of low, medium, high, xhigh, max"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Load(envOf(llmEnv(map[string]string{
				envLLMProvider: tt.provider,
				envLLMModel:    "m",
				envLLMAPIKey:   "k",
				envLLMEffort:   tt.effort,
			})))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Load error = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.LLM.Effort != tt.want {
				t.Fatalf("Effort = %q, want %q", cfg.LLM.Effort, tt.want)
			}
		})
	}
}

func TestTimeoutSettings(t *testing.T) {
	tests := []struct {
		name    string
		env     string
		set     string
		llm     time.Duration
		run     time.Duration
		wantErr string
	}{
		{name: "request timeout default", env: envLLMRequestTimeout, llm: 10 * time.Minute, run: 0},
		{name: "run timeout default is no limit", env: envAgentRunTimeout, llm: 10 * time.Minute, run: 0},
		{name: "request timeout zero means none", env: envLLMRequestTimeout, set: "0", llm: 0, run: 0},
		{name: "request timeout zero with unit", env: envLLMRequestTimeout, set: "0s", llm: 0, run: 0},
		{name: "run timeout zero means none", env: envAgentRunTimeout, set: "0", llm: 10 * time.Minute, run: 0},
		{name: "request timeout at the minimum", env: envLLMRequestTimeout, set: "10s", llm: 10 * time.Second, run: 0},
		{name: "request timeout at the maximum", env: envLLMRequestTimeout, set: "60m", llm: time.Hour, run: 0},
		{name: "run timeout at the minimum", env: envAgentRunTimeout, set: "1m", llm: 10 * time.Minute, run: time.Minute},
		{name: "run timeout has no maximum", env: envAgentRunTimeout, set: "72h", llm: 10 * time.Minute, run: 72 * time.Hour},
		{name: "request timeout below the minimum", env: envLLMRequestTimeout, set: "9s", wantErr: envLLMRequestTimeout + ": must be 0 or between 10s and 1h0m0s"},
		{name: "request timeout above the maximum", env: envLLMRequestTimeout, set: "61m", wantErr: envLLMRequestTimeout + ": must be 0 or between 10s and 1h0m0s"},
		{name: "request timeout negative", env: envLLMRequestTimeout, set: "-1m", wantErr: envLLMRequestTimeout + ": must be 0 or between"},
		{name: "request timeout without a unit", env: envLLMRequestTimeout, set: "120", wantErr: envLLMRequestTimeout + ": "},
		{name: "run timeout below the minimum", env: envAgentRunTimeout, set: "59s", wantErr: envAgentRunTimeout + ": must be 0 or at least 1m0s"},
		{name: "run timeout negative", env: envAgentRunTimeout, set: "-5m", wantErr: envAgentRunTimeout + ": must be 0 or at least 1m0s"},
		{name: "run timeout not a duration", env: envAgentRunTimeout, set: "soon", wantErr: envAgentRunTimeout + ": "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extra := map[string]string{}
			if tt.set != "" {
				extra[tt.env] = tt.set
			}
			cfg, err := Load(envOf(llmEnv(extra)))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Load error = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.LLM.RequestTimeout != tt.llm || cfg.Agent.RunTimeout != tt.run {
				t.Fatalf("request timeout %s and run timeout %s, want %s and %s", cfg.LLM.RequestTimeout, cfg.Agent.RunTimeout, tt.llm, tt.run)
			}
		})
	}
}

func TestMetricsSetting(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
		fails bool
	}{
		{name: "unset is on", want: true},
		{name: "true", value: "true", want: true},
		{name: "false turns it off", value: "false"},
		{name: "false with spaces", value: " false "},
		{name: "off is refused", value: "off", fails: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			set := map[string]string{}
			if tt.value != "" {
				set[envMetrics] = tt.value
			}
			cfg, err := Load(envOf(llmEnv(set)))
			if tt.fails {
				if err == nil || !strings.Contains(err.Error(), envMetrics+": must be true or false") {
					t.Fatalf("expected a boolean error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.Metrics != tt.want {
				t.Fatalf("Metrics = %v, want %v", cfg.Metrics, tt.want)
			}
		})
	}
}

func TestProvisionDirSetting(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "unset is off", want: ""},
		{name: "a path", value: "/provision", want: "/provision"},
		{name: "spaces are trimmed", value: " /provision ", want: "/provision"},
		{name: "blank is off", value: "   ", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Load(envOf(llmEnv(map[string]string{envProvisionDir: tt.value})))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.ProvisionDir != tt.want {
				t.Fatalf("ProvisionDir = %q, want %q", cfg.ProvisionDir, tt.want)
			}
		})
	}
}
