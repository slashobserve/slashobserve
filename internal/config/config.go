package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
)

type LogFormat string

const (
	LogFormatJSON LogFormat = "json"
	LogFormatText LogFormat = "text"
)

const (
	envHTTPAddr    = "SLASHOBSERVE_HTTP_ADDR"
	envDatabaseURL = "SLASHOBSERVE_DATABASE_URL"
	envLogLevel    = "SLASHOBSERVE_LOG_LEVEL"
	envLogFormat   = "SLASHOBSERVE_LOG_FORMAT"
)

type Config struct {
	HTTPAddr    string
	DatabaseURL string
	LogLevel    slog.Level
	LogFormat   LogFormat
}

func Load(lookup func(string) (string, bool)) (Config, error) {
	cfg := Config{HTTPAddr: ":8080", LogLevel: slog.LevelInfo, LogFormat: LogFormatJSON}
	var errs []error

	if v := get(lookup, envHTTPAddr); v != "" {
		cfg.HTTPAddr = v
	}
	if _, _, err := net.SplitHostPort(cfg.HTTPAddr); err != nil {
		errs = append(errs, fmt.Errorf("%s: %w", envHTTPAddr, err))
	}

	cfg.DatabaseURL = get(lookup, envDatabaseURL)
	if cfg.DatabaseURL == "" {
		errs = append(errs, fmt.Errorf("%s is required", envDatabaseURL))
	}

	if v := get(lookup, envLogLevel); v != "" {
		if err := cfg.LogLevel.UnmarshalText([]byte(v)); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", envLogLevel, err))
		}
	}

	if v := get(lookup, envLogFormat); v != "" {
		switch format := LogFormat(strings.ToLower(v)); format {
		case LogFormatJSON, LogFormatText:
			cfg.LogFormat = format
		default:
			errs = append(errs, fmt.Errorf("%s: must be json or text, got %q", envLogFormat, v))
		}
	}

	return cfg, errors.Join(errs...)
}

func get(lookup func(string) (string, bool), key string) string {
	v, _ := lookup(key)
	return strings.TrimSpace(v)
}
