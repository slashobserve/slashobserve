package grafanaconn

import (
	"strings"
	"testing"
)

func TestDashboardUIDReadsGrafanaDashboardLinks(t *testing.T) {
	const host = "https://grafana.example.com"
	cases := []struct {
		name string
		link string
		base string
		uid  string
	}{
		{"plain", host + "/d/abc123", host, "abc123"},
		{"slug and query", host + "/d/abc123/checkout?orgId=1&from=now-6h#top", host, "abc123"},
		{"trailing slash", host + "/d/abc123/", host, "abc123"},
		{"uid with dash and underscore", host + "/d/a_b-C9/x", host, "a_b-C9"},
		{"longest uid", host + "/d/" + strings.Repeat("a", 40), host, strings.Repeat("a", 40)},
		{"subpath base", "https://corp.example.com/grafana/d/abc123/checkout", "https://corp.example.com/grafana", "abc123"},
		{"base with trailing slash", "https://corp.example.com/grafana/d/abc123", "https://corp.example.com/grafana/", "abc123"},
		{"single panel embed", host + "/d-solo/abc123/checkout?panelId=2", host, "abc123"},
		{"host and scheme case", "HTTPS://Grafana.Example.COM/d/abc123", host, "abc123"},
		{"explicit default port", "https://grafana.example.com:443/d/abc123", host, "abc123"},
		{"custom port", "http://localhost:3000/d/abc123/x", "http://localhost:3000", "abc123"},
		{"other host", "https://other.example.com/d/abc123", host, ""},
		{"longer host", "https://grafana.example.com.evil.test/d/abc123", host, ""},
		{"other scheme", "http://grafana.example.com/d/abc123", host, ""},
		{"other port", "https://grafana.example.com:3000/d/abc123", host, ""},
		{"user info host trick", "https://grafana.example.com@evil.test/d/abc123", host, ""},
		{"dashboards page", host + "/dashboards", host, ""},
		{"dashboards folder page", host + "/dashboards/f/abc123/x", host, ""},
		{"no uid", host + "/d/", host, ""},
		{"over-long uid", host + "/d/" + strings.Repeat("a", 41), host, ""},
		{"path traversal", host + "/d/../api", host, ""},
		{"encoded path traversal", host + "/d/%2e%2e/api", host, ""},
		{"encoded slash in uid", host + "/d/abc%2Fdef", host, ""},
		{"traversal in slug", host + "/d/abc123/..", host, ""},
		{"extra segments", host + "/d/abc123/slug/more", host, ""},
		{"outside the subpath", "https://corp.example.com/d/abc123", "https://corp.example.com/grafana", ""},
		{"sibling of the subpath", "https://corp.example.com/grafana2/d/abc123", "https://corp.example.com/grafana", ""},
		{"not a url", "grafana dashboard", host, ""},
		{"not http", "ftp://grafana.example.com/d/abc123", host, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			uid, ok := dashboardUID(c.link, c.base)
			if ok != (c.uid != "") || uid != c.uid {
				t.Fatalf("dashboardUID(%q, %q) = %q, %v; want %q", c.link, c.base, uid, ok, c.uid)
			}
		})
	}
}
