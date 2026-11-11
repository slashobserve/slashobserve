package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/goccy/go-yaml"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const maxServiceAccountDescription = 256

var (
	serviceAccountNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
	serviceAccountKeyPattern  = regexp.MustCompile(`^sok_[A-Za-z0-9_-]{43}$`)
	serviceAccountRoles       = []string{"viewer", "editor", "admin"}
)

type ServiceAccount struct {
	Name        string
	Role        string
	Description string
	TakesRuns   bool
	Key         datasource.Secret
}

type serviceAccountsFile struct {
	ServiceAccounts *[]fileServiceAccount `yaml:"serviceAccounts"`
}

type fileServiceAccount struct {
	Name        string `yaml:"name"`
	Role        string `yaml:"role"`
	Description string `yaml:"description"`
	TakesRuns   bool   `yaml:"takesRuns"`
	KeyFile     string `yaml:"keyFile"`
}

func loadServiceAccounts(path string) ([]ServiceAccount, error) {
	content, err := os.ReadFile(path) //nolint:gosec // the path is operator configuration
	if err != nil {
		return nil, err
	}
	var doc serviceAccountsFile
	decoder := yaml.NewDecoder(bytes.NewReader(content), yaml.DisallowUnknownField())
	if err := decoder.Decode(&doc); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("invalid service accounts file: %s", decodeMessage(err))
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("invalid service accounts file: multiple YAML documents are not supported")
	}
	if doc.ServiceAccounts == nil {
		return nil, errors.New("invalid service accounts file: serviceAccounts: required")
	}

	var errs []error
	names := map[string]int{}
	keys := map[string]int{}
	out := make([]ServiceAccount, 0, len(*doc.ServiceAccounts))
	for i, raw := range *doc.ServiceAccounts {
		entryPath := fmt.Sprintf("serviceAccounts[%d]", i)
		account, accountErrs := convertServiceAccount(entryPath, raw)
		errs = append(errs, accountErrs...)
		if first, taken := names[account.Name]; taken {
			errs = append(errs, fmt.Errorf("%s.name: %q is already used by serviceAccounts[%d]", entryPath, account.Name, first))
		} else if account.Name != "" {
			names[account.Name] = i
		}
		if key := account.Key.Reveal(); key != "" {
			if first, taken := keys[key]; taken {
				errs = append(errs, fmt.Errorf("%s.keyFile: holds the same key as serviceAccounts[%d]; every account needs its own key", entryPath, first))
			} else {
				keys[key] = i
			}
		}
		out = append(out, account)
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return out, nil
}

func convertServiceAccount(path string, raw fileServiceAccount) (ServiceAccount, []error) {
	var errs []error
	fail := func(field, format string, args ...any) {
		errs = append(errs, fmt.Errorf("%s.%s: %s", path, field, fmt.Sprintf(format, args...)))
	}
	account := ServiceAccount{Name: raw.Name, Role: raw.Role, Description: raw.Description, TakesRuns: raw.TakesRuns}
	if !serviceAccountNamePattern.MatchString(raw.Name) {
		fail("name", "must match %s", serviceAccountNamePattern)
	}
	if !slices.Contains(serviceAccountRoles, raw.Role) {
		fail("role", "must be viewer, editor or admin, got %q", raw.Role)
	}
	if utf8.RuneCountInString(raw.Description) > maxServiceAccountDescription || strings.ContainsFunc(raw.Description, isControlOrFormat) {
		fail("description", "must be at most %d characters without control characters", maxServiceAccountDescription)
	}
	if raw.KeyFile == "" {
		fail("keyFile", "required")
		return account, errs
	}
	key, err := readServiceAccountKey(raw.KeyFile)
	if err != nil {
		errs = append(errs, fmt.Errorf("%s.keyFile: %w", path, err))
		return account, errs
	}
	account.Key = key
	return account, errs
}

func readServiceAccountKey(file string) (datasource.Secret, error) {
	b, err := os.ReadFile(file) //nolint:gosec // the path is operator configuration
	if err != nil {
		return datasource.Secret{}, err
	}
	key := strings.TrimSpace(string(b))
	if !serviceAccountKeyPattern.MatchString(key) {
		return datasource.Secret{}, fmt.Errorf("%s does not hold a key: it must be sok_ followed by 43 base64url characters", file)
	}
	return datasource.NewSecret(key), nil
}

func isControlOrFormat(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) }
