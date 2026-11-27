package harness

import (
	"log/slog"
	"os"
	"strconv"
)

const (
	EnvServerURL = "HARNESS_SERVER_URL"
	EnvToken     = "HARNESS_TOKEN"
	EnvSlots     = "HARNESS_SLOTS"
	EnvLogLevel  = "HARNESS_LOG_LEVEL"

	TokenPrefix = "sob_"
)

var inheritedEnvNames = []string{"PATH", "HOME", "TZ"}

func childEnv(serverURL, token string, slots int, level slog.Level) []string {
	env := []string{
		EnvServerURL + "=" + serverURL,
		EnvToken + "=" + token,
		EnvSlots + "=" + strconv.Itoa(slots),
		EnvLogLevel + "=" + levelName(level),
	}
	for _, name := range inheritedEnvNames {
		if v, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+v)
		}
	}
	return env
}

func levelName(level slog.Level) string {
	switch {
	case level < slog.LevelInfo:
		return "debug"
	case level < slog.LevelWarn:
		return "info"
	case level < slog.LevelError:
		return "warn"
	default:
		return "error"
	}
}
