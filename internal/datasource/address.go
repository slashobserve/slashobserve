package datasource

import (
	"net/netip"
	"regexp"
	"strings"
)

var (
	addressToken = regexp.MustCompile(`[0-9A-Za-z:.\[\]%_-]+`)
	hostPort     = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*(\.[A-Za-z0-9_-]+)*:\d{2,5}$`)
)

func NamesAddress(message string) bool {
	for _, token := range addressToken.FindAllString(message, -1) {
		token = strings.Trim(token, ".:,")
		if token == "" {
			continue
		}
		if _, err := netip.ParseAddr(token); err == nil {
			return true
		}
		if _, err := netip.ParseAddrPort(token); err == nil || hostPort.MatchString(token) {
			return true
		}
	}
	return false
}
