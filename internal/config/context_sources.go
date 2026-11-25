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

var toolNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)

var contextSourceUIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,39}$`)

type ContextSource struct {
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

type contextSourcesFile struct {
	ContextSources *[]fileContextSource `yaml:"contextSources"`
}

type fileContextSource struct {
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

func loadContextSources(path string, lookup func(string) (string, bool)) ([]ContextSource, error) {
	content, err := os.ReadFile(path) //nolint:gosec // the path is operator configuration
	if err != nil {
		return nil, err
	}
	var doc contextSourcesFile
	decoder := yaml.NewDecoder(bytes.NewReader(content), yaml.DisallowUnknownField())
	if err := decoder.Decode(&doc); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("invalid context sources file: %s", decodeMessage(err))
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("invalid context sources file: multiple YAML documents are not supported")
	}
	if doc.ContextSources == nil {
		return nil, errors.New("invalid context sources file: contextSources: required")
	}
	var errs []error
	seen := map[string]int{}
	out := make([]ContextSource, 0, len(*doc.ContextSources))
	for i, raw := range *doc.ContextSources {
		entryPath := fmt.Sprintf("contextSources[%d]", i)
		source, sourceErrs := convertContextSource(entryPath, raw, lookup)
		errs = append(errs, sourceErrs...)
		if first, taken := seen[source.UID]; taken {
			errs = append(errs, fmt.Errorf("%s.uid: %q is already used by contextSources[%d]", entryPath, source.UID, first))
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

const (
	maxContextSourceNameRunes        = 100
	maxContextSourceDescriptionRunes = 300
)

func ValidContextSourceName(name string) bool {
	return name != "" && plainText(name, maxContextSourceNameRunes)
}

func ValidContextSourceDescription(description string) bool {
	return plainText(description, maxContextSourceDescriptionRunes)
}

func plainText(s string, maxRunes int) bool {
	return utf8.RuneCountInString(s) <= maxRunes && !strings.ContainsFunc(s, isControlOrFormat)
}

func convertContextSource(path string, raw fileContextSource, lookup func(string) (string, bool)) (ContextSource, []error) {
	var errs []error
	fail := func(field, format string, args ...any) {
		errs = append(errs, fmt.Errorf("%s.%s: %s", path, field, fmt.Sprintf(format, args...)))
	}
	source := ContextSource{UID: raw.UID, Name: strings.TrimSpace(raw.Name), URL: strings.TrimSpace(raw.URL), Description: strings.TrimSpace(raw.Description), SecretHeader: strings.TrimSpace(raw.SecretHeader), AllowPrivate: raw.AllowPrivate}
	if !contextSourceUIDPattern.MatchString(raw.UID) {
		fail("uid", "must match %s", contextSourceUIDPattern)
	}
	if source.Name == "" {
		source.Name = raw.UID
	}
	if !ValidContextSourceName(source.Name) {
		fail("name", "must be 1 to 100 characters without control or format characters")
	}
	if !ValidContextSourceDescription(source.Description) {
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
		case !toolNamePattern.MatchString(name):
			return nil, fmt.Errorf("%q is not a tool name: use 1 to 128 letters, digits, dots, dashes or underscores", name)
		case seen[name]:
			return nil, fmt.Errorf("%q is listed twice", name)
		}
		seen[name] = true
		names = append(names, name)
	}
	return names, nil
}
