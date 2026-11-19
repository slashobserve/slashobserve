package config

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

func TestSecretKeySetting(t *testing.T) {
	thirtyTwo := bytes.Repeat([]byte{0xab}, 32)
	tests := []struct {
		name  string
		value string
		want  []byte
	}{
		{"unset", "", nil},
		{"blank", "   ", nil},
		{"hex", strings.Repeat("ab", 32), thirtyTwo},
		{"upper case hex", strings.Repeat("AB", 32), thirtyTwo},
		{"padded base64", "q6urq6urq6urq6urq6urq6urq6urq6urq6urq6urq6s=", thirtyTwo},
		{"unpadded base64", "q6urq6urq6urq6urq6urq6urq6urq6urq6urq6urq6s", thirtyTwo},
		{"longer than 32 bytes", strings.Repeat("ab", 48), bytes.Repeat([]byte{0xab}, 48)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Load(envOf(llmEnv(map[string]string{envSecretKey: tt.value})))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if !bytes.Equal([]byte(cfg.SecretKey.Reveal()), tt.want) {
				t.Fatalf("SecretKey = %x, want %x", cfg.SecretKey.Reveal(), tt.want)
			}
		})
	}
}

func TestSecretKeyRefusesShortOrUnreadableValuesWithoutEchoingThem(t *testing.T) {
	for _, value := range []string{
		strings.Repeat("ab", 31),
		"q6urq6urq6urq6urq6urq6urq6urq6urq6urq6ur",
		"not a key at all, just words!",
	} {
		t.Run(value, func(t *testing.T) {
			_, err := Load(envOf(llmEnv(map[string]string{envSecretKey: value})))
			if err == nil || !strings.Contains(err.Error(), envSecretKey+": must be at least 32 bytes") {
				t.Fatalf("Load error = %v", err)
			}
			if strings.Contains(err.Error(), value) {
				t.Fatalf("the error echoes the value: %v", err)
			}
		})
	}
}

func TestTheOIDCSecretModelKeyAndSealingKeyNeverPrint(t *testing.T) {
	const oidc, model, sealing = "oidc-client-secret-value", "sk-model-key-value", "abababababababababababababababababababababababababababababababab"
	cfg, err := Load(envOf(llmEnv(map[string]string{
		envOIDCClientSecret: oidc,
		envLLMProvider:      LLMProviderAnthropic,
		envLLMAPIKey:        model,
		envSecretKey:        sealing,
	})))
	if err != nil {
		t.Fatal(err)
	}
	var logged strings.Builder
	slog.New(slog.NewTextHandler(&logged, nil)).Info("config", slog.Any("config", cfg))
	for name, shown := range map[string]string{"%v": fmt.Sprintf("%v", cfg), "%+v": fmt.Sprintf("%+v", cfg), "%#v": fmt.Sprintf("%#v", cfg), "slog": logged.String()} {
		for _, secret := range []string{oidc, model, sealing, strings.Repeat("\xab", 32)} {
			if strings.Contains(shown, secret) {
				t.Errorf("%s of the config contains a secret value", name)
			}
		}
	}
}
