package datasource

import (
	"errors"
	"net"
	"net/http"
	"net/netip"
	"syscall"
	"time"
)

// net/http keeps 2 idle connections per host; every call beyond that closes its socket, and the closed sockets fill the ephemeral port range.
const idleConnsPerHost = 1024

var ErrLinkLocalAddress = errors.New("the address is link-local")

func refuseLinkLocal(_, address string, _ syscall.RawConn) error {
	ap, err := netip.ParseAddrPort(address)
	if err != nil {
		return ErrLinkLocalAddress
	}
	addr := ap.Addr().Unmap()
	if addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsUnspecified() {
		return ErrLinkLocalAddress
	}
	return nil
}

func NewHTTPTransport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxIdleConns = 0
	t.MaxIdleConnsPerHost = idleConnsPerHost
	t.DialContext = NewDialer().DialContext
	return t
}

func NewDialer() *net.Dialer {
	return &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second, Control: refuseLinkLocal}
}
