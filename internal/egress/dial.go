package egress

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"net/netip"
	"syscall"
	"time"
)

type dialFunc func(ctx context.Context, network, address string) (net.Conn, error)

var blockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("255.255.255.255/32"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("fec0::/10"),
}

func blocked(addr netip.Addr) bool {
	addr = addr.Unmap()
	if addr.IsLoopback() || addr.IsPrivate() || addr.IsUnspecified() || addr.IsMulticast() ||
		addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsInterfaceLocalMulticast() {
		return true
	}
	for _, prefix := range blockedPrefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func refuseBlocked(network, address string, _ syscall.RawConn) error {
	ap, err := netip.ParseAddrPort(address)
	if err != nil || blocked(ap.Addr()) {
		return ErrBlockedAddress
	}
	return nil
}

func newTransport(dial dialFunc, roots *x509.CertPool) *http.Transport {
	if dial == nil {
		dial = (&net.Dialer{Timeout: 5 * time.Second, Control: refuseBlocked}).DialContext
	}
	return &http.Transport{
		Proxy:                  nil,
		DialContext:            dial,
		TLSHandshakeTimeout:    5 * time.Second,
		ResponseHeaderTimeout:  5 * time.Second,
		MaxResponseHeaderBytes: 64 << 10,
		TLSClientConfig:        &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots},
		ForceAttemptHTTP2:      true,
		IdleConnTimeout:        30 * time.Second,
	}
}
