package observeagent

import (
	"log/slog"
	"os"
	"strconv"
)

const (
	EnvServerURL = "OBSERVE_AGENT_SERVER_URL"
	EnvToken     = "OBSERVE_AGENT_TOKEN"
	EnvSlots     = "OBSERVE_AGENT_SLOTS"
	EnvLogLevel  = "OBSERVE_AGENT_LOG_LEVEL"

	TokenPrefix = "soa_"
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
