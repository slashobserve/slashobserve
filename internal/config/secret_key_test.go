package config

import (
	"bytes"
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
			if !bytes.Equal(cfg.SecretKey, tt.want) {
				t.Fatalf("SecretKey = %x, want %x", cfg.SecretKey, tt.want)
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
