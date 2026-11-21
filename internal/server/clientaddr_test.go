package server

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/audit"
)

func resolvedAddress(t *testing.T, trusted []string, peer string, forwarded ...string) string {
	t.Helper()
	header := http.Header{}
	for _, value := range forwarded {
		header.Add("X-Forwarded-For", value)
	}
	return resolvedAddressWith(t, trusted, peer, header)
}

func resolvedAddressWith(t *testing.T, trusted []string, peer string, header http.Header) string {
	t.Helper()
	proxies := make([]netip.Prefix, len(trusted))
	for i, entry := range trusted {
		proxies[i] = netip.MustParsePrefix(entry)
	}
	middleware, err := auditMiddleware(audit.New(audit.Config{}))
	if err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	if err := trustProxies(engine, proxies); err != nil {
		t.Fatal(err)
	}
	var got string
	engine.Group(apiPrefix, middleware...).GET("/dashboards", func(c *gin.Context) {
		scope, _ := audit.ScopeOf(c.Request.Context())
		got = scope.IP.String()
		c.Status(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboards", nil)
	req.RemoteAddr = peer
	req.Header = header
	engine.ServeHTTP(httptest.NewRecorder(), req)
	return got
}

func TestTheClientAddress(t *testing.T) {
	tests := []struct {
		name      string
		trusted   []string
		peer      string
		forwarded []string
		want      string
	}{
		{"unset uses the socket peer", nil, "192.0.2.7:4100", nil, "192.0.2.7"},
		{"unset ignores a forwarding header", nil, "192.0.2.7:4100", []string{"203.0.113.9"}, "192.0.2.7"},
		{"a trusted peer with one hop", []string{"10.0.0.5/32"}, "10.0.0.5:4100", []string{"203.0.113.9"}, "203.0.113.9"},
		{"a trusted peer with a chain takes the rightmost address that is not a proxy", []string{"10.0.0.0/8"}, "10.0.0.5:4100", []string{"203.0.113.9, 10.0.0.7"}, "203.0.113.9"},
		{"a chain across several header lines", []string{"10.0.0.0/8"}, "10.0.0.5:4100", []string{"203.0.113.9", "10.0.0.7"}, "203.0.113.9"},
		{"an untrusted peer sending a header is ignored", []string{"10.0.0.0/8"}, "192.0.2.7:4100", []string{"203.0.113.9"}, "192.0.2.7"},
		{"a spoofed leftmost entry is not taken", []string{"10.0.0.0/8"}, "10.0.0.5:4100", []string{"198.51.100.1, 203.0.113.9, 10.0.0.7"}, "203.0.113.9"},
		{"a trusted peer with no header uses itself", []string{"10.0.0.0/8"}, "10.0.0.5:4100", nil, "10.0.0.5"},
		{"a garbled header falls back to the peer", []string{"10.0.0.0/8"}, "10.0.0.5:4100", []string{"203.0.113.9, not-an-address"}, "10.0.0.5"},
		{"an IPv6 proxy range", []string{"2001:db8::/32"}, "[2001:db8::1]:4100", []string{"2001:db9::42, 2001:db8::2"}, "2001:db9::42"},
		{"an IPv6 client behind an IPv4 proxy", []string{"10.0.0.0/8"}, "10.0.0.5:4100", []string{"2001:db8::42"}, "2001:db8::42"},
		{"a mapped client is shown as IPv4", []string{"10.0.0.0/8"}, "10.0.0.5:4100", []string{"::ffff:203.0.113.9"}, "203.0.113.9"},
		{"a zoned link-local peer is recorded without its zone", nil, "[fe80::1%eth0]:4100", nil, "fe80::1"},
		{"a peer with no port is still recorded", nil, "192.0.2.8", nil, "192.0.2.8"},
		{"a mapped peer matches an IPv4 range", []string{"10.0.0.0/8"}, "[::ffff:10.0.0.5]:4100", []string{"203.0.113.9"}, "203.0.113.9"},
		{"a proxy only listed by address", []string{"10.0.0.5/32"}, "10.0.0.6:4100", []string{"203.0.113.9"}, "10.0.0.6"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolvedAddress(t, tt.trusted, tt.peer, tt.forwarded...); got != tt.want {
				t.Fatalf("client address = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestOnlyXForwardedForIsRead(t *testing.T) {
	for _, name := range []string{"X-Real-Ip", "Forwarded", "X-Client-Ip"} {
		header := http.Header{name: {"203.0.113.9"}}
		if got := resolvedAddressWith(t, []string{"10.0.0.0/8"}, "10.0.0.5:4100", header); got != "10.0.0.5" {
			t.Errorf("%s from a trusted peer gave %s, want the peer 10.0.0.5", name, got)
		}
	}
}
