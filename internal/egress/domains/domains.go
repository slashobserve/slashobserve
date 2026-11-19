package domains

import (
	"errors"
	"net/netip"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/idna"
)

var ErrInvalid = errors.New("invalid domain")

type InvalidError struct {
	Reason string
}

func (e *InvalidError) Error() string { return e.Reason }

func (e *InvalidError) Is(target error) bool { return target == ErrInvalid }

func invalid(reason string) error { return &InvalidError{Reason: reason} }

const (
	maxNameBytes  = 253
	maxLabelBytes = 63
)

type Pattern struct {
	host     string
	wildcard bool
}

type List []Pattern

func ParsePattern(entry string) (Pattern, error) {
	rest, wildcard := strings.CutPrefix(strings.TrimSpace(entry), "*.")
	host, err := NormalizeHost(rest)
	if err != nil {
		if wildcard && errors.Is(err, errSingleLabel) {
			return Pattern{}, invalid("a wildcard must sit before a full domain, like *.example.com, not before a top-level name like *.com")
		}
		return Pattern{}, err
	}
	return Pattern{host: host, wildcard: wildcard}, nil
}

func (p Pattern) String() string {
	if p.wildcard {
		return "*." + p.host
	}
	return p.host
}

func (p Pattern) Matches(host string) bool {
	if p.wildcard {
		return strings.HasSuffix(host, "."+p.host)
	}
	return host == p.host
}

func (l List) Allows(host string) bool {
	for _, p := range l {
		if p.Matches(host) {
			return true
		}
	}
	return false
}

var errSingleLabel = invalid("a domain needs at least two parts separated by a dot, like example.com; localhost and bare names are not allowed")

func NormalizeHost(host string) (string, error) {
	name := strings.ToLower(strings.TrimSpace(host))
	name = strings.TrimSuffix(name, ".")
	if err := screen(name); err != nil {
		return "", err
	}
	if isASCII(name) {
		if err := validLabels(name); err != nil {
			return "", err
		}
	}
	ascii, err := idna.Lookup.ToASCII(name)
	if err != nil {
		return "", invalid("the name is not a valid internationalized domain")
	}
	if len(ascii) > maxNameBytes {
		return "", invalid("a domain can be at most 253 characters")
	}
	if err := validLabels(ascii); err != nil {
		return "", err
	}
	return ascii, nil
}

func isASCII(name string) bool {
	for i := 0; i < len(name); i++ {
		if name[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

func screen(name string) error {
	switch {
	case name == "":
		return invalid("enter a domain, like example.com")
	case strings.IndexFunc(name, unicode.IsSpace) >= 0:
		return invalid("a domain cannot contain spaces")
	case strings.Contains(name, "*"):
		return invalid("* is allowed only as the first part of a pattern, like *.example.com")
	case strings.ContainsAny(name, "/?#"):
		return invalid("enter only the domain name, without a scheme, path, query or fragment")
	case strings.Contains(name, "@"):
		return invalid("enter only the domain name, without a user name")
	case strings.ContainsAny(name, "[]"):
		return invalid("an IP address is not allowed; use a domain name")
	}
	if _, err := netip.ParseAddr(name); err == nil {
		return invalid("an IP address is not allowed; use a domain name")
	}
	if strings.Contains(name, ":") {
		return invalid("enter the domain without a port; the allowlist matches the host on any port")
	}
	return nil
}

func validLabels(name string) error {
	labels := strings.Split(name, ".")
	if len(labels) < 2 {
		return errSingleLabel
	}
	for _, label := range labels {
		if err := validLabel(label); err != nil {
			return err
		}
	}
	return nil
}

func validLabel(label string) error {
	if len(label) == 0 {
		return invalid("a domain cannot have an empty part, like a..b")
	}
	if len(label) > maxLabelBytes {
		return invalid("each part between dots can be at most 63 characters")
	}
	if label[0] == '-' || label[len(label)-1] == '-' {
		return invalid("a part between dots cannot start or end with a hyphen")
	}
	for i := 0; i < len(label); i++ {
		c := label[i]
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
			return invalid("a domain can hold only letters, digits, hyphens and dots")
		}
	}
	return nil
}
