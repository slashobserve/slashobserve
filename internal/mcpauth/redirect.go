package mcpauth

import (
	"net/url"
	"slices"
	"strings"
)

var loopbackHosts = []string{"localhost", "127.0.0.1", "::1"}

func parseRedirect(raw string) (*url.URL, bool) {
	if raw == "" || len(raw) > maxRedirectURIBytes || strings.Contains(raw, "#") {
		return nil, false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Opaque != "" || u.User != nil || u.Hostname() == "" {
		return nil, false
	}
	switch u.Scheme {
	case "https":
		return u, true
	case "http":
		return u, slices.Contains(loopbackHosts, u.Hostname())
	}
	return nil, false
}

func validRedirect(raw string) bool {
	_, ok := parseRedirect(raw)
	return ok
}

func isLoopback(raw string) bool {
	u, ok := parseRedirect(raw)
	return ok && u.Scheme == "http"
}

func redirectHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Host
}

func matchRedirect(registered []string, presented string) (string, bool) {
	got, ok := parseRedirect(presented)
	if !ok {
		return "", false
	}
	for _, candidate := range registered {
		if candidate == presented {
			return presented, true
		}
		if got.Scheme != "http" {
			continue
		}
		want, ok := parseRedirect(candidate)
		if ok && want.Scheme == "http" && sameLoopbackTarget(want, got) {
			return presented, true
		}
	}
	return "", false
}

func sameLoopbackTarget(a, b *url.URL) bool {
	return a.Hostname() == b.Hostname() && loopbackPath(a) == loopbackPath(b) && a.RawQuery == b.RawQuery
}

func loopbackPath(u *url.URL) string {
	if p := u.EscapedPath(); p != "" {
		return p
	}
	return "/"
}

func appendQuery(redirectURI string, values url.Values) string {
	u, err := url.Parse(redirectURI)
	if err != nil {
		return redirectURI
	}
	q := u.Query()
	for key, vs := range values {
		for _, v := range vs {
			q.Add(key, v)
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func canonicalResource(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	return strings.TrimSuffix(u.String(), "/")
}
