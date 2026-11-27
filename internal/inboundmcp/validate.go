package inboundmcp

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"github.com/slashobserve/slashobserve/internal/config"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/egress/domains"
)

const (
	maxURLBytes    = 2048
	maxSecretBytes = 4096
)

var (
	uidPattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,39}$`)
	headerPattern = regexp.MustCompile("^[!#$%&'*+.^_`|~0-9A-Za-z-]{1,100}$")

	reservedHeaders = map[string]bool{
		"host": true, "content-length": true, "content-type": true, "accept": true, "user-agent": true,
		"connection": true, "transfer-encoding": true, "upgrade": true, "te": true, "trailer": true,
		"expect": true, "cookie": true, "mcp-session-id": true, "mcp-protocol-version": true,
	}
)

type InvalidError struct {
	Fields []config.FieldError
}

func (e *InvalidError) Error() string {
	parts := make([]string, len(e.Fields))
	for i, f := range e.Fields {
		parts[i] = f.Error()
	}
	return strings.Join(parts, "; ")
}

func invalid(field, message string) error {
	return &InvalidError{Fields: []config.FieldError{{Field: field, Message: message}}}
}

func checkUID(uid string) error {
	if !uidPattern.MatchString(uid) {
		return invalid("uid", "must start with a letter or digit and have at most 40 letters, digits, hyphens or underscores")
	}
	return nil
}

func checkName(name string) error {
	if !config.ValidMCPSourceName(name) {
		return invalid("name", "must be 1 to 100 characters without control or format characters")
	}
	return nil
}

func checkDescription(description string) error {
	if !config.ValidMCPSourceDescription(description) {
		return invalid("description", "must be at most 300 characters on one line, without control or format characters")
	}
	return nil
}

func checkAudience(audience datasource.Audience) error {
	if audience != datasource.AudienceEveryone && audience != datasource.AudienceAdmins {
		return invalid("audience", "must be everyone or admins")
	}
	return nil
}

func checkAddress(raw string, private bool) (string, error) {
	if raw == "" || len(raw) > maxURLBytes {
		return "", invalid("url", "must be an https URL of at most 2048 bytes")
	}
	u, err := url.Parse(raw)
	switch {
	case err != nil || u.Hostname() == "":
		return "", invalid("url", "must be an https URL such as https://mcp.example.com/mcp")
	case u.Scheme != "https" && (!private || u.Scheme != "http"):
		return "", invalid("url", "must start with https://")
	case u.User != nil:
		return "", invalid("url", "must not contain a login; send the secret in its own field")
	case u.RawQuery != "" || u.ForceQuery || u.Fragment != "":
		return "", invalid("url", "must not contain a query or a fragment; send the secret in its own field")
	}
	host, err := domains.NormalizeHost(u.Hostname())
	if err != nil {
		return "", invalid("url", "the host must be a domain name; an IP address or localhost cannot be reached")
	}
	if _, err := domains.ParsePattern(host); err != nil {
		return "", invalid("url", "the host must be a domain name; an IP address or localhost cannot be reached")
	}
	return host, nil
}

func checkHeader(name string) error {
	if !headerPattern.MatchString(name) || reservedHeaders[strings.ToLower(name)] {
		return invalid("secret_header", "must be a plain HTTP header name that the connection does not already use, such as Authorization or X-Api-Key")
	}
	return nil
}

func checkSecret(secret string) error {
	if secret == "" || len(secret) > maxSecretBytes || strings.ContainsFunc(secret, unicode.IsControl) {
		return invalid("secret", fmt.Sprintf("must be 1 to %d bytes without control characters", maxSecretBytes))
	}
	return nil
}
