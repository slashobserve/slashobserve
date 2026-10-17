package config

import (
	"path/filepath"
	"strings"
	"testing"
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
