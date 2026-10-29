package domains

import (
	"errors"
	"net/netip"
	"strings"
	"unicode"

	"golang.org/x/net/idna"
)

var ErrInvalid = errors.New("invalid domain")

const (
	maxNameBytes  = 253
	maxLabelBytes = 63
	forbidden     = "/:@?#[]*"
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

func NormalizeHost(host string) (string, error) {
	name := strings.ToLower(strings.TrimSpace(host))
	name = strings.TrimSuffix(name, ".")
	if name == "" || strings.ContainsAny(name, forbidden) || strings.IndexFunc(name, unicode.IsSpace) >= 0 {
		return "", ErrInvalid
	}
	if _, err := netip.ParseAddr(name); err == nil {
		return "", ErrInvalid
	}
	ascii, err := idna.Lookup.ToASCII(name)
	if err != nil {
		return "", ErrInvalid
	}
	if len(ascii) > maxNameBytes || !validLabels(ascii) {
		return "", ErrInvalid
	}
	return ascii, nil
}

func validLabels(name string) bool {
	labels := strings.Split(name, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if !validLabel(label) {
			return false
		}
	}
	return true
}

func validLabel(label string) bool {
	if len(label) == 0 || len(label) > maxLabelBytes || label[0] == '-' || label[len(label)-1] == '-' {
		return false
	}
	for i := 0; i < len(label); i++ {
		c := label[i]
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
			return false
		}
	}
	return true
}
