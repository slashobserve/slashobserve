package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type LogFormat string

const (
	LogFormatJSON LogFormat = "json"
	LogFormatText LogFormat = "text"
)

const (
	defaultHTTPAddr   = ":8080"
	defaultSessionTTL = 168 * time.Hour
	minSessionTTL     = 5 * time.Minute
	maxSessionTTL     = 720 * time.Hour
	allowAnyDomain    = "*"
)

var emailDomainPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)

const (
	envHTTPAddr    = "SLASHOBSERVE_HTTP_ADDR"
	envDatabaseURL = "SLASHOBSERVE_DATABASE_URL"
	envLogLevel    = "SLASHOBSERVE_LOG_LEVEL"
	envLogFormat   = "SLASHOBSERVE_LOG_FORMAT"

	envPublicURL          = "SLASHOBSERVE_PUBLIC_URL"
	envOIDCIssuerURL      = "SLASHOBSERVE_OIDC_ISSUER_URL"
	envOIDCInternalURL    = "SLASHOBSERVE_OIDC_INTERNAL_URL"
	envOIDCClientID       = "SLASHOBSERVE_OIDC_CLIENT_ID"
	envOIDCClientSecret   = "SLASHOBSERVE_OIDC_CLIENT_SECRET"
	envSessionTTL         = "SLASHOBSERVE_SESSION_TTL"
	envAllowedEmailDomain = "SLASHOBSERVE_AUTH_ALLOWED_EMAIL_DOMAINS"

	envDatasourcesFile = "SLASHOBSERVE_DATASOURCES_FILE"
)

var serverEnvNames = []string{
	envHTTPAddr, envDatabaseURL, envLogLevel, envLogFormat,
	envPublicURL, envOIDCIssuerURL, envOIDCInternalURL, envOIDCClientID, envOIDCClientSecret,
	envSessionTTL, envAllowedEmailDomain, envDatasourcesFile,
}

type Config struct {
	HTTPAddr    string
	DatabaseURL string
	LogLevel    slog.Level
	LogFormat   LogFormat

	PublicURL           *url.URL
	OIDCIssuerURL       string
	OIDCInternalURL     *url.URL
	OIDCClientID        string
	OIDCClientSecret    string
	SessionTTL          time.Duration
	AllowedEmailDomains []string
	AllowAnyEmailDomain bool

	DatasourcesFile string
	Datasources     []Datasource
}

func Load(lookup func(string) (string, bool)) (Config, error) {
	cfg := Config{LogLevel: slog.LevelInfo, LogFormat: LogFormatJSON, SessionTTL: defaultSessionTTL}
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

	errs = append(errs, loadURLs(lookup, &cfg)...)
	errs = append(errs, loadClient(lookup, &cfg)...)
	errs = append(errs, loadSessionTTL(lookup, &cfg)...)
	errs = append(errs, loadEmailDomains(lookup, &cfg)...)

	if path := get(lookup, envDatasourcesFile); path != "" {
		cfg.DatasourcesFile = path
		datasources, err := loadDatasources(path, lookup)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", envDatasourcesFile, err))
		}
		cfg.Datasources = datasources
	}

	return cfg, errors.Join(errs...)
}

func loadURLs(lookup func(string) (string, bool), cfg *Config) []error {
	var errs []error

	if v := get(lookup, envPublicURL); v == "" {
		errs = append(errs, fmt.Errorf("%s is required", envPublicURL))
	} else if u, err := parseBaseURL(v); err != nil {
		errs = append(errs, fmt.Errorf("%s: %w, got %q", envPublicURL, err, v))
	} else {
		cfg.PublicURL = u
	}

	if v := get(lookup, envOIDCIssuerURL); v == "" {
		errs = append(errs, fmt.Errorf("%s is required", envOIDCIssuerURL))
	} else if u, err := url.Parse(v); err != nil || !isAbsoluteHTTP(u) {
		errs = append(errs, fmt.Errorf("%s: must be an absolute http or https URL, got %q", envOIDCIssuerURL, v))
	} else {
		cfg.OIDCIssuerURL = v
	}

	if v := get(lookup, envOIDCInternalURL); v != "" {
		if u, err := parseBaseURL(v); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w, got %q", envOIDCInternalURL, err, v))
		} else {
			cfg.OIDCInternalURL = u
		}
	}

	return errs
}

func loadClient(lookup func(string) (string, bool), cfg *Config) []error {
	var errs []error

	cfg.OIDCClientID = get(lookup, envOIDCClientID)
	if cfg.OIDCClientID == "" {
		errs = append(errs, fmt.Errorf("%s is required", envOIDCClientID))
	}

	cfg.OIDCClientSecret = get(lookup, envOIDCClientSecret)
	if cfg.OIDCClientSecret == "" {
		errs = append(errs, fmt.Errorf("%s is required", envOIDCClientSecret))
	}

	return errs
}

func loadSessionTTL(lookup func(string) (string, bool), cfg *Config) []error {
	var errs []error

	if v := get(lookup, envSessionTTL); v != "" {
		ttl, err := time.ParseDuration(v)
		switch {
		case err != nil:
			errs = append(errs, fmt.Errorf("%s: %w", envSessionTTL, err))
		case ttl < minSessionTTL || ttl > maxSessionTTL:
			errs = append(errs, fmt.Errorf("%s: must be between %s and %s, got %q", envSessionTTL, minSessionTTL, maxSessionTTL, v))
		default:
			cfg.SessionTTL = ttl
		}
	}

	return errs
}

func loadEmailDomains(lookup func(string) (string, bool), cfg *Config) []error {
	var errs []error

	if v := get(lookup, envAllowedEmailDomain); v == "" {
		errs = append(errs, fmt.Errorf("%s is required; list the allowed domains or set it to %s to allow every account", envAllowedEmailDomain, allowAnyDomain))
	} else if v == allowAnyDomain {
		cfg.AllowAnyEmailDomain = true
	} else if domains, err := parseEmailDomains(v); err != nil {
		errs = append(errs, err)
	} else {
		cfg.AllowedEmailDomains = domains
	}

	return errs
}

func parseBaseURL(v string) (*url.URL, error) {
	u, err := url.Parse(v)
	if err != nil || !isAbsoluteHTTP(u) {
		return nil, errors.New("must be an absolute http or https URL")
	}
	if u.Path != "" && u.Path != "/" {
		return nil, errors.New("must not have a path")
	}
	if u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.User != nil {
		return nil, errors.New("must not have a query, fragment or user info")
	}
	u.Path, u.RawPath = "", ""
	u.Host = strings.ToLower(u.Host)
	return u, nil
}

func isAbsoluteHTTP(u *url.URL) bool {
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.Hostname() != ""
}

func parseEmailDomains(v string) ([]string, error) {
	var domains []string
	seen := map[string]bool{}
	for item := range strings.SplitSeq(v, ",") {
		item = strings.ToLower(strings.TrimSpace(item))
		if item == "" {
			continue
		}
		if item == allowAnyDomain {
			return nil, fmt.Errorf("%s: %s cannot be combined with domains", envAllowedEmailDomain, allowAnyDomain)
		}
		if !emailDomainPattern.MatchString(item) {
			return nil, fmt.Errorf("%s: invalid domain %q", envAllowedEmailDomain, item)
		}
		if !seen[item] {
			seen[item] = true
			domains = append(domains, item)
		}
	}
	if len(domains) == 0 {
		return nil, fmt.Errorf("%s: at least one domain is required", envAllowedEmailDomain)
	}
	return domains, nil
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
