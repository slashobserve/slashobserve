package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/internal/egress/domains"
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
	maxProviderName   = 64
	allowAnyDomain    = "*"

	outboundDomainsNone = "none"
	MaxOutboundDomains  = 100

	maxAdminEmails    = 100
	maxGroupRoles     = 100
	maxGroupNameBytes = 256

	LLMProviderAnthropic = "anthropic"
	LLMProviderOpenAI    = "openai"
	LLMProviderReplay    = "replay"

	defaultLLMModel          = "claude-sonnet-5-5"
	defaultLLMEffort         = "medium"
	defaultLLMMaxTokens      = 16000
	minLLMMaxTokens          = 1024
	maxLLMMaxTokens          = 64000
	defaultLLMRequestTimeout = 120 * time.Second
	minLLMRequestTimeout     = 10 * time.Second
	maxLLMRequestTimeout     = 10 * time.Minute
)

var instanceRoles = []string{"admin", "editor", "commenter", "viewer"}

var llmEfforts = []string{"low", "medium", "high", "xhigh", "max"}

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
	envAuthProviderName   = "SLASHOBSERVE_AUTH_PROVIDER_NAME"

	envDatasourcesFile = "SLASHOBSERVE_DATASOURCES_FILE"

	envAPIContractCheck = "SLASHOBSERVE_API_CONTRACT_CHECK"

	envOutboundDomains = "SLASHOBSERVE_OUTBOUND_DOMAINS"

	envAdminEmails    = "SLASHOBSERVE_ADMIN_EMAILS"
	envOIDCGroupRoles = "SLASHOBSERVE_OIDC_GROUP_ROLES"

	envLLMProvider       = "SLASHOBSERVE_LLM_PROVIDER"
	envLLMModel          = "SLASHOBSERVE_LLM_MODEL"
	envLLMAPIKey         = "SLASHOBSERVE_LLM_API_KEY"      //nolint:gosec // an environment variable name, not a credential
	envLLMAPIKeyFile     = "SLASHOBSERVE_LLM_API_KEY_FILE" //nolint:gosec // an environment variable name, not a credential
	envLLMBaseURL        = "SLASHOBSERVE_LLM_BASE_URL"
	envLLMEffort         = "SLASHOBSERVE_LLM_EFFORT"
	envLLMMaxTokens      = "SLASHOBSERVE_LLM_MAX_TOKENS" //nolint:gosec // an environment variable name, not a credential
	envLLMRequestTimeout = "SLASHOBSERVE_LLM_REQUEST_TIMEOUT"
	envLLMReplayDir      = "SLASHOBSERVE_LLM_REPLAY_DIR"
)

var serverEnvNames = []string{
	envHTTPAddr, envDatabaseURL, envLogLevel, envLogFormat,
	envPublicURL, envOIDCIssuerURL, envOIDCInternalURL, envOIDCClientID, envOIDCClientSecret,
	envSessionTTL, envAllowedEmailDomain, envAuthProviderName, envDatasourcesFile, envAPIContractCheck, envOutboundDomains,
	envAdminEmails, envOIDCGroupRoles,
	envLLMProvider, envLLMModel, envLLMAPIKey, envLLMAPIKeyFile, envLLMBaseURL,
	envLLMEffort, envLLMMaxTokens, envLLMRequestTimeout, envLLMReplayDir,
}

type GroupRoleSetting struct {
	Group string
	Role  string
}

type LLM struct {
	Provider       string
	Model          string
	APIKey         string
	BaseURL        *url.URL
	Effort         string
	MaxTokens      int
	RequestTimeout time.Duration
	ReplayDir      string
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
	AuthProviderName    string
	AdminEmails         []string
	OIDCGroupRoles      []GroupRoleSetting

	DatasourcesFile string
	Datasources     []Datasource

	APIContractCheck bool
	LLM              LLM

	OutboundDomainsLocked bool
	OutboundDomains       domains.List
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
	errs = append(errs, loadProviderName(lookup, &cfg)...)
	errs = append(errs, loadAdminEmails(lookup, &cfg)...)
	errs = append(errs, loadGroupRoles(lookup, &cfg)...)

	if path := get(lookup, envDatasourcesFile); path != "" {
		cfg.DatasourcesFile = path
		datasources, err := loadDatasources(path, lookup)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", envDatasourcesFile, err))
		}
		cfg.Datasources = datasources
	}

	if v := get(lookup, envAPIContractCheck); v != "" {
		switch v {
		case "true":
			cfg.APIContractCheck = true
		case "false":
		default:
			errs = append(errs, fmt.Errorf("%s: must be true or false, got %q", envAPIContractCheck, v))
		}
	}
	errs = append(errs, loadLLM(lookup, &cfg)...)
	errs = append(errs, loadOutboundDomains(lookup, &cfg)...)

	return cfg, errors.Join(errs...)
}

func loadURLs(lookup func(string) (string, bool), cfg *Config) []error {
	var errs []error

	if v := get(lookup, envPublicURL); v == "" {
		errs = append(errs, fmt.Errorf("%s is required", envPublicURL))
	} else if u, err := parseBaseURL(v); err != nil {
		errs = append(errs, fmt.Errorf("%s: %w", envPublicURL, err))
	} else {
		cfg.PublicURL = u
	}

	if v := get(lookup, envOIDCIssuerURL); v == "" {
		errs = append(errs, fmt.Errorf("%s is required", envOIDCIssuerURL))
	} else if u, err := url.Parse(v); err != nil || !isAbsoluteHTTP(u) {
		errs = append(errs, fmt.Errorf("%s: must be an absolute http or https URL", envOIDCIssuerURL))
	} else {
		cfg.OIDCIssuerURL = v
	}

	if v := get(lookup, envOIDCInternalURL); v != "" {
		if u, err := parseBaseURL(v); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", envOIDCInternalURL, err))
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

func loadOutboundDomains(lookup func(string) (string, bool), cfg *Config) []error {
	v := get(lookup, envOutboundDomains)
	if v == "" {
		return nil
	}
	cfg.OutboundDomainsLocked = true
	if strings.EqualFold(v, outboundDomainsNone) {
		return nil
	}
	var errs []error
	seen := map[string]bool{}
	for _, item := range strings.Split(v, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		pattern, err := domains.ParsePattern(item)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %q: %w", envOutboundDomains, item, err))
			continue
		}
		if !seen[pattern.String()] {
			seen[pattern.String()] = true
			cfg.OutboundDomains = append(cfg.OutboundDomains, pattern)
		}
	}
	if len(cfg.OutboundDomains) > MaxOutboundDomains {
		errs = append(errs, fmt.Errorf("%s: at most %d entries, got %d", envOutboundDomains, MaxOutboundDomains, len(cfg.OutboundDomains)))
	}
	return errs
}

func loadProviderName(lookup func(string) (string, bool), cfg *Config) []error {
	name := get(lookup, envAuthProviderName)
	if utf8.RuneCountInString(name) > maxProviderName {
		return []error{fmt.Errorf("%s: must be at most %d characters", envAuthProviderName, maxProviderName)}
	}
	cfg.AuthProviderName = name

	return nil
}

func loadAdminEmails(lookup func(string) (string, bool), cfg *Config) []error {
	var errs []error
	for i, item := range strings.Split(get(lookup, envAdminEmails), ",") {
		email := strings.ToLower(strings.TrimSpace(item))
		if email == "" {
			continue
		}
		local, domain, found := strings.Cut(email, "@")
		if !found || local == "" || domain == "" || strings.Contains(domain, "@") {
			errs = append(errs, fmt.Errorf("%s: entry %d is not an email address", envAdminEmails, i+1))
			continue
		}
		if !slices.Contains(cfg.AdminEmails, email) {
			cfg.AdminEmails = append(cfg.AdminEmails, email)
		}
	}
	if len(cfg.AdminEmails) > maxAdminEmails {
		errs = append(errs, fmt.Errorf("%s: at most %d entries, got %d", envAdminEmails, maxAdminEmails, len(cfg.AdminEmails)))
	}
	return errs
}

func loadGroupRoles(lookup func(string) (string, bool), cfg *Config) []error {
	var errs []error
	entries := 0
	for i, item := range strings.Split(get(lookup, envOIDCGroupRoles), ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		entries++
		cut := strings.LastIndex(item, "=")
		if cut < 0 {
			errs = append(errs, fmt.Errorf("%s: entry %d must be group=role", envOIDCGroupRoles, i+1))
			continue
		}
		group, role := strings.TrimSpace(item[:cut]), strings.ToLower(strings.TrimSpace(item[cut+1:]))
		if len(group) < 1 || len(group) > maxGroupNameBytes || strings.ContainsFunc(group, unicode.IsControl) {
			errs = append(errs, fmt.Errorf("%s: entry %d needs a group name of 1 to %d bytes without control characters", envOIDCGroupRoles, i+1, maxGroupNameBytes))
			continue
		}
		if !slices.Contains(instanceRoles, role) {
			errs = append(errs, fmt.Errorf("%s: entry %d has role %q, want one of %s", envOIDCGroupRoles, i+1, role, strings.Join(instanceRoles, ", ")))
			continue
		}
		cfg.OIDCGroupRoles = append(cfg.OIDCGroupRoles, GroupRoleSetting{Group: group, Role: role})
	}
	if entries > maxGroupRoles {
		errs = append(errs, fmt.Errorf("%s: at most %d entries, got %d", envOIDCGroupRoles, maxGroupRoles, entries))
	}
	return errs
}

func loadLLM(lookup func(string) (string, bool), cfg *Config) []error {
	llm := LLM{
		Effort:         defaultLLMEffort,
		MaxTokens:      defaultLLMMaxTokens,
		RequestTimeout: defaultLLMRequestTimeout,
		Model:          get(lookup, envLLMModel),
		ReplayDir:      get(lookup, envLLMReplayDir),
	}
	var errs []error

	switch provider := strings.ToLower(get(lookup, envLLMProvider)); provider {
	case "", LLMProviderAnthropic, LLMProviderOpenAI, LLMProviderReplay:
		llm.Provider = provider
	default:
		errs = append(errs, fmt.Errorf("%s: must be %s, %s or %s, got %q", envLLMProvider, LLMProviderAnthropic, LLMProviderOpenAI, LLMProviderReplay, provider))
	}

	key, err := loadLLMAPIKey(lookup, llm.Provider)
	if err != nil {
		errs = append(errs, err)
	}
	llm.APIKey = key

	if v := get(lookup, envLLMBaseURL); v != "" {
		if u, err := parseLLMBaseURL(v); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", envLLMBaseURL, err))
		} else {
			llm.BaseURL = u
		}
	}

	if v := strings.ToLower(get(lookup, envLLMEffort)); v != "" {
		if !slices.Contains(llmEfforts, v) {
			errs = append(errs, fmt.Errorf("%s: must be one of %s, got %q", envLLMEffort, strings.Join(llmEfforts, ", "), v))
		} else {
			llm.Effort = v
		}
	}

	if v := get(lookup, envLLMMaxTokens); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < minLLMMaxTokens || n > maxLLMMaxTokens {
			errs = append(errs, fmt.Errorf("%s: must be a number between %d and %d, got %q", envLLMMaxTokens, minLLMMaxTokens, maxLLMMaxTokens, v))
		} else {
			llm.MaxTokens = n
		}
	}

	if v := get(lookup, envLLMRequestTimeout); v != "" {
		d, err := time.ParseDuration(v)
		switch {
		case err != nil:
			errs = append(errs, fmt.Errorf("%s: %w", envLLMRequestTimeout, err))
		case d < minLLMRequestTimeout || d > maxLLMRequestTimeout:
			errs = append(errs, fmt.Errorf("%s: must be between %s and %s, got %q", envLLMRequestTimeout, minLLMRequestTimeout, maxLLMRequestTimeout, v))
		default:
			llm.RequestTimeout = d
		}
	}

	errs = append(errs, requireLLMSettings(&llm)...)
	cfg.LLM = llm
	return errs
}

func requireLLMSettings(llm *LLM) []error {
	var errs []error
	switch llm.Provider {
	case LLMProviderAnthropic:
		if llm.Model == "" {
			llm.Model = defaultLLMModel
		}
		if llm.APIKey == "" {
			errs = append(errs, fmt.Errorf("%s or %s is required when %s is %s", envLLMAPIKey, envLLMAPIKeyFile, envLLMProvider, llm.Provider))
		}
	case LLMProviderOpenAI:
		if llm.Model == "" {
			errs = append(errs, fmt.Errorf("%s is required when %s is %s", envLLMModel, envLLMProvider, llm.Provider))
		}
	case LLMProviderReplay:
		if llm.ReplayDir == "" {
			errs = append(errs, fmt.Errorf("%s is required when %s is %s", envLLMReplayDir, envLLMProvider, llm.Provider))
		}
	}
	return errs
}

func loadLLMAPIKey(lookup func(string) (string, bool), provider string) (string, error) {
	key, file := get(lookup, envLLMAPIKey), get(lookup, envLLMAPIKeyFile)
	switch {
	case key != "" && file != "":
		return "", fmt.Errorf("%s and %s are mutually exclusive", envLLMAPIKey, envLLMAPIKeyFile)
	case file == "" || (provider != LLMProviderAnthropic && provider != LLMProviderOpenAI):
		return key, nil
	}
	b, err := os.ReadFile(file) //nolint:gosec // the path is operator configuration
	if err != nil {
		return "", fmt.Errorf("%s: cannot read the file: %w", envLLMAPIKeyFile, err)
	}
	key = strings.TrimSpace(string(b))
	if key == "" {
		return "", fmt.Errorf("%s: the file is empty", envLLMAPIKeyFile)
	}
	return key, nil
}

func parseLLMBaseURL(v string) (*url.URL, error) {
	u, err := url.Parse(v)
	if err != nil || !isAbsoluteHTTP(u) {
		return nil, errors.New("must be an absolute http or https URL")
	}
	if u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.User != nil {
		return nil, errors.New("must not have a query, fragment or user info")
	}
	u.Host = strings.ToLower(u.Host)
	return u, nil
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
