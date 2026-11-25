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

var contextSourceUIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,39}$`)

type ContextSource struct {
	UID          string
	Name         string
	URL          string
	Audience     datasource.Audience
	SecretHeader string
	Secret       datasource.Secret
}

type contextSourcesFile struct {
	ContextSources *[]fileContextSource `yaml:"contextSources"`
}

type fileContextSource struct {
	UID          string `yaml:"uid"`
	Name         string `yaml:"name"`
	URL          string `yaml:"url"`
	Audience     string `yaml:"audience"`
	SecretHeader string `yaml:"secretHeader"`
	SecretEnv    string `yaml:"secretEnv"`
	SecretFile   string `yaml:"secretFile"`
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

func convertContextSource(path string, raw fileContextSource, lookup func(string) (string, bool)) (ContextSource, []error) {
	var errs []error
	fail := func(field, format string, args ...any) {
		errs = append(errs, fmt.Errorf("%s.%s: %s", path, field, fmt.Sprintf(format, args...)))
	}
	source := ContextSource{UID: raw.UID, Name: strings.TrimSpace(raw.Name), URL: strings.TrimSpace(raw.URL), SecretHeader: strings.TrimSpace(raw.SecretHeader)}
	if !contextSourceUIDPattern.MatchString(raw.UID) {
		fail("uid", "must match %s", contextSourceUIDPattern)
	}
	if source.Name == "" {
		source.Name = raw.UID
	}
	if utf8.RuneCountInString(source.Name) > 100 || strings.ContainsFunc(source.Name, isControlOrFormat) {
		fail("name", "must be at most 100 characters without control characters")
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
	return source, errs
}
