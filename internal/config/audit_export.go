package config

import (
	"bufio"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

type AuditExport struct {
	Endpoint *url.URL
	Headers  map[string]datasource.Secret
}

var headerName = regexp.MustCompile("^[!#$%&'*+.^_`|~0-9A-Za-z-]+$")

func loadAuditExport(lookup func(string) (string, bool), cfg *Config) []error {
	endpoint, file := get(lookup, envAuditOTLPEndpoint), get(lookup, envAuditOTLPHeaders)
	if endpoint == "" {
		if file != "" {
			return []error{fmt.Errorf("%s requires %s", envAuditOTLPHeaders, envAuditOTLPEndpoint)}
		}
		return nil
	}
	u, err := parseURLWithPath(endpoint)
	if err != nil {
		return []error{fmt.Errorf("%s: %w", envAuditOTLPEndpoint, err)}
	}
	cfg.AuditExport.Endpoint = u
	if file == "" {
		return nil
	}
	headers, err := loadHeadersFile(file)
	if err != nil {
		return []error{fmt.Errorf("%s: %w", envAuditOTLPHeaders, err)}
	}
	cfg.AuditExport.Headers = headers
	return nil
}

func loadHeadersFile(path string) (map[string]datasource.Secret, error) {
	f, err := os.Open(path) //nolint:gosec // the path is operator configuration
	if err != nil {
		return nil, fmt.Errorf("cannot read the file: %w", err)
	}
	defer func() { _ = f.Close() }()
	headers := map[string]datasource.Secret{}
	seen := map[string]int{}
	scanner := bufio.NewScanner(f)
	for n := 1; scanner.Scan(); n++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		name, value, ok := strings.Cut(line, ":")
		name, value = strings.TrimSpace(name), strings.TrimSpace(value)
		switch {
		case !ok || !headerName.MatchString(name):
			return nil, fmt.Errorf("line %d: must be a header name, a colon and a value", n)
		case value == "" || strings.ContainsFunc(value, isControl):
			return nil, fmt.Errorf("line %d: the value of header %q is empty or holds a control character", n, name)
		}
		canonical := http.CanonicalHeaderKey(name)
		if first, dup := seen[canonical]; dup {
			return nil, fmt.Errorf("line %d: header %q is already set on line %d", n, canonical, first)
		}
		seen[canonical] = n
		headers[canonical] = datasource.NewSecret(value)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the file: %w", err)
	}
	if len(headers) == 0 {
		return nil, errors.New("the file has no header")
	}
	return headers, nil
}

func isControl(r rune) bool {
	return (r < 0x20 && r != '\t') || r == 0x7f
}
