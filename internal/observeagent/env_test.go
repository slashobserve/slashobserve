package observeagent

import (
	"log/slog"
	"slices"
	"strings"
	"testing"
)

func TestChildEnv(t *testing.T) {
	t.Setenv("SLASHOBSERVE_DATABASE_URL", "postgres://user:hunter2@db/app")
	t.Setenv("SLASHOBSERVE_LLM_API_KEY", "sk-provider-key")
	t.Setenv("SLASHOBSERVE_SECRET_KEY", "secret-key-material")
	t.Setenv("PATH", "/usr/local/bin:/usr/bin")
	t.Setenv("HOME", "/home/slashobserve")
	t.Setenv("TZ", "UTC")

	env := childEnv("http://127.0.0.1:8080", "soa_token", 4, slog.LevelWarn)

	want := []string{
		"OBSERVE_AGENT_SERVER_URL=http://127.0.0.1:8080",
		"OBSERVE_AGENT_TOKEN=soa_token",
		"OBSERVE_AGENT_SLOTS=4",
		"OBSERVE_AGENT_LOG_LEVEL=warn",
		"PATH=/usr/local/bin:/usr/bin",
		"HOME=/home/slashobserve",
		"TZ=UTC",
	}
	if !slices.Equal(env, want) {
		t.Fatalf("childEnv = %q, want %q", env, want)
	}
	for _, entry := range env {
		for _, leaked := range []string{"hunter2", "sk-provider-key", "secret-key-material", "SLASHOBSERVE_"} {
			if strings.Contains(entry, leaked) {
				t.Fatalf("childEnv leaks %q in %q", leaked, entry)
			}
		}
	}
}

func TestChildEnvOmitsUnsetInheritedVariables(t *testing.T) {
	t.Setenv("PATH", "/bin")
	unsetForTest(t, "HOME")
	unsetForTest(t, "TZ")

	env := childEnv("http://127.0.0.1:1", "soa_t", 1, slog.LevelInfo)

	if len(env) != 5 || env[4] != "PATH=/bin" {
		t.Fatalf("childEnv = %q, want the four variables and PATH only", env)
	}
}

func TestChildEnvLogLevel(t *testing.T) {
	tests := []struct {
		level slog.Level
		want  string
	}{
		{slog.LevelDebug, "debug"},
		{slog.LevelDebug - 4, "debug"},
		{slog.LevelInfo, "info"},
		{slog.LevelWarn, "warn"},
		{slog.LevelError, "error"},
		{slog.LevelError + 4, "error"},
	}
	for _, tt := range tests {
		env := childEnv("http://127.0.0.1:1", "soa_t", 1, tt.level)
		if want := "OBSERVE_AGENT_LOG_LEVEL=" + tt.want; env[3] != want {
			t.Errorf("level %v: got %q, want %q", tt.level, env[3], want)
		}
	}
}
