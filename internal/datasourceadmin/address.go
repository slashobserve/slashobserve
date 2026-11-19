package datasourceadmin

import (
	"net"
	"net/netip"
	"net/url"
	"strings"

	"github.com/slashobserve/slashobserve/internal/config"
	"github.com/slashobserve/slashobserve/internal/egress/domains"
)

type endpoint struct {
	host string
}

func (e endpoint) domain() (domains.Pattern, bool) {
	pattern, err := domains.ParsePattern(e.host)
	return pattern, err == nil
}

func endpointOf(def Definition) (endpoint, *config.FieldError) {
	if def.Type == TypeClickHouse {
		host, _, err := net.SplitHostPort(def.Address)
		if err != nil || host == "" {
			return endpoint{}, &config.FieldError{Field: "address", Message: "must be host:port"}
		}
		return checkHost(host)
	}
	u, err := url.Parse(def.Address)
	switch {
	case err != nil || u.Host == "" || u.Hostname() == "":
		return endpoint{}, &config.FieldError{Field: "address", Message: "must be an http or https URL such as http://prometheus:9090"}
	case u.Scheme != "http" && u.Scheme != "https":
		return endpoint{}, &config.FieldError{Field: "address", Message: "must start with http:// or https://"}
	case u.User != nil:
		return endpoint{}, &config.FieldError{Field: "address", Message: "must not contain a login; use the username and credential fields"}
	}
	return checkHost(u.Hostname())
}

func checkHost(host string) (endpoint, *config.FieldError) {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if addr, err := netip.ParseAddr(strings.SplitN(host, "%", 2)[0]); err == nil {
		if addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsUnspecified() || addr.IsMulticast() {
			return endpoint{}, &config.FieldError{Field: "address", Message: "must not be a link-local, unspecified or multicast address"}
		}
	}
	return endpoint{host: host}, nil
}
