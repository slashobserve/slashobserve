package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
)

type LogFormat string

const (
	LogFormatJSON LogFormat = "json"
	LogFormatText LogFormat = "text"
)

const defaultHTTPAddr = ":8080"

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
	cfg := Config{LogLevel: slog.LevelInfo, LogFormat: LogFormatJSON}
	var errs []error

	httpAddr, err := HTTPAddr(lookup)
	if err != nil {
		errs = append(errs, err)
	}
	cfg.HTTPAddr = httpAddr

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

func HTTPAddr(lookup func(string) (string, bool)) (string, error) {
	addr := defaultHTTPAddr
	if v := get(lookup, envHTTPAddr); v != "" {
		addr = v
	}
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr, fmt.Errorf("%s: %w", envHTTPAddr, err)
	}
	if n, err := strconv.ParseUint(port, 10, 16); err != nil || n == 0 {
		return addr, fmt.Errorf("%s: port must be a number between 1 and 65535, got %q", envHTTPAddr, port)
	}
	return addr, nil
}

func get(lookup func(string) (string, bool), key string) string {
	v, _ := lookup(key)
	return strings.TrimSpace(v)
}
