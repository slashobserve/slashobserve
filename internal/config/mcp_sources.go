package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/goccy/go-yaml"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

var (
	mcpToolNamePattern  = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)
	mcpSourceUIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,39}$`)
)

func ValidMCPToolName(name string) bool {
	return mcpToolNamePattern.MatchString(name) && strings.ContainsFunc(name, isASCIILetterOrDigit)
}

func ValidMCPSourceUID(uid string) bool {
	return mcpSourceUIDPattern.MatchString(uid)
}

func isASCIILetterOrDigit(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
}

type MCPSource struct {
	UID          string
	Name         string
	Description  string
	URL          string
	Audience     datasource.Audience
	SecretHeader string
	Secret       datasource.Secret
	AllowPrivate bool
	Approve      []string
}

type mcpSourcesFile struct {
	MCPSources *[]fileMCPSource `yaml:"mcpSources"`
}

type fileMCPSource struct {
	UID          string   `yaml:"uid"`
	Name         string   `yaml:"name"`
	Description  string   `yaml:"description"`
	URL          string   `yaml:"url"`
	Audience     string   `yaml:"audience"`
	SecretHeader string   `yaml:"secretHeader"`
	SecretEnv    string   `yaml:"secretEnv"`
	SecretFile   string   `yaml:"secretFile"`
	AllowPrivate bool     `yaml:"allowPrivate"`
	Approve      []string `yaml:"approve"`
}

func loadMCPSources(path string, lookup func(string) (string, bool)) ([]MCPSource, error) {
	content, err := os.ReadFile(path) //nolint:gosec // the path is operator configuration
	if err != nil {
		return nil, err
	}
	var doc mcpSourcesFile
	decoder := yaml.NewDecoder(bytes.NewReader(content), yaml.DisallowUnknownField())
	if err := decoder.Decode(&doc); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("invalid MCP sources file: %s", decodeMessage(err))
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("invalid MCP sources file: multiple YAML documents are not supported")
	}
	if doc.MCPSources == nil {
		return nil, errors.New("invalid MCP sources file: mcpSources: required")
	}
	var errs []error
	seen := map[string]int{}
	out := make([]MCPSource, 0, len(*doc.MCPSources))
	for i, raw := range *doc.MCPSources {
		entryPath := fmt.Sprintf("mcpSources[%d]", i)
		source, sourceErrs := convertMCPSource(entryPath, raw, lookup)
		errs = append(errs, sourceErrs...)
		if first, taken := seen[source.UID]; taken {
			errs = append(errs, fmt.Errorf("%s.uid: %q is already used by mcpSources[%d]", entryPath, source.UID, first))
		} else if source.UID != "" {
			seen[source.UID] = i
		}
		out = append(out, source)
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return out, nil
}

func loadMCPSourcesSetting(lookup func(string) (string, bool), cfg *Config) []error {
	path := get(lookup, envMCPSourcesFile)
	if path == "" {
		return nil
	}
	cfg.MCPSourcesFile = path
	sources, err := loadMCPSources(path, lookup)
	cfg.MCPSources = sources
	if err != nil {
		return []error{fmt.Errorf("%s: %w", envMCPSourcesFile, err)}
	}
	return nil
}

const (
	maxMCPSourceNameRunes        = 100
	maxMCPSourceDescriptionRunes = 300
)

func ValidMCPSourceName(name string) bool {
	return name != "" && plainText(name, maxMCPSourceNameRunes)
}

func ValidMCPSourceDescription(description string) bool {
	return plainText(description, maxMCPSourceDescriptionRunes)
}

func plainText(s string, maxRunes int) bool {
	return utf8.RuneCountInString(s) <= maxRunes && !strings.ContainsFunc(s, isControlOrFormat)
}

func convertMCPSource(path string, raw fileMCPSource, lookup func(string) (string, bool)) (MCPSource, []error) {
	var errs []error
	fail := func(field, format string, args ...any) {
		errs = append(errs, fmt.Errorf("%s.%s: %s", path, field, fmt.Sprintf(format, args...)))
	}
	source := MCPSource{UID: raw.UID, Name: strings.TrimSpace(raw.Name), URL: strings.TrimSpace(raw.URL), Description: strings.TrimSpace(raw.Description), SecretHeader: strings.TrimSpace(raw.SecretHeader), AllowPrivate: raw.AllowPrivate}
	if !ValidMCPSourceUID(raw.UID) {
		fail("uid", "must match %s", mcpSourceUIDPattern)
	}
	if source.Name == "" {
		source.Name = raw.UID
	}
	if !ValidMCPSourceName(source.Name) {
		fail("name", "must be 1 to 100 characters without control or format characters")
	}
	if !ValidMCPSourceDescription(source.Description) {
		fail("description", "must be at most 300 characters on one line, without control or format characters")
	}
	if source.URL == "" {
		fail("url", "required")
	}
	switch audience := datasource.Audience(raw.Audience); audience {
	case "", datasource.AudienceEveryone:
		source.Audience = datasource.AudienceEveryone
	case datasource.AudienceAdmins:
		source.Audience = audience
	default:
		fail("audience", "must be %s or %s, got %q", datasource.AudienceEveryone, datasource.AudienceAdmins, raw.Audience)
	}
	secret, secretErrs := resolveSecret(path, "secret", raw.SecretEnv, raw.SecretFile, false, lookup)
	errs = append(errs, secretErrs...)
	source.Secret = secret
	if !secret.IsZero() && source.SecretHeader == "" {
		fail("secretHeader", "required when a secret is set")
	}
	if secret.IsZero() && source.SecretHeader != "" && raw.SecretEnv == "" && raw.SecretFile == "" {
		fail("secretHeader", "needs secretEnv or secretFile")
	}
	approve, err := approvedToolNames(raw.Approve)
	if err != nil {
		fail("approve", "%v", err)
	}
	source.Approve = approve
	return source, errs
}

func approvedToolNames(raw []string) ([]string, error) {
	seen := make(map[string]bool, len(raw))
	names := make([]string, 0, len(raw))
	for _, entry := range raw {
		name := strings.TrimSpace(entry)
		switch {
		case !ValidMCPToolName(name):
			return nil, fmt.Errorf("%q is not a tool name: use 1 to 128 letters, digits, dots, dashes or underscores, with at least one letter or digit", name)
		case seen[name]:
			return nil, fmt.Errorf("%q is listed twice", name)
		}
		seen[name] = true
		names = append(names, name)
	}
	return names, nil
}
