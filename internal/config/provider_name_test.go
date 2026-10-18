package config

import (
	"strings"
	"testing"
)

func TestAuthProviderNameIsTrimmedAndOptional(t *testing.T) {
	tests := []struct {
		name  string
		extra map[string]string
		want  string
	}{
		{"unset", nil, ""},
		{"blank", map[string]string{envAuthProviderName: "   "}, ""},
		{"trimmed", map[string]string{envAuthProviderName: "  Dex \t"}, "Dex"},
		{"inner spaces kept", map[string]string{envAuthProviderName: "Acme  SSO"}, "Acme  SSO"},
		{"exactly the limit", map[string]string{envAuthProviderName: strings.Repeat("a", maxProviderName)}, strings.Repeat("a", maxProviderName)},
		{"limit counts characters not bytes", map[string]string{envAuthProviderName: strings.Repeat("é", maxProviderName)}, strings.Repeat("é", maxProviderName)},
		{"spaces around the limit do not count", map[string]string{envAuthProviderName: " " + strings.Repeat("a", maxProviderName) + " "}, strings.Repeat("a", maxProviderName)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Load(envOf(llmEnv(tt.extra)))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.AuthProviderName != tt.want {
				t.Fatalf("AuthProviderName = %q, want %q", cfg.AuthProviderName, tt.want)
			}
		})
	}
}

func TestAuthProviderNameAboveTheLimitIsAnError(t *testing.T) {
	for _, name := range []string{strings.Repeat("a", maxProviderName+1), strings.Repeat("é", maxProviderName+1)} {
		_, err := Load(envOf(llmEnv(map[string]string{envAuthProviderName: name})))
		if err == nil || !strings.Contains(err.Error(), envAuthProviderName+": must be at most 64 characters") {
			t.Fatalf("Load with %d characters: err = %v, want the provider name limit error", len([]rune(name)), err)
		}
		if strings.Contains(err.Error(), name) {
			t.Fatal("the error must not echo the value")
		}
	}
}
