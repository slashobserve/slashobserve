package datasource

import "net/http"

// net/http keeps 2 idle connections per host; every call beyond that closes its socket, and the closed sockets fill the ephemeral port range.
const idleConnsPerHost = 1024

func NewHTTPTransport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxIdleConns = 0
	t.MaxIdleConnsPerHost = idleConnsPerHost
	return t
}
