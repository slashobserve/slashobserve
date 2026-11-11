package mcpauth

import (
	"net/url"
	"strings"
	"testing"
)

func TestValidRedirect(t *testing.T) {
	cases := []struct {
		uri  string
		want bool
	}{
		{"https://claude.ai/api/mcp/auth_callback", true},
		{"https://vscode.dev/redirect", true},
		{"http://localhost:3118/callback", true},
		{"http://127.0.0.1:33418", true},
		{"http://[::1]/cb", true},
		{"http://example.com/cb", false},
		{"http://localhost.evil.test/cb", false},
		{"cursor://x/cb", false},
		{"https://h/cb#frag", false},
		{"https://h/cb#", false},
		{"https://user@h/cb", false},
		{"https:///cb", false},
		{"https://h/" + strings.Repeat("a", 2049), false},
		{"javascript:alert(1)", false},
		{"/relative", false},
		{"", false},
	}
	for _, c := range cases {
		if got := validRedirect(c.uri); got != c.want {
			t.Errorf("validRedirect(%q) = %v, want %v", c.uri, got, c.want)
		}
	}
}

func TestMatchRedirect(t *testing.T) {
	cases := []struct {
		name       string
		registered []string
		presented  string
		want       bool
	}{
		{"https byte for byte", []string{"https://claude.ai/api/mcp/auth_callback"}, "https://claude.ai/api/mcp/auth_callback", true},
		{"https with another port", []string{"https://app.example/cb"}, "https://app.example:8443/cb", false},
		{"https with another path", []string{"https://app.example/cb"}, "https://app.example/cb2", false},
		{"https with a query added", []string{"https://app.example/cb"}, "https://app.example/cb?x=1", false},
		{"loopback port ignored", []string{"http://localhost/callback"}, "http://localhost:51234/callback", true},
		{"loopback registered port ignored", []string{"http://localhost:3118/callback"}, "http://localhost:51234/callback", true},
		{"loopback ipv6", []string{"http://[::1]/cb"}, "http://[::1]:7000/cb", true},
		{"localhost and 127.0.0.1 differ", []string{"http://localhost/callback"}, "http://127.0.0.1:51234/callback", false},
		{"empty path equals slash", []string{"http://127.0.0.1:33418"}, "http://127.0.0.1:40000/", true},
		{"slash equals empty path", []string{"http://127.0.0.1:33418/"}, "http://127.0.0.1:40000", true},
		{"loopback path differs", []string{"http://localhost/callback"}, "http://localhost:1/other", false},
		{"loopback query differs", []string{"http://localhost/callback?a=1"}, "http://localhost:1/callback?a=2", false},
		{"loopback query missing", []string{"http://localhost/callback?a=1"}, "http://localhost:1/callback", false},
		{"https is not matched by a loopback registration", []string{"http://localhost/callback"}, "https://localhost/callback", false},
		{"non loopback http is never matched", []string{"http://localhost/callback"}, "http://example.com/callback", false},
		{"second registered uri", []string{"https://a.example/cb", "http://localhost/cb"}, "http://localhost:9/cb", true},
		{"unregistered", []string{"https://a.example/cb"}, "https://b.example/cb", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := matchRedirect(c.registered, c.presented)
			if ok != c.want {
				t.Fatalf("matchRedirect(%v, %q) = %v, want %v", c.registered, c.presented, ok, c.want)
			}
			if ok && got != c.presented {
				t.Fatalf("matched %q, want the presented %q", got, c.presented)
			}
		})
	}
}

func TestRedirectHostAndLoopback(t *testing.T) {
	cases := []struct {
		uri, host string
		local     bool
	}{
		{"http://localhost:51234/callback", "localhost:51234", true},
		{"http://[::1]:7000/cb", "[::1]:7000", true},
		{"https://claude.ai/api/mcp/auth_callback", "claude.ai", false},
		{"https://app.example:8443/cb", "app.example:8443", false},
	}
	for _, c := range cases {
		if got := redirectHost(c.uri); got != c.host {
			t.Errorf("redirectHost(%q) = %q, want %q", c.uri, got, c.host)
		}
		if got := isLoopback(c.uri); got != c.local {
			t.Errorf("isLoopback(%q) = %v, want %v", c.uri, got, c.local)
		}
	}
}

func TestAppendQueryKeepsTheExistingQuery(t *testing.T) {
	cases := []struct {
		name, base string
		add        url.Values
		want       url.Values
	}{
		{"no query", "http://localhost:1/cb", url.Values{"code": {"c"}, "state": {"s"}}, url.Values{"code": {"c"}, "state": {"s"}}},
		{"existing query", "https://app.example/cb?tenant=a", url.Values{"code": {"c"}}, url.Values{"tenant": {"a"}, "code": {"c"}}},
		{"values are escaped", "http://localhost/cb", url.Values{"state": {"a b&c=d"}}, url.Values{"state": {"a b&c=d"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			u, err := url.Parse(appendQuery(c.base, c.add))
			if err != nil {
				t.Fatal(err)
			}
			got := u.Query()
			for key, want := range c.want {
				if len(got[key]) != 1 || got[key][0] != want[0] {
					t.Errorf("query %q = %v, want %v", key, got[key], want)
				}
			}
			if len(got) != len(c.want) {
				t.Errorf("query = %v, want only %v", got, c.want)
			}
		})
	}
}

func TestCanonicalResource(t *testing.T) {
	cases := []struct{ a, b string }{
		{"http://Observe.Test:8080/mcp", "http://observe.test:8080/mcp"},
		{"http://observe.test:8080/mcp/", "http://observe.test:8080/mcp"},
		{"HTTP://observe.test:8080/mcp", "http://observe.test:8080/mcp"},
	}
	for _, c := range cases {
		if !canonicalEqual(c.a, c.b) {
			t.Errorf("%q and %q should be the same resource", c.a, c.b)
		}
	}
	for _, other := range []string{"http://observe.test:8080", "http://observe.test:8080/mcp2", "http://observe.test:8081/mcp", "http://observe.test:8080/mcp?x=1", "http://other.test:8080/mcp", "https://observe.test:8080/mcp"} {
		if canonicalEqual(other, "http://observe.test:8080/mcp") {
			t.Errorf("%q must not equal this server's resource", other)
		}
	}
}
